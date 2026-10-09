package plugins_test

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newJingyuPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("jingyu")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "jingyu"})
	require.NoError(t, err)
	return plugin
}

func jingyuObject(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(data, &result))
	return result
}

func TestJingyuImageProtocol(t *testing.T) {
	plugin := newJingyuPlugin(t)
	for _, name := range plugin.Meta.Protocols[0].Models {
		for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
			bindings := jsplugin.DefaultRegistry.Generation().LookupEndpointCandidates(http.MethodPost, path, name)
			assert.NotEmpty(t, bindings, name)
			assert.True(t, len(bindings) > 0 && bindings[0].Plugin.Meta.Key == "jingyu", name)
		}
	}
	body := map[string]any{
		"model": "client-alias", "prompt": "a cat", "size": "3840x2160", "resolution": "4K", "quality": "high", "n": 1,
		"images": []any{"https://cdn.example/first.png", "https://cdn.example/last.png"},
		"seed":   0, "watermark": false, "response_format": "b64_json",
		"extra": map[string]any{"future": []any{0, false, map[string]any{"enabled": false}}},
	}
	before, err := common.Marshal(body)
	require.NoError(t, err)
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{
		"model": "client-alias", "operation": "edit", "body": map[string]any{"kind": "json", "value": body},
	})
	require.NoError(t, err)
	intent := jingyuObject(t, value)
	assert.Equal(t, "client-alias", intent["model"])
	assert.Equal(t, "image_to_image", intent["action"])
	request := intent["requestBody"].(map[string]any)
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl": "https://custom.example/", "apiKey": "test-key", "model": "client-alias", "upstreamModel": "gpt-image-2.5-sunburst", "requestBody": request,
	})
	require.NoError(t, err)
	descriptor := jingyuObject(t, value)
	assert.Equal(t, "https://custom.example/v1/image/generations", descriptor["url"])
	assert.Equal(t, "Bearer test-key", descriptor["headers"].(map[string]any)["Authorization"])
	upstream := descriptor["body"].(map[string]any)
	assert.Equal(t, "gpt-image-2.5-sunburst", upstream["model"])
	assert.Equal(t, "3840x2160", upstream["aspect_ratio"])
	assert.Equal(t, "4K", upstream["image_size"])
	assert.Equal(t, "url", upstream["response_format"])
	assert.Equal(t, true, upstream["async"])
	assert.Equal(t, float64(1), upstream["task_count"])
	assert.Equal(t, body["images"], upstream["image_urls"])
	assert.Equal(t, float64(0), upstream["seed"])
	assert.Equal(t, false, upstream["watermark"])
	assert.Equal(t, jingyuObject(t, body)["extra"], upstream["extra"])
	after, err := common.Marshal(body)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after), "conversion must not alter the original client request used by retries")

	value, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{
		"model": "nano-banana-2", "operation": "edit", "body": map[string]any{"kind": "multipart", "fields": map[string]any{
			"prompt": []string{"a cat"}, "n": []string{"1"}, "image_urls": []string{`["https://cdn.example/first.png"]`},
			"watermark": []string{"false"}, "extra": []string{`{"future":[0,false]}`},
		}},
	})
	require.NoError(t, err)
	request = jingyuObject(t, value)["requestBody"].(map[string]any)
	assert.Equal(t, false, request["watermark"])
	assert.Equal(t, []any{float64(0), false}, request["extra"].(map[string]any)["future"])
}

func TestJingyuImageValidation(t *testing.T) {
	plugin := newJingyuPlugin(t)
	for _, tc := range []struct {
		name    string
		field   string
		value   any
		message string
	}{
		{"batch", "n", 2, "n must be 1"},
		{"zero", "n", 0, "n must be 1"},
		{"fraction", "n", 1.5, "n must be 1"},
		{"overflow", "n", 1e30, "n must be 1"},
		{"string count", "n", "1", "n must be 1"},
		{"tasks", "task_count", 2, "task_count must be 1"},
		{"data URL", "image", "data:image/png;base64,aGVsbG8=", "HTTP(S) URLs"},
		{"mask", "mask", "https://cdn.example/mask.png", "mask is not supported"},
		{"stream", "stream", true, "stream is not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"model": "nano-banana-2", "prompt": "a cat", tc.field: tc.value}
			for _, path := range [][]string{{"openai_image", "decodeRequest"}, {"createImage"}} {
				export := "protocols"
				if len(path) == 1 {
					export = "native"
				}
				_, err := plugin.Engine.CallPath(t.Context(), export, path, map[string]any{
					"model": "nano-banana-2", "operation": "generate", "body": map[string]any{"kind": "json", "value": body},
				})
				require.ErrorContains(t, err, tc.message)
			}
		})
	}
}

func TestJingyuTaskLifecycle(t *testing.T) {
	plugin := newJingyuPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"publicTaskId": "public-id"}, map[string]any{
		"body": map[string]any{"id": "task_vendor", "task_id": "task_vendor", "status": "queued", "created": 1791529679},
	})
	require.NoError(t, err)
	assert.Equal(t, "task_vendor", jingyuObject(t, value)["taskId"])
	value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{"baseUrl": "https://jingyuapi.art", "apiKey": "test-key", "taskId": "task_vendor"})
	require.NoError(t, err)
	assert.Equal(t, "https://jingyuapi.art/v1/image/generations/task_vendor", jingyuObject(t, value)["url"])
	const url = "https://cdn.example/result.png"
	for _, tc := range []struct {
		name    string
		payload map[string]any
		status  string
		count   float64
	}{
		{"running", map[string]any{"status": "IN_PROGRESS", "progress": "1%"}, "IN_PROGRESS", 0},
		{"unknown", map[string]any{"status": "new_state"}, "UNKNOWN", 0},
		{"failure", map[string]any{"status": "FAILURE", "fail_reason": "generation rejected"}, "FAILURE", 0},
		{"empty success", map[string]any{"status": "SUCCESS", "results": []any{map[string]any{"revised_prompt": "a cat"}}}, "FAILURE", 0},
		{"actual log", map[string]any{"status": "SUCCESS", "results": []any{map[string]any{"url": url, "b64_json": "", "revised_prompt": ""}}, "result_urls": []any{url}, "result_asset_urls": []any{url}}, "SUCCESS", 1},
		{"URL fallback", map[string]any{"status": "success", "results": []any{}, "result_urls": []any{url}}, "SUCCESS", 1},
		{"split payload", map[string]any{"status": "SUCCESS", "results": []any{map[string]any{"url": url}, map[string]any{"b64_json": "abc"}}}, "SUCCESS", 1},
		{"invalid upstream count", map[string]any{"status": "SUCCESS", "results": []any{map[string]any{"url": url}}, "usage": map[string]any{"image_count": 1e30}}, "SUCCESS", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"code": "success", "message": "", "data": tc.payload}
			encoded, err := common.Marshal(body)
			require.NoError(t, err)
			adaptor := taskplugin.New(plugin)
			adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
			task := &model.Task{TaskID: "public-id", Properties: model.Properties{OriginModelName: "alias", UpstreamModelName: "gpt-image-2.5-sunburst"}}
			result, err := adaptor.ParseTaskResult(task, &http.Response{StatusCode: http.StatusOK}, encoded)
			require.NoError(t, err)
			assert.Equal(t, tc.status, result.Status)
			if tc.count == 0 {
				assert.Empty(t, result.UsageFacts)
			} else {
				assert.Equal(t, tc.count, result.UsageFacts["image_count"])
			}
			if tc.status == "SUCCESS" {
				value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "render"}, map[string]any{}, map[string]any{"created_at": 123, "data": body})
				require.NoError(t, err)
				data := jingyuObject(t, value)["data"].([]any)
				assert.Equal(t, url, data[0].(map[string]any)["url"])
			}
		})
	}
	value, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"publicTaskId": "public-id"}, map[string]any{"body": map[string]any{"data": []any{map[string]any{"url": url}}}})
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", jingyuObject(t, value)["immediate"].(map[string]any)["status"])
}

func TestJingyuNativeImageEndpoints(t *testing.T) {
	plugin := newJingyuPlugin(t)
	generation := jsplugin.DefaultRegistry.Generation()
	submit, found := generation.LookupDeclaredRoute(http.MethodPost, "/jingyu/v1/image/generations")
	require.True(t, found)
	query, found := generation.LookupDeclaredRoute(http.MethodGet, "/jingyu/v1/image/generations/:task_id")
	require.True(t, found)
	assert.Equal(t, "jingyu", submit.Plugin.Meta.Key)
	assert.Equal(t, "jingyu", query.Plugin.Meta.Key)
	body := map[string]any{
		"model": "nano-banana-2", "prompt": "a cat", "async": false, "response_format": "b64_json",
		"aspect_ratio": "16:9", "image_size": "4K", "images": []any{"https://cdn.example/reference.png"},
		"seed": 0, "watermark": false, "extra": map[string]any{"future": []any{0, false}},
	}
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{submit.Route.Decode}, map[string]any{
		"body": map[string]any{"kind": "json", "value": body},
	})
	require.NoError(t, err)
	intent := jingyuObject(t, value)
	assert.Equal(t, "image_to_image", intent["action"])
	assert.Equal(t, jingyuObject(t, body), intent["requestBody"], "native decoding must preserve the vendor format and explicit false")
	for _, upstream := range []string{"vendor", "new_api"} {
		t.Run(upstream, func(t *testing.T) {
			ctx := map[string]any{
				"baseUrl": "https://upstream.example", "apiKey": "test-key", "model": "nano-banana-2", "upstreamModel": "gpt-image-2.5-sunburst",
				"upstream": map[string]any{"kind": upstream}, "requestBody": intent["requestBody"], "taskId": "public-task",
			}
			prefix := ""
			if upstream == "new_api" {
				prefix = "/jingyu"
			}
			value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			descriptor := jingyuObject(t, value)
			assert.Equal(t, "https://upstream.example"+prefix+"/v1/image/generations", descriptor["url"])
			assert.Equal(t, "gpt-image-2.5-sunburst", descriptor["body"].(map[string]any)["model"])
			assert.Equal(t, false, descriptor["body"].(map[string]any)["async"])
			value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
			require.NoError(t, err)
			assert.Equal(t, "https://upstream.example"+prefix+"/v1/image/generations/public-task", jingyuObject(t, value)["url"])
		})
	}

	task := &model.Task{TaskID: "public-task", Platform: "jingyu", Status: model.TaskStatusSubmitted, Progress: "0%", CreatedAt: 123}
	task.PrivateData.UpstreamTaskID = "private-vendor-task"
	task.SetData(map[string]any{"id": "private-vendor-task", "task_id": "private-vendor-task", "status": "queued", "created": 123})
	view, err := service.BuildTaskPluginView(task)
	require.NoError(t, err)
	value, err = plugin.Engine.CallPath(t.Context(), "native", []string{submit.Route.Render}, map[string]any{}, jingyuObject(t, view))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"id": "public-task", "task_id": "public-task", "status": "queued", "created": float64(123)}, jingyuObject(t, value))

	for _, status := range []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure} {
		t.Run(string(status), func(t *testing.T) {
			task.Status, task.Progress = status, "100%"
			task.FailReason = ""
			if status == model.TaskStatusFailure {
				task.FailReason = "host polling timeout"
			}
			task.SetData(map[string]any{"code": "success", "data": map[string]any{
				"task_id": "private-vendor-task", "status": "IN_PROGRESS", "progress": "1%",
				"results": []any{map[string]any{"url": "https://cdn.example/result.png"}},
			}})
			view, err := service.BuildTaskPluginView(task)
			require.NoError(t, err)
			value, err := plugin.Engine.CallPath(t.Context(), "native", []string{query.Route.Render}, map[string]any{}, jingyuObject(t, view))
			require.NoError(t, err)
			response := jingyuObject(t, value)
			data := response["data"].(map[string]any)
			assert.Equal(t, "success", response["code"])
			assert.Equal(t, "public-task", data["task_id"])
			assert.Equal(t, string(status), data["status"], "host terminal status must override the stale upstream snapshot")
			if status == model.TaskStatusFailure {
				assert.Equal(t, task.FailReason, data["fail_reason"])
			}
		})
	}

	response := map[string]any{"created": 123, "data": []any{map[string]any{"b64_json": "QUFB"}}}
	value, err = plugin.Engine.CallPath(t.Context(), "native", []string{submit.Route.Render}, map[string]any{}, map[string]any{"status": "SUCCESS", "data": response})
	require.NoError(t, err)
	assert.Equal(t, jingyuObject(t, response), jingyuObject(t, value), "synchronous native image output must keep its original envelope")
	value, err = plugin.Engine.CallPath(t.Context(), "native", []string{query.Route.Render}, map[string]any{}, map[string]any{"task_id": "public-task", "status": "SUCCESS", "data": response})
	require.NoError(t, err)
	queryResponse := jingyuObject(t, value)
	value, err = plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"taskId": "public-task"}, queryResponse)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", jingyuObject(t, value)["status"], "a gateway upstream must accept native retrieval of an immediate result")
}

func TestJingyuVideoProtocol(t *testing.T) {
	plugin := newJingyuPlugin(t)
	for _, name := range plugin.Meta.Protocols[1].Models {
		assert.NotEmpty(t, jsplugin.DefaultRegistry.Generation().LookupEndpointCandidates(http.MethodPost, "/v1/videos", name), name)
	}
	body := map[string]any{"model": "client-alias", "prompt": "a cat", "seconds": "5", "size": "1280x720", "input_reference": "https://cdn.example/cat.png", "seed": 0, "generate_audio": false, "extra": map[string]any{"doubao": map[string]any{"watermark": false}, "future": []any{0, false}}}
	before, err := common.Marshal(body)
	require.NoError(t, err)
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "client-alias", "upstreamModel": "doubao-seedance-2-0-260128", "body": map[string]any{"kind": "json", "value": body}})
	require.NoError(t, err)
	intent := jingyuObject(t, value)
	assert.Equal(t, "image_to_video", intent["action"])
	request := intent["requestBody"].(map[string]any)
	ctx := map[string]any{"baseUrl": "https://upstream.example/", "apiKey": "test-key", "model": "client-alias", "upstreamModel": "doubao-seedance-2-0-260128", "action": intent["action"], "requestBody": request}
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	descriptor := jingyuObject(t, value)
	assert.Equal(t, "https://upstream.example/v1/video/generations", descriptor["url"])
	payload := descriptor["body"].(map[string]any)
	assert.Equal(t, "doubao-seedance-2-0-260128", payload["model"])
	assert.Equal(t, float64(5), payload["duration"])
	assert.Equal(t, "16:9", payload["aspect_ratio"])
	assert.Equal(t, "720p", payload["resolution"])
	assert.Equal(t, false, payload["generate_audio"])
	assert.Equal(t, float64(0), payload["seed"])
	assert.Equal(t, jingyuObject(t, body)["extra"], payload["extra"])
	assert.Equal(t, []any{map[string]any{"type": "image", "role": "first_frame", "url": body["input_reference"]}}, payload["references"])
	assert.NotContains(t, payload, "seconds")
	assert.NotContains(t, payload, "input_reference")
	after, err := common.Marshal(body)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after), "retry conversion must preserve the original request")
	value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"video_count": float64(1), "seconds": float64(5), "resolution": "720p", "audio": "disabled"}, jingyuObject(t, value))
	ctx["usagePurpose"] = "billing_ratios"
	value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"seconds": float64(5)}, jingyuObject(t, value))
}

func TestJingyuVideoUploadsAndNative(t *testing.T) {
	plugin := newJingyuPlugin(t)
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "OpenAI", true: "native"}[native], func(t *testing.T) {
			fields := map[string]any{"prompt": []string{"a cat"}, "seconds": []string{"5"}, "generate_audio": []string{"false"}}
			fileField, export, path := "input_reference", "protocols", []string{"openai_video", "decodeRequest"}
			if native {
				fields = map[string]any{"request": []string{`{"model":"starvideos_o3","prompt":"a cat","duration":5,"references":[{"type":"image","role":"reference_image","file_key":"photo"}],"extra":{"future":[0,false]}}`}}
				fileField, export, path = "photo", "native", []string{"createVideo"}
			}
			value, err := plugin.Engine.CallPath(t.Context(), export, path, map[string]any{"model": "starvideos_o3", "body": map[string]any{"kind": "multipart", "fields": fields, "files": []any{map[string]any{"field": fileField, "ref": "request_file:" + fileField, "filename": "cat.png"}}}})
			require.NoError(t, err)
			intent := jingyuObject(t, value)
			ctx := map[string]any{"model": "starvideos_o3", "action": intent["action"], "baseUrl": "https://upstream.example", "apiKey": "test-key", "upstream": map[string]any{"kind": "new_api"}, "requestBody": intent["requestBody"]}
			value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			descriptor := jingyuObject(t, value)
			assert.Equal(t, "https://upstream.example/jingyu/v1/video/generations", descriptor["url"])
			assert.Equal(t, "multipart", descriptor["bodyType"])
			parts := descriptor["parts"].([]any)
			var payload map[string]any
			require.NoError(t, common.UnmarshalJsonStr(parts[0].(map[string]any)["value"].(string), &payload))
			assert.Equal(t, "reference_image", payload["references"].([]any)[0].(map[string]any)["role"])
			assert.Equal(t, "request_file:"+fileField, parts[1].(map[string]any)["fileRef"])
		})
	}
}

func TestJingyuVideoValidation(t *testing.T) {
	plugin := newJingyuPlugin(t)
	for _, tc := range []struct {
		name, field string
		value       any
		message     string
	}{
		{"zero", "duration", 0, "integer between"}, {"overflow", "duration", 1e30, "integer between"}, {"fraction", "duration", 1.5, "integer between"},
		{"batch", "task_count", 2, "must be 1"}, {"audio type", "generate_audio", 0, "boolean"}, {"bad reference", "references", []any{map[string]any{"type": "image", "role": "first_frame", "url": "data:image/png;base64,abc"}}, "HTTP(S)"},
		{"unbound file", "references", []any{map[string]any{"type": "image", "role": "first_frame", "file_key": "missing"}}, "uploaded file"},
		{"legacy media", "video_url", "https://cdn.example/a.mp4", "not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, native := range []bool{false, true} {
				export, path := "protocols", []string{"openai_video", "decodeRequest"}
				if native {
					export, path = "native", []string{"createVideo"}
				}
				_, err := plugin.Engine.CallPath(t.Context(), export, path, map[string]any{"model": "doubao-seedance-2-0-260128", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "a cat", "duration": 5, tc.field: tc.value}}})
				require.ErrorContains(t, err, tc.message)
			}
		})
	}
}

func TestJingyuVideoLifecycleAndContent(t *testing.T) {
	plugin := newJingyuPlugin(t)
	ctx := map[string]any{"model": "doubao-seedance-2-0-260128", "action": "text_to_video", "taskId": "vendor-id", "baseUrl": "https://upstream.example", "apiKey": "test-key"}
	for _, tc := range []struct{ vendor, host string }{{"queued", "QUEUED"}, {"processing", "IN_PROGRESS"}, {"succeeded", "SUCCESS"}, {"failed", "FAILURE"}, {"new_state", "UNKNOWN"}} {
		value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, map[string]any{"task_id": "vendor-id", "status": tc.vendor, "progress": 45})
		require.NoError(t, err)
		assert.Equal(t, tc.host, jingyuObject(t, value)["status"])
	}
	body := map[string]any{"task_id": "vendor-id", "status": "succeeded", "duration": 6, "resolution": "1080p", "generate_audio": false}
	value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, body)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"video_count": float64(1), "seconds": float64(6), "resolution": "1080p", "audio": "disabled"}, jingyuObject(t, value))
	body["duration"] = 1e30
	value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, map[string]any{"status": "SUCCESS"}, body)
	require.NoError(t, err)
	assert.NotContains(t, jingyuObject(t, value), "seconds", "invalid upstream duration must preserve reserved usage")
	value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example/v1/video/generations/vendor-id", jingyuObject(t, value)["url"])
	task := &model.Task{TaskID: "public-id", Platform: "jingyu", Action: "text_to_video", Status: model.TaskStatusSuccess, Progress: "100%", CreatedAt: 123, Properties: model.Properties{OriginModelName: "doubao-seedance-2-0-260128"}}
	task.PrivateData.UpstreamTaskID = "vendor-id"
	task.SetData(map[string]any{"task_id": "vendor-id", "status": "processing", "duration": 5, "resolution": "720p", "aspect_ratio": "16:9"})
	view, err := service.BuildTaskPluginView(task)
	require.NoError(t, err)
	value, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "render"}, map[string]any{}, jingyuObject(t, view))
	require.NoError(t, err)
	assert.Equal(t, "public-id", jingyuObject(t, value)["task_id"])
	assert.Equal(t, "5", jingyuObject(t, value)["seconds"])
	assert.Equal(t, "1280x720", jingyuObject(t, value)["size"])
	adaptor := taskplugin.New(plugin)
	artifacts, err := adaptor.ListArtifacts(task)
	require.NoError(t, err)
	assert.Len(t, artifacts, 1, "completed tasks without a public URL must still provide content")
	videoJSON, err := adaptor.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var video map[string]any
	require.NoError(t, common.Unmarshal(videoJSON, &video))
	assert.Equal(t, "public-id", video["id"])
	assert.Equal(t, "video", video["object"])
	assert.Equal(t, "completed", video["status"])
	assert.NotContains(t, video, "task_id")
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		ctx["artifactKey"], ctx["upstreamTaskId"] = "video", "vendor-id"
		ctx["clientRequest"] = map[string]any{"method": method, "headers": map[string]any{"Range": "bytes=0-99", "If-None-Match": "etag"}}
		value, err := plugin.Engine.Call(t.Context(), "buildContentRequest", ctx)
		require.NoError(t, err)
		descriptor := jingyuObject(t, value)
		assert.Equal(t, "https://upstream.example/v1/video/generations/vendor-id/content", descriptor["url"])
		assert.Equal(t, method, descriptor["method"])
		assert.Equal(t, map[string]any{"Authorization": "Bearer test-key", "Range": "bytes=0-99", "If-None-Match": "etag"}, descriptor["headers"])
		binding, found := jsplugin.DefaultRegistry.Generation().LookupDeclaredRoute(method, "/jingyu/v1/video/generations/:task_id/content")
		require.True(t, found)
		assert.Equal(t, jsplugin.RouteTypeContent, binding.Route.Type)
	}
}
