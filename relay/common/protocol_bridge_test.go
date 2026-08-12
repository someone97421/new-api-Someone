package common

import (
	"bytes"
	"net/http/httptest"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyProtocolBridgeConfigCopiesMappedAndExplicitZeroValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(`{
		"aspect_ratio":"16:9",
		"seed":0,
		"watermark":false,
		"vendor":{"camera":"orbit"}
	}`))
	_, err := rootcommon.GetBodyStorage(c)
	require.NoError(t, err)

	info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{
		ProtocolBridge: &dto.ProtocolBridgeConfig{
			PassthroughFields: []string{"seed", "watermark", "vendor.camera"},
			FieldMappings: map[string]string{
				"aspect_ratio": "generationConfig.responseFormat.image.aspectRatio",
			},
		},
	}}}

	result, err := ApplyProtocolBridgeConfig(c, []byte(`{"model":"upstream"}`), info)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"upstream",
		"seed":0,
		"watermark":false,
		"vendor":{"camera":"orbit"},
		"generationConfig":{"responseFormat":{"image":{"aspectRatio":"16:9"}}}
	}`, string(result))
}

func TestApplyProtocolBridgeConfigDoesNotInventMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(`{"model":"client"}`))
	_, err := rootcommon.GetBodyStorage(c)
	require.NoError(t, err)
	info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{
		ProtocolBridge: &dto.ProtocolBridgeConfig{PassthroughFields: []string{"seed"}},
	}}}

	result, err := ApplyProtocolBridgeConfig(c, []byte(`{"model":"upstream"}`), info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"upstream"}`, string(result))
}

func TestApplyProtocolBridgeConfigUsesKnownMediaFieldMappings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(`{
		"aspect_ratio":"9:16",
		"image_size":"4K",
		"seed":0,
		"generate_audio":false,
		"references":[{"type":"image","role":"reference_image","url":"https://example.com/ref.png"}]
	}`))
	_, err := rootcommon.GetBodyStorage(c)
	require.NoError(t, err)
	info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{
		ProtocolBridge: &dto.ProtocolBridgeConfig{
			PassthroughFields: []string{"seed", "generate_audio", "references"},
			FieldMappings: map[string]string{
				"aspect_ratio": "generationConfig.imageConfig.aspectRatio",
				"image_size":   "generationConfig.imageConfig.imageSize",
			},
		},
	}}}

	result, err := ApplyProtocolBridgeConfig(c, []byte(`{"contents":[]}`), info)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"contents":[],
		"seed":0,
		"generate_audio":false,
		"references":[{"type":"image","role":"reference_image","url":"https://example.com/ref.png"}],
		"generationConfig":{"imageConfig":{"aspectRatio":"9:16","imageSize":"4K"}}
	}`, string(result))
}
