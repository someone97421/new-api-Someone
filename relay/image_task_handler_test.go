package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskFetchUsesConfiguredQueryEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var requestedPath string
	var authorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream-Test", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"code":"success","data":{"status":"running"}}`)
	}))
	t.Cleanup(upstream.Close)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/images/generations/task%2Fwith%20space?model=image-model", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task/with space"}}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: upstream.URL + "/",
			ApiKey:         "upstream-key",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				ImageTaskEndpoints: &dto.ImageTaskEndpoints{
					QueryPath: "/v1/image/generations/{task_id}",
				},
			},
		},
	}

	apiErr := ImageTaskFetch(ctx, info)

	require.Nil(t, apiErr)
	assert.Equal(t, "/v1/image/generations/task%2Fwith%20space", requestedPath)
	assert.Equal(t, "Bearer upstream-key", authorization)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "ok", recorder.Header().Get("X-Upstream-Test"))
	assert.JSONEq(t, `{"code":"success","data":{"status":"running"}}`, recorder.Body.String())
}

func TestImageTaskFetchRequiresConfiguredQueryEndpoint(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/images/generations/task-1?model=image-model", nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: "task-1"}}

	apiErr := ImageTaskFetch(ctx, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})

	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), "image_task_endpoints.query_path")
}
