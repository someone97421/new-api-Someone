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

const (
	openaiTaskImageKey = "openai-task-image"
	openaiTaskVideoKey = "openai-task-video"
)

func loadOpenAITaskPlugin(t *testing.T, key string) (*jsplugin.LoadedPlugin, *jsplugin.Registry) {
	t.Helper()
	source, err := builtinplugins.Source(key)
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: key})
	require.NoError(t, err)
	return plugin, registry
}

func callPluginHook(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func callPluginProtocolHook(t *testing.T, plugin *jsplugin.LoadedPlugin, protocol string, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, hook}, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func callPluginProtocolHookError(t *testing.T, plugin *jsplugin.LoadedPlugin, protocol string, hook string, args ...any) error {
	t.Helper()
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, hook}, args...)
	require.Error(t, err)
	return err
}

// -----------------------------------------------------------------------------
// OpenAI Task Image 插件测试
// -----------------------------------------------------------------------------

func TestOpenAITaskImagePluginManifestAndEndpoints(t *testing.T) {
	plugin, registry := loadOpenAITaskPlugin(t, openaiTaskImageKey)
	require.NotEmpty(t, plugin.Meta.Models)
	assert.Equal(t, openaiTaskImageKey, plugin.Meta.Key)

	for _, model := range plugin.Meta.Models {
		for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
			binding, found := registry.Generation().LookupEndpoint("POST", path, model)
			require.True(t, found, model+" "+path)
			assert.Same(t, plugin, binding.Plugin)
			assert.Equal(t, "openai_image", binding.Protocol)
		}
	}

	keys := make([]string, 0, len(plugin.Meta.UsageSchema))
	for k := range plugin.Meta.UsageSchema {
		keys = append(keys, k)
	}
	assert.Equal(t, []string{"image_count"}, keys)
}

func TestOpenAITaskImagePluginDecodeRequest(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskImageKey)

	t.Run("valid JSON text-to-image", func(t *testing.T) {
		decoded := callPluginProtocolHook(t, plugin, "openai_image", "decodeRequest", map[string]any{
			"model": "gpt-image-1",
			"body": map[string]any{
				"kind": "json",
				"value": map[string]any{
					"model":       "gpt-image-1",
					"prompt":      "a flying dinosaur",
					"n":           2,
					"size":        "1024x1024",
					"custom_flag": "preserved",
					"async":       true,
					"task_count":  10,
				},
			},
		})
		assert.Equal(t, "submit", decoded["kind"])
		assert.Equal(t, "gpt-image-1", decoded["model"])
		assert.Equal(t, "text_to_image", decoded["action"])

		reqBody, ok := decoded["requestBody"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "a flying dinosaur", reqBody["prompt"])
		assert.EqualValues(t, 2, reqBody["n"])
		assert.Equal(t, "1024x1024", reqBody["size"])
		assert.Equal(t, "preserved", reqBody["custom_flag"])
		assert.NotContains(t, reqBody, "async")
		assert.NotContains(t, reqBody, "task_count")
	})

	t.Run("JSON image-to-image with images array", func(t *testing.T) {
		decoded := callPluginProtocolHook(t, plugin, "openai_image", "decodeRequest", map[string]any{
			"model": "gpt-image-1",
			"body": map[string]any{
				"kind": "json",
				"value": map[string]any{
					"model":  "gpt-image-1",
					"prompt": "vary this image",
					"image":  "https://example.com/ref.png",
				},
			},
		})
		assert.Equal(t, "image_to_image", decoded["action"])
		reqBody := decoded["requestBody"].(map[string]any)
		images, ok := reqBody["images"].([]any)
		require.True(t, ok)
		assert.Equal(t, []any{"https://example.com/ref.png"}, images)
	})

	t.Run("multipart decode with file reference", func(t *testing.T) {
		decoded := callPluginProtocolHook(t, plugin, "openai_image", "decodeRequest", map[string]any{
			"model": "gpt-image-1",
			"body": map[string]any{
				"kind": "multipart",
				"fields": map[string]any{
					"prompt": []any{"edit this picture"},
					"n":      []any{"3"},
					"size":   []any{"512x512"},
				},
				"files": []any{
					map[string]any{"ref": "request_file:image", "field": "image", "filename": "ref.png", "mimeType": "image/png"},
				},
			},
		})
		assert.Equal(t, "image_to_image", decoded["action"])
		reqBody := decoded["requestBody"].(map[string]any)
		assert.Equal(t, "edit this picture", reqBody["prompt"])
		assert.EqualValues(t, 3, reqBody["n"])
		assert.Equal(t, "512x512", reqBody["size"])
		images := reqBody["images"].([]any)
		require.Len(t, images, 1)
		fileItem := images[0].(map[string]any)
		assert.Equal(t, "request_file:image", fileItem["__fileRef"])
		assert.Equal(t, "dataUrl", fileItem["encoding"])
	})

	t.Run("validation: missing prompt", func(t *testing.T) {
		err := callPluginProtocolHookError(t, plugin, "openai_image", "decodeRequest", map[string]any{
			"model": "gpt-image-1",
			"body": map[string]any{
				"kind":  "json",
				"value": map[string]any{"model": "gpt-image-1", "prompt": ""},
			},
		})
		assert.Contains(t, err.Error(), "field prompt is required")
	})

	t.Run("validation: invalid n boundary", func(t *testing.T) {
		for _, invalidN := range []any{0, 9, -1, "abc"} {
			err := callPluginProtocolHookError(t, plugin, "openai_image", "decodeRequest", map[string]any{
				"model": "gpt-image-1",
				"body": map[string]any{
					"kind":  "json",
					"value": map[string]any{"model": "gpt-image-1", "prompt": "test", "n": invalidN},
				},
			})
			assert.Contains(t, err.Error(), "n must be an integer between 1 and 8")
		}
	})
}

func TestOpenAITaskImagePluginSubmitAndQuery(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskImageKey)

	t.Run("buildSubmitRequest handles trailing slash and Bearer token", func(t *testing.T) {
		req := callPluginHook(t, plugin, "buildSubmitRequest", map[string]any{
			"baseUrl":     "https://api.example.com///",
			"apiKey":      "sk-test-secret",
			"requestBody": map[string]any{"model": "gpt-image-1", "prompt": "a dog", "n": 1},
		})
		assert.Equal(t, "https://api.example.com/v1/images/generations", req["url"])
		assert.Equal(t, "POST", req["method"])
		headers := req["headers"].(map[string]any)
		assert.Equal(t, "Bearer sk-test-secret", headers["Authorization"])
		assert.Equal(t, "application/json", headers["Content-Type"])
	})

	t.Run("parseSubmitResponse: immediate completion on image payload", func(t *testing.T) {
		parsed := callPluginHook(t, plugin, "parseSubmitResponse",
			map[string]any{"publicTaskId": "pub_task_123"},
			map[string]any{
				"body": map[string]any{
					"data": []any{
						map[string]any{"url": "https://cdn.example.com/image1.png"},
					},
				},
			},
		)
		assert.Equal(t, "pub_task_123", parsed["taskId"])
		imm, ok := parsed["immediate"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "SUCCESS", imm["status"])
		assert.Equal(t, "100%", imm["progress"])
		assert.Equal(t, "https://cdn.example.com/image1.png", imm["url"])
	})

	t.Run("parseSubmitResponse: async task ID branch", func(t *testing.T) {
		for _, body := range []map[string]any{
			{"id": "task_id_1"},
			{"task_id": "task_id_2"},
			{"data": map[string]any{"task_id": "task_id_3"}},
			{"output": map[string]any{"task_id": "task_id_4"}},
		} {
			parsed := callPluginHook(t, plugin, "parseSubmitResponse",
				map[string]any{"publicTaskId": "pub_task_123"},
				map[string]any{"body": body},
			)
			assert.NotEmpty(t, parsed["taskId"])
			assert.NotContains(t, parsed, "immediate")
		}
	})

	t.Run("parseSubmitResponse: missing task id throws", func(t *testing.T) {
		_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse",
			map[string]any{"publicTaskId": "pub_task_123"},
			map[string]any{"body": map[string]any{"something_else": 1}},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing task id")
	})

	t.Run("buildQueryRequest formats path and headers", func(t *testing.T) {
		query := callPluginHook(t, plugin, "buildQueryRequest", map[string]any{
			"baseUrl": "https://api.example.com/",
			"apiKey":  "sk-test-secret",
			"taskId":  "task/special#123",
		})
		assert.Equal(t, "https://api.example.com/v1/tasks/task%2Fspecial%23123", query["url"])
		assert.Equal(t, "GET", query["method"])
		headers := query["headers"].(map[string]any)
		assert.Equal(t, "Bearer sk-test-secret", headers["Authorization"])
	})
}

func TestOpenAITaskImagePluginParseTaskResult(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskImageKey)

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus string
		wantURL    string
		wantReason string
	}{
		{"queued", map[string]any{"status": "queued"}, "QUEUED", "", ""},
		{"pending", map[string]any{"state": "pending"}, "QUEUED", "", ""},
		{"in_progress with progress", map[string]any{"status": "processing", "progress": "60%"}, "IN_PROGRESS", "", ""},
		{"succeeded with data url", map[string]any{"status": "succeeded", "data": []any{map[string]any{"url": "https://cdn.example.com/res.png"}}}, "SUCCESS", "https://cdn.example.com/res.png", ""},
		{"completed with results url", map[string]any{"task_status": "completed", "results": []any{map[string]any{"url": "https://cdn.example.com/res2.png"}}}, "SUCCESS", "https://cdn.example.com/res2.png", ""},
		{"failed with message", map[string]any{"status": "failed", "error": map[string]any{"message": "nsfw detected"}}, "FAILURE", "", "nsfw detected"},
		{"unknown status", map[string]any{"status": "weird_vendor_state"}, "UNKNOWN", "", "unrecognized status: weird_vendor_state"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callPluginHook(t, plugin, "parseTaskResult", map[string]any{}, tt.body)
			assert.Equal(t, tt.wantStatus, res["status"])
			if tt.wantURL != "" {
				assert.Equal(t, tt.wantURL, res["url"])
			}
			if tt.wantReason != "" {
				assert.Contains(t, res["reason"], tt.wantReason)
			}
		})
	}
}

func TestOpenAITaskImagePluginUsageAndArtifacts(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskImageKey)

	t.Run("extractUsage returns requested count", func(t *testing.T) {
		facts := callPluginHook(t, plugin, "extractUsage", map[string]any{
			"requestBody": map[string]any{"n": 4},
		})
		assert.EqualValues(t, 4, facts["image_count"])
	})

	t.Run("extractUsageOnSubmit and extractUsageOnComplete count payloads", func(t *testing.T) {
		body := map[string]any{
			"data": []any{
				map[string]any{"url": "https://cdn.example.com/1.png"},
				map[string]any{"b64_json": "AAAA"},
				map[string]any{"empty": true},
			},
		}
		submitFacts := callPluginHook(t, plugin, "extractUsageOnSubmit", map[string]any{}, body)
		assert.EqualValues(t, 1, submitFacts["image_count"]) // max(1 url, 1 b64)

		body2 := map[string]any{
			"data": []any{
				map[string]any{"url": "https://cdn.example.com/1.png"},
				map[string]any{"url": "https://cdn.example.com/2.png"},
			},
		}
		compFacts := callPluginHook(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{}, body2)
		assert.EqualValues(t, 2, compFacts["image_count"])

		emptyFacts := callPluginHook(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{}, map[string]any{})
		assert.Empty(t, emptyFacts)
	})

	t.Run("listArtifacts and buildContentRequest", func(t *testing.T) {
		taskSuccess := map[string]any{
			"status": "SUCCESS",
			"data": map[string]any{
				"data": []any{
					map[string]any{"url": "https://cdn.example.com/1.png"},
					map[string]any{"b64_json": "aW1hZ2VEYXRh"},
					map[string]any{"url": "/v1/files/download?id=3"},
				},
			},
		}
		val, err := plugin.Engine.Call(t.Context(), "listArtifacts", taskSuccess)
		require.NoError(t, err)
		artifacts := val.([]any)
		require.Len(t, artifacts, 3)
		assert.Equal(t, "image-1", artifacts[0].(map[string]any)["key"])
		assert.Equal(t, "image-2", artifacts[1].(map[string]any)["key"])
		assert.Equal(t, "image-3", artifacts[2].(map[string]any)["key"])

		// 1) 公网 CDN URL -> credentialless: true
		req1 := callPluginHook(t, plugin, "buildContentRequest", map[string]any{
			"baseUrl":       "https://api.vendor.com",
			"apiKey":        "secret-key",
			"artifactKey":   "image-1",
			"data":          taskSuccess["data"],
			"clientRequest": map[string]any{"method": "GET"},
		})
		assert.Equal(t, "https://cdn.example.com/1.png", req1["url"])
		assert.Equal(t, true, req1["credentialless"])
		assert.Nil(t, req1["headers"])

		// 2) Base64 payload -> data URL with credentialless: true
		req2 := callPluginHook(t, plugin, "buildContentRequest", map[string]any{
			"baseUrl":       "https://api.vendor.com",
			"apiKey":        "secret-key",
			"artifactKey":   "image-2",
			"data":          taskSuccess["data"],
			"clientRequest": map[string]any{"method": "GET"},
		})
		assert.Equal(t, "data:image/png;base64,aW1hZ2VEYXRh", req2["url"])
		assert.Equal(t, true, req2["credentialless"])

		// 3) 相对路径 -> 拼接 baseUrl 并带 Bearer 头
		req3 := callPluginHook(t, plugin, "buildContentRequest", map[string]any{
			"baseUrl":       "https://api.vendor.com/",
			"apiKey":        "secret-key",
			"artifactKey":   "image-3",
			"data":          taskSuccess["data"],
			"clientRequest": map[string]any{"method": "GET"},
		})
		assert.Equal(t, "https://api.vendor.com/v1/files/download?id=3", req3["url"])
		assert.Equal(t, false, req3["credentialless"])
		headers3 := req3["headers"].(map[string]any)
		assert.Equal(t, "Bearer secret-key", headers3["Authorization"])
	})
}

// -----------------------------------------------------------------------------
// OpenAI Task Video 插件测试
// -----------------------------------------------------------------------------

func TestOpenAITaskVideoPluginManifestAndEndpoints(t *testing.T) {
	plugin, registry := loadOpenAITaskPlugin(t, openaiTaskVideoKey)
	require.NotEmpty(t, plugin.Meta.Models)
	assert.Equal(t, openaiTaskVideoKey, plugin.Meta.Key)

	for _, model := range plugin.Meta.Models {
		binding, found := registry.Generation().LookupEndpoint("POST", "/v1/videos", model)
		require.True(t, found, model)
		assert.Same(t, plugin, binding.Plugin)
		assert.Equal(t, "openai_video", binding.Protocol)
	}

	keys := make([]string, 0, len(plugin.Meta.UsageSchema))
	for k := range plugin.Meta.UsageSchema {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"resolution", "seconds"}, keys)
}

func TestOpenAITaskVideoPluginDecodeRequest(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskVideoKey)

	t.Run("JSON text-to-video decode", func(t *testing.T) {
		decoded := callPluginProtocolHook(t, plugin, "openai_video", "decodeRequest", map[string]any{
			"model": "gpt-video-1",
			"body": map[string]any{
				"kind": "json",
				"value": map[string]any{
					"model":      "gpt-video-1",
					"prompt":     "ocean waves rolling",
					"seconds":    10,
					"size":       "1280x720",
					"extra_prop": "passed",
				},
			},
		})
		assert.Equal(t, "submit", decoded["kind"])
		assert.Equal(t, "text_to_video", decoded["action"])
		reqBody := decoded["requestBody"].(map[string]any)
		assert.Equal(t, "ocean waves rolling", reqBody["prompt"])
		assert.EqualValues(t, 10, reqBody["seconds"])
		assert.Equal(t, "1280x720", reqBody["size"])
		assert.Equal(t, "passed", reqBody["extra_prop"])
	})

	t.Run("JSON image-to-video decode with input_reference", func(t *testing.T) {
		decoded := callPluginProtocolHook(t, plugin, "openai_video", "decodeRequest", map[string]any{
			"model": "gpt-video-1",
			"body": map[string]any{
				"kind": "json",
				"value": map[string]any{
					"model":           "gpt-video-1",
					"input_reference": "https://example.com/frame.png",
				},
			},
		})
		assert.Equal(t, "image_to_video", decoded["action"])
	})

	t.Run("multipart decode with input_reference file", func(t *testing.T) {
		decoded := callPluginProtocolHook(t, plugin, "openai_video", "decodeRequest", map[string]any{
			"model": "gpt-video-1",
			"body": map[string]any{
				"kind": "multipart",
				"fields": map[string]any{
					"prompt":  []any{"animate this art"},
					"seconds": []any{"6"},
				},
				"files": []any{
					map[string]any{"ref": "request_file:input_reference", "field": "input_reference", "filename": "art.png", "mimeType": "image/png"},
				},
			},
		})
		assert.Equal(t, "image_to_video", decoded["action"])
		reqBody := decoded["requestBody"].(map[string]any)
		assert.Equal(t, "animate this art", reqBody["prompt"])
		assert.EqualValues(t, 6, reqBody["seconds"])
		fileObj := reqBody["input_reference"].(map[string]any)
		assert.Equal(t, "request_file:input_reference", fileObj["__fileRef"])
	})

	t.Run("validation: missing prompt and reference image", func(t *testing.T) {
		err := callPluginProtocolHookError(t, plugin, "openai_video", "decodeRequest", map[string]any{
			"model": "gpt-video-1",
			"body": map[string]any{
				"kind":  "json",
				"value": map[string]any{"model": "gpt-video-1"},
			},
		})
		assert.Contains(t, err.Error(), "field prompt is required when no reference image is provided")
	})

	t.Run("validation: seconds bounds", func(t *testing.T) {
		for _, invalidSec := range []any{0, -5, 3601, "abc"} {
			err := callPluginProtocolHookError(t, plugin, "openai_video", "decodeRequest", map[string]any{
				"model": "gpt-video-1",
				"body": map[string]any{
					"kind":  "json",
					"value": map[string]any{"model": "gpt-video-1", "prompt": "test", "seconds": invalidSec},
				},
			})
			assert.Contains(t, err.Error(), "seconds must be a positive integer between 1 and 3600")
		}
	})
}

func TestOpenAITaskVideoPluginSubmitAndQuery(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskVideoKey)

	t.Run("buildSubmitRequest JSON mode without files", func(t *testing.T) {
		req := callPluginHook(t, plugin, "buildSubmitRequest", map[string]any{
			"baseUrl":     "https://video.example.com",
			"apiKey":      "sk-video-secret",
			"requestBody": map[string]any{"model": "gpt-video-1", "prompt": "a rocket launching"},
		})
		assert.Equal(t, "https://video.example.com/v1/videos", req["url"])
		assert.Equal(t, "POST", req["method"])
		headers := req["headers"].(map[string]any)
		assert.Equal(t, "Bearer sk-video-secret", headers["Authorization"])
		assert.Equal(t, "application/json", headers["Content-Type"])
	})

	t.Run("buildSubmitRequest multipart mode with files", func(t *testing.T) {
		req := callPluginHook(t, plugin, "buildSubmitRequest", map[string]any{
			"baseUrl":     "https://video.example.com/",
			"apiKey":      "sk-video-secret",
			"requestBody": map[string]any{"model": "gpt-video-1", "prompt": "rocket"},
			"files": []any{
				map[string]any{"field": "input_reference", "ref": "file_ref_1", "filename": "rocket.png"},
			},
		})
		assert.Equal(t, "https://video.example.com/v1/videos", req["url"])
		assert.Equal(t, "multipart", req["bodyType"])
		parts := req["parts"].([]any)
		require.NotEmpty(t, parts)
	})

	t.Run("parseSubmitResponse immediate completion on direct video URL", func(t *testing.T) {
		parsed := callPluginHook(t, plugin, "parseSubmitResponse",
			map[string]any{"publicTaskId": "pub_vid_123"},
			map[string]any{"body": map[string]any{"url": "https://cdn.example.com/vid.mp4"}},
		)
		imm := parsed["immediate"].(map[string]any)
		assert.Equal(t, "SUCCESS", imm["status"])
		assert.Equal(t, "https://cdn.example.com/vid.mp4", imm["url"])
	})

	t.Run("parseSubmitResponse async task ID", func(t *testing.T) {
		parsed := callPluginHook(t, plugin, "parseSubmitResponse",
			map[string]any{"publicTaskId": "pub_vid_123"},
			map[string]any{"body": map[string]any{"task_id": "vid_async_456"}},
		)
		assert.Equal(t, "vid_async_456", parsed["taskId"])
		assert.NotContains(t, parsed, "immediate")
	})

	t.Run("buildQueryRequest URL and headers", func(t *testing.T) {
		query := callPluginHook(t, plugin, "buildQueryRequest", map[string]any{
			"baseUrl": "https://video.example.com",
			"apiKey":  "sk-video-secret",
			"taskId":  "vid_task_789",
		})
		assert.Equal(t, "https://video.example.com/v1/videos/vid_task_789", query["url"])
		assert.Equal(t, "GET", query["method"])
	})
}

func TestOpenAITaskVideoPluginParseTaskResult(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskVideoKey)

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus string
		wantURL    string
	}{
		{"queued", map[string]any{"status": "queued"}, "QUEUED", ""},
		{"in_progress", map[string]any{"task_status": "in_progress", "progress": 45}, "IN_PROGRESS", ""},
		{"completed", map[string]any{"status": "completed", "output": map[string]any{"video_url": "https://cdn.example.com/video.mp4"}}, "SUCCESS", "https://cdn.example.com/video.mp4"},
		{"failed", map[string]any{"status": "failed", "error": map[string]any{"message": "gpu out of memory"}}, "FAILURE", ""},
		{"unknown", map[string]any{"status": "strange_state"}, "UNKNOWN", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := callPluginHook(t, plugin, "parseTaskResult", map[string]any{}, tt.body)
			assert.Equal(t, tt.wantStatus, res["status"])
			if tt.wantURL != "" {
				assert.Equal(t, tt.wantURL, res["url"])
			}
		})
	}
}

func TestOpenAITaskVideoPluginUsageAndRender(t *testing.T) {
	plugin, _ := loadOpenAITaskPlugin(t, openaiTaskVideoKey)

	t.Run("extractUsage default 5s and 720p", func(t *testing.T) {
		facts := callPluginHook(t, plugin, "extractUsage", map[string]any{
			"requestBody": map[string]any{},
		})
		assert.EqualValues(t, 5, facts["seconds"])
		assert.Equal(t, "720p", facts["resolution"])
	})

	t.Run("extractUsage infer resolution from size", func(t *testing.T) {
		facts := callPluginHook(t, plugin, "extractUsage", map[string]any{
			"requestBody": map[string]any{"duration": 12, "size": "1920x1080"},
		})
		assert.EqualValues(t, 12, facts["seconds"])
		assert.Equal(t, "1080p", facts["resolution"])
	})

	t.Run("extractUsageOnComplete uses completed facts", func(t *testing.T) {
		body := map[string]any{
			"usage": map[string]any{
				"output_video_duration": 15,
			},
			"resolution": "1080p",
		}
		facts := callPluginHook(t, plugin, "extractUsageOnComplete", map[string]any{}, map[string]any{}, body)
		assert.EqualValues(t, 15, facts["seconds"])
		assert.Equal(t, "1080p", facts["resolution"])
	})

	t.Run("protocols.openai_video.render standard DTO", func(t *testing.T) {
		taskSuccess := map[string]any{
			"task_id":    "vid_task_123",
			"status":     "SUCCESS",
			"progress":   "100%",
			"created_at": 1700000000,
			"updated_at": 1700000050,
			"properties": map[string]any{"origin_model_name": "gpt-video-1"},
		}
		rendered := callPluginProtocolHook(t, plugin, "openai_video", "render", map[string]any{"model": "gpt-video-1"}, taskSuccess)
		assert.Equal(t, "vid_task_123", rendered["id"])
		assert.Equal(t, "video", rendered["object"])
		assert.Equal(t, "gpt-video-1", rendered["model"])
		assert.Equal(t, "completed", rendered["status"])
		assert.EqualValues(t, 100, rendered["progress"])
		assert.EqualValues(t, 1700000000, rendered["created_at"])
		assert.EqualValues(t, 1700000050, rendered["completed_at"])
	})

	t.Run("listArtifacts and buildContentRequest", func(t *testing.T) {
		taskSuccess := map[string]any{
			"status":  "SUCCESS",
			"task_id": "vid_task_123",
			"data": map[string]any{
				"url": "https://cdn.example.com/video.mp4",
			},
		}
		val, err := plugin.Engine.Call(t.Context(), "listArtifacts", taskSuccess)
		require.NoError(t, err)
		artifacts := val.([]any)
		require.Len(t, artifacts, 1)
		assert.Equal(t, "video", artifacts[0].(map[string]any)["key"])
		assert.Equal(t, "video/mp4", artifacts[0].(map[string]any)["mimeType"])

		// 绝对 URL -> 直接使用，credentialless: true
		req1 := callPluginHook(t, plugin, "buildContentRequest", map[string]any{
			"baseUrl":       "https://api.video.com",
			"apiKey":        "secret-key",
			"artifactKey":   "video",
			"data":          taskSuccess["data"],
			"clientRequest": map[string]any{"method": "GET"},
		})
		assert.Equal(t, "https://cdn.example.com/video.mp4", req1["url"])
		assert.Equal(t, true, req1["credentialless"])

		// 相对路径 -> 拼接 baseUrl 并带 Bearer 头
		taskRelative := map[string]any{
			"data": map[string]any{
				"url": "/v1/videos/vid_task_123/content",
			},
		}
		req2 := callPluginHook(t, plugin, "buildContentRequest", map[string]any{
			"baseUrl":       "https://api.video.com",
			"apiKey":        "secret-key",
			"artifactKey":   "video",
			"data":          taskRelative["data"],
			"clientRequest": map[string]any{"method": "GET"},
		})
		assert.Equal(t, "https://api.video.com/v1/videos/vid_task_123/content", req2["url"])
		assert.Equal(t, false, req2["credentialless"])
		headers2 := req2["headers"].(map[string]any)
		assert.Equal(t, "Bearer secret-key", headers2["Authorization"])
	})
}
