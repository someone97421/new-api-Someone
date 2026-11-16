package plugins_test

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMagnificPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("magnific")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "magnific"})
	require.NoError(t, err)
	return plugin
}

func magnificObject(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func magnificDecode(t *testing.T, plugin *jsplugin.LoadedPlugin, name, operation string, body map[string]any) map[string]any {
	t.Helper()
	protocol := "openai_image"
	if operation == "video" {
		protocol = "openai_video"
	}
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{
		"model": name, "operation": operation, "body": body,
	})
	require.NoError(t, err)
	return magnificObject(t, value)
}

func TestMagnificGenerationContracts(t *testing.T) {
	plugin := newMagnificPlugin(t)
	for _, tc := range []struct {
		model, operation, path string
		fields                 map[string]any
	}{
		{"mystic", "generate", "mystic", map[string]any{"vendor_model": "fluid"}},
		{"magnific-upscaler", "edit", "image-upscaler", map[string]any{"image": "aGVsbG8=", "scale_factor": "4x", "creativity": 0}},
		{"magnific-upscaler-precision", "edit", "image-upscaler-precision", map[string]any{"image": "aGVsbG8="}},
		{"magnific-upscaler-precision-v2", "edit", "image-upscaler-precision-v2", map[string]any{"image": "https://cdn.example/input.png", "scale_factor": 3}},
		{"magnific-relight", "edit", "image-relight", map[string]any{"image": "https://cdn.example/input.png", "change_background": false}},
		{"magnific-style-transfer", "edit", "image-style-transfer", map[string]any{"image": "aGVsbG8=", "reference_image": "aGVsbG8="}},
		{"magnific-expand-flux-pro", "edit", "image-expand/flux-pro", map[string]any{"image": "aGVsbG8=", "left": 0}},
		{"magnific-expand-seedream-4.5", "edit", "image-expand/seedream-v4-5", map[string]any{"image": "aGVsbG8=", "left": 0, "right": 100, "top": 0, "bottom": 100}},
		{"flux-dev", "generate", "text-to-image/flux-dev", nil},
		{"flux-pro-v1-1", "generate", "text-to-image/flux-pro-v1-1", nil},
		{"flux-kontext-pro", "edit", "text-to-image/flux-kontext-pro", map[string]any{"image": "https://cdn.example/input.png"}},
		{"flux-2-pro", "edit", "text-to-image/flux-2-pro", map[string]any{"image": []any{"aGVsbG8=", "aGVsbG8="}, "size": "1024x768"}},
		{"flux-2-turbo", "generate", "text-to-image/flux-2-turbo", map[string]any{"size": "1024x1024"}},
		{"flux-2-klein", "edit", "text-to-image/flux-2-klein", map[string]any{"image": "aGVsbG8="}},
		{"seedream-4.0", "generate", "text-to-image/seedream-v4", nil},
		{"seedream-4.0", "edit", "text-to-image/seedream-v4-edit", map[string]any{"image": "aGVsbG8="}},
		{"seedream-4.5", "generate", "text-to-image/seedream-v4-5", nil},
		{"seedream-4.5", "edit", "text-to-image/seedream-v4-5-edit", map[string]any{"image": "aGVsbG8="}},
		{"nano-banana-pro", "generate", "text-to-image/nano-banana-pro", nil},
		{"gpt-image-2", "generate", "text-to-image/gpt-image-2", map[string]any{"n": 3}},
		{"gpt-image-2", "edit", "text-to-image/gpt-image-2-edit", map[string]any{"image": []any{"upl_img_example", "https://cdn.example/input.png"}}},
		{"gpt-image-2.5", "generate", "text-to-image/gpt-image-2-5", map[string]any{"variant": "sunburst", "quality": "max"}},
		{"gpt-image-2.5", "edit", "text-to-image/gpt-image-2-5-edit", map[string]any{"image": "aGVsbG8="}},
		{"kling-v2-1-pro", "video", "image-to-video/kling-v2-1-pro", map[string]any{"input_reference": "https://cdn.example/input.png", "seconds": "10"}},
		{"kling-v2-5-pro", "video", "image-to-video/kling-v2-5-pro", map[string]any{"seconds": 5}},
		{"kling-v2-6-pro", "video", "image-to-video/kling-v2-6-pro", map[string]any{"seconds": 10, "generate_audio": false}},
		{"minimax-hailuo-02-1080p", "video", "image-to-video/minimax-hailuo-02-1080p", map[string]any{"input_reference": "aGVsbG8="}},
		{"minimax-hailuo-2.3-1080p", "video", "image-to-video/minimax-hailuo-2-3-1080p", nil},
		{"seedance-pro-1080p", "video", "image-to-video/seedance-pro-1080p", map[string]any{"seconds": "5"}},
		{"wan-2.6-1080p", "video", "text-to-video/wan-v2-6-1080p", map[string]any{"size": "1920x1080"}},
		{"wan-2.6-1080p", "video", "image-to-video/wan-v2-6-1080p", map[string]any{"input_reference": "https://cdn.example/input.png"}},
		{"ltx-2-pro", "video", "text-to-video/ltx-2-pro", map[string]any{"fps": 50, "generate_audio": false}},
		{"ltx-2-pro", "video", "image-to-video/ltx-2-pro", map[string]any{"input_reference": "https://cdn.example/input.png"}},
		{"runway-gen4-turbo", "video", "image-to-video/runway-gen4-turbo", map[string]any{"input_reference": "aGVsbG8=", "size": "1280x720"}},
		{"veo-3.1", "video", "text-to-video/veo-3-1", map[string]any{"seconds": "8", "generate_audio": false}},
		{"veo-3.1", "video", "image-to-video/veo-3-1", map[string]any{"input_reference": "https://cdn.example/input.png"}},
		{"veo-3.1-fast", "video", "text-to-video/veo-3-1-fast", nil},
		{"veo-3.1-fast", "video", "image-to-video/veo-3-1-fast", map[string]any{"input_reference": "https://cdn.example/input.png"}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body := map[string]any{"model": tc.model, "prompt": "a cat", "seed": 0, "future": map[string]any{"flags": []any{false, 0}}}
			for k, v := range tc.fields {
				body[k] = v
			}
			original, err := common.Marshal(body)
			require.NoError(t, err)
			intent := magnificDecode(t, plugin, tc.model, tc.operation, map[string]any{"kind": "json", "value": body})
			ctx := map[string]any{"model": tc.model, "requestBody": intent["requestBody"], "baseUrl": "https://api.magnific.com/", "apiKey": "test-key", "publicTaskId": "public-task"}
			value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			descriptor := magnificObject(t, value)
			assert.Equal(t, "https://api.magnific.com/v1/ai/"+tc.path, descriptor["url"])
			assert.Equal(t, "test-key", descriptor["headers"].(map[string]any)["x-magnific-api-key"])
			payload := descriptor["body"].(map[string]any)
			assert.Equal(t, float64(0), payload["seed"])
			assert.Equal(t, map[string]any{"flags": []any{false, float64(0)}}, payload["future"])
			assert.NotContains(t, payload, "seconds")
			value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			facts := magnificObject(t, value)
			if tc.operation == "video" {
				assert.Positive(t, facts["seconds"])
				assert.Equal(t, float64(1), facts["video_count"])
			} else {
				want := float64(1)
				if n, ok := tc.fields["n"].(int); ok {
					want = float64(n)
				}
				assert.Equal(t, want, facts["image_count"])
			}
			value, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"statusCode": 202, "body": map[string]any{"data": map[string]any{"task_id": "private-id", "status": "CREATED", "generated": []any{}}}})
			require.NoError(t, err)
			parsed := magnificObject(t, value)
			assert.Equal(t, "private-id", parsed["taskId"])
			assert.NotContains(t, parsed, "immediate")
			assert.Equal(t, tc.path, parsed["state"].(map[string]any)["endpoint"])
			after, err := common.Marshal(body)
			require.NoError(t, err)
			assert.JSONEq(t, string(original), string(after), "故障转移使用的原始请求不能被改写")
		})
	}
}

func TestMagnificNativeFilesAndChaining(t *testing.T) {
	plugin := newMagnificPlugin(t)
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"submit_mystic"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{"model": "fluid", "fixed_generation": false, "structure_strength": 0}}})
	require.NoError(t, err)
	intent := magnificObject(t, value)
	assert.Equal(t, "mystic", intent["model"])
	ctx := map[string]any{"model": "mystic", "requestBody": intent["requestBody"], "baseUrl": "https://gateway.example", "apiKey": "gateway-key", "upstream": map[string]any{"kind": "new_api"}}
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := magnificObject(t, value)
	assert.Equal(t, "https://gateway.example/magnific/v1/ai/mystic", descriptor["url"])
	assert.Equal(t, "Bearer gateway-key", descriptor["headers"].(map[string]any)["Authorization"])
	assert.NotContains(t, descriptor["headers"], "x-magnific-api-key")
	assert.Equal(t, "fluid", descriptor["body"].(map[string]any)["model"])

	intent = magnificDecode(t, plugin, "client-alias", "edit", map[string]any{
		"kind": "multipart", "fields": map[string]any{"prompt": []string{"a cat"}, "n": []string{"2"}, "watermark": []string{"false"}},
		"files": []any{map[string]any{"ref": "request_file:image[]", "field": "image[]", "mimeType": "image/png", "size": 5}, map[string]any{"ref": "request_file:image[]#1", "field": "image[]", "mimeType": "image/jpeg", "size": 6}},
	})
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "client-alias", "upstreamModel": "gpt-image-2", "requestBody": intent["requestBody"], "baseUrl": "https://api.magnific.com", "apiKey": "test-key"})
	require.NoError(t, err)
	descriptor = magnificObject(t, value)
	assert.Equal(t, "https://api.magnific.com/v1/ai/text-to-image/gpt-image-2-edit", descriptor["url"])
	payload := descriptor["body"].(map[string]any)
	assert.Equal(t, false, payload["watermark"])
	assert.Equal(t, float64(2), payload["num_images"])
	refs := payload["reference_images"].([]any)
	require.Len(t, refs, 2)
	assert.Equal(t, "request_file:image[]#1", refs[1].(map[string]any)["__fileRef"])
	assert.Equal(t, "base64", refs[0].(map[string]any)["encoding"])
}

func TestMagnificTaskResultsAndArtifacts(t *testing.T) {
	plugin := newMagnificPlugin(t)
	adaptor := taskplugin.New(plugin)
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.magnific.com", ApiKey: "must-not-leak"}})
	task := &model.Task{TaskID: "public-id", Properties: model.Properties{OriginModelName: "alias", UpstreamModelName: "kling-v2-6-pro"}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "private-id", PluginState: []byte(`{"endpoint":"image-to-video/kling-v2-6-pro"}`)}}
	value, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": "https://api.magnific.com", "apiKey": "test-key", "taskId": "private/id", "state": map[string]any{"endpoint": "image-to-video/kling-v2-6-pro"}})
	require.NoError(t, err)
	assert.Equal(t, "https://api.magnific.com/v1/ai/image-to-video/kling-v2-6/private%2Fid", magnificObject(t, value)["url"])
	for _, tc := range []struct {
		status, normalized string
		generated          []any
	}{
		{"CREATED", "QUEUED", []any{}}, {"IN_PROGRESS", "IN_PROGRESS", []any{}},
		{"FAILED", "FAILURE", []any{}}, {"NEW_STATE", "UNKNOWN", []any{}},
		{"COMPLETED", "FAILURE", []any{map[string]any{"url": "not-a-documented-generated-entry"}}},
		{"COMPLETED", "SUCCESS", []any{"https://cdn.example/video.mp4"}},
	} {
		t.Run(tc.status+tc.normalized, func(t *testing.T) {
			data, err := common.Marshal(map[string]any{"data": map[string]any{"task_id": "private-id", "status": tc.status, "generated": tc.generated}})
			require.NoError(t, err)
			result, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: 200}, data)
			require.NoError(t, err)
			assert.Equal(t, tc.normalized, result.Status)
			if tc.normalized == "SUCCESS" {
				assert.Equal(t, float64(1), result.UsageFacts["video_count"])
				assert.NotContains(t, result.UsageFacts, "seconds", "没有实际时长时保留冻结的请求时长")
				task.Data = data
				artifacts, err := adaptor.ListArtifacts(task)
				require.NoError(t, err)
				require.Len(t, artifacts, 1)
				content, err := adaptor.BuildContentRequest(t.Context(), task, artifacts[0].Key, channel.TaskArtifactClientRequest{Method: http.MethodHead})
				require.NoError(t, err)
				assert.Equal(t, "https://cdn.example/video.mp4", content.URL)
				assert.True(t, content.Credentialless)
				assert.Empty(t, content.Headers)
			}
		})
	}
	value, err = plugin.Engine.CallPath(t.Context(), "native", []string{"task"}, map[string]any{}, map[string]any{"task_id": "public-id", "status": "FAILURE", "fail_reason": "polling exhausted", "data": map[string]any{"data": map[string]any{"task_id": "private-id", "status": "IN_PROGRESS"}}})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"task_id": "public-id", "status": "FAILED", "error_message": "polling exhausted"}, magnificObject(t, value)["data"])
}

func TestMagnificSynchronousRemoveBackground(t *testing.T) {
	plugin := newMagnificPlugin(t)
	intent := magnificDecode(t, plugin, "magnific-remove-background", "edit", map[string]any{"kind": "json", "value": map[string]any{"image": "https://cdn.example/image.png?a=1&b=2"}})
	ctx := map[string]any{"model": "magnific-remove-background", "requestBody": intent["requestBody"], "baseUrl": "https://api.magnific.com", "apiKey": "test-key", "publicTaskId": "public-id"}
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := magnificObject(t, value)
	assert.Equal(t, "application/x-www-form-urlencoded", descriptor["headers"].(map[string]any)["Content-Type"])
	assert.Equal(t, "image_url=https%3A%2F%2Fcdn.example%2Fimage.png%3Fa%3D1%26b%3D2", descriptor["body"])
	body := map[string]any{"original": "https://cdn.example/source.png", "high_resolution": "https://cdn.example/high.png", "preview": "https://cdn.example/small.png", "url": "https://cdn.example/high.png"}
	value, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"body": body})
	require.NoError(t, err)
	parsed := magnificObject(t, value)
	assert.Equal(t, "SUCCESS", parsed["immediate"].(map[string]any)["status"])
	value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"state": parsed["state"]}, parsed["immediate"], body)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": float64(1)}, magnificObject(t, value), "原图/预览/高清不能累计成三张输出")
}

func TestMagnificRejectsInvalidBillableInputs(t *testing.T) {
	plugin := newMagnificPlugin(t)
	for _, tc := range []struct {
		name, model, operation string
		fields                 map[string]any
	}{
		{"negative count", "gpt-image-2", "generate", map[string]any{"n": -1}},
		{"fractional count", "gpt-image-2", "generate", map[string]any{"n": 1.5}},
		{"oversized batch", "gpt-image-2", "generate", map[string]any{"num_images": 11}},
		{"conflicting counts", "gpt-image-2", "generate", map[string]any{"num_images": 2, "n": 3}},
		{"nested batch", "gpt-image-2", "generate", map[string]any{"extra": map[string]any{"num_images": 20}}},
		{"single image model", "mystic", "generate", map[string]any{"n": 2}},
		{"invalid quality", "gpt-image-2", "generate", map[string]any{"quality": "max"}},
		{"negative duration", "veo-3.1", "video", map[string]any{"seconds": -1}},
		{"huge duration", "veo-3.1", "video", map[string]any{"metadata": map[string]any{"duration": 1e30}}},
		{"unsupported duration", "veo-3.1", "video", map[string]any{"seconds": 7}},
		{"conflicting duration", "veo-3.1", "video", map[string]any{"seconds": 8, "duration": 6}},
		{"string boolean", "veo-3.1", "video", map[string]any{"generate_audio": "false"}},
		{"pixel bound", "flux-2-turbo", "generate", map[string]any{"image_size": map[string]any{"width": 1e20}}},
		{"base64-only URL", "magnific-upscaler", "edit", map[string]any{"image": "https://cdn.example/image.png"}},
		{"URL-only base64", "flux-kontext-pro", "edit", map[string]any{"image": "aGVsbG8="}},
		{"missing edit reference", "gpt-image-2", "edit", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"prompt": "a cat"}
			for k, v := range tc.fields {
				body[k] = v
			}
			protocol := "openai_image"
			if tc.operation == "video" {
				protocol = "openai_video"
			}
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{"model": tc.model, "operation": tc.operation, "body": map[string]any{"kind": "json", "value": body}})
			require.Error(t, err)
		})
	}
}

func TestMagnificProtocolSpecificParameters(t *testing.T) {
	plugin := newMagnificPlugin(t)
	for _, tc := range []struct {
		model, operation string
		input, expected  map[string]any
	}{
		{"veo-3.1", "video", map[string]any{"size": "1920x1080"}, map[string]any{"resolution": "1080p", "aspect_ratio": "16:9"}},
		{"veo-3.1-fast", "video", map[string]any{"size": "2160x3840"}, map[string]any{"resolution": "4k", "aspect_ratio": "9:16"}},
		{"kling-v2-6-pro", "video", map[string]any{"size": "1280x720"}, map[string]any{"aspect_ratio": "widescreen_16_9"}},
		{"nano-banana-pro", "edit", map[string]any{"image": []any{"https://cdn.example/a.jpg", "https://cdn.example/b.webp"}}, map[string]any{"reference_images": []any{map[string]any{"image": "https://cdn.example/a.jpg", "mime_type": "image/jpeg"}, map[string]any{"image": "https://cdn.example/b.webp", "mime_type": "image/webp"}}}},
		{"minimax-hailuo-2.3-1080p", "video", map[string]any{"input_reference": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"}}, map[string]any{"first_frame_image": "https://cdn.example/a.png", "last_frame_image": "https://cdn.example/b.png"}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			tc.input["prompt"] = "a cat"
			intent := magnificDecode(t, plugin, tc.model, tc.operation, map[string]any{"kind": "json", "value": tc.input})
			ctx := map[string]any{"model": tc.model, "requestBody": intent["requestBody"], "baseUrl": "https://api.magnific.com", "apiKey": "test"}
			value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			payload := magnificObject(t, value)["body"].(map[string]any)
			for key, expected := range tc.expected {
				assert.Equal(t, expected, payload[key], key)
			}
			value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			if resolution, ok := tc.expected["resolution"]; ok {
				assert.Equal(t, resolution, magnificObject(t, value)["resolution"])
			}
		})
	}
	for _, prompt := range []string{"[Pan left] A cat", "true", `{"scene":"cat"}`} {
		intent := magnificDecode(t, plugin, "gpt-image-2", "edit", map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []string{prompt}}, "files": []any{map[string]any{"ref": "request_file:image", "field": "image", "mimeType": "image/png", "size": 5}}})
		assert.Equal(t, prompt, intent["requestBody"].(map[string]any)["payload"].(map[string]any)["prompt"])
	}
	for _, tc := range []struct {
		hook    string
		payload map[string]any
	}{
		{"submit_image_upscaler_precision_v2", map[string]any{"image": "https://cdn.example/input.png"}},
		{"submit_text_to_image_seedream_v4_edit", map[string]any{"prompt": "a cat"}},
	} {
		value, err := plugin.Engine.CallPath(t.Context(), "native", []string{tc.hook}, map[string]any{"body": map[string]any{"kind": "json", "value": tc.payload}})
		require.NoError(t, err)
		intent := magnificObject(t, value)
		value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": intent["model"], "requestBody": intent["requestBody"], "baseUrl": "https://api.magnific.com", "apiKey": "test"})
		require.NoError(t, err)
		assert.NotContains(t, magnificObject(t, value)["body"], "scale_factor")
	}
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "veo-3.1", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "a cat", "size": "1920x1080", "resolution": "720p"}}})
	require.ErrorContains(t, err, "size and resolution conflict")
	intent := magnificDecode(t, plugin, "gpt-image-2", "generate", map[string]any{"kind": "json", "value": map[string]any{"prompt": "a cat", "n": 3}})
	ctx := map[string]any{"model": "gpt-image-2", "requestBody": intent["requestBody"], "usagePurpose": "billing_ratios"}
	value, err := plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": float64(3)}, magnificObject(t, value))
	value, err = plugin.Engine.Call(t.Context(), "extractUsageOnSubmit", ctx, map[string]any{"data": map[string]any{"generated": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"}}})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"image_count": float64(2)}, magnificObject(t, value))
}
