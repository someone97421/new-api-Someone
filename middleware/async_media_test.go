package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncMediaRequestOnlyInterceptsSupportedPostEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method string
		url    string
		want   bool
	}{
		{name: "image JSON", method: http.MethodPost, url: "/v1/images/generations?async=true", want: true},
		{name: "image multipart", method: http.MethodPost, url: "/v1/images/edits?async=1", want: true},
		{name: "video", method: http.MethodPost, url: "/v1/videos?async=true", want: true},
		{name: "sync remains sync", method: http.MethodPost, url: "/v1/images/generations", want: false},
		{name: "unsupported chat", method: http.MethodPost, url: "/v1/chat/completions?async=true", want: false},
		{name: "status query", method: http.MethodGet, url: "/v1/images/generations/task?async=true", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.url, nil)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			require.NotNil(t, context.Request)
			assert.Equal(t, test.want, test.method == http.MethodPost && isAsyncMediaRequest(context))
		})
	}
}

func TestAsyncMediaInternalRequestBindsUpstreamChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodGet, "/v1/images/generations/task-1?model=image-model", nil)
	jobID := "job_channel_binding_contract"
	request.Header.Set("X-New-API-Async-Job-ID", jobID)
	request.Header.Set("X-New-API-Async-Worker-Signature", service.AsyncMediaInternalSignature(jobID))
	request.Header.Set(service.AsyncMediaInternalChannelHeader, "77")

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	AsyncMediaEnqueue()(context)

	channelID, ok := common.GetContextKey(context, constant.ContextKeyTokenSpecificChannelId)
	require.True(t, ok)
	assert.Equal(t, "77", channelID)
	assert.True(t, context.GetBool(asyncMediaOriginChannelContextKey))
}

func TestSetupContextReportsFinalChannelToAsyncWorker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	jobID := "job_selected_channel_contract"
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	context.Request.Header.Set("X-New-API-Async-Job-ID", jobID)
	context.Request.Header.Set("X-New-API-Async-Worker-Signature", service.AsyncMediaInternalSignature(jobID))

	apiErr := SetupContextForSelectedChannel(context, &model.Channel{Id: 88, Key: "upstream-key"}, "image-model")
	require.Nil(t, apiErr)
	assert.Equal(t, "88", recorder.Header().Get(service.AsyncMediaInternalChannelHeader))
}
