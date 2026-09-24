package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
)

type taskArtifactArchiveHandler struct{}

func (taskArtifactArchiveHandler) Type() string {
	return model.SystemTaskTypeTaskArtifactArchive
}

func (taskArtifactArchiveHandler) Enabled() bool {
	return service.GetTaskArtifactStore().Enabled()
}

func (taskArtifactArchiveHandler) Interval() time.Duration {
	return 60 * time.Second
}

func (taskArtifactArchiveHandler) NewPayload() any {
	return nil
}

func (taskArtifactArchiveHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	store := service.GetTaskArtifactStore()
	if !store.Enabled() {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, nil, nil)
		return
	}

	summary := runTaskArtifactArchivePass(ctx, store)
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// taskArtifactArchiveBatchSize bounds one archive pass. Tasks already archived
// are excluded by the candidate query, so finished work never blocks newer work.
const taskArtifactArchiveBatchSize = 5

type taskArtifactArchiveSummary struct {
	Archived int `json:"archived"`
	Skipped  int `json:"skipped"`
	Failed   int `json:"failed"`
	Cleaned  int `json:"cleaned"`
}

func runTaskArtifactArchivePass(ctx context.Context, store service.TaskArtifactStore) *taskArtifactArchiveSummary {
	summary := &taskArtifactArchiveSummary{}

	retentionHours := system_setting.DefaultTaskArtifactStoreRetentionHours
	if localStore, ok := store.(*service.LocalArtifactStore); ok {
		retentionHours = localStore.RetentionHours()
	}

	cutoff := time.Now().Add(-time.Duration(retentionHours) * time.Hour).Unix()
	for _, task := range model.ListTerminalTasksForArtifactArchive(cutoff, taskArtifactArchiveBatchSize) {
		if ctx.Err() != nil {
			return summary
		}
		archiveTaskArtifacts(ctx, store, task, summary)
	}

	if cleaner, ok := store.(service.ArtifactCleaner); ok {
		cleaned, err := cleaner.CleanExpired(ctx)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("task artifact cleanup error: %s", redactArtifactLogError(err)))
		}
		summary.Cleaned = cleaned
	}

	return summary
}

// artifactLogURLPattern matches absolute URLs so archive logs and recorded
// failures never carry provider URLs that may embed credentials or signatures.
var artifactLogURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

func redactArtifactLogError(err error) string {
	if err == nil {
		return ""
	}
	return artifactLogURLPattern.ReplaceAllString(err.Error(), "[redacted-url]")
}

// recordTaskArtifactArchiveState stores the archive outcome on the task. It
// never changes the task status or its billing: archiving is a separate delivery
// step that can fail and retry on later passes.
func recordTaskArtifactArchiveState(ctx context.Context, task *model.Task, archivedAt int64, message string) {
	if err := model.MarkTaskArtifactsArchived(task, archivedAt, message); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("failed to record artifact archive state for task %s: %s", task.TaskID, redactArtifactLogError(err)))
	}
}

func archiveTaskArtifacts(ctx context.Context, store service.TaskArtifactStore, task *model.Task, summary *taskArtifactArchiveSummary) {
	var artifacts []relaychannel.TaskArtifact
	if taskHasPluginExecution(task) {
		projected, err := projectTaskArtifacts(task)
		if err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("failed to project artifacts for task %s: %s", task.TaskID, redactArtifactLogError(err)))
			recordTaskArtifactArchiveState(ctx, task, 0, redactArtifactLogError(err))
			summary.Failed++
			return
		}
		artifacts = projected
	} else if legacyVideoAvailable(task) {
		artifacts = []relaychannel.TaskArtifact{
			{Key: "video", Type: "video"},
		}
	}

	if len(artifacts) == 0 {
		// There is nothing to archive; recording it keeps the task from occupying
		// every later batch.
		recordTaskArtifactArchiveState(ctx, task, common.GetTimestamp(), "")
		summary.Skipped++
		return
	}

	var firstErr error
	for _, artifact := range artifacts {
		if ctx.Err() != nil {
			return
		}

		ref, err := store.Resolve(task, artifact.Key)
		if err == nil && ref != nil {
			summary.Skipped++
			continue
		}

		descriptor, err := resolveArtifactContentDescriptor(task, artifact.Key)
		if err != nil || descriptor == nil {
			logger.LogWarn(ctx, fmt.Sprintf("failed to resolve content request for task %s artifact %s: %s", task.TaskID, artifact.Key, redactArtifactLogError(err)))
			if firstErr == nil {
				firstErr = err
			}
			summary.Failed++
			continue
		}

		if err := fetchAndPersistArtifact(ctx, store, task, artifact, descriptor); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("failed to archive task %s artifact %s: %s", task.TaskID, artifact.Key, redactArtifactLogError(err)))
			if firstErr == nil {
				firstErr = err
			}
			summary.Failed++
			continue
		}
		summary.Archived++
	}

	if firstErr == nil {
		recordTaskArtifactArchiveState(ctx, task, common.GetTimestamp(), "")
		return
	}
	// A failed attempt stays eligible for the next pass; the recorded reason is
	// redacted before it is persisted.
	recordTaskArtifactArchiveState(ctx, task, 0, redactArtifactLogError(firstErr))
}

func resolveArtifactContentDescriptor(task *model.Task, artifactKey string) (*relaychannel.TaskContentRequest, error) {
	if !taskHasPluginExecution(task) {
		if artifactKey != "video" || !legacyVideoAvailable(task) {
			return nil, errors.New("legacy video unavailable")
		}
		return &relaychannel.TaskContentRequest{
			URL:            task.GetResultURL(),
			Method:         http.MethodGet,
			Credentialless: true,
		}, nil
	}

	adaptor, err := initTaskArtifactAdaptor(task)
	if err != nil {
		return nil, err
	}
	provider, ok := adaptor.(relaychannel.TaskContentRequestProvider)
	if !ok {
		return nil, errors.New("adaptor does not provide content requests")
	}
	return provider.BuildContentRequest(task, artifactKey, relaychannel.TaskArtifactClientRequest{
		Method: http.MethodGet,
	})
}

func fetchAndPersistArtifact(ctx context.Context, store service.TaskArtifactStore, task *model.Task, artifact relaychannel.TaskArtifact, descriptor *relaychannel.TaskContentRequest) error {
	rawURL := strings.TrimSpace(descriptor.URL)
	if rawURL == "" {
		return errors.New("empty descriptor URL")
	}

	if strings.HasPrefix(rawURL, "data:") {
		parts := strings.SplitN(rawURL, ",", 2)
		if len(parts) != 2 || !strings.Contains(parts[0], ";base64") {
			return errors.New("unsupported data URL")
		}
		data, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return err
		}
		headerMime := strings.TrimPrefix(parts[0], "data:")
		headerMime = strings.TrimSuffix(headerMime, ";base64")
		mimeType := artifact.MimeType
		if mimeType == "" {
			mimeType = headerMime
		}
		_, err = store.Persist(ctx, task, types.TaskArtifact{
			Key:      artifact.Key,
			Type:     artifact.Type,
			MimeType: mimeType,
		}, bytes.NewReader(data))
		return err
	}

	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return err
	}
	proxy := strings.TrimSpace(channelModel.GetSetting().Proxy)
	if err := validateTaskMediaURL(rawURL, proxy); err != nil {
		return err
	}

	client := service.GetSSRFProtectedHTTPClient()
	if proxy != "" {
		client, err = service.GetHttpClientWithProxy(proxy)
		if err != nil {
			return err
		}
	}

	var bodyReader io.Reader
	if len(descriptor.Body) > 0 {
		bodyReader = bytes.NewReader(descriptor.Body)
	}

	method := strings.ToUpper(strings.TrimSpace(descriptor.Method))
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return err
	}
	for k, v := range descriptor.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	mimeType := strings.TrimSpace(artifact.MimeType)
	if mimeType == "" {
		mimeType = strings.TrimSpace(resp.Header.Get("Content-Type"))
	}

	_, err = store.Persist(ctx, task, types.TaskArtifact{
		Key:      artifact.Key,
		Type:     artifact.Type,
		MimeType: mimeType,
	}, resp.Body)
	return err
}
