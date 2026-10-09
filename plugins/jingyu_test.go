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
	for _, name := range plugin.Meta.Models {
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
			body := map[string]any{"prompt": "a cat", tc.field: tc.value}
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{
				"model": "nano-banana-2", "operation": "generate", "body": map[string]any{"kind": "json", "value": body},
			})
			require.ErrorContains(t, err, tc.message)
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
