package gemini

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertImageRequestToGeminiNativeImageGeneration(t *testing.T) {
	n := uint(1)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"},
	}

	converted, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Prompt:  "一只像素小恐龙",
		N:       &n,
		Size:    "1536x1024",
		Quality: "high",
	})
	require.NoError(t, err)
	request, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.Len(t, request.Contents, 1)
	require.Len(t, request.Contents[0].Parts, 1)
	assert.Equal(t, "user", request.Contents[0].Role)
	assert.Equal(t, "一只像素小恐龙", request.Contents[0].Parts[0].Text)
	assert.Equal(t, []string{"TEXT", "IMAGE"}, request.GenerationConfig.ResponseModalities)
	assert.Nil(t, request.GenerationConfig.CandidateCount)
	assert.Nil(t, request.GenerationConfig.ResponseFormat)
	assert.JSONEq(t, `{"aspectRatio":"3:2","imageSize":"2K"}`, string(request.GenerationConfig.ImageConfig))
}

func TestConvertExtendedOpenAIImageRequestToGeminiNativeImageGeneration(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"},
	}
	one := uint(1)
	async := true
	images, err := common.Marshal([]string{"https://example.com/reference.png"})
	require.NoError(t, err)

	converted, err := convertGeminiNativeImageRequest(dto.ImageRequest{
		Prompt:      "保持主体，改为夜景",
		AspectRatio: "16:9",
		ImageSize:   "4K",
		TaskCount:   &one,
		Async:       &async,
		Images:      images,
	}, info, func(reference string) (string, string, error) {
		require.Equal(t, "https://example.com/reference.png", reference)
		return "image/png", base64.StdEncoding.EncodeToString([]byte("reference")), nil
	})
	require.NoError(t, err)
	require.Len(t, converted.Contents, 1)
	require.Len(t, converted.Contents[0].Parts, 2)
	assert.Equal(t, "保持主体，改为夜景", converted.Contents[0].Parts[0].Text)
	require.NotNil(t, converted.Contents[0].Parts[1].InlineData)
	assert.Equal(t, "image/png", converted.Contents[0].Parts[1].InlineData.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("reference")), converted.Contents[0].Parts[1].InlineData.Data)
	assert.Nil(t, converted.GenerationConfig.ResponseFormat)
	assert.JSONEq(t, `{"aspectRatio":"16:9","imageSize":"4K"}`, string(converted.GenerationConfig.ImageConfig))
}

func TestConvertGeminiNativeImageRequestSupportsCompatibilityResponseFormat(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	originalMode := settings.ImageGenerationConfigMode
	settings.ImageGenerationConfigMode = model_setting.GeminiImageGenerationConfigResponseFormat
	t.Cleanup(func() { settings.ImageGenerationConfigMode = originalMode })

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"},
	}
	converted, err := convertGeminiNativeImageRequest(dto.ImageRequest{
		Prompt:      "legacy gateway",
		AspectRatio: "1:1",
		ImageSize:   "2K",
	}, info, func(string) (string, string, error) {
		return "", "", nil
	})
	require.NoError(t, err)
	require.NotNil(t, converted.GenerationConfig.ResponseFormat)
	assert.Equal(t, "1:1", converted.GenerationConfig.ResponseFormat.Image.AspectRatio)
	assert.Equal(t, "2K", converted.GenerationConfig.ResponseFormat.Image.ImageSize)
	assert.Empty(t, converted.GenerationConfig.ImageConfig)
}

func TestConvertGeminiNativeImageRequestRejectsTooManyReferences(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"},
	}
	references := make([]string, maxGeminiImageReferences+1)
	for index := range references {
		references[index] = fmt.Sprintf("https://example.com/reference-%d.png", index)
	}
	images, err := common.Marshal(references)
	require.NoError(t, err)

	_, err = convertGeminiNativeImageRequest(dto.ImageRequest{
		Prompt: "too many references",
		Images: images,
	}, info, func(string) (string, string, error) {
		return "image/png", "data", nil
	})
	require.EqualError(t, err, "Gemini image generation accepts at most 16 reference images")
}

func TestConvertImageRequestRejectsMultipleGeminiNativeImagesForChannelRetry(t *testing.T) {
	n := uint(2)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"},
	}

	_, err := (&Adaptor{}).ConvertImageRequest(nil, info, dto.ImageRequest{
		Prompt: "两只像素小恐龙",
		N:      &n,
	})
	require.EqualError(t, err, "Gemini native image generation supports exactly one image per request")
}

func TestGeminiNativeImageHandlerReturnsOpenAIImageResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-3.1-flash-image",
		},
	}

	finishReason := "STOP"
	payload := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				FinishReason: &finishReason,
				Content: dto.GeminiChatContent{
					Role: "model",
					Parts: []dto.GeminiPart{
						{Text: "增强后的提示词"},
						{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: "aW1hZ2U="}},
					},
				},
			},
		},
		UsageMetadata: dto.GeminiUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 20,
			TotalTokenCount:      30,
		},
		HasUsageMetadata: true,
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}

	usage, apiErr := GeminiNativeImageHandler(context, info, response)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 30, usage.TotalTokens)
	var imageResponse dto.ImageResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &imageResponse))
	require.Len(t, imageResponse.Data, 1)
	assert.Equal(t, "aW1hZ2U=", imageResponse.Data[0].B64Json)
	assert.Equal(t, "增强后的提示词", imageResponse.Data[0].RevisedPrompt)
}
