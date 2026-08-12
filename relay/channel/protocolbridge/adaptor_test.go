package protocolbridge

import (
	"bytes"
	"net/http/httptest"
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPT2GeminiImageUsesBuiltInMediaConversionAndCustomMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(`{
		"model":"public-image","prompt":"pixel dinosaur","aspect_ratio":"16:9",
		"image_size":"2K","seed":0,"vendor":{"style":"plush"}
	}`))
	_, err := rootcommon.GetBodyStorage(c)
	require.NoError(t, err)
	seed := int64(0)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGPT2Gemini,
			UpstreamModelName: "gemini-3.1-flash-image",
			ChannelOtherSettings: dto.ChannelOtherSettings{ProtocolBridge: &dto.ProtocolBridgeConfig{
				FieldMappings: map[string]string{"vendor.style": "vendorStyle"},
			}},
		},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
		Model: "gemini-3.1-flash-image", Prompt: "pixel dinosaur", AspectRatio: "16:9", ImageSize: "2K", Seed: &seed,
	})
	require.NoError(t, err)
	body, ok := converted.(map[string]any)
	require.True(t, ok)
	encoded, err := rootcommon.Marshal(body)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"aspectRatio":"16:9"`)
	assert.Contains(t, string(encoded), `"imageSize":"2K"`)
	assert.Contains(t, string(encoded), `"seed":0`)
	assert.Contains(t, string(encoded), `"vendorStyle":"plush"`)
}

func TestGPT2GeminiImageNormalizesCanvasNanoBananaAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations?async=true", bytes.NewBufferString(`{
		"model":"nano-banana-2","prompt":"pixel dinosaur","response_format":"url",
		"task_count":1,"async":true,"aspect_ratio":"16:9","image_size":"2K"
	}`))
	_, err := rootcommon.GetBodyStorage(c)
	require.NoError(t, err)
	taskCount := uint(1)
	async := true
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "nano-banana-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGPT2Gemini,
			ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
			UpstreamModelName: "nano-banana-2",
		},
	}
	request := dto.ImageRequest{
		Model: "nano-banana-2", Prompt: "pixel dinosaur", ResponseFormat: "url",
		TaskCount: &taskCount, Async: &async, AspectRatio: "16:9", ImageSize: "2K",
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(c, info, request)
	require.NoError(t, err)
	assert.Equal(t, "gemini-3.1-flash-image", info.UpstreamModelName)

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-image:generateContent", requestURL)

	body, ok := converted.(map[string]any)
	require.True(t, ok)
	encoded, err := rootcommon.Marshal(body)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"responseModalities":["TEXT","IMAGE"]`)
	assert.Contains(t, string(encoded), `"aspectRatio":"16:9"`)
	assert.Contains(t, string(encoded), `"imageSize":"2K"`)
	assert.NotContains(t, string(encoded), `"task_count"`)
	assert.NotContains(t, string(encoded), `"async"`)
}

func TestGPT2GeminiImageKeepsExplicitChannelModelMapping(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		OriginModelName: "nano-banana-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGPT2Gemini,
			UpstreamModelName: "gemini-3.1-flash-image-preview",
			IsModelMapped:     true,
		},
	}

	_, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Model: "gemini-3.1-flash-image-preview", Prompt: "pixel dinosaur",
	})
	require.NoError(t, err)
	assert.Equal(t, "gemini-3.1-flash-image-preview", info.UpstreamModelName)
}

func TestGemini2GPTChatChoosesOpenAIEndpoint(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeGemini,
		RelayFormat:    types.RelayFormatGemini,
		RequestURLPath: "/v1beta/models/client-model:generateContent",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeGemini2GPT,
			ChannelBaseUrl:    "https://upstream.example",
			UpstreamModelName: "gpt-image-vendor",
		},
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example/v1/chat/completions", requestURL)
}

func TestGemini2GPTImageSupportsCustomSubmitPath(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeGemini2GPT,
			ChannelBaseUrl: "https://upstream.example",
			ChannelOtherSettings: dto.ChannelOtherSettings{ImageTaskEndpoints: &dto.ImageTaskEndpoints{
				SubmitPath: "/v1/image/generations",
			}},
		},
	}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example/v1/image/generations", requestURL)
}

func TestGemini2GPTConvertsGeminiImageRequestToOpenAIImages(t *testing.T) {
	seed := int64(0)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:       constant.ChannelTypeGemini2GPT,
		UpstreamModelName: "vendor-image-model",
	}}
	request := &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{Text: "pixel dinosaur"}}}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			Seed:               &seed,
			ResponseFormat: &dto.GeminiResponseFormat{Image: &dto.GeminiResponseImageConfig{
				AspectRatio: "16:9",
				ImageSize:   "2K",
			}},
		},
	}

	converted, err := (&Adaptor{}).ConvertGeminiRequest(nil, info, request)
	require.NoError(t, err)
	assert.Equal(t, relayconstant.RelayModeImagesGenerations, info.RelayMode)
	body, ok := converted.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "vendor-image-model", body["model"])
	assert.Equal(t, "pixel dinosaur", body["prompt"])
	assert.Equal(t, "16:9", body["aspect_ratio"])
	assert.Equal(t, "2K", body["image_size"])
	assert.Equal(t, float64(0), body["seed"])
}
