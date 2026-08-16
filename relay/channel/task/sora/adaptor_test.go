package sora

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAdaptorUsesConfiguredVideoTaskEndpoints(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.EscapedPath()
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(server.Close)

	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: server.URL,
			ChannelOtherSettings: dto.ChannelOtherSettings{VideoTaskEndpoints: &dto.VideoTaskEndpoints{
				SubmitPath: "/v1/video/generations",
				QueryPath:  "/v1/video/generations/{task_id}",
				RemixPath:  "/v1/video/generations/{video_id}/remix",
			}},
		}}
	adaptor.Init(info)

	submitURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/video/generations", submitURL)
	info.Action = constant.TaskActionRemix
	info.OriginTaskID = "video/with space"
	remixURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/v1/video/generations/video%2Fwith%20space/remix", remixURL)

	response, err := adaptor.FetchTask(server.URL, "test-key", map[string]any{"task_id": "task/with space"}, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	assert.Equal(t, "/v1/video/generations/task%2Fwith%20space", requestedPath)
}

func TestTaskAdaptorKeepsDefaultVideoTaskEndpoints(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://video.example",
		}}
	adaptor.Init(info)

	submitURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://video.example/v1/videos", submitURL)
}

func TestTaskAdaptorParsesSucceededVideoGeneration(t *testing.T) {
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult([]byte(`{
		"task_id":"task_123",
		"status":"succeeded",
		"progress":100,
		"url":"https://assets.example/video.mp4",
		"result_asset_url":"https://assets.example/video.mp4"
	}`))

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://assets.example/video.mp4", result.Url)
	assert.Equal(t, "100%", result.Progress)
}

func TestSoraBuildRequestBodyReturnsReplayablePassThroughBody(t *testing.T) {
	payload := []byte("opaque-sora-request-body")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/octet-stream")
	defer common.CleanupBodyStorage(c)

	info := &relaycommon.RelayInfo{}
	body, err := (&TaskAdaptor{}).BuildRequestBody(c, info)
	require.NoError(t, err)
	replayable, ok := body.(common.ReplayableBody)
	require.True(t, ok)

	sent, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, payload, sent)
	assert.EqualValues(t, len(payload), replayable.Size())

	replayBody, err := replayable.NewReader()
	require.NoError(t, err)
	replay, err := io.ReadAll(replayBody)
	require.NoError(t, err)
	require.NoError(t, replayBody.Close())
	assert.Equal(t, payload, replay)
}
