package protocolbridge

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
	openai openai.Adaptor
	gemini gemini.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.openai.Init(info)
	a.gemini.Init(info)
}

func (a *Adaptor) upstream(info *relaycommon.RelayInfo) (interface {
	GetRequestURL(*relaycommon.RelayInfo) (string, error)
	SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error
	DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error)
	DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError)
}, error) {
	if info == nil {
		return nil, errors.New("missing relay info")
	}
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		return &a.gemini, nil
	}
	if info.ChannelType == constant.ChannelTypeGemini2GPT {
		return &a.openai, nil
	}
	return nil, errors.New("invalid protocol bridge channel type")
}

func (a *Adaptor) bridge(c *gin.Context, info *relaycommon.RelayInfo, value any, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return relaycommon.MarshalAndApplyProtocolBridge(c, value, info)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		value, err := a.gemini.ConvertOpenAIRequest(c, info, request)
		return a.bridge(c, info, value, err)
	}
	value, err := a.openai.ConvertOpenAIRequest(c, info, request)
	return a.bridge(c, info, value, err)
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if info.ChannelType == constant.ChannelTypeGemini2GPT {
		if geminiRequestsImage(request) {
			imageRequest, err := geminiImageToOpenAIRequest(request, info)
			if err != nil {
				return nil, err
			}
			info.RelayMode = relayconstant.RelayModeImagesGenerations
			value, err := a.openai.ConvertImageRequest(c, info, imageRequest)
			return a.bridge(c, info, value, err)
		}
		value, err := a.openai.ConvertGeminiRequest(c, info, request)
		return a.bridge(c, info, value, err)
	}
	value, err := a.gemini.ConvertGeminiRequest(c, info, request)
	return a.bridge(c, info, value, err)
}

func geminiRequestsImage(request *dto.GeminiChatRequest) bool {
	if request == nil {
		return false
	}
	for _, modality := range request.GenerationConfig.ResponseModalities {
		if strings.EqualFold(modality, "IMAGE") {
			return true
		}
	}
	return request.GenerationConfig.ResponseFormat != nil && request.GenerationConfig.ResponseFormat.Image != nil
}

func geminiImageToOpenAIRequest(request *dto.GeminiChatRequest, info *relaycommon.RelayInfo) (dto.ImageRequest, error) {
	imageRequest := dto.ImageRequest{Model: info.UpstreamModelName}
	if request.GenerationConfig.Seed != nil {
		imageRequest.Seed = request.GenerationConfig.Seed
	}
	if image := request.GenerationConfig.ResponseFormat; image != nil && image.Image != nil {
		imageRequest.AspectRatio = image.Image.AspectRatio
		imageRequest.ImageSize = image.Image.ImageSize
	}
	for _, content := range request.Contents {
		for _, part := range content.Parts {
			if imageRequest.Prompt == "" && strings.TrimSpace(part.Text) != "" {
				imageRequest.Prompt = part.Text
			}
			if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "image/") {
				reference := fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)
				raw, err := rootcommon.Marshal(reference)
				if err != nil {
					return dto.ImageRequest{}, err
				}
				imageRequest.Image = raw
			}
			if part.FileData != nil && strings.HasPrefix(part.FileData.MimeType, "image/") {
				raw, err := rootcommon.Marshal(part.FileData.FileUri)
				if err != nil {
					return dto.ImageRequest{}, err
				}
				imageRequest.ImageURL = raw
			}
		}
	}
	if strings.TrimSpace(imageRequest.Prompt) == "" {
		return dto.ImageRequest{}, errors.New("Gemini image request requires a text prompt")
	}
	return imageRequest, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		value, err := a.gemini.ConvertClaudeRequest(c, info, request)
		return a.bridge(c, info, value, err)
	}
	value, err := a.openai.ConvertClaudeRequest(c, info, request)
	return a.bridge(c, info, value, err)
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		value, err := a.gemini.ConvertOpenAIResponsesRequest(c, info, request)
		return a.bridge(c, info, value, err)
	}
	value, err := a.openai.ConvertOpenAIResponsesRequest(c, info, request)
	return a.bridge(c, info, value, err)
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		value, err := a.gemini.ConvertImageRequest(c, info, request)
		return a.bridge(c, info, value, err)
	}
	value, err := a.openai.ConvertImageRequest(c, info, request)
	return a.bridge(c, info, value, err)
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		value, err := a.gemini.ConvertEmbeddingRequest(c, info, request)
		return a.bridge(c, info, value, err)
	}
	value, err := a.openai.ConvertEmbeddingRequest(c, info, request)
	return a.bridge(c, info, value, err)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.ChannelType == constant.ChannelTypeGPT2Gemini {
		return a.gemini.ConvertAudioRequest(c, info, request)
	}
	return a.openai.ConvertAudioRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return a.openai.ConvertRerankRequest(c, relayMode, request)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info != nil && info.ChannelType == constant.ChannelTypeGemini2GPT &&
		(info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits) {
		if endpoints := info.ChannelOtherSettings.ImageTaskEndpoints; endpoints != nil {
			if path := strings.TrimSpace(endpoints.SubmitPath); path != "" {
				return strings.TrimRight(info.ChannelBaseUrl, "/") + path, nil
			}
		}
	}
	upstream, err := a.upstream(info)
	if err != nil {
		return "", err
	}
	return upstream.GetRequestURL(info)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	upstream, err := a.upstream(info)
	if err != nil {
		return err
	}
	return upstream.SetupRequestHeader(c, header, info)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	upstream, err := a.upstream(info)
	if err != nil {
		return nil, err
	}
	return upstream.DoRequest(c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info != nil && info.ChannelType == constant.ChannelTypeGemini2GPT &&
		(info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits) {
		return convertOpenAIImageResponseToGemini(c, resp)
	}
	upstream, err := a.upstream(info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	return upstream.DoResponse(c, resp, info)
}

func convertOpenAIImageResponseToGemini(c *gin.Context, resp *http.Response) (any, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	var payload struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
			Revised string `json:"revised_prompt"`
		} `json:"data"`
		Usage dto.Usage `json:"usage"`
	}
	if err := rootcommon.Unmarshal(data, &payload); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	response := dto.GeminiChatResponse{Candidates: make([]dto.GeminiChatCandidate, 0, len(payload.Data))}
	for _, item := range payload.Data {
		parts := make([]dto.GeminiPart, 0, 2)
		if item.Revised != "" {
			parts = append(parts, dto.GeminiPart{Text: item.Revised})
		}
		if item.B64JSON != "" {
			parts = append(parts, dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: "image/png", Data: item.B64JSON}})
		} else if item.URL != "" {
			parts = append(parts, dto.GeminiPart{FileData: &dto.GeminiFileData{FileUri: item.URL, MimeType: "image/*"}})
		}
		response.Candidates = append(response.Candidates, dto.GeminiChatCandidate{Content: dto.GeminiChatContent{Role: "model", Parts: parts}})
	}
	encoded, err := rootcommon.Marshal(response)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", encoded)
	return &payload.Usage, nil
}

func (a *Adaptor) GetModelList() []string {
	return lo.Uniq(append(append([]string{}, openai.ModelList...), gemini.ModelList...))
}

func (a *Adaptor) GetChannelName() string { return "protocol_bridge" }
