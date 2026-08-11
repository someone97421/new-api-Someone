package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const asyncMediaInternalJobHeader = "X-New-API-Async-Job-ID"
const asyncMediaInternalSignatureHeader = "X-New-API-Async-Worker-Signature"
const AsyncMediaInternalChannelHeader = "X-New-API-Upstream-Channel-ID"

type asyncMediaResponseWriter struct {
	header http.Header
	status int
	file   *os.File
	mutex  sync.Mutex
}

func (w *asyncMediaResponseWriter) Header() http.Header {
	return w.header
}

func (w *asyncMediaResponseWriter) WriteHeader(statusCode int) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.status != 0 {
		return
	}
	w.status = statusCode
}

func (w *asyncMediaResponseWriter) Write(data []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.file.Write(data)
}

func (w *asyncMediaResponseWriter) Flush() {}

func StartAsyncMediaJobWorkers(handler http.Handler) {
	if handler == nil {
		return
	}
	if err := InitAsyncMediaStorage(); err != nil {
		common.SysError("初始化异步媒体存储失败: " + err.Error())
		return
	}
	workerCount := constant.AsyncMediaWorkers
	if workerCount <= 0 {
		workerCount = 1
	}
	for index := 0; index < workerCount; index++ {
		workerID := fmt.Sprintf("%s-async-media-%d-%d", common.NodeName, os.Getpid(), index)
		go runAsyncMediaWorker(handler, workerID)
	}
	go runAsyncMediaMaintenance()
	common.SysLog(fmt.Sprintf("异步媒体 Worker 已启动，数量：%d，接收新任务：%t", workerCount, constant.AsyncMediaEnabled))
}

func runAsyncMediaWorker(handler http.Handler, workerID string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		job, claimed, err := model.ClaimNextAsyncMediaJob(workerID, time.Now().Unix()+int64(constant.AsyncMediaLeaseSeconds))
		if err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("异步媒体任务认领失败: %v", err))
			<-ticker.C
			continue
		}
		if !claimed {
			<-ticker.C
			continue
		}
		processAsyncMediaJob(handler, workerID, job)
	}
}

func processAsyncMediaJob(handler http.Handler, workerID string, job *model.AsyncMediaJob) {
	if constant.TaskTimeoutMinutes > 0 && time.Now().Unix()-job.CreatedAt > int64(constant.TaskTimeoutMinutes)*60 {
		failAsyncMediaJob(workerID, job, http.StatusGatewayTimeout, "异步媒体任务已超过最大处理时间", model.AsyncMediaBillingReconciliationPending)
		return
	}
	stopHeartbeat := make(chan struct{})
	go func() {
		interval := time.Duration(max(10, constant.AsyncMediaLeaseSeconds/3)) * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := model.RenewAsyncMediaJobLease(job.JobID, workerID, time.Now().Unix()+int64(constant.AsyncMediaLeaseSeconds)); err != nil {
					logger.LogWarn(context.Background(), fmt.Sprintf("异步媒体任务续租失败 job=%s: %v", job.JobID, err))
					return
				}
			case <-stopHeartbeat:
				return
			}
		}
	}()
	defer close(stopHeartbeat)

	if job.ResponseFile != "" {
		processStoredAsyncMediaResponse(workerID, job)
		return
	}
	if job.UpstreamTaskID != "" && job.OriginTaskID == "" {
		processAsyncImageUpstreamTask(handler, workerID, job)
		return
	}
	if job.Status == model.AsyncMediaJobStatusWaitingUpstream || job.OriginTaskID != "" {
		processAsyncMediaUpstreamTask(workerID, job)
		return
	}
	processAsyncMediaRelayRequest(handler, workerID, job)
}

func processAsyncMediaRelayRequest(handler http.Handler, workerID string, job *model.AsyncMediaJob) {
	token, err := model.GetTokenById(job.TokenID)
	if err != nil || token == nil || token.UserId != job.UserID {
		failAsyncMediaJob(workerID, job, http.StatusUnauthorized, "用于提交任务的令牌已不可用", model.AsyncMediaBillingRefunded)
		return
	}
	requestPath, err := ResolveAsyncMediaPath(job.RequestFile)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingRefunded)
		return
	}
	requestBody, err := os.Open(requestPath)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingRefunded)
		return
	}
	defer requestBody.Close()

	query, err := url.ParseQuery(job.RawQuery)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusBadRequest, "无效的异步任务查询参数", model.AsyncMediaBillingRefunded)
		return
	}
	query.Del("async")
	requestURL := job.RequestPath
	if encoded := query.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	request, err := http.NewRequestWithContext(context.Background(), job.Method, requestURL, requestBody)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingRefunded)
		return
	}
	var headers map[string][]string
	if job.RequestHeaders != "" {
		if err := common.UnmarshalJsonStr(job.RequestHeaders, &headers); err != nil {
			failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingRefunded)
			return
		}
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	request.Header.Set(asyncMediaInternalJobHeader, job.JobID)
	request.Header.Set(asyncMediaInternalSignatureHeader, AsyncMediaInternalSignature(job.JobID))
	request.ContentLength = job.RequestSize

	responsePath, responseFile, err := CreateAsyncMediaResponseFile(job.JobID)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingRefunded)
		return
	}
	writer := &asyncMediaResponseWriter{header: make(http.Header), file: responseFile}
	handler.ServeHTTP(writer, request)
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	if err := responseFile.Sync(); err != nil {
		_ = responseFile.Close()
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
		return
	}
	_ = responseFile.Close()
	responseHeaders, _ := common.Marshal(writer.header)
	if writer.status < http.StatusOK || writer.status >= http.StatusMultipleChoices {
		errorMessage := readAsyncMediaError(responsePath)
		_ = DeleteAsyncMediaPath(responsePath)
		_ = DeleteAsyncMediaPath(job.RequestFile)
		failAsyncMediaJob(workerID, job, writer.status, errorMessage, model.AsyncMediaBillingRefunded)
		return
	}

	originTask, err := model.GetTaskByAsyncJobID(job.JobID)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
		return
	}
	if originTask != nil {
		_ = DeleteAsyncMediaPath(responsePath)
		_ = DeleteAsyncMediaPath(job.RequestFile)
		err = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":              model.AsyncMediaJobStatusWaitingUpstream,
			"billing_status":      model.AsyncMediaBillingDelegated,
			"origin_task_id":      originTask.TaskID,
			"upstream_channel_id": originTask.ChannelId,
			"http_status":         writer.status,
			"response_headers":    string(responseHeaders),
			"next_run_at":         time.Now().Unix() + 3,
		})
		if err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("异步媒体任务等待上游状态写入失败 job=%s: %v", job.JobID, err))
		}
		return
	}

	files, err := StoreAsyncMediaResults(job, responsePath, writer.header.Get("Content-Type"))
	if err == nil && len(files) == 0 {
		upstreamTaskID := extractAsyncMediaTaskID(responsePath)
		modelName := extractAsyncMediaRequestModel(job)
		if upstreamTaskID != "" && modelName != "" {
			upstreamChannelID, parseErr := strconv.Atoi(strings.TrimSpace(writer.header.Get(AsyncMediaInternalChannelHeader)))
			if parseErr != nil || upstreamChannelID <= 0 {
				_ = DeleteAsyncMediaPath(responsePath)
				_ = DeleteAsyncMediaPath(job.RequestFile)
				failAsyncMediaJob(workerID, job, http.StatusInternalServerError, "异步图片任务缺少原始上游渠道信息", model.AsyncMediaBillingReconciliationPending)
				return
			}
			_ = DeleteAsyncMediaPath(responsePath)
			_ = DeleteAsyncMediaPath(job.RequestFile)
			if updateErr := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
				"status":              model.AsyncMediaJobStatusWaitingUpstream,
				"billing_status":      model.AsyncMediaBillingSettled,
				"upstream_channel_id": upstreamChannelID,
				"upstream_task_id":    upstreamTaskID,
				"model_name":          modelName,
				"http_status":         writer.status,
				"response_headers":    string(responseHeaders),
				"next_run_at":         time.Now().Unix() + 3,
			}); updateErr != nil {
				logger.LogError(context.Background(), fmt.Sprintf("异步图片任务轮询状态写入失败 job=%s: %v", job.JobID, updateErr))
			}
			return
		}
		err = fmt.Errorf("上游响应中没有可转存的图片、视频或任务 ID")
	}
	if err != nil {
		_ = DeleteAsyncMediaPath(job.RequestFile)
		if updateErr := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":           model.AsyncMediaJobStatusWaitingUpstream,
			"billing_status":   model.AsyncMediaBillingSettled,
			"http_status":      writer.status,
			"response_headers": string(responseHeaders),
			"response_file":    responsePath,
			"error":            err.Error(),
			"next_run_at":      time.Now().Unix() + 30,
		}); updateErr != nil {
			logger.LogError(context.Background(), fmt.Sprintf("异步媒体结果转存重试状态写入失败 job=%s: %v", job.JobID, updateErr))
		}
		return
	}
	if err := model.CreateAsyncMediaFiles(files); err != nil {
		DeleteAsyncMediaFiles(files)
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
		return
	}
	_ = DeleteAsyncMediaPath(job.RequestFile)
	_ = DeleteAsyncMediaPath(responsePath)
	completedAt := time.Now().Unix()
	if err := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
		"status":           model.AsyncMediaJobStatusSucceeded,
		"billing_status":   model.AsyncMediaBillingSettled,
		"http_status":      writer.status,
		"response_headers": string(responseHeaders),
		"response_file":    "",
		"completed_at":     completedAt,
		"expires_at":       completedAt + int64(constant.AsyncMediaRetentionHours)*3600,
	}); err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("异步媒体任务成功状态写入失败 job=%s: %v", job.JobID, err))
	}
}

func processAsyncImageUpstreamTask(handler http.Handler, workerID string, job *model.AsyncMediaJob) {
	if job.UpstreamChannelID <= 0 {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, "异步图片任务缺少原始上游渠道信息", model.AsyncMediaBillingReconciliationPending)
		return
	}
	token, err := model.GetTokenById(job.TokenID)
	if err != nil || token == nil || token.UserId != job.UserID {
		failAsyncMediaJob(workerID, job, http.StatusUnauthorized, "用于查询任务的令牌已不可用", model.AsyncMediaBillingReconciliationPending)
		return
	}
	requestURL := "/v1/images/generations/" + url.PathEscape(job.UpstreamTaskID) + "?model=" + url.QueryEscape(job.ModelName)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
		return
	}
	request.Header.Set("Authorization", "Bearer sk-"+token.Key)
	request.Header.Set(asyncMediaInternalJobHeader, job.JobID)
	request.Header.Set(asyncMediaInternalSignatureHeader, AsyncMediaInternalSignature(job.JobID))
	request.Header.Set(AsyncMediaInternalChannelHeader, strconv.Itoa(job.UpstreamChannelID))
	responsePath, responseFile, err := CreateAsyncMediaResponseFile(job.JobID)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
		return
	}
	writer := &asyncMediaResponseWriter{header: make(http.Header), file: responseFile}
	handler.ServeHTTP(writer, request)
	_ = responseFile.Close()
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	if writer.status == http.StatusTooManyRequests || writer.status >= http.StatusInternalServerError {
		errorMessage := readAsyncMediaError(responsePath)
		_ = DeleteAsyncMediaPath(responsePath)
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"http_status": writer.status,
			"error":       errorMessage,
			"next_run_at": time.Now().Unix() + 15,
		})
		return
	}
	if writer.status < http.StatusOK || writer.status >= http.StatusMultipleChoices {
		errorMessage := readAsyncMediaError(responsePath)
		_ = DeleteAsyncMediaPath(responsePath)
		failAsyncMediaJob(workerID, job, writer.status, errorMessage, model.AsyncMediaBillingReconciliationPending)
		return
	}
	files, err := StoreAsyncMediaResults(job, responsePath, writer.header.Get("Content-Type"))
	if err != nil {
		_ = DeleteAsyncMediaPath(responsePath)
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"error":       err.Error(),
			"next_run_at": time.Now().Unix() + 15,
		})
		return
	}
	if len(files) == 0 {
		terminalError := extractAsyncMediaUpstreamTerminalError(responsePath)
		_ = DeleteAsyncMediaPath(responsePath)
		if terminalError != "" {
			failAsyncMediaJob(workerID, job, http.StatusBadGateway, terminalError, model.AsyncMediaBillingSettled)
			return
		}
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"error":       "上游图片任务仍在处理中",
			"next_run_at": time.Now().Unix() + 5,
		})
		return
	}
	_ = DeleteAsyncMediaPath(responsePath)
	if err := model.CreateAsyncMediaFiles(files); err != nil {
		DeleteAsyncMediaFiles(files)
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"error":       err.Error(),
			"next_run_at": time.Now().Unix() + 15,
		})
		return
	}
	completedAt := time.Now().Unix()
	if err := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
		"status":         model.AsyncMediaJobStatusSucceeded,
		"billing_status": model.AsyncMediaBillingSettled,
		"error":          "",
		"completed_at":   completedAt,
		"expires_at":     completedAt + int64(constant.AsyncMediaRetentionHours)*3600,
	}); err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("异步图片任务成功状态写入失败 job=%s: %v", job.JobID, err))
	}
}

func extractAsyncMediaUpstreamTerminalError(responsePath string) string {
	absolute, err := ResolveAsyncMediaPath(responsePath)
	if err != nil {
		return ""
	}
	body, err := os.ReadFile(absolute)
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	code := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["code"])))
	if code != "" && code != "<nil>" && code != "success" {
		return firstAsyncMediaErrorMessage(payload, "upstream image task query failed")
	}
	statusSource := payload
	if data, ok := payload["data"].(map[string]any); ok {
		statusSource = data
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(statusSource["status"])))
	switch status {
	case "failure", "failed", "error", "cancelled", "canceled":
		return firstAsyncMediaErrorMessage(payload, "upstream image task failed")
	default:
		return ""
	}
}

func firstAsyncMediaErrorMessage(payload map[string]any, fallback string) string {
	if data, ok := payload["data"].(map[string]any); ok {
		for _, key := range []string{"fail_reason", "error", "message"} {
			if value := strings.TrimSpace(fmt.Sprint(data[key])); value != "" && value != "<nil>" {
				return value
			}
		}
	}
	for _, key := range []string{"message", "error"} {
		if value := strings.TrimSpace(fmt.Sprint(payload[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return fallback
}

func extractAsyncMediaTaskID(responsePath string) string {
	absolute, err := ResolveAsyncMediaPath(responsePath)
	if err != nil {
		return ""
	}
	body, err := os.ReadFile(absolute)
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"task_id", "taskId", "id"} {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		for _, key := range []string{"task_id", "taskId", "id"} {
			if value, ok := data[key].(string); ok && value != "" {
				return value
			}
		}
	}
	return ""
}

func extractAsyncMediaRequestModel(job *model.AsyncMediaJob) string {
	absolute, err := ResolveAsyncMediaPath(job.RequestFile)
	if err != nil {
		return ""
	}
	contentType := ""
	var headers http.Header
	if err := common.UnmarshalJsonStr(job.RequestHeaders, &headers); err == nil {
		contentType = headers.Get("Content-Type")
	}
	mediaType, params, _ := mime.ParseMediaType(contentType)
	if mediaType == "multipart/form-data" {
		file, err := os.Open(absolute)
		if err != nil {
			return ""
		}
		defer file.Close()
		reader := multipart.NewReader(file, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err != nil {
				return ""
			}
			if part.FormName() != "model" {
				_ = part.Close()
				continue
			}
			value, _ := io.ReadAll(io.LimitReader(part, 4096))
			_ = part.Close()
			return strings.TrimSpace(string(value))
		}
	}
	body, err := os.ReadFile(absolute)
	if err != nil {
		return ""
	}
	var request struct {
		Model string `json:"model"`
	}
	if err := common.Unmarshal(body, &request); err != nil {
		return ""
	}
	return strings.TrimSpace(request.Model)
}

func processStoredAsyncMediaResponse(workerID string, job *model.AsyncMediaJob) {
	contentType := "application/json"
	if job.ResponseHeaders != "" {
		var headers http.Header
		if err := common.UnmarshalJsonStr(job.ResponseHeaders, &headers); err == nil {
			contentType = headers.Get("Content-Type")
		}
	}
	files, err := StoreAsyncMediaResults(job, job.ResponseFile, contentType)
	if err != nil || len(files) == 0 {
		if err == nil {
			err = fmt.Errorf("上游响应中没有可转存的图片或视频")
		}
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"error":       err.Error(),
			"next_run_at": time.Now().Unix() + 30,
		})
		return
	}
	if err := model.CreateAsyncMediaFiles(files); err != nil {
		DeleteAsyncMediaFiles(files)
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"error":       err.Error(),
			"next_run_at": time.Now().Unix() + 30,
		})
		return
	}
	_ = DeleteAsyncMediaPath(job.ResponseFile)
	completedAt := time.Now().Unix()
	if err := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
		"status":         model.AsyncMediaJobStatusSucceeded,
		"billing_status": model.AsyncMediaBillingSettled,
		"response_file":  "",
		"error":          "",
		"completed_at":   completedAt,
		"expires_at":     completedAt + int64(constant.AsyncMediaRetentionHours)*3600,
	}); err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("异步媒体结果转存成功状态写入失败 job=%s: %v", job.JobID, err))
	}
}

func processAsyncMediaUpstreamTask(workerID string, job *model.AsyncMediaJob) {
	originTask, err := model.GetTaskByAsyncJobID(job.JobID)
	if err != nil {
		failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
		return
	}
	if originTask == nil {
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":      model.AsyncMediaJobStatusWaitingUpstream,
			"next_run_at": time.Now().Unix() + 5,
		})
		return
	}
	switch originTask.Status {
	case model.TaskStatusFailure:
		failAsyncMediaJob(workerID, job, http.StatusBadGateway, originTask.FailReason, model.AsyncMediaBillingRefunded)
	case model.TaskStatusSuccess:
		resultURL := originTask.GetResultURL()
		if resultURL == "" {
			failAsyncMediaJob(workerID, job, http.StatusBadGateway, "原生异步任务成功但没有媒体地址", model.AsyncMediaBillingReconciliationPending)
			return
		}
		file, err := StoreAsyncMediaURLResult(job, resultURL)
		if err != nil {
			_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
				"status":         model.AsyncMediaJobStatusWaitingUpstream,
				"billing_status": model.AsyncMediaBillingReconciliationPending,
				"error":          err.Error(),
				"next_run_at":    time.Now().Unix() + 30,
			})
			return
		}
		if err := model.CreateAsyncMediaFiles([]*model.AsyncMediaFile{file}); err != nil {
			DeleteAsyncMediaFiles([]*model.AsyncMediaFile{file})
			failAsyncMediaJob(workerID, job, http.StatusInternalServerError, err.Error(), model.AsyncMediaBillingReconciliationPending)
			return
		}
		completedAt := time.Now().Unix()
		if err := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":         model.AsyncMediaJobStatusSucceeded,
			"billing_status": model.AsyncMediaBillingDelegated,
			"completed_at":   completedAt,
			"expires_at":     completedAt + int64(constant.AsyncMediaRetentionHours)*3600,
		}); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("原生异步媒体结果写入失败 job=%s: %v", job.JobID, err))
		}
	default:
		_ = model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
			"status":         model.AsyncMediaJobStatusWaitingUpstream,
			"billing_status": model.AsyncMediaBillingDelegated,
			"origin_task_id": originTask.TaskID,
			"next_run_at":    time.Now().Unix() + 5,
		})
	}
}

func failAsyncMediaJob(workerID string, job *model.AsyncMediaJob, status int, message string, billingStatus string) {
	_ = DeleteAsyncMediaPath(job.RequestFile)
	if message == "" {
		message = http.StatusText(status)
	}
	if err := model.UpdateAsyncMediaJob(job.JobID, workerID, map[string]any{
		"status":         model.AsyncMediaJobStatusFailed,
		"billing_status": billingStatus,
		"http_status":    status,
		"error":          message,
		"completed_at":   time.Now().Unix(),
	}); err != nil {
		logger.LogError(context.Background(), fmt.Sprintf("异步媒体任务失败状态写入失败 job=%s: %v", job.JobID, err))
	}
}

func readAsyncMediaError(relative string) string {
	absolute, err := ResolveAsyncMediaPath(relative)
	if err != nil {
		return err.Error()
	}
	file, err := os.Open(absolute)
	if err != nil {
		return err.Error()
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, 16*1024))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(bytes.TrimSpace(body)))
}

func runAsyncMediaMaintenance() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for now := range ticker.C {
		if err := model.MarkExpiredRunningAsyncMediaJobsFailed(now.Unix()); err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("清理过期异步媒体租约失败: %v", err))
		}
		files, err := model.FindExpiredAsyncMediaFiles(now.Unix(), 100)
		if err != nil {
			logger.LogError(context.Background(), fmt.Sprintf("查询过期异步媒体文件失败: %v", err))
			continue
		}
		for _, file := range files {
			if err := DeleteAsyncMediaPath(file.Path); err != nil {
				logger.LogWarn(context.Background(), fmt.Sprintf("删除过期异步媒体文件失败 file=%s: %v", file.FileID, err))
				continue
			}
			if err := model.MarkAsyncMediaFileDeleted(file.FileID, now.Unix()); err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("标记异步媒体文件删除失败 file=%s: %v", file.FileID, err))
			}
		}
	}
}

func AsyncMediaFileSignature(fileID string, expiresAt int64) string {
	return common.GenerateHMAC(fileID + ":" + strconv.FormatInt(expiresAt, 10))
}

func AsyncMediaInternalSignature(jobID string) string {
	return common.GenerateHMAC("async-media-worker:" + jobID)
}

func ValidateAsyncMediaInternalRequest(jobID string, signature string) bool {
	if jobID == "" || signature == "" {
		return false
	}
	return hmac.Equal([]byte(AsyncMediaInternalSignature(jobID)), []byte(signature))
}

func ValidateAsyncMediaFileSignature(fileID string, expiresAt int64, signature string) bool {
	if expiresAt < time.Now().Unix() || signature == "" {
		return false
	}
	return hmac.Equal([]byte(AsyncMediaFileSignature(fileID, expiresAt)), []byte(signature))
}
