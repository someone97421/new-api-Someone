package gemini

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
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
	assert.JSONEq(t, `{"aspectRatio":"3:2","imageSize":"2K"}`, string(request.GenerationConfig.ImageConfig))
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
