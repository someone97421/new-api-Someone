package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, req)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return geminiRequest, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) {
		return convertGeminiNativeImageRequest(request, info, loadGeminiImageReference)
	}

	// convert size to aspect ratio but allow user to specify aspect ratio
	aspectRatio := resolveGeminiImageAspectRatio(request)
	imageSize := resolveGeminiImageSize(request)

	imageN := lo.FromPtrOr(request.N, uint(1))
	if imageN == 0 {
		imageN = 1
	}

	if !strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return nil, errors.New("not supported model for Gemini image generation")
	}

	// build gemini imagen request
	geminiRequest := dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(imageN),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult", // default allow adult
			ImageSize:        imageSize,
		},
	}

	return geminiRequest, nil
}

type geminiImageReferenceLoader func(reference string) (mimeType string, data string, err error)

const maxGeminiImageReferences = 16

func convertGeminiNativeImageRequest(request dto.ImageRequest, info *relaycommon.RelayInfo, loadReference geminiImageReferenceLoader) (*dto.GeminiChatRequest, error) {
	if info == nil || !model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) {
		return nil, errors.New("not supported model for Gemini image generation")
	}

	imageN := lo.FromPtrOr(request.N, uint(1))
	if request.TaskCount != nil {
		imageN = *request.TaskCount
	}
	if imageN == 0 {
		imageN = 1
	}
	if imageN != 1 {
		return nil, errors.New("Gemini native image generation supports exactly one image per request")
	}

	parts := []dto.GeminiPart{{Text: request.Prompt}}
	references, err := extractGeminiImageReferences(request)
	if err != nil {
		return nil, err
	}
	for _, reference := range references {
		mimeType, data, err := loadReference(reference)
		if err != nil {
			return nil, fmt.Errorf("load Gemini reference image: %w", err)
		}
		parts = append(parts, dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: data},
		})
	}

	imageConfig := &dto.GeminiResponseImageConfig{
		AspectRatio: resolveGeminiImageAspectRatio(request),
		ImageSize:   resolveGeminiImageSize(request),
	}
	generationConfig := dto.GeminiChatGenerationConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
		Seed:               request.Seed,
	}
	if model_setting.GetGeminiSettings().ImageGenerationConfigMode == model_setting.GeminiImageGenerationConfigLegacy {
		legacyImageConfig, err := common.Marshal(imageConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal legacy Gemini image config: %w", err)
		}
		generationConfig.ImageConfig = legacyImageConfig
	} else {
		generationConfig.ResponseFormat = &dto.GeminiResponseFormat{Image: imageConfig}
	}
	return &dto.GeminiChatRequest{
		Contents:         []dto.GeminiChatContent{{Role: "user", Parts: parts}},
		GenerationConfig: generationConfig,
	}, nil
}

func resolveGeminiImageAspectRatio(request dto.ImageRequest) string {
	if aspectRatio := strings.TrimSpace(request.AspectRatio); aspectRatio != "" {
		return aspectRatio
	}
	size := strings.TrimSpace(request.Size)
	if strings.Contains(size, ":") {
		return size
	}
	switch size {
	case "1536x1024":
		return "3:2"
	case "1024x1536":
		return "2:3"
	case "1024x1792":
		return "9:16"
	case "1792x1024":
		return "16:9"
	default:
		return "1:1"
	}
}

func resolveGeminiImageSize(request dto.ImageRequest) string {
	if imageSize := strings.ToUpper(strings.TrimSpace(request.ImageSize)); imageSize != "" {
		return imageSize
	}
	if resolution := strings.ToUpper(strings.TrimSpace(request.Resolution)); resolution != "" {
		return resolution
	}
	switch strings.ToLower(strings.TrimSpace(request.Quality)) {
	case "hd", "high", "2k":
		return "2K"
	case "standard", "medium", "low", "auto", "1k":
		return "1K"
	default:
		return ""
	}
}

func extractGeminiImageReferences(request dto.ImageRequest) ([]string, error) {
	references := make([]string, 0)
	seen := make(map[string]bool)
	fields := []struct {
		name string
		raw  []byte
	}{
		{name: "image", raw: request.Image},
		{name: "images", raw: request.Images},
		{name: "image_url", raw: request.ImageURL},
		{name: "image_urls", raw: request.ImageURLs},
	}
	for _, field := range fields {
		name := field.name
		raw := field.raw
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var single string
		if err := common.Unmarshal(raw, &single); err == nil {
			single = strings.TrimSpace(single)
			if single != "" && !seen[single] {
				seen[single] = true
				references = append(references, single)
			}
			continue
		}
		var multiple []string
		if err := common.Unmarshal(raw, &multiple); err != nil {
			return nil, fmt.Errorf("%s must be a URL string or URL string array", name)
		}
		for _, reference := range multiple {
			reference = strings.TrimSpace(reference)
			if reference != "" && !seen[reference] {
				seen[reference] = true
				references = append(references, reference)
			}
		}
	}
	if len(references) > maxGeminiImageReferences {
		return nil, fmt.Errorf("Gemini image generation accepts at most %d reference images", maxGeminiImageReferences)
	}
	return references, nil
}

func loadGeminiImageReference(reference string) (string, string, error) {
	switch {
	case strings.HasPrefix(reference, "https://") || strings.HasPrefix(reference, "http://"):
		return service.GetImageFromUrl(reference)
	case strings.HasPrefix(reference, "data:image/"):
		return service.DecodeBase64FileData(reference)
	default:
		return "", "", errors.New("reference image must be an HTTP/HTTPS URL or image data URI")
	}
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled &&
		!model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(info.UpstreamModelName, "-thinking-") {
			parts := strings.Split(info.UpstreamModelName, "-thinking-")
			info.UpstreamModelName = parts[0]
		} else if strings.HasSuffix(info.UpstreamModelName, "-thinking") { // 旧的适配
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		} else if strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-nothinking")
		} else if baseModel, level, ok := reasoning.TrimEffortSuffix(info.UpstreamModelName); ok && level != "" {
			info.UpstreamModelName = baseModel
		}
	}

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)
	baseURL := geminiVersionBaseURL(info.ChannelBaseUrl, version)

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/models/%s:predict", baseURL, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/models/%s:%s", baseURL, info.UpstreamModelName, action), nil
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/models/%s:%s", baseURL, info.UpstreamModelName, action), nil
}

func geminiVersionBaseURL(baseURL string, version string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	version = strings.Trim(strings.TrimSpace(version), "/")
	if version == "" || strings.HasSuffix(baseURL, "/"+version) {
		return baseURL
	}
	return baseURL + "/" + version
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, request)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			dimensions := lo.FromPtrOr(request.Dimensions, 0)
			if dimensions > 0 {
				geminiRequest["outputDimensionality"] = dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	result, err := relayconvert.ConvertRequest(c, info, types.RelayFormatGemini, &request)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return geminiRequest, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeResponses {
		if info.IsStream {
			return GeminiResponsesStreamHandler(c, info, resp)
		}
		return GeminiResponsesHandler(c, info, resp)
	}

	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}
	if info.RelayMode == constant.RelayModeImagesGenerations && model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) {
		return GeminiNativeImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
