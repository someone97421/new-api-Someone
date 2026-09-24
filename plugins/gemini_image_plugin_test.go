package plugins_test

import (
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const geminiImagePluginKey = "gemini-image"

func loadGeminiImagePlugin(t *testing.T) (*jsplugin.LoadedPlugin, *jsplugin.Registry) {
	t.Helper()
	source, err := builtinplugins.Source(geminiImagePluginKey)
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: geminiImagePluginKey})
	require.NoError(t, err)
	return plugin, registry
}
func geminiImageDecode(t *testing.T, plugin *jsplugin.LoadedPlugin, model string, body map[string]any, operation string) map[string]any {
	t.Helper()
	value, err := plugin.Engine.CallPath(
		t.Context(),
		"protocols",
		[]string{"openai_image", "decodeRequest"},
		map[string]any{"body": body, "model": model, "operation": operation},
	)
	require.NoError(t, err)
	encoded, marshalErr := common.Marshal(value)
	require.NoError(t, marshalErr)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func geminiImageDecodeError(t *testing.T, plugin *jsplugin.LoadedPlugin, body map[string]any, operation string) error {
	t.Helper()
	_, err := plugin.Engine.CallPath(
		t.Context(),
		"protocols",
		[]string{"openai_image", "decodeRequest"},
		map[string]any{"body": body, "model": "nano-banana", "operation": operation},
	)
	require.Error(t, err)
	return err
}

func geminiImageJSONRequest(model string, fields map[string]any) map[string]any {
	value := map[string]any{"model": model}
	for key, item := range fields {
		value[key] = item
	}
	return map[string]any{"kind": "json", "value": value}
}

// deterministicGeminiImageResponse is the vendor answer shape the plugin must
// parse: one text part plus one inline image, both camelCase (proto3 JSON) and
// snake_case spellings are exercised below.
func deterministicGeminiImageResponse() map[string]any {
	return map[string]any{
		"candidates": []any{map[string]any{
			"finishReason": "STOP",
			"content": map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{"text": "revised prompt"},
					map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}},
				},
			},
		}},
		"usageMetadata": map[string]any{"promptTokenCount": 10, "candidatesTokenCount": 20, "totalTokenCount": 30},
	}
}

func TestGeminiImagePluginServesTheOpenAIImagesEndpoints(t *testing.T) {
	plugin, registry := loadGeminiImagePlugin(t)
	require.NotEmpty(t, plugin.Meta.Models)
	for _, model := range plugin.Meta.Models {
		for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
			binding, found := registry.Generation().LookupEndpoint("POST", path, model)
			require.True(t, found, model+" "+path)
			assert.Same(t, plugin, binding.Plugin)
			assert.Equal(t, "openai_image", binding.Protocol)
		}
	}
	keys := make([]string, 0, len(plugin.Meta.UsageSchema))
	for key, schema := range plugin.Meta.UsageSchema {
		keys = append(keys, key)
		assert.NotEmpty(t, schema.Description, key)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"image_count", "image_size"}, keys)
}

func TestGeminiImagePluginDecodesTextToImage(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	decoded := geminiImageDecode(t, plugin, "nano-banana", geminiImageJSONRequest("nano-banana", map[string]any{
		"prompt":     "a cat",
		"size":       "1792x1024",
		"image_size": "2k",
		"seed":       0,
	}), "generate")

	assert.Equal(t, "submit", decoded["kind"])
	assert.Equal(t, "nano-banana", decoded["model"])
	assert.Equal(t, "text_to_image", decoded["action"])
	requestBody, ok := decoded["requestBody"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "a cat", requestBody["prompt"])
	assert.EqualValues(t, 1, requestBody["n"])
	assert.Equal(t, "16:9", requestBody["aspect_ratio"])
	assert.Equal(t, "2K", requestBody["image_size"])
	assert.EqualValues(t, 0, requestBody["seed"])
	assert.Equal(t, "official", requestBody["image_config_mode"])
	assert.NotContains(t, requestBody, "images")
}

func TestGeminiImagePluginDecodesReferenceImages(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	decoded := geminiImageDecode(t, plugin, "gemini-3-pro-image", geminiImageJSONRequest("gemini-3-pro-image", map[string]any{
		"prompt": "make it night",
		"image":  "https://cdn.example/reference.png",
		"images": []any{"data:image/png;base64,aW1hZ2U="},
	}), "edit")

	assert.Equal(t, "image_to_image", decoded["action"])
	requestBody, ok := decoded["requestBody"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"https://cdn.example/reference.png", "data:image/png;base64,aW1hZ2U="}, requestBody["images"])
	// An explicit size keeps the model default aspect ratio instead of guessing.
	assert.NotContains(t, requestBody, "aspect_ratio")
}

func TestGeminiImagePluginDecodesMultipartEdits(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	decoded := geminiImageDecode(t, plugin, "nano-banana-pro", map[string]any{
		"kind":   "multipart",
		"fields": map[string][]string{"model": {"nano-banana-pro"}, "prompt": {"restyle"}, "n": {"1"}},
		"files": []any{map[string]any{
			"ref": "request_file:image", "field": "image", "filename": "reference.png", "mimeType": "image/png", "size": 12,
		}},
	}, "edit")

	requestBody, ok := decoded["requestBody"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "image_to_image", decoded["action"])
	assert.Equal(t, []any{map[string]any{"__fileRef": "request_file:image", "encoding": "base64"}}, requestBody["images"])
}

func TestGeminiImagePluginRejectsUnsupportedRequests(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	cases := []struct {
		name  string
		body  map[string]any
		error string
	}{
		{"missing prompt", geminiImageJSONRequest("nano-banana", map[string]any{}), "field prompt is required"},
		{"multiple images", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "n": 2}), "exactly one image per request"},
		{"fractional count", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "n": 1.5}), "n must be a positive integer"},
		{"bad response format", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "response_format": "webp"}), "response_format must be url or b64_json"},
		{"bad image size", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "image_size": "8K"}), "image_size must be one of 512, 1K, 2K, or 4K"},
		{"bad size", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "size": "wide"}), "size must be WIDTHxHEIGHT or an aspect ratio"},
		{"bad aspect ratio", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "aspect_ratio": "wide"}), "aspect_ratio must look like 16:9"},
		{"bad seed", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "seed": -1}), "seed must be an integer between"},
		{"bad reference", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "image": "not an image!"}), "image must be a Base64 value, a data URL, or an HTTP URL"},
		{"unknown config mode", geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "image_config_mode": "other"}), "image_config_mode must be official or response_format"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.ErrorContains(t, geminiImageDecodeError(t, plugin, testCase.body, "generate"), testCase.error)
		})
	}

	t.Run("too many references", func(t *testing.T) {
		images := make([]any, 0, 17)
		for index := range 17 {
			images = append(images, "https://cdn.example/"+string(rune('a'+index))+".png")
		}
		body := geminiImageJSONRequest("nano-banana", map[string]any{"prompt": "x", "images": images})
		assert.ErrorContains(t, geminiImageDecodeError(t, plugin, body, "edit"), "at most 16 reference images")
	})
}

func TestGeminiImagePluginBuildsTheVendorRequest(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	requestBody := map[string]any{
		"model":             "nano-banana",
		"prompt":            "a cat",
		"n":                 1,
		"aspect_ratio":      "16:9",
		"image_size":        "2K",
		"image_config_mode": "official",
		"images":            []any{"data:image/png;base64,aW1hZ2U="},
	}
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"requestBody":   requestBody,
		"baseUrl":       "https://generativelanguage.googleapis.com",
		"upstreamModel": "nano-banana",
		"apiKey":        "gemini-key",
		"files":         []any{},
	})
	require.NoError(t, err)
	descriptor := value.(map[string]any)
	assert.Equal(t, "POST", descriptor["method"])
	assert.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-image:generateContent", descriptor["url"])
	headers := descriptor["headers"].(map[string]any)
	assert.Equal(t, "gemini-key", headers["x-goog-api-key"])

	body := descriptor["body"].(map[string]any)
	contents := body["contents"].([]any)
	require.Len(t, contents, 1)
	parts := contents[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2)
	assert.Equal(t, "a cat", parts[0].(map[string]any)["text"])
	assert.Equal(t, map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "aW1hZ2U="}}, parts[1])
	generationConfig := body["generationConfig"].(map[string]any)
	assert.Equal(t, []any{"TEXT", "IMAGE"}, generationConfig["responseModalities"])
	assert.Equal(t, map[string]any{"aspectRatio": "16:9", "imageSize": "2K"}, generationConfig["imageConfig"])
	assert.NotContains(t, generationConfig, "responseFormat")
}

func TestGeminiImagePluginBuildsTheCompatConfigAndVersionedRoot(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"requestBody": map[string]any{
			"model": "gemini-3.1-flash-image", "prompt": "x", "n": 1,
			"image_config_mode": "response_format", "aspect_ratio": "1:1", "image_size": "4K",
		},
		"baseUrl":       "https://proxy.example/gemini/v1beta/",
		"upstreamModel": "gemini-3.1-flash-image",
		"apiKey":        "key",
	})
	require.NoError(t, err)
	descriptor := value.(map[string]any)
	assert.Equal(t, "https://proxy.example/gemini/v1beta/models/gemini-3.1-flash-image:generateContent", descriptor["url"])
	generationConfig := descriptor["body"].(map[string]any)["generationConfig"].(map[string]any)
	assert.Equal(t, map[string]any{"image": map[string]any{"aspectRatio": "1:1", "imageSize": "4K"}}, generationConfig["responseFormat"])
	assert.NotContains(t, generationConfig, "imageConfig")
}

func TestGeminiImagePluginForwardsUploadedReferenceFiles(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"requestBody": map[string]any{
			"model": "nano-banana", "prompt": "x", "n": 1, "image_config_mode": "official",
			"images": []any{map[string]any{"__fileRef": "request_file:image", "encoding": "base64"}},
		},
		"baseUrl":       "https://generativelanguage.googleapis.com",
		"upstreamModel": "nano-banana",
		"apiKey":        "key",
		"files": []any{map[string]any{
			"ref": "request_file:image", "field": "image", "filename": "a.png", "mimeType": "image/webp", "size": 3,
		}},
	})
	require.NoError(t, err)
	parts := value.(map[string]any)["body"].(map[string]any)["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 2)
	inline := parts[1].(map[string]any)["inlineData"].(map[string]any)
	assert.Equal(t, "image/webp", inline["mimeType"])
	assert.Equal(t, map[string]any{"__fileRef": "request_file:image", "encoding": "base64"}, inline["data"])
}

func TestGeminiImagePluginParsesTheSynchronousResult(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	value, err := plugin.Engine.Call(
		t.Context(),
		"parseSubmitResponse",
		map[string]any{"publicTaskId": "task_public", "model": "nano-banana"},
		map[string]any{"statusCode": 200, "headers": map[string]any{}, "body": deterministicGeminiImageResponse()},
	)
	require.NoError(t, err)
	parsed := value.(map[string]any)
	assert.Equal(t, "task_public", parsed["taskId"])
	immediate := parsed["immediate"].(map[string]any)
	assert.Equal(t, "SUCCESS", immediate["status"])
	assert.Equal(t, "100%", immediate["progress"])
	assert.NotNil(t, parsed["taskData"])

	t.Run("snake_case vendor answers", func(t *testing.T) {
		body := map[string]any{
			"candidates": []any{map[string]any{"content": map[string]any{
				"parts": []any{map[string]any{"inline_data": map[string]any{"mime_type": "image/jpeg", "data": "aW1hZ2U="}}},
			}}},
		}
		snake, snakeErr := plugin.Engine.Call(
			t.Context(),
			"parseSubmitResponse",
			map[string]any{"publicTaskId": "task_public"},
			map[string]any{"statusCode": 200, "body": body},
		)
		require.NoError(t, snakeErr)
		assert.Equal(t, "SUCCESS", snake.(map[string]any)["immediate"].(map[string]any)["status"])
	})

	t.Run("blocked answers fail with the vendor reason", func(t *testing.T) {
		body := map[string]any{"promptFeedback": map[string]any{"blockReason": "SAFETY"}}
		blocked, blockedErr := plugin.Engine.Call(
			t.Context(),
			"parseSubmitResponse",
			map[string]any{"publicTaskId": "task_public"},
			map[string]any{"statusCode": 200, "body": body},
		)
		require.NoError(t, blockedErr)
		immediate := blocked.(map[string]any)["immediate"].(map[string]any)
		assert.Equal(t, "FAILURE", immediate["status"])
		assert.Equal(t, "no images generated: SAFETY", immediate["reason"])
	})

	t.Run("text-only answers fail", func(t *testing.T) {
		body := map[string]any{"candidates": []any{map[string]any{
			"finishReason": "STOP",
			"content":      map[string]any{"parts": []any{map[string]any{"text": "I cannot do that"}}},
		}}}
		empty, emptyErr := plugin.Engine.Call(
			t.Context(),
			"parseSubmitResponse",
			map[string]any{"publicTaskId": "task_public"},
			map[string]any{"statusCode": 200, "body": body},
		)
		require.NoError(t, emptyErr)
		immediate := empty.(map[string]any)["immediate"].(map[string]any)
		assert.Equal(t, "FAILURE", immediate["status"])
		assert.Equal(t, "no images generated", immediate["reason"])
	})
}

func TestGeminiImagePluginRendersOpenAIImages(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	value, err := plugin.Engine.CallPath(
		t.Context(),
		"protocols",
		[]string{"openai_image", "render"},
		map[string]any{"model": "nano-banana"},
		map[string]any{"task_id": "task_public", "status": "SUCCESS", "created_at": 1700000000, "data": deterministicGeminiImageResponse()},
	)
	require.NoError(t, err)
	rendered := value.(map[string]any)
	assert.EqualValues(t, 1700000000, rendered["created"])
	data := rendered["data"].([]any)
	require.Len(t, data, 1)
	entry := data[0].(map[string]any)
	assert.Equal(t, "aW1hZ2U=", entry["b64_json"])
	assert.Equal(t, "revised prompt", entry["revised_prompt"])
	assert.NotContains(t, entry, "url")

	t.Run("vendor urls stay urls", func(t *testing.T) {
		body := map[string]any{"candidates": []any{map[string]any{"content": map[string]any{
			"parts": []any{map[string]any{"fileData": map[string]any{"fileUri": "https://cdn.example/a.png"}}},
		}}}}
		remote, remoteErr := plugin.Engine.CallPath(
			t.Context(),
			"protocols",
			[]string{"openai_image", "render"},
			map[string]any{"model": "nano-banana"},
			map[string]any{"task_id": "task_public", "status": "SUCCESS", "data": body},
		)
		require.NoError(t, remoteErr)
		entry := remote.(map[string]any)["data"].([]any)[0].(map[string]any)
		assert.Equal(t, "https://cdn.example/a.png", entry["url"])
		assert.NotContains(t, remote.(map[string]any), "created")
	})
}

func TestGeminiImagePluginReportsBillingFacts(t *testing.T) {
	plugin, _ := loadGeminiImagePlugin(t)
	requestBody := map[string]any{"model": "nano-banana", "prompt": "x", "n": 1, "image_size": "2K"}

	reserved, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{"model": "nano-banana", "requestBody": requestBody})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": int64(1), "image_size": "2K"}, reserved)

	ratios, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
		"model": "nano-banana", "requestBody": requestBody, "usagePurpose": "billing_ratios",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": int64(1)}, ratios)

	onSubmit, err := plugin.Engine.Call(t.Context(), "extractUsageOnSubmit", map[string]any{"model": "nano-banana"}, deterministicGeminiImageResponse())
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": int64(1)}, onSubmit)

	onComplete, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": "nano-banana"}, map[string]any{}, deterministicGeminiImageResponse())
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": int64(1)}, onComplete)

	// An image-less completion keeps the reservation instead of lowering it.
	kept, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"model": "nano-banana"}, map[string]any{}, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, kept)
}
