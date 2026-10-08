package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupFileRelayTest(t *testing.T) (system_setting.FileRelaySettings, *gin.Engine) {
	t.Helper()
	cfg := system_setting.DefaultFileRelaySettings()
	cfg.Enabled, cfg.Strict = true, true
	cfg.Directory, cfg.PublicURL = t.TempDir(), "https://relay.example"
	cfg.RetryCount = 0
	raw, err := common.Marshal(cfg)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	wasNil := common.OptionMap == nil
	if wasNil {
		common.OptionMap = make(map[string]string)
	}
	old, existed := common.OptionMap[system_setting.FileRelaySettingsKey]
	common.OptionMap[system_setting.FileRelaySettingsKey] = string(raw)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if existed {
			common.OptionMap[system_setting.FileRelaySettingsKey] = old
		} else {
			delete(common.OptionMap, system_setting.FileRelaySettingsKey)
		}
		if wasNil && len(common.OptionMap) == 0 {
			common.OptionMap = nil
		}
	})
	router := gin.New()
	router.POST("/v1/file-relay", CreateFileRelay)
	router.GET(service.FileRelayContentPath+":id", FileRelayContent)
	router.HEAD(service.FileRelayContentPath+":id", FileRelayContent)
	return cfg, router
}

func TestFileRelayUploadAndContent(t *testing.T) {
	_, router := setupFileRelayTest(t)
	fetch := system_setting.GetFetchSetting()
	originalFetch := *fetch
	fetch.EnableSSRFProtection, fetch.AllowPrivateIp = true, true
	fetch.DomainFilterMode, fetch.IpFilterMode = false, false
	fetch.AllowedPorts = []string{"1-65535"}
	t.Cleanup(func() { *fetch = originalFetch })
	service.InitHttpClient()
	payload := []byte("file relay regression content\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	for _, source := range []string{"url", "base64", "data-url", "raw-data-url", "multipart"} {
		t.Run(source, func(t *testing.T) {
			var body bytes.Buffer
			contentType := "application/json"
			if source == "multipart" {
				writer := multipart.NewWriter(&body)
				part, err := writer.CreateFormFile("file", "sample.txt")
				require.NoError(t, err)
				_, err = part.Write(payload)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				contentType = writer.FormDataContentType()
			} else {
				input := map[string]string{"name": "sample.txt", "content_type": "text/plain"}
				switch source {
				case "url":
					input["url"] = upstream.URL + "/sample.txt"
				case "base64":
					input["base64"] = base64.StdEncoding.EncodeToString(payload)
				case "data-url":
					input["url"] = "data:text/plain;base64," + base64.StdEncoding.EncodeToString(payload)
				case "raw-data-url":
					input["url"] = "data:text/plain;base64," + base64.RawStdEncoding.EncodeToString(payload)
				}
				raw, err := common.Marshal(input)
				require.NoError(t, err)
				body.Write(raw)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/file-relay", &body)
			req.Header.Set("Content-Type", contentType)
			created := httptest.NewRecorder()
			router.ServeHTTP(created, req)
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
			var ref service.RelayedFile
			require.NoError(t, common.Unmarshal(created.Body.Bytes(), &ref))
			assert.Equal(t, int64(len(payload)), ref.Size)
			assert.Equal(t, "sample.txt", ref.Name)
			assert.Equal(t, "https://relay.example"+service.FileRelayContentPath+ref.ID, ref.URL)
			assert.Greater(t, ref.ExpiresAt, time.Now().Unix())
			for _, tc := range []struct {
				name, method, byteRange, wantBody string
				status                            int
			}{
				{"get", http.MethodGet, "", string(payload), http.StatusOK},
				{"head", http.MethodHead, "", "", http.StatusOK},
				{"range", http.MethodGet, "bytes=5-9", string(payload[5:10]), http.StatusPartialContent},
			} {
				t.Run(tc.name, func(t *testing.T) {
					req := httptest.NewRequest(tc.method, service.FileRelayContentPath+ref.ID, nil)
					if tc.byteRange != "" {
						req.Header.Set("Range", tc.byteRange)
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(response, req)
					assert.Equal(t, tc.status, response.Code)
					assert.Equal(t, tc.wantBody, response.Body.String())
					assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
					assert.Contains(t, response.Header().Get("Content-Disposition"), "sample.txt")
					if tc.byteRange != "" {
						assert.Equal(t, "bytes 5-9/"+strconv.Itoa(len(payload)), response.Header().Get("Content-Range"))
					}
				})
			}
		})
	}
}

func TestFileRelayExpiryAndInvalidBase64(t *testing.T) {
	cfg, router := setupFileRelayTest(t)
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "permanent"}[permanent], func(t *testing.T) {
			local := cfg
			if permanent {
				local.RetentionHours = 0
			}
			ref, err := service.RelayFile(context.Background(), local, service.FileRelaySource{Base64: "aGVsbG8="})
			require.NoError(t, err)
			metadata := filepath.Join(cfg.Directory, ref.ID+".json")
			body := filepath.Join(cfg.Directory, ref.ID+".bin")
			if permanent {
				assert.Zero(t, ref.ExpiresAt)
			} else {
				ref.ExpiresAt = time.Now().Add(-time.Second).Unix()
				raw, err := common.Marshal(ref)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(metadata, raw, 0600))
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, service.FileRelayContentPath+ref.ID, nil))
			if permanent {
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Equal(t, "hello", response.Body.String())
			} else {
				assert.Equal(t, http.StatusNotFound, response.Code)
			}
			require.FileExists(t, body)
			require.NoError(t, service.CleanupRelayedFiles(cfg, time.Now().Add(10*365*24*time.Hour)))
			if permanent {
				assert.FileExists(t, body)
				assert.FileExists(t, metadata)
			} else {
				assert.NoFileExists(t, body)
				assert.NoFileExists(t, metadata)
			}
		})
	}
	for _, encoded := range []string{"%%%", strings.Repeat("YWFh", 200) + "%", "data:text/plain,hello"} {
		t.Run("invalid-base64-"+strconv.Itoa(len(encoded)), func(t *testing.T) {
			local := cfg
			local.Directory = t.TempDir()
			ref, err := service.RelayFile(context.Background(), local, service.FileRelaySource{Base64: encoded})
			require.Error(t, err)
			assert.Nil(t, ref)
			entries, err := os.ReadDir(local.Directory)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
}

func TestFileRelayImageResponse(t *testing.T) {
	cfg, _ := setupFileRelayTest(t)
	for _, shape := range []string{"array", "object"} {
		t.Run(shape, func(t *testing.T) {
			item := `{"b64_json":"aGVsbG8=","revised_prompt":"preserved","vendor":{"id":9007199254740993}}`
			data := item
			if shape == "array" {
				data = "[" + item + `,{"revised_prompt":"no image"}]`
			}
			body := []byte(`{"created":9007199254740993,"usage":{"total_tokens":9007199254740995,"nested":{"images":1}},"unknown":{"id":18446744073709551615},"data":` + data + `}`)
			result, err := service.RelayImageResponse(context.Background(), body)
			require.NoError(t, err)
			var before, after map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(body, &before))
			require.NoError(t, common.Unmarshal(result, &after))
			for _, key := range []string{"created", "usage", "unknown"} {
				assert.JSONEq(t, string(before[key]), string(after[key]))
				assert.Equal(t, string(before[key]), string(after[key]))
			}
			var images []map[string]json.RawMessage
			if shape == "array" {
				require.NoError(t, common.Unmarshal(after["data"], &images))
				require.Len(t, images, 2)
				assert.JSONEq(t, `{"revised_prompt":"no image"}`, string(mustFileRelayJSON(t, images[1])))
			} else {
				var image map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(after["data"], &image))
				images = append(images, image)
			}
			assert.Equal(t, `"aGVsbG8="`, string(images[0]["b64_json"]))
			assert.Equal(t, `"preserved"`, string(images[0]["revised_prompt"]))
			assert.Equal(t, `{"id":9007199254740993}`, string(images[0]["vendor"]))
			var localURL string
			require.NoError(t, common.Unmarshal(images[0]["url"], &localURL))
			ref, file, err := service.OpenRelayedFile(cfg, strings.TrimPrefix(localURL, cfg.PublicURL+service.FileRelayContentPath))
			require.NoError(t, err)
			defer file.Close()
			stored, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, "hello", string(stored))
			assert.Equal(t, localURL, ref.URL)
		})
	}
	for _, tc := range []struct {
		name                     string
		enabled, strict, wantErr bool
	}{
		{"fallback", true, false, false},
		{"strict", true, true, true},
		{"disabled", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			local := cfg
			local.Enabled, local.Strict = tc.enabled, tc.strict
			raw := mustFileRelayJSON(t, local)
			common.OptionMapRWMutex.Lock()
			common.OptionMap[system_setting.FileRelaySettingsKey] = string(raw)
			common.OptionMapRWMutex.Unlock()
			body := []byte(`{"usage":{"total_tokens":12},"data":[{"b64_json":"%%%"}]}`)
			result, err := service.RelayImageResponse(context.Background(), body)
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, body, result)
			}
		})
	}
}

func mustFileRelayJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := common.Marshal(value)
	require.NoError(t, err)
	return raw
}

func TestFileRelayPersistentCache(t *testing.T) {
	cfg, _ := setupFileRelayTest(t)
	var downloads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		_, _ = w.Write([]byte("cached content"))
	}))
	defer upstream.Close()
	key := "task:provider:artifact"
	_, err := service.FindCachedRelayFile(cfg, key)
	require.Error(t, err)
	ref, err := service.RelayCachedFile(context.Background(), cfg, key, service.FileRelaySource{URL: upstream.URL + "/file", Client: upstream.Client()})
	require.NoError(t, err)
	assert.Equal(t, int32(1), downloads.Load())
	// 从磁盘重新载入配置和索引，复用时即使源已不可用也必须命中。
	reloaded, err := system_setting.ParseFileRelaySettings(string(mustFileRelayJSON(t, cfg)))
	require.NoError(t, err)
	cached, err := service.FindCachedRelayFile(reloaded, key)
	require.NoError(t, err)
	assert.Equal(t, ref, cached)
	upstream.Close()
	again, err := service.RelayCachedFile(context.Background(), reloaded, key, service.FileRelaySource{URL: upstream.URL, Client: upstream.Client()})
	require.NoError(t, err)
	assert.Equal(t, ref, again)
	assert.Equal(t, int32(1), downloads.Load())
	_, file, err := service.OpenRelayedFile(reloaded, again.ID)
	require.NoError(t, err)
	defer file.Close()
	stored, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, "cached content", string(stored))
	require.NoError(t, file.Close())
	require.NoError(t, service.CleanupRelayedFiles(cfg, time.Unix(ref.ExpiresAt, 0)))
	_, err = service.FindCachedRelayFile(cfg, key)
	assert.Error(t, err)
	entries, err := os.ReadDir(cfg.Directory)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestFileRelaySettingsBoundaries(t *testing.T) {
	defaults, err := system_setting.ParseFileRelaySettings("")
	require.NoError(t, err)
	assert.Equal(t, system_setting.DefaultFileRelaySettings(), defaults)
	for _, tc := range []struct {
		field    string
		min, max int
	}{
		{"retention_hours", 0, 87600},
		{"cleanup_interval_minutes", 1, 1440},
		{"download_timeout_seconds", 1, 3600},
		{"retry_count", 0, 5},
	} {
		for _, value := range []int{tc.min - 1, tc.min, tc.max, tc.max + 1} {
			t.Run(tc.field+"/"+string(mustFileRelayJSON(t, value)), func(t *testing.T) {
				_, err := system_setting.ParseFileRelaySettings(string(mustFileRelayJSON(t, map[string]any{tc.field: value})))
				if value < tc.min || value > tc.max {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	}
	for _, raw := range []string{
		`{`, `{"directory":"  "}`, `{"public_url":"ftp://example.com"}`,
		`{"public_url":"https://user:password@example.com"}`, `{"public_url":"https://example.com?q=1"}`,
		`{"public_url":"https://example.com#fragment"}`, `{"public_url":"https:///missing-host"}`,
	} {
		_, err := system_setting.ParseFileRelaySettings(raw)
		assert.Error(t, err, raw)
	}
	for _, publicURL := range []string{"", "http://example.com", "https://example.com/base/"} {
		cfg, err := system_setting.ParseFileRelaySettings(string(mustFileRelayJSON(t, map[string]string{"directory": " ./files ", "public_url": " " + publicURL + " "})))
		require.NoError(t, err)
		assert.Equal(t, "./files", cfg.Directory)
		assert.Equal(t, strings.TrimRight(publicURL, "/"), cfg.PublicURL)
	}
}

func TestFileRelayTaskMediaRangeCache(t *testing.T) {
	setupFileRelayTest(t)
	task := setupGenericTaskTest(t)
	allowPrivateTaskMediaTest(t)
	var downloads atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		assert.Empty(t, r.Header.Get("Range"))
		assert.Empty(t, r.Header.Get("If-Range"))
		assert.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer upstream.Close()
	for _, tc := range []struct {
		name, byteRange, body, contentRange string
	}{
		{"first-range", "bytes=0-3", "0123", "bytes 0-3/10"},
		{"cached-range", "bytes=6-9", "6789", "bytes 6-9/10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Params = gin.Params{{Key: "artifact_key", Value: "video-main"}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/task_generic/artifacts/video-main/content", nil)
			c.Request.Header.Set("Range", tc.byteRange)
			var descriptor *relaychannel.TaskContentRequest
			if tc.name == "first-range" {
				descriptor = &relaychannel.TaskContentRequest{URL: upstream.URL, Method: http.MethodGet}
			} else {
				upstream.Close()
				// 即使上游描述符不可用，本地缓存仍能服务另一段内容。
			}
			require.NoError(t, proxyTaskMedia(c, task, "video", descriptor))
			assert.Equal(t, http.StatusPartialContent, recorder.Code)
			assert.Equal(t, tc.body, recorder.Body.String())
			assert.Equal(t, tc.contentRange, recorder.Header().Get("Content-Range"))
			assert.Equal(t, int32(1), downloads.Load())
		})
	}
}

func TestFileRelayImageCaptureStrictFailure(t *testing.T) {
	setupFileRelayTest(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	original := c.Writer
	captured := service.CaptureImageResponse(original)
	c.Writer = captured
	c.Header("Content-Length", "999")
	c.Header("ETag", "upstream-etag")
	c.Header("Content-Encoding", "gzip")
	c.JSON(http.StatusOK, gin.H{"usage": gin.H{"total_tokens": 7}, "data": []gin.H{{"b64_json": "%%%"}}})
	assert.Empty(t, recorder.Body.String())
	assert.False(t, original.Written())
	assert.True(t, captured.Written())
	// 与图片 handler 一样，在交付捕获内容前恢复原 writer。
	c.Writer = original
	captured.Send(c)
	assert.Same(t, original, c.Writer)
	assert.Equal(t, http.StatusBadGateway, recorder.Code)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "file_relay_failed", response.Error.Code)
	assert.NotContains(t, recorder.Body.String(), "b64_json")
	assert.Empty(t, recorder.Header().Get("Content-Length"))
	assert.Empty(t, recorder.Header().Get("ETag"))
	assert.Empty(t, recorder.Header().Get("Content-Encoding"))
}

func TestFileRelayArtifactCacheIdentity(t *testing.T) {
	setupFileRelayTest(t)
	task := &model.Task{UserId: 7, TaskID: "two-video-artifacts"}
	for _, key := range []string{"video-main", "video", "video-main", "video"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/content", nil)
		descriptor := &relaychannel.TaskContentRequest{URL: "data:video/mp4;base64," + base64.RawStdEncoding.EncodeToString([]byte(key))}
		require.NoError(t, proxyTaskMedia(c, task, key, descriptor))
		assert.Equal(t, key, recorder.Body.String())
	}
}
