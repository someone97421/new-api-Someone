package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// serveTaskPluginImageProtocol answers an OpenAI Images API request that a
// task plugin claimed. The API is synchronous: the durable submission runs
// first on a context that survives a client disconnect, an asynchronous
// upstream task is then polled inside the request until it is terminal or the
// protocol timeout elapses, and the plugin's render hook finally shapes the
// OpenAI image response from the terminal task.
func serveTaskPluginImageProtocol(c *gin.Context, pinned pluginruntime.PinnedEndpoint, deps pluginProtocolBridgeDeps) {
	deps = deps.withDefaults()
	requestValue, exists := c.Get(pluginruntime.ContextKeyProtocolRequest)
	protocolRequest, ok := requestValue.(pluginruntime.ProtocolRequestContext)
	if !exists || !ok || protocolRequest.Protocol != pinned.Protocol || pinned.Plugin == nil {
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	clientRequest := c.Request
	var relayInfo *relaycommon.RelayInfo
	var relayInfoErr error
	var outcome *taskSubmissionOutcome
	var taskErr *dto.TaskError
	// DashScope bills the images as soon as it accepts the upstream call. A
	// client that gives up while the submission is in flight must not cancel
	// that call, the task row, its settlement, or its consume log, so the
	// submission runs on its own bounded context exactly like the Responses
	// bridge. Only the in-request wait and the response write observe the
	// client connection.
	func() {
		submissionContext, cancelSubmission := context.WithTimeout(
			context.WithoutCancel(clientRequest.Context()),
			deps.submissionTimeout,
		)
		c.Request = clientRequest.Clone(submissionContext)
		defer func() {
			c.Request = clientRequest
			cancelSubmission()
		}()

		relayInfo, relayInfoErr = relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
		if relayInfoErr != nil {
			return
		}
		relayInfo.IsStream = false
		relayInfo.OriginModelName = c.GetString("resolved_task_model")
		if action := c.GetString("task_action"); action != "" {
			relayInfo.Action = action
		}
		if taskErr = relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
			return
		}
		if taskErr = relay.ApplyOriginTaskAffinity(c, relayInfo); taskErr != nil {
			return
		}
		outcome, taskErr = deps.submit(c, relayInfo)
	}()
	if clientRequest.Context().Err() != nil {
		// The client is gone. A durable task keeps its settlement and consume
		// log; an asynchronous one is finished by the background poller.
		return
	}
	if relayInfoErr != nil {
		logger.LogError(c, common.LogText("build task protocol relay info failed: %s", relayInfoErr.Error()))
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	if taskErr != nil {
		respondTaskPluginImageError(c, taskErr)
		return
	}
	// Retries can select another provider; the final response belongs to the
	// plugin pinned by the channel that actually accepted the submission.
	if value, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint); exists {
		selected, ok := value.(pluginruntime.PinnedEndpoint)
		if !ok || selected.Plugin == nil || selected.Generation != pinned.Generation ||
			selected.Protocol != pinned.Protocol || selected.Model != pinned.Model ||
			selected.Operation.Name != pinned.Operation.Name {
			respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
			return
		}
		pinned = selected
	}
	if outcome == nil || outcome.Task == nil || outcome.Task.Platform != constant.TaskPlatform(pinned.Plugin.Meta.Key) {
		logger.LogError(c, common.LogText("task protocol submission returned an invalid durable outcome"))
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	// Rendering keeps the original client body, including response_format.
	protocolRequest.UpstreamModel = outcome.Task.Properties.UpstreamModelName
	task := outcome.Task
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		if taskErr = waitTaskPluginImageTask(c, task, deps); taskErr != nil {
			if c.Request.Context().Err() == nil {
				respondTaskPluginImageError(c, taskErr)
			}
			return
		}
	}
	if task.Status == model.TaskStatusFailure {
		reason := strings.TrimSpace(task.FailReason)
		if reason == "" {
			reason = "image generation failed"
		}
		// The fail reason comes from the upstream, so mask the channel address in it.
		taskErr = service.TaskErrorWrapper(errors.New(reason), "image_generation_failed", http.StatusBadRequest, common.UpstreamAddresses(c)...)
		taskErr.LocalError = true
		respondTaskPluginImageError(c, taskErr)
		return
	}
	view, err := service.BuildTaskPluginView(task)
	if err != nil {
		logger.LogError(c, common.LogText("build task protocol view failed: %s", err.Error()))
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	viewValue, err := taskPluginProtocolJSONValue(view)
	if err != nil {
		logger.LogError(c, common.LogText("encode task protocol view failed: %s", err.Error()))
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	payload, err := pinned.Plugin.Engine.CallPathWithAdmissionTimeout(c.Request.Context(), deps.admissionTimeout, "protocols", []string{pinned.Protocol, "render"}, protocolRequest.JSValueFor(pinned.Plugin.Meta), viewValue)
	if err != nil {
		logger.LogError(c, common.LogText("task protocol image render hook failed: %s", err.Error()))
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	response, ok := payload.(map[string]any)
	data, hasData := response["data"].([]any)
	if !ok || !hasData {
		logger.LogError(c, common.LogText("task protocol image render hook must return an object with a data array"))
		respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		return
	}
	response = maps.Clone(response)
	data = slices.Clone(data)
	response["data"] = data
	if _, present := response["created"]; !present {
		createdAt := task.CreatedAt
		if createdAt == 0 {
			createdAt = task.SubmitTime
		}
		response["created"] = createdAt
	}
	for index, entry := range data {
		item, isObject := entry.(map[string]any)
		if !isObject {
			continue
		}
		item = maps.Clone(item)
		data[index] = item
		artifactKey, hasArtifact := item["artifact_key"]
		delete(item, "artifact_key")
		imageURL, _ := item["url"].(string)
		if hasArtifact {
			key, validKey := artifactKey.(string)
			if !validKey || !taskArtifactKeyPattern.MatchString(key) {
				respondPluginProtocolError(c, http.StatusBadGateway, "image_download_failed", "Image artifact is invalid after generation completed")
				return
			}
			if taskPluginImageResponseFormat(protocolRequest) == "b64_json" || strings.TrimSpace(imageURL) == "" {
				encoded, downloadErr := deps.downloadArtifact(c, task, key)
				if downloadErr != nil {
					logger.LogWarn(c, common.LogText("task protocol image artifact download failed after generation completed"))
					respondPluginProtocolError(c, http.StatusBadGateway, "image_download_failed", "Image download failed after generation completed")
					return
				}
				item["b64_json"] = encoded
			} else if _, artifactErr := taskPluginImageArtifactProvider(task, key); artifactErr != nil {
				respondPluginProtocolError(c, http.StatusBadGateway, "image_download_failed", "Image artifact is invalid after generation completed")
				return
			}
			continue
		}
		if taskPluginImageResponseFormat(protocolRequest) != "b64_json" {
			continue
		}
		if existing, _ := item["b64_json"].(string); strings.TrimSpace(imageURL) == "" || existing != "" {
			continue
		}
		_, encoded, downloadErr := deps.downloadImage(imageURL)
		if downloadErr != nil {
			logger.LogWarn(c, common.LogText("task protocol image download failed; returning the upstream URL instead of b64_json: %s", common.MaskSensitiveInfo(downloadErr.Error(), common.UpstreamAddresses(c)...)))
			continue
		}
		item["b64_json"] = encoded
	}
	body, err := common.Marshal(response)
	if err == nil {
		// Channel selection refreshes this setting on every retry, so delivery
		// uses the provider that actually accepted the submission.
		channelSetting, _ := common.GetContextKeyType[kitdto.ChannelSettings](c, constant.ContextKeyChannelSetting)
		cfg := system_setting.GetFileRelaySettings().ForChannel(channelSetting.FileRelay)
		body, err = service.RelayImageResponse(c.Request.Context(), cfg, body)
	}
	if err != nil {
		respondPluginProtocolError(c, http.StatusBadGateway, "file_relay_failed", "File relay failed after generation completed")
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

func taskPluginImageArtifactProvider(task *model.Task, artifactKey string) (relaychannel.TaskContentRequestProvider, error) {
	adaptor, err := initTaskArtifactAdaptor(task)
	if err != nil {
		return nil, err
	}
	artifactsProvider, ok := adaptor.(relaychannel.TaskArtifactProvider)
	if !ok {
		return nil, errors.New("image artifact listing is unavailable")
	}
	artifacts, err := artifactsProvider.ListArtifacts(task)
	if err != nil {
		return nil, err
	}
	artifacts, err = validateProjectedTaskArtifacts(artifacts)
	if err != nil {
		return nil, err
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Key == artifactKey && artifact.Type == "image" {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("image artifact not found")
	}
	contentProvider, ok := adaptor.(relaychannel.TaskContentRequestProvider)
	if !ok {
		return nil, errors.New("image artifact content is unavailable")
	}
	return contentProvider, nil
}

// downloadTaskPluginImageArtifact 通过成功任务的供应商读取素材，仅在响应内编码图片。
func downloadTaskPluginImageArtifact(c *gin.Context, task *model.Task, artifactKey string) (string, error) {
	contentProvider, err := taskPluginImageArtifactProvider(task, artifactKey)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), taskMediaResponseHeaderTimeout)
	defer cancel()
	descriptor, err := contentProvider.BuildContentRequest(ctx, task, artifactKey, relaychannel.TaskArtifactClientRequest{Method: http.MethodGet})
	if err != nil {
		return "", err
	}
	if descriptor == nil {
		return "", errors.New("image artifact content request is unavailable")
	}
	rawURL := strings.TrimSpace(descriptor.URL)
	parsedURL, err := url.Parse(rawURL)
	if err != nil || len(rawURL) > 64<<10 || parsedURL == nil ||
		(parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
		parsedURL.Host == "" || parsedURL.User != nil || parsedURL.Fragment != "" ||
		isTaskMediaFallbackLoop(rawURL, task.TaskID) || isSelfTaskMediaURL(c, parsedURL) {
		return "", errTaskMediaRequestRejected
	}
	method := strings.ToUpper(strings.TrimSpace(descriptor.Method))
	if method == "" {
		method = http.MethodGet
	}
	if (method != http.MethodGet && method != http.MethodPost) || len(descriptor.Body) > 1<<20 ||
		(descriptor.Credentialless && (method != http.MethodGet || descriptor.Body != nil || len(descriptor.Headers) != 0)) {
		return "", errTaskMediaRequestRejected
	}
	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil || channel == nil {
		return "", errors.New("image artifact channel is unavailable")
	}
	setting := channel.GetSetting()
	proxy := strings.TrimSpace(setting.Proxy)
	if err := validateTaskMediaURL(rawURL, proxy); err != nil {
		return "", err
	}
	client := service.GetSSRFProtectedHTTPClient()
	if proxy != "" {
		client, err = service.GetHttpClientWithProxySettings(proxy, setting)
		if err != nil {
			return "", err
		}
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(descriptor.Body))
	if err != nil {
		return "", err
	}
	if err := applyTaskMediaRequestHeaders(req.Header, descriptor.Headers); err != nil {
		return "", err
	}
	client = taskMediaRedirectClient(client, proxy, c, nil, descriptor.Credentialless)
	resp, err := doTaskMediaRequest(client, req, taskMediaResponseHeaderTimeout)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image artifact upstream returned HTTP %d", resp.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || (!strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream") {
		return "", errors.New("image artifact returned invalid content type")
	}
	maxBytes := int64(constant.MaxFileDownloadMB) * 1024 * 1024
	if resp.ContentLength > maxBytes {
		return "", errors.New("image artifact exceeds download size limit")
	}
	image, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if len(image) == 0 || int64(len(image)) > maxBytes {
		return "", errors.New("image artifact is empty or exceeds download size limit")
	}
	if !strings.HasPrefix(http.DetectContentType(image), "image/") {
		return "", errors.New("image artifact returned non-image content")
	}
	return base64.StdEncoding.EncodeToString(image), nil
}

// waitTaskPluginImageTask polls an asynchronous image task inside the client
// request through the ordinary task polling path, so completion settles
// billing exactly as the background poller would, until the task is terminal
// or the protocol timeout elapses. A client disconnect stops waiting; the
// background poller then finishes the durable task.
func waitTaskPluginImageTask(c *gin.Context, task *model.Task, deps pluginProtocolBridgeDeps) *dto.TaskError {
	ctx, cancel := context.WithTimeout(c.Request.Context(), deps.submissionTimeout)
	defer cancel()
	for {
		timer := time.NewTimer(deps.imagePollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) && c.Request.Context().Err() == nil {
				return service.TaskErrorWrapperLocal(fmt.Errorf("image generation is still running; task %s settles when it completes", task.TaskID), "task_timeout", http.StatusGatewayTimeout)
			}
			return service.TaskErrorWrapperLocal(ctx.Err(), "request_cancelled", http.StatusRequestTimeout)
		case <-timer.C:
		}
		if err := deps.pollTask(ctx, task); err != nil {
			logger.LogWarn(c, common.LogText("task protocol image poll failed for task %s: %s", task.TaskID, common.MaskSensitiveInfo(err.Error(), common.UpstreamAddresses(c)...)))
			continue
		}
		if task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
			return nil
		}
	}
}

// pollTaskPluginImageTask runs one poll round for a single task through the
// same code path as the scheduled task poller, including status CAS and
// completion settlement.
func pollTaskPluginImageTask(ctx context.Context, task *model.Task) error {
	upstreamID := task.GetUpstreamTaskID()
	if upstreamID == "" {
		return errors.New("task has no upstream id")
	}
	return service.UpdateVideoTasks(ctx, task.Platform, map[int][]string{task.ChannelId: {upstreamID}}, map[string]*model.Task{upstreamID: task})
}

func taskPluginImageResponseFormat(request pluginruntime.ProtocolRequestContext) string {
	body, ok := request.Body.(map[string]any)
	if !ok {
		return ""
	}
	switch body["kind"] {
	case string(pluginruntime.BodyJSON):
		value, _ := body["value"].(map[string]any)
		format, _ := value["response_format"].(string)
		return strings.TrimSpace(format)
	case string(pluginruntime.BodyMultipart), string(pluginruntime.BodyForm):
		fields, _ := body["fields"].(map[string][]string)
		if values := fields["response_format"]; len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

// respondTaskPluginImageError writes an OpenAI error envelope for the Images
// API. DashScope failure bodies arrive as JSON {code, message}; their message
// is surfaced so a client sees the vendor reason instead of raw JSON.
func respondTaskPluginImageError(c *gin.Context, taskErr *dto.TaskError) {
	service.RecordRequestPolicyTermination(c, taskSubmissionAPIError(taskErr))
	status := http.StatusInternalServerError
	if taskErr.StatusCode >= 400 && taskErr.StatusCode <= 599 {
		status = taskErr.StatusCode
	}
	code := taskErr.Code
	message := taskErr.Message
	var upstream struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if common.UnmarshalJsonStr(message, &upstream) == nil && strings.TrimSpace(upstream.Message) != "" {
		message = upstream.Message
		if strings.TrimSpace(upstream.Code) != "" {
			code = upstream.Code
		}
	}
	errorType := "new_api_error"
	if status < http.StatusInternalServerError {
		errorType = "invalid_request_error"
	}
	c.JSON(status, gin.H{"error": gin.H{
		"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
		"type":    errorType,
		"code":    code,
	}})
}
