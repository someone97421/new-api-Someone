package service

import (
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncMediaWorkerReplaysSyncImageRequestAndStoresBase64Result(t *testing.T) {
	truncate(t)
	token := &model.Token{Id: 901, UserId: 902, Key: "async-worker-key", Name: "async-worker-token"}
	require.NoError(t, model.DB.Create(token).Error)

	jobID := "job_sync_worker_contract"
	requestFile, requestSize, err := SaveAsyncMediaRequest(jobID, strings.NewReader(`{"model":"image-model","prompt":"cat"}`))
	require.NoError(t, err)
	job := &model.AsyncMediaJob{
		JobID:          jobID,
		UserID:         token.UserId,
		TokenID:        token.Id,
		Method:         http.MethodPost,
		RequestPath:    "/v1/images/generations",
		RawQuery:       "async=true&trace=1",
		RequestHeaders: `{"Content-Type":["application/json"]}`,
		RequestFile:    requestFile,
		RequestSize:    requestSize,
	}
	require.NoError(t, model.CreateAsyncMediaJob(job))
	claimed, won, err := model.ClaimNextAsyncMediaJob("worker-contract", time.Now().Unix()+300)
	require.NoError(t, err)
	require.True(t, won)

	imageBytes := []byte("generated-image")
	handlerCalled := false
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handlerCalled = true
		assert.Equal(t, "", request.URL.Query().Get("async"))
		assert.Equal(t, "1", request.URL.Query().Get("trace"))
		assert.Equal(t, "Bearer sk-async-worker-key", request.Header.Get("Authorization"))
		assert.Equal(t, jobID, request.Header.Get(asyncMediaInternalJobHeader))
		assert.True(t, ValidateAsyncMediaInternalRequest(jobID, request.Header.Get(asyncMediaInternalSignatureHeader)))
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		assert.JSONEq(t, `{"model":"image-model","prompt":"cat"}`, string(body))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(imageBytes) + `"}]}`))
	})

	processAsyncMediaJob(handler, "worker-contract", claimed)
	assert.True(t, handlerCalled)
	persisted, err := model.GetAsyncMediaJobForUser(jobID, token.UserId)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, model.AsyncMediaJobStatusSucceeded, persisted.Status)
	assert.Equal(t, model.AsyncMediaBillingSettled, persisted.BillingStatus)
	require.Len(t, persisted.Files, 1)
	absolute, err := ResolveAsyncMediaPath(persisted.Files[0].Path)
	require.NoError(t, err)
	stored, err := os.ReadFile(absolute)
	require.NoError(t, err)
	assert.Equal(t, imageBytes, stored)
}

func TestAsyncMediaWorkerLocalizesCompletedNativeTaskDataURI(t *testing.T) {
	truncate(t)
	jobID := "job_native_worker_contract"
	job := &model.AsyncMediaJob{
		JobID:         jobID,
		UserID:        903,
		TokenID:       904,
		Method:        http.MethodPost,
		RequestPath:   "/v1/videos",
		Status:        model.AsyncMediaJobStatusWaitingUpstream,
		BillingStatus: model.AsyncMediaBillingDelegated,
		OriginTaskID:  "task_native_contract",
		NextRunAt:     time.Now().Unix(),
	}
	require.NoError(t, model.CreateAsyncMediaJob(job))
	videoBytes := []byte("generated-video")
	originTask := &model.Task{
		TaskID:     "task_native_contract",
		AsyncJobID: jobID,
		UserId:     job.UserID,
		Status:     model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(videoBytes),
		},
	}
	require.NoError(t, model.DB.Create(originTask).Error)

	claimed, won, err := model.ClaimNextAsyncMediaJob("worker-native", time.Now().Unix()+300)
	require.NoError(t, err)
	require.True(t, won)
	processAsyncMediaJob(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("native async completion must not replay the submit request")
	}), "worker-native", claimed)

	persisted, err := model.GetAsyncMediaJobForUser(jobID, job.UserID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, model.AsyncMediaJobStatusSucceeded, persisted.Status)
	assert.Equal(t, model.AsyncMediaBillingDelegated, persisted.BillingStatus)
	require.Len(t, persisted.Files, 1)
}

func TestAsyncMediaWorkerPollsNativeImageTaskReturnedBySyncRelay(t *testing.T) {
	truncate(t)
	token := &model.Token{Id: 905, UserId: 906, Key: "native-image-key", Name: "native-image-token"}
	require.NoError(t, model.DB.Create(token).Error)
	jobID := "job_native_image_contract"
	requestFile, requestSize, err := SaveAsyncMediaRequest(jobID, strings.NewReader(`{"model":"native-image-model","prompt":"cat"}`))
	require.NoError(t, err)
	job := &model.AsyncMediaJob{
		JobID:          jobID,
		UserID:         token.UserId,
		TokenID:        token.Id,
		Method:         http.MethodPost,
		RequestPath:    "/v1/images/generations",
		RawQuery:       "async=true",
		RequestHeaders: `{"Content-Type":["application/json"]}`,
		RequestFile:    requestFile,
		RequestSize:    requestSize,
	}
	require.NoError(t, model.CreateAsyncMediaJob(job))
	claimed, won, err := model.ClaimNextAsyncMediaJob("worker-native-image-submit", time.Now().Unix()+300)
	require.NoError(t, err)
	require.True(t, won)

	imageBytes := []byte("native-image-result")
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodPost {
			writer.Header().Set(AsyncMediaInternalChannelHeader, "77")
			_, _ = writer.Write([]byte(`{"task_id":"upstream-image-1","status":"queued"}`))
			return
		}
		assert.Equal(t, "/v1/images/generations/upstream-image-1", request.URL.Path)
		assert.Equal(t, "native-image-model", request.URL.Query().Get("model"))
		assert.Equal(t, "77", request.Header.Get(AsyncMediaInternalChannelHeader))
		_, _ = writer.Write([]byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(imageBytes) + `"}]}`))
	})
	processAsyncMediaJob(handler, "worker-native-image-submit", claimed)

	var waiting model.AsyncMediaJob
	require.NoError(t, model.DB.Where("job_id = ?", jobID).First(&waiting).Error)
	assert.Equal(t, model.AsyncMediaJobStatusWaitingUpstream, waiting.Status)
	assert.Equal(t, 77, waiting.UpstreamChannelID)
	assert.Equal(t, "upstream-image-1", waiting.UpstreamTaskID)
	assert.Equal(t, "native-image-model", waiting.ModelName)
	require.NoError(t, model.DB.Model(&model.AsyncMediaJob{}).Where("job_id = ?", jobID).Update("next_run_at", time.Now().Unix()).Error)

	claimed, won, err = model.ClaimNextAsyncMediaJob("worker-native-image-poll", time.Now().Unix()+300)
	require.NoError(t, err)
	require.True(t, won)
	processAsyncMediaJob(handler, "worker-native-image-poll", claimed)

	persisted, err := model.GetAsyncMediaJobForUser(jobID, token.UserId)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, model.AsyncMediaJobStatusSucceeded, persisted.Status)
	require.Len(t, persisted.Files, 1)
}

func TestExtractAsyncMediaUpstreamTerminalError(t *testing.T) {
	responsePath, responseFile, err := CreateAsyncMediaResponseFile("job_terminal_error")
	require.NoError(t, err)
	_, err = responseFile.Write([]byte(`{
		"code":"success",
		"data":{"status":"failed","fail_reason":"provider rejected the prompt"}
	}`))
	require.NoError(t, err)
	require.NoError(t, responseFile.Close())
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(responsePath) })

	assert.Equal(t, "provider rejected the prompt", extractAsyncMediaUpstreamTerminalError(responsePath))
}
