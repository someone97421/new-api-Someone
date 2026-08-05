package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelRequestUsesImageTaskQueryModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/images/generations/task-1?model=nano-banana-2", nil)

	request, shouldSelectChannel, err := getModelRequest(ctx)

	require.NoError(t, err)
	assert.True(t, shouldSelectChannel)
	assert.Equal(t, "nano-banana-2", request.Model)
}

func TestGetModelRequestRejectsImageTaskQueryWithoutModel(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/images/generations/task-1", nil)

	_, _, err := getModelRequest(ctx)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "model")
}

func TestAdvancedCustomChannelSupportsConfiguredImageTaskQueryPath(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/images/generations",
			UpstreamPath: "/v1/image/generations",
			Converter:    "none",
		}}},
		ImageTaskEndpoints: &dto.ImageTaskEndpoints{QueryPath: "/v1/image/generations/{task_id}"},
	})

	assert.True(t, channelSupportsRequestPath(channel, "/v1/images/generations/task-1", "nano-banana-2"))
}
