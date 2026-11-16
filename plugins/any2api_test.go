package plugins_test

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func any2APIPlugin(t *testing.T, key string) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source(key)
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: key})
	require.NoError(t, err, "Source 必须返回可独立安装的完整插件")
	return plugin
}

func any2APIObject(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func TestAny2APISharedModelRetryConversion(t *testing.T) {
	// 每次选中渠道后从同一客户端请求重解码，映射与参数不得沿用前一家供应商。
	body := map[string]any{"model": "seedream-5.0-pro", "prompt": "一只小恐龙", "seed": 0, "watermark": false, "future": map[string]any{"items": []any{0, false}}}
	before, err := common.Marshal(body)
	require.NoError(t, err)
	for _, tc := range []struct{ key, mapped, capability string }{
		{"libtv", "doubao-seedream-5-0-pro", "doubao-seedream-5-0-pro"},
		{"xyq", "seedream_5.0_pro", "seedream_5.0_pro"},
		{"jimeng-reverse", "image_5.0_pro", "image_5.0_pro"},
		{"magnific-reverse", "seedream-5-pro", "magnific-workflow-image"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			plugin := any2APIPlugin(t, tc.key)
			value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{
				"model": "seedream-5.0-pro", "upstreamModel": tc.mapped, "body": map[string]any{"kind": "json", "value": body},
			})
			require.NoError(t, err)
			intent := any2APIObject(t, value)
			assert.Equal(t, "seedream-5.0-pro", intent["model"])
			ctx := map[string]any{"model": intent["model"], "upstreamModel": tc.mapped, "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://hub.example/api/hub/suppliers/provider/native/", "apiKey": "test-key"}
			value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			request := any2APIObject(t, value)
			assert.Equal(t, "https://hub.example/api/hub/suppliers/provider/native/api/v1/tasks/json", request["url"])
			assert.Equal(t, map[string]any{"X-API-Key": "test-key"}, request["headers"])
			payload := request["body"].(map[string]any)
			assert.Equal(t, tc.capability, payload["capability_id"])
			assert.Equal(t, tc.mapped, payload["model"])
			extra := payload["extra"].(map[string]any)
			assert.Equal(t, false, extra["watermark"])
			assert.Equal(t, map[string]any{"items": []any{float64(0), false}}, extra["future"])
			if tc.key == "xyq" {
				assert.Equal(t, float64(0), payload["seed"])
			} else {
				assert.Equal(t, float64(0), extra["seed"])
			}
			ctx["baseUrl"] = "https://gateway.example/prefix"
			ctx["upstream"] = map[string]any{"kind": "new_api"}
			value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			request = any2APIObject(t, value)
			assert.Equal(t, "https://gateway.example/prefix/"+tc.key+"/api/v1/tasks/json", request["url"])
			assert.Equal(t, map[string]any{"Authorization": "Bearer test-key"}, request["headers"])
			ctx["taskId"] = "upstream/task"
			value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
			require.NoError(t, err)
			assert.Equal(t, "https://gateway.example/prefix/"+tc.key+"/api/v1/tasks/upstream%2Ftask", any2APIObject(t, value)["url"])
		})
	}
	after, err := common.Marshal(body)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestAny2APINativeMultipartAndSync(t *testing.T) {
	for _, tc := range []struct{ key, model, countField, extraField string }{
		{"libtv", "lib-image-2", "count", "extra"}, {"xyq", "gpt_image_2", "", ""},
		{"jimeng-reverse", "image_5.0_pro", "generate_count", ""}, {"magnific-reverse", "gpt-2", "", "extra_json"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			plugin := any2APIPlugin(t, tc.key)
			fields := map[string][]string{"model": {tc.model}, "prompt": {"cat"}, "async": {"false"}}
			if tc.countField != "" {
				fields[tc.countField] = []string{"2"}
			}
			if tc.extraField != "" {
				fields[tc.extraField] = []string{`{"future":[0,false]}`}
			}
			files := []any{map[string]any{"field": "references", "ref": "request_file:references", "filename": "one.png"}, map[string]any{"field": "references", "ref": "request_file:references#1", "filename": "two.png"}}
			value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "multipart", "fields": fields, "files": files}})
			require.NoError(t, err)
			intent := any2APIObject(t, value)
			ctx := map[string]any{"model": intent["model"], "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://vendor.example", "apiKey": "key"}
			value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
			require.NoError(t, err)
			request := any2APIObject(t, value)
			assert.Equal(t, "multipart", request["bodyType"])
			assert.Equal(t, "https://vendor.example/api/v1/tasks", request["url"])
			parts := request["parts"].([]any)
			assert.Contains(t, parts, map[string]any{"name": "async", "value": "false"})
			assert.Contains(t, parts, map[string]any{"name": "references", "fileRef": "request_file:references#1", "filename": "two.png"})
			if tc.extraField != "" {
				assert.Contains(t, parts, map[string]any{"name": tc.extraField, "value": `{"future":[0,false]}`})
			}
			value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
			require.NoError(t, err)
			count := float64(1)
			if tc.countField != "" {
				count = 2
			}
			assert.Equal(t, count, any2APIObject(t, value)["image_count"])
			if tc.extraField == "" {
				fields["extra"] = []string{`{"watermark":false}`}
				_, err = plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "multipart", "fields": fields, "files": files}})
				require.ErrorContains(t, err, "multipart 不支持 extra")
			}
		})
	}
}

func TestAny2APILifecycleArtifactsAndQuantity(t *testing.T) {
	for _, key := range []string{"libtv", "xyq", "jimeng-reverse", "magnific-reverse"} {
		t.Run(key, func(t *testing.T) {
			plugin := any2APIPlugin(t, key)
			ctx := map[string]any{"action": "generate_image", "taskId": "private"}
			for state, expected := range map[string]string{"created": "QUEUED", "validating": "IN_PROGRESS", "uploading": "IN_PROGRESS", "preparing": "IN_PROGRESS", "submitted": "IN_PROGRESS", "polling": "IN_PROGRESS", "downloading": "IN_PROGRESS", "success": "SUCCESS", "failed": "FAILURE", "timeout": "FAILURE", "cancelled": "FAILURE", "new-state": "UNKNOWN"} {
				body := map[string]any{"ok": true, "data": map[string]any{"id": "private", "status": state, "images": []any{map[string]any{"local_path": "/files/outputs/private/a.png"}}, "cost": map[string]any{"power": 999999}}}
				value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", ctx, body)
				require.NoError(t, err)
				parsed := any2APIObject(t, value)
				assert.Equal(t, expected, parsed["status"], state)
				value, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", ctx, map[string]any{"body": body})
				require.NoError(t, err)
				submitted := any2APIObject(t, value)
				assert.Equal(t, "private", submitted["taskId"])
				if expected == "SUCCESS" || expected == "FAILURE" {
					assert.Equal(t, expected, submitted["immediate"].(map[string]any)["status"])
				} else {
					assert.Nil(t, submitted["immediate"])
				}
			}
			for _, media := range []string{"image", "video"} {
				for _, public := range []bool{false, true} {
					item := map[string]any{"local_path": "/files/outputs/private/a.bin", "media_type": media}
					if public {
						item["url"] = "https://cdn.example/result"
					}
					body := map[string]any{"ok": true, "data": map[string]any{"id": "private", "status": "success", "images": []any{item, map[string]any{}}, "raw": map[string]any{"secret": "private-credential"}}}
					task := map[string]any{"task_id": "public", "action": "generate_" + media, "status": "SUCCESS", "data": body}
					value, err := plugin.Engine.Call(t.Context(), "listArtifacts", task)
					require.NoError(t, err)
					encoded, err := common.Marshal(value)
					require.NoError(t, err)
					assert.JSONEq(t, `[{"key":"`+media+`_0","type":"`+media+`"}]`, string(encoded))
					value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"action": "generate_" + media}, map[string]any{"status": "SUCCESS"}, body)
					require.NoError(t, err)
					assert.Equal(t, map[string]any{media + "_count": float64(1)}, any2APIObject(t, value))
					contentCtx := map[string]any{"baseUrl": "https://hub.example/api/hub/suppliers/provider/native", "apiKey": "key", "data": body, "upstreamTaskId": "private", "artifactKey": media + "_0", "clientRequest": map[string]any{"method": "HEAD"}}
					value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", contentCtx)
					require.NoError(t, err)
					request := any2APIObject(t, value)
					assert.Equal(t, "HEAD", request["method"])
					assert.Equal(t, "https://hub.example/api/hub/suppliers/provider/native/files/outputs/private/a.bin", request["url"])
					assert.Equal(t, map[string]any{"X-API-Key": "key"}, request["headers"])
					contentCtx["upstream"] = map[string]any{"kind": "new_api"}
					value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", contentCtx)
					require.NoError(t, err)
					assert.Equal(t, "https://hub.example/api/hub/suppliers/provider/native/v1/tasks/private/artifacts/"+media+"_0/content", any2APIObject(t, value)["url"])
					if public {
						delete(item, "local_path")
						value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", contentCtx)
						require.NoError(t, err)
						request = any2APIObject(t, value)
						assert.Equal(t, "https://cdn.example/result", request["url"])
						assert.Equal(t, true, request["credentialless"])
						assert.Nil(t, request["headers"])
					}
					if media == "image" {
						value, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "render"}, map[string]any{}, task)
						require.NoError(t, err)
						entry := any2APIObject(t, value)["data"].([]any)[0].(map[string]any)
						assert.Equal(t, "image_0", entry["artifact_key"])
						if !public {
							assert.Nil(t, entry["url"])
						}
					}
					value, err = plugin.Engine.CallPath(t.Context(), "native", []string{"taskView"}, map[string]any{}, task)
					require.NoError(t, err)
					presented := any2APIObject(t, value)["data"].(map[string]any)
					assert.Equal(t, "public", presented["id"])
					assert.Empty(t, presented["raw"])
				}
			}
		})
	}
}

func TestLovartFinalParametersAndArtifacts(t *testing.T) {
	plugin := any2APIPlugin(t, "lovart")
	body := map[string]any{"generatorName": "qwenimage/qwen-image-3-0", "prompt": "cat", "count": 2, "modelParams": map[string]any{"n": 3}, "inputArgs": map[string]any{"n": 4, "seed": 0, "watermark": false, "future": []any{0, false}}}
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": body}})
	require.NoError(t, err)
	intent := any2APIObject(t, value)
	ctx := map[string]any{"model": intent["model"], "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://hub.example/api/hub/suppliers/lovart/native", "apiKey": "key"}
	value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	assert.Equal(t, float64(4), any2APIObject(t, value)["image_count"])
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	request := any2APIObject(t, value)
	assert.Equal(t, "https://hub.example/api/hub/suppliers/lovart/native/api/tasks", request["url"])
	var args map[string]any
	for _, part := range request["parts"].([]any) {
		field := part.(map[string]any)
		if field["name"] == "inputArgs" {
			require.NoError(t, common.UnmarshalJsonStr(field["value"].(string), &args))
		}
	}
	assert.Equal(t, float64(4), args["n"])
	assert.Equal(t, false, args["watermark"])
	assert.Equal(t, float64(0), args["seed"])
	assert.Equal(t, []any{float64(0), false}, args["future"])
	for _, tc := range []struct{ status, expected string }{{"queued", "QUEUED"}, {"running", "IN_PROGRESS"}, {"completed", "SUCCESS"}, {"failed", "FAILURE"}, {"stopped", "FAILURE"}, {"other", "UNKNOWN"}} {
		resultBody := map[string]any{"id": "private", "status": tc.status, "outputArtifacts": []any{map[string]any{"index": 7, "type": "image", "bytes": 100}, map[string]any{"type": "image"}}}
		value, err = plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"action": "generate_image"}, resultBody)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, any2APIObject(t, value)["status"])
		value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{"baseUrl": "https://hub.example/prefix", "apiKey": "key", "upstreamTaskId": "private", "artifactKey": "image_0", "data": resultBody, "clientRequest": map[string]any{"method": "GET"}})
		require.NoError(t, err)
		assert.Equal(t, "https://hub.example/prefix/api/tasks/private/outputs/0", any2APIObject(t, value)["url"])
		value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", map[string]any{"action": "generate_image"}, map[string]any{"status": tc.expected}, resultBody)
		require.NoError(t, err)
		if tc.expected == "SUCCESS" {
			assert.Equal(t, float64(1), any2APIObject(t, value)["image_count"])
		} else {
			assert.Empty(t, any2APIObject(t, value))
		}
	}
	for _, invalid := range []any{0, -1, 129, 1.5, true} {
		body["inputArgs"] = map[string]any{"n": invalid}
		_, err = plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": body}})
		require.ErrorContains(t, err, "n 必须是")
	}
}

func TestAny2APIMagnificWorkflowMappingAndBilling(t *testing.T) {
	plugin := any2APIPlugin(t, "magnific-reverse")
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createJSONTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{"capability_id": "magnific-workflow-video", "model": "kling-40-omni", "prompt": "cat", "async": false, "extra": map[string]any{"numberOfVideos": 2, "withSoundEffects": false}}}})
	require.NoError(t, err)
	intent := any2APIObject(t, value)
	assert.Equal(t, "kling-40-omni", intent["model"])
	ctx := map[string]any{"model": intent["model"], "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://vendor.example", "apiKey": "key"}
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	payload := any2APIObject(t, value)["body"].(map[string]any)
	assert.Equal(t, "magnific-workflow-video", payload["capability_id"])
	assert.Equal(t, "kling-40-omni", payload["model"])
	assert.Equal(t, false, payload["async"])
	value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
	require.NoError(t, err)
	assert.Equal(t, float64(2), any2APIObject(t, value)["video_count"])
}

func TestAny2APIBillingBoundsAndMetadata(t *testing.T) {
	for _, key := range []string{"libtv", "xyq", "jimeng-reverse", "magnific-reverse", "lovart", "rh-reverse"} {
		plugin := any2APIPlugin(t, key)
		schemas := []map[string]jsplugin.UsageFieldSchema{plugin.Meta.UsageSchema}
		for _, profile := range plugin.Meta.UsageProfiles {
			schemas = append(schemas, profile.Schema)
		}
		for _, schema := range schemas {
			for _, field := range schema {
				for _, locale := range []string{"en", "zh", "zh-TW", "fr", "ru", "ja", "vi"} {
					assert.NotEmpty(t, field.Description[locale], key+" "+locale)
				}
				assert.Contains(t, field.Description["en"], "unit price")
			}
		}
		if key == "lovart" || key == "rh-reverse" {
			continue
		}
		for _, invalid := range []any{0, -1, 129, 1.5, true} {
			_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": "alias", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "cat", "extra": map[string]any{"numberOfGenerations": invalid}}}})
			require.ErrorContains(t, err, "必须是 1 到 128")
		}
		_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": "alias", "body": map[string]any{"kind": "json", "value": map[string]any{"image": "https://cdn.example/a.png"}}})
		require.ErrorContains(t, err, "multipart")
	}
	plugin := any2APIPlugin(t, "rh-reverse")
	for _, model := range []string{"nano-banana2-1", "nano-banana2-1-text", "nano-banana2-1-edit", "kling-v4", "kling-v4-text", "kling-v4-image", "kling-v4-reference", "kling-v4-canvas", "kling-v4-canvas-text", "kling-v4-canvas-start", "kling-v4-canvas-start-end", "kling-v4-canvas-reference"} {
		assert.Contains(t, plugin.Meta.Models, model)
		protocol := "openai_image"
		if strings.HasPrefix(model, "kling-") {
			protocol = "openai_video"
		}
		value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{protocol, "decodeRequest"}, map[string]any{"model": "shared-alias", "upstreamModel": model, "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "cat"}}})
		require.NoError(t, err)
		intent := any2APIObject(t, value)
		value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "shared-alias", "upstreamModel": model, "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://vendor.example", "apiKey": "key"})
		require.NoError(t, err)
		assert.Contains(t, any2APIObject(t, value)["parts"], map[string]any{"name": "modelKey", "value": model})
	}
}

func TestLovartMappedMultipartAndPublicID(t *testing.T) {
	plugin := any2APIPlugin(t, "lovart")
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "shared-video", "upstreamModel": "kling/kling-v4", "body": map[string]any{"kind": "multipart",
			"fields": map[string][]string{"model": {"shared-video"}, "prompt": {"cat"}, "seconds": {"5"}, "imageUrl": {"https://cdn.example/a.png", "https://cdn.example/b.png"}, "inputArgs": {`{"watermark":false,"seed":0}`}},
			"files":  []any{map[string]any{"field": "input_reference", "ref": "request_file:input_reference", "filename": "a.png"}, map[string]any{"field": "input_reference", "ref": "request_file:input_reference#1", "filename": "b.png"}},
		},
	})
	require.NoError(t, err)
	intent := any2APIObject(t, value)
	assert.Equal(t, "shared-video", intent["model"])
	ctx := map[string]any{"model": intent["model"], "upstreamModel": "kling/kling-v4", "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://gateway.example/prefix", "apiKey": "key", "upstream": map[string]any{"kind": "new_api"}}
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
	require.NoError(t, err)
	request := any2APIObject(t, value)
	assert.Equal(t, "https://gateway.example/prefix/lovart/api/tasks", request["url"])
	assert.Equal(t, map[string]any{"Authorization": "Bearer key"}, request["headers"])
	parts := request["parts"].([]any)
	assert.Contains(t, parts, map[string]any{"name": "generatorName", "value": "kling/kling-v4"})
	assert.Contains(t, parts, map[string]any{"name": "imageUrl", "value": "https://cdn.example/b.png"})
	assert.Contains(t, parts, map[string]any{"name": "image", "fileRef": "request_file:input_reference#1", "filename": "b.png"})
	for _, public := range []bool{false, true} {
		artifact := map[string]any{"index": 0, "type": "video", "bytes": 100, "outputPath": "/private/result.mp4"}
		if public {
			artifact["content"] = "https://cdn.example/result.mp4"
		}
		body := map[string]any{"id": "private", "status": "completed", "outputArtifacts": []any{artifact}, "rawRecord": map[string]any{"secret": "private-credential"}}
		ctx["data"] = body
		ctx["artifactKey"] = "video_0"
		ctx["upstreamTaskId"] = "private"
		ctx["clientRequest"] = map[string]any{"method": "HEAD"}
		value, err = plugin.Engine.Call(t.Context(), "buildContentRequest", ctx)
		require.NoError(t, err)
		request = any2APIObject(t, value)
		if public {
			assert.Equal(t, true, request["credentialless"])
			assert.Nil(t, request["headers"])
		} else {
			assert.Equal(t, "https://gateway.example/prefix/v1/tasks/private/artifacts/video_0/content", request["url"])
			assert.Equal(t, map[string]any{"Authorization": "Bearer key"}, request["headers"])
		}
		value, err = plugin.Engine.CallPath(t.Context(), "native", []string{"taskView"}, map[string]any{}, map[string]any{"task_id": "public", "status": "SUCCESS", "data": body})
		require.NoError(t, err)
		presented := any2APIObject(t, value)
		assert.Equal(t, "public", presented["id"])
		assert.Nil(t, presented["rawRecord"])
		assert.NotContains(t, presented["outputArtifacts"].([]any)[0], "outputPath")
	}
}

func TestRHReverseLocalImageArtifact(t *testing.T) {
	plugin := any2APIPlugin(t, "rh-reverse")
	data := map[string]any{"id": "private", "status": "success", "outputUrl": "/outputs/private/a.png"}
	ctx := map[string]any{"action": "generate_image", "artifactKey": "image", "upstreamTaskId": "private", "baseUrl": "https://hub.example/api/hub/suppliers/rh/native", "apiKey": "key", "data": data, "clientRequest": map[string]any{"method": "GET"}}
	value, err := plugin.Engine.Call(t.Context(), "buildContentRequest", ctx)
	require.NoError(t, err)
	request := any2APIObject(t, value)
	assert.Equal(t, "https://hub.example/api/hub/suppliers/rh/native/outputs/private/a.png", request["url"])
	assert.Equal(t, map[string]any{"X-API-Key": "key"}, request["headers"])
	value, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "render"}, map[string]any{}, map[string]any{"status": "SUCCESS", "data": data})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"artifact_key": "image"}, any2APIObject(t, value)["data"].([]any)[0])
}

func TestAny2APIMultipartPromptRemainsText(t *testing.T) {
	for _, tc := range []struct{ key, model string }{
		{"libtv", "lib-image-2"}, {"xyq", "gpt_image_2"}, {"jimeng-reverse", "image_5.0_pro"},
		{"magnific-reverse", "gpt-2"}, {"lovart", "qwenimage/qwen-image-3-0"}, {"rh-reverse", "gpt-image-2-5-flare"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			plugin := any2APIPlugin(t, tc.key)
			for _, prompt := range []string{"[Portrait] a cat", "{a cat}", "true", "false"} {
				value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{
					"model": tc.model, "body": map[string]any{"kind": "multipart", "fields": map[string][]string{"model": {tc.model}, "prompt": {prompt}}},
				})
				require.NoError(t, err)
				intent := any2APIObject(t, value)
				value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": tc.model, "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://vendor.example", "apiKey": "key"})
				require.NoError(t, err)
				request := any2APIObject(t, value)
				var sent any
				if body, ok := request["body"].(map[string]any); ok {
					sent = body["prompt"]
				}
				if parts, ok := request["parts"].([]any); ok {
					for _, value := range parts {
						part := value.(map[string]any)
						if part["name"] == "prompt" {
							sent = part["value"]
						}
						if part["name"] == "inputArgs" {
							var args map[string]any
							require.NoError(t, common.UnmarshalJsonStr(part["value"].(string), &args))
							sent = args["prompt"]
						}
					}
				}
				assert.Equal(t, prompt, sent)
			}
		})
	}
}
