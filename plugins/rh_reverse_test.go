package plugins_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel"
	taskplugin "github.com/QuantumNous/new-api/relay/channel/task/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rhReversePlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("rh-reverse")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "rh-reverse"})
	require.NoError(t, err)
	return plugin
}

func rhReverseObject(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(encoded, &result))
	return result
}

func TestRHReverseProtocolConversion(t *testing.T) {
	plugin := rhReversePlugin(t)
	for _, tc := range []struct {
		protocol, model, mapped, action string
		body                            map[string]any
		want                            map[string]string
	}{
		{"openai_image", "gpt-image-2", "gpt-image-2-5-flare", "generate_image", map[string]any{
			"prompt": "a cat", "n": 1, "size": "2048x1152", "image": []any{"https://cdn.example/a.png", "https://cdn.example/b.png"},
			"modelParams": map[string]any{"future": []any{0, false, map[string]any{"enabled": false}}}, "seed": 0, "watermark": false,
		}, map[string]string{"modelKey": "gpt-image-2-5-flare", "resolution": "2k", "aspectRatio": "16:9", "seed": "0", "watermark": "false", "modelParams": `{"future":[0,false,{"enabled":false}]}`}},
		{"openai_video", "doubao-seedance-2-0-260128", "seedance2-global", "generate_video", map[string]any{
			"prompt": "a cat", "seconds": "6", "size": "1280x720", "input_reference": "https://cdn.example/a.png", "generate_audio": false,
		}, map[string]string{"modelKey": "seedance2-global", "duration": "6", "aspectRatio": "16:9", "resolution": "720p", "generateAudio": "false"}},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			tc.body["model"] = tc.model
			before, err := common.Marshal(tc.body)
			require.NoError(t, err)
			value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{tc.protocol, "decodeRequest"}, map[string]any{
				"model": tc.model, "upstreamModel": tc.mapped, "body": map[string]any{"kind": "json", "value": tc.body},
			})
			require.NoError(t, err)
			intent := rhReverseObject(t, value)
			assert.Equal(t, tc.model, intent["model"])
			assert.Equal(t, tc.action, intent["action"])
			for _, upstream := range []string{"vendor", "new_api"} {
				ctx := map[string]any{"model": tc.model, "upstreamModel": tc.mapped, "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://rh.example/", "apiKey": "test-key", "upstream": map[string]any{"kind": upstream}}
				value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", ctx)
				require.NoError(t, err)
				request := rhReverseObject(t, value)
				path, header, key := "/api/tasks", "X-API-Key", "test-key"
				if upstream == "new_api" {
					path, header, key = "/rh-reverse/api/tasks", "Authorization", "Bearer test-key"
				}
				assert.Equal(t, "https://rh.example"+path, request["url"])
				assert.Equal(t, map[string]any{header: key}, request["headers"])
				assert.Equal(t, "multipart", request["bodyType"])
				fields := map[string][]string{}
				for _, part := range request["parts"].([]any) {
					entry := part.(map[string]any)
					fields[entry["name"].(string)] = append(fields[entry["name"].(string)], entry["value"].(string))
				}
				for name, expected := range tc.want {
					assert.Equal(t, []string{expected}, fields[name], name)
				}
				assert.Equal(t, "https://cdn.example/a.png", fields["imageUrl"][0])
				assert.NotContains(t, fields, "seconds")
				assert.NotContains(t, fields, "n")
				ctx["taskId"] = "upstream/task"
				value, err = plugin.Engine.Call(t.Context(), "buildQueryRequest", ctx)
				require.NoError(t, err)
				query := rhReverseObject(t, value)
				assert.Equal(t, "https://rh.example"+path+"/upstream%2Ftask", query["url"])
				assert.Equal(t, map[string]any{header: key}, query["headers"])
				// JSON 客户端请求必须能经宿主转换成上游的 multipart，不能要求客户端原本就是表单。
				channelType := constant.ChannelTypeTaskPlugin
				if upstream == "new_api" {
					channelType = constant.ChannelTypeNewAPI
				}
				info := &relaycommon.RelayInfo{OriginModelName: tc.model, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType, ChannelBaseUrl: "https://rh.example", UpstreamModelName: tc.mapped, ApiKey: "test-key"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: tc.action}}
				adaptor := taskplugin.New(plugin)
				adaptor.Init(info)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(before))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set("task_request", intent["requestBody"])
				wire, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				form, err := multipart.NewReader(wire, strings.TrimPrefix(c.GetHeader("Content-Type"), "multipart/form-data; boundary=")).ReadForm(1024)
				require.NoError(t, err)
				assert.Equal(t, fields, form.Value)
				require.NoError(t, form.RemoveAll())
			}
			after, err := common.Marshal(tc.body)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after), "重试转换不能修改原始请求")
		})
	}
}

func TestRHReverseNativeMultipartFiles(t *testing.T) {
	plugin := rhReversePlugin(t)
	var input bytes.Buffer
	writer := multipart.NewWriter(&input)
	require.NoError(t, writer.WriteField("modelKey", "seedance2"))
	require.NoError(t, writer.WriteField("resolution", "720p"))
	require.NoError(t, writer.WriteField("generateAudio", "false"))
	require.NoError(t, writer.WriteField("modelParams", `{"seed":0,"watermark":false,"future":[0,false]}`))
	for _, filename := range []string{"first.png", "last.png"} {
		file, err := writer.CreateFormFile("image", filename)
		require.NoError(t, err)
		_, err = file.Write([]byte(filename))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/rh-reverse/api/tasks", bytes.NewReader(input.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	inbound, err := common.ParseMultipartFormReusable(c)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, inbound.RemoveAll()) })
	files := []any{
		map[string]any{"ref": "request_file:image", "field": "image", "filename": "first.png"},
		map[string]any{"ref": "request_file:image#1", "field": "image", "filename": "last.png"},
	}
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{
		"kind": "multipart", "fields": inbound.Value, "files": files,
	}})
	require.NoError(t, err)
	intent := rhReverseObject(t, value)
	assert.Equal(t, "seedance2", intent["model"])
	info := &relaycommon.RelayInfo{OriginModelName: "seedance2", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeTaskPlugin, ChannelBaseUrl: "https://rh.example", UpstreamModelName: "seedance2", ApiKey: "test-key"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "generate_video"}}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	c.Set("task_request", intent["requestBody"])
	body, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	content, err := io.ReadAll(body)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(content), strings.TrimPrefix(c.GetHeader("Content-Type"), "multipart/form-data; boundary="))
	form, err := reader.ReadForm(1024)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, form.RemoveAll()) })
	assert.Equal(t, []string{"seedance2"}, form.Value["modelKey"])
	assert.Equal(t, []string{"false"}, form.Value["generateAudio"])
	assert.JSONEq(t, `{"seed":0,"watermark":false,"future":[0,false]}`, form.Value["modelParams"][0])
	require.Len(t, form.File["image"], 2)
	for _, file := range form.File["image"] {
		opened, err := file.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(opened)
		require.NoError(t, err)
		require.NoError(t, opened.Close())
		assert.Equal(t, file.Filename, string(data))
	}
}

func TestRHReverseSubmitHTTPMultipart(t *testing.T) {
	service.InitHttpClient()
	plugin := rhReversePlugin(t)
	for _, fileCount := range []int{0, 2} {
		name := "generate"
		if fileCount > 0 {
			name = "edit"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/tasks", r.URL.Path)
				if err := r.ParseMultipartForm(1024); err != nil {
					http.Error(w, "multipart header or boundary missing", http.StatusBadRequest)
					return
				}
				defer r.MultipartForm.RemoveAll()
				assert.Equal(t, "seedream5-flash", r.FormValue("modelKey"))
				assert.Equal(t, "一只小恐龙", r.FormValue("prompt"))
				assert.Equal(t, "2k", r.FormValue("resolution"))
				assert.Equal(t, "9:16", r.FormValue("aspectRatio"))
				assert.Equal(t, "jpeg", r.FormValue("outputFormat"))
				assert.Len(t, r.MultipartForm.File["image"], fileCount)
				for _, file := range r.MultipartForm.File["image"] {
					opened, err := file.Open()
					if !assert.NoError(t, err) {
						return
					}
					content, err := io.ReadAll(opened)
					assert.NoError(t, err)
					assert.NoError(t, opened.Close())
					assert.Equal(t, file.Filename, string(content))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"rh-task","status":"queued"}`))
			}))
			defer server.Close()
			fields := map[string]any{"model": "seedream5-flash", "prompt": "一只小恐龙", "resolution": "2k", "aspect_ratio": "9:16", "output_format": "jpeg"}
			encoded, err := common.Marshal(fields)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(encoded))
			c.Request.Header.Set("Content-Type", "application/json")
			bodyContext := map[string]any{"kind": "json", "value": fields}
			if fileCount > 0 {
				var input bytes.Buffer
				writer := multipart.NewWriter(&input)
				for key, value := range fields {
					require.NoError(t, writer.WriteField(key, value.(string)))
				}
				files := []any{}
				for i, filename := range []string{"first.png", "second.png"} {
					part, err := writer.CreateFormFile("image[]", filename)
					require.NoError(t, err)
					_, err = io.WriteString(part, filename)
					require.NoError(t, err)
					files = append(files, map[string]any{"field": "image[]", "ref": jsplugin.FileReference("image[]", i), "filename": filename})
				}
				require.NoError(t, writer.Close())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(input.Bytes()))
				c.Request.Header.Set("Content-Type", writer.FormDataContentType())
				form, err := common.ParseMultipartFormReusable(c)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, form.RemoveAll()) })
				bodyContext = map[string]any{"kind": "multipart", "fields": form.Value, "files": files}
			}
			value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{
				"model": "seedream5-flash", "operation": name, "body": bodyContext,
			})
			require.NoError(t, err)
			intent := rhReverseObject(t, value)
			c.Set("task_request", intent["requestBody"])
			info := &relaycommon.RelayInfo{OriginModelName: "seedream5-flash", ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeTaskPlugin, ChannelBaseUrl: server.URL, UpstreamModelName: "seedream5-flash", ApiKey: "test-key",
			}, TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: "generate_image"}}
			adaptor := taskplugin.New(plugin)
			adaptor.Init(info)
			body, err := adaptor.BuildRequestBody(c, info)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(c, info, body)
			require.NoError(t, err)
			defer response.Body.Close()
			assert.Equal(t, http.StatusOK, response.StatusCode)
		})
	}
}

func TestRHReverseValidation(t *testing.T) {
	plugin := rhReversePlugin(t)
	for _, tc := range []struct {
		name    string
		patch   map[string]any
		message string
	}{
		{"批量", map[string]any{"n": 2}, "必须为 1"},
		{"零数量", map[string]any{"n": 0}, "必须为 1"},
		{"浮点数量", map[string]any{"modelParams": map[string]any{"generateNum": 1.5}}, "必须为 1"},
		{"扩展批量", map[string]any{"modelParams": map[string]any{"generateNum": 2}}, "必须为 1"},
		{"时长溢出", map[string]any{"duration": 1e30}, "-1 到 3600"},
		{"嵌套时长", map[string]any{"modelParams": map[string]any{"duration": 3601}}, "-1 到 3600"},
		{"负时长", map[string]any{"duration": -2}, "-1 到 3600"},
		{"样片", map[string]any{"modelParams": map[string]any{"draft": true}}, "样片"},
		{"私有样片标识", map[string]any{"modelParams": map[string]any{"draftTaskId": "private"}}, "样片"},
		{"布尔零", map[string]any{"generateAudio": 0}, "布尔"},
		{"非法扩展", map[string]any{"modelParams": []any{}}, "JSON 对象"},
		{"非法素材", map[string]any{"imageUrl": "file:///tmp/private.png"}, "HTTP(S)"},
		{"mask", map[string]any{"mask": "https://cdn.example/mask.png"}, "mask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"modelKey": "seedance2", "prompt": "a cat"}
			for k, v := range tc.patch {
				body[k] = v
			}
			_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": body}})
			require.ErrorContains(t, err, tc.message)
			_, err = plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{"model": "seedance2", "body": map[string]any{"kind": "json", "value": body}})
			require.ErrorContains(t, err, tc.message)
		})
	}
	for _, duration := range []int{-1, 0, 3600} {
		_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"createTask"}, map[string]any{"body": map[string]any{"kind": "json", "value": map[string]any{"modelKey": "seedance2", "duration": duration}}})
		require.NoError(t, err, "自适应时长不作为计费乘数")
	}
}

func TestRHReverseLifecycleAndUsage(t *testing.T) {
	plugin := rhReversePlugin(t)
	for _, tc := range []struct{ status, url, want string }{
		{"queued", "", "QUEUED"}, {"running", "", "IN_PROGRESS"}, {"failed", "", "FAILURE"}, {"new-state", "", "UNKNOWN"},
		{"success", "", "FAILURE"}, {"success", "https://cdn.example/result.png", "SUCCESS"},
	} {
		t.Run(tc.status+tc.want, func(t *testing.T) {
			body := map[string]any{"id": "private-id", "status": tc.status, "outputUrl": tc.url}
			value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{"taskId": "private-id"}, body)
			require.NoError(t, err)
			parsed := rhReverseObject(t, value)
			assert.Equal(t, tc.want, parsed["status"])
			value, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{"action": "generate_image"}, map[string]any{"body": body})
			require.NoError(t, err)
			submitted := rhReverseObject(t, value)
			assert.Equal(t, "private-id", submitted["taskId"])
			if tc.want == "SUCCESS" || tc.want == "FAILURE" {
				assert.Equal(t, tc.want, submitted["immediate"].(map[string]any)["status"])
			} else {
				assert.Nil(t, submitted["immediate"])
			}
			for _, media := range []string{"image", "video"} {
				ctx := map[string]any{"action": "generate_" + media, "requestBody": map[string]any{"payload": map[string]any{}}}
				value, err = plugin.Engine.Call(t.Context(), "extractUsage", ctx)
				require.NoError(t, err)
				assert.Equal(t, map[string]any{media + "_count": float64(1)}, rhReverseObject(t, value))
				value, err = plugin.Engine.Call(t.Context(), "extractUsageOnComplete", ctx, parsed, body)
				require.NoError(t, err)
				expected := map[string]any{}
				if tc.want == "SUCCESS" {
					expected[media+"_count"] = float64(1)
				}
				assert.Equal(t, expected, rhReverseObject(t, value))
			}
		})
	}
	body := map[string]any{"id": "private-id", "status": "running", "inputPath": "/private/input.png", "outputPath": "/private/result.mp4", "rawRecord": map[string]any{"secret": "never-public"}, "runninghubTaskId": "private-runninghub", "duration": 5, "outputUrl": "https://cdn.example/result.mp4"}
	task := map[string]any{"task_id": "public-id", "status": "FAILURE", "fail_reason": "轮询超时", "data": body}
	value, err := plugin.Engine.CallPath(t.Context(), "native", []string{"taskView"}, map[string]any{}, task)
	require.NoError(t, err)
	presented := rhReverseObject(t, value)
	assert.Equal(t, "public-id", presented["id"])
	assert.Equal(t, "failed", presented["status"])
	assert.Equal(t, "轮询超时", presented["error"])
	for _, key := range []string{"inputPath", "outputPath", "rawRecord", "runninghubTaskId"} {
		assert.Nil(t, presented[key])
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeTaskPlugin, ChannelBaseUrl: "https://rh.example", ApiKey: "must-not-leak"}}
	adaptor := taskplugin.New(plugin)
	adaptor.Init(info)
	encoded, err := common.Marshal(body)
	require.NoError(t, err)
	stored := &model.Task{TaskID: "public-id", Action: "generate_video", Status: model.TaskStatusSuccess, Data: encoded}
	artifacts, err := adaptor.ListArtifacts(stored)
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "video", artifacts[0].Key)
	request, err := adaptor.BuildContentRequest(stored, "video", channel.TaskArtifactClientRequest{Method: http.MethodHead, Headers: map[string]string{"Range": "bytes=0-99"}})
	require.NoError(t, err)
	assert.True(t, request.Credentialless)
	assert.Empty(t, request.Headers)
	assert.Equal(t, "https://cdn.example/result.mp4", request.URL)
	assert.Equal(t, http.MethodHead, request.Method)
}

func TestRHReverseExtendsWithOnlyModelID(t *testing.T) {
	source, err := builtinplugins.Source("rh-reverse")
	require.NoError(t, err)
	source = strings.Replace(source, "const IMAGE_MODELS = [", "const IMAGE_MODELS = [\"future-image\",", 1)
	registry := jsplugin.NewRegistry()
	plugin, err := registry.RegisterFactory(source, jsplugin.Options{Key: "rh-reverse"})
	require.NoError(t, err)
	_, found := registry.Generation().LookupEndpoint(http.MethodPost, "/v1/images/generations", "future-image")
	require.True(t, found)
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_image", "decodeRequest"}, map[string]any{"model": "future-image", "body": map[string]any{"kind": "json", "value": map[string]any{"prompt": "cat"}}})
	require.NoError(t, err)
	intent := rhReverseObject(t, value)
	value, err = plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{"model": "future-image", "action": intent["action"], "requestBody": intent["requestBody"], "baseUrl": "https://rh.example", "apiKey": "key"})
	require.NoError(t, err)
	assert.Contains(t, rhReverseObject(t, value)["parts"], map[string]any{"name": "modelKey", "value": "future-image"})
}
