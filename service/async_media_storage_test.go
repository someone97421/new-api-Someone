package service

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAsyncMediaStoragePath(t *testing.T) {
	_, err := ValidateAsyncMediaStoragePath(" ")
	require.Error(t, err)

	resolved, err := ValidateAsyncMediaStoragePath("./data/async-media")
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(resolved))
}

func TestStoreAsyncMediaResultsRecognizesWhaleResultURLArrays(t *testing.T) {
	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	originalHTTPClient := httpClient
	originalProtectedHTTPClient := ssrfProtectedHTTPClient
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		httpClient = originalHTTPClient
		ssrfProtectedHTTPClient = originalProtectedHTTPClient
	})

	imageBytes := []byte("whale-image")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(imageBytes)
	}))
	t.Cleanup(server.Close)
	httpClient = server.Client()

	job := &model.AsyncMediaJob{JobID: "job_whale_result_urls", UserID: 7}
	responsePath, responseFile, err := CreateAsyncMediaResponseFile(job.JobID)
	require.NoError(t, err)
	payload, err := common.Marshal(map[string]any{
		"code": "success",
		"data": map[string]any{
			"status":            "success",
			"result_urls":       []string{server.URL + "/result.png"},
			"result_asset_urls": []string{server.URL + "/result.png"},
		},
	})
	require.NoError(t, err)
	_, err = responseFile.Write(payload)
	require.NoError(t, err)
	require.NoError(t, responseFile.Close())
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(responsePath) })

	files, err := StoreAsyncMediaResults(job, responsePath, "application/json")
	require.NoError(t, err)
	require.Len(t, files, 1)
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(files[0].Path) })

	absolute, err := ResolveAsyncMediaPath(files[0].Path)
	require.NoError(t, err)
	stored, err := os.ReadFile(absolute)
	require.NoError(t, err)
	assert.Equal(t, imageBytes, stored)
}

func TestStoreAsyncMediaResultsDecodesBase64WithoutDatabaseBlob(t *testing.T) {
	job := &model.AsyncMediaJob{JobID: "job_base64_result", UserID: 7}
	responsePath, responseFile, err := CreateAsyncMediaResponseFile(job.JobID)
	require.NoError(t, err)
	pngBytes := []byte("meaningful-image-bytes")
	payload, err := common.Marshal(map[string]any{
		"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(pngBytes)}},
	})
	require.NoError(t, err)
	_, err = responseFile.Write(payload)
	require.NoError(t, err)
	require.NoError(t, responseFile.Close())
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(responsePath) })

	files, err := StoreAsyncMediaResults(job, responsePath, "application/json")
	require.NoError(t, err)
	require.Len(t, files, 1)
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(files[0].Path) })

	absolute, err := ResolveAsyncMediaPath(files[0].Path)
	require.NoError(t, err)
	stored, err := os.ReadFile(absolute)
	require.NoError(t, err)
	assert.Equal(t, pngBytes, stored)
	assert.Equal(t, int64(len(pngBytes)), files[0].Size)
	assert.Greater(t, files[0].ExpiresAt, time.Now().Unix())
}

func TestAsyncMediaFileSignatureRejectsTamperingAndExpiry(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).Unix()
	signature := AsyncMediaFileSignature("file_contract", expiresAt)
	assert.True(t, ValidateAsyncMediaFileSignature("file_contract", expiresAt, signature))
	assert.False(t, ValidateAsyncMediaFileSignature("file_other", expiresAt, signature))
	assert.False(t, ValidateAsyncMediaFileSignature("file_contract", time.Now().Add(-time.Second).Unix(), signature))
}

func TestAsyncMediaInternalSignatureCannotBeForgedWithJobIDAlone(t *testing.T) {
	signature := AsyncMediaInternalSignature("job_internal_contract")
	assert.True(t, ValidateAsyncMediaInternalRequest("job_internal_contract", signature))
	assert.False(t, ValidateAsyncMediaInternalRequest("job_other", signature))
	assert.False(t, ValidateAsyncMediaInternalRequest("job_internal_contract", "job_internal_contract"))
}

func TestStoreAsyncMediaResultsExtractsGeminiInlineData(t *testing.T) {
	truncate(t)
	job := &model.AsyncMediaJob{JobID: "job_gemini_inline_data", UserID: 1}
	responseFile, file, err := CreateAsyncMediaResponseFile(job.JobID)
	require.NoError(t, err)
	payload := `{"candidates":[{"content":{"parts":[{"text":"done"},{"inlineData":{"mimeType":"image/png","data":"` + base64.StdEncoding.EncodeToString([]byte("png-data")) + `"}}]}}]}`
	_, err = file.WriteString(payload)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	files, err := StoreAsyncMediaResults(job, responseFile, "application/json")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "image/png", files[0].MimeType)
	assert.Equal(t, int64(len("png-data")), files[0].Size)
	DeleteAsyncMediaFiles(files)
}

func TestExtractAsyncMediaMarkdownImageURLs(t *testing.T) {
	assert.Equal(t, []string{"https://example.com/a.jpg", "http://example.com/b.png"}, extractAsyncMediaMarkdownImageURLs("![a](https://example.com/a.jpg) text ![b](http://example.com/b.png)"))
	assert.Empty(t, extractAsyncMediaMarkdownImageURLs("[ordinary link](https://example.com/a.jpg)"))
}
