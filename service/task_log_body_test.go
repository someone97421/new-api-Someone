package service

import (
	"bytes"
	"mime/multipart"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeTaskLogBodyMasksCredentialsAndBinaryPayloads(t *testing.T) {
	body := []byte(`{
		"prompt":"draw a cat",
		"api_key":"sk-secret-value",
		"headers":{"Authorization":"Bearer hidden-token"},
		"callback":"https://example.test/hook?access_token=hidden&task=1",
		"image":"data:image/png;base64,abcdef",
		"image_base64":"raw-binary-content"
	}`)

	sanitized := SanitizeTaskLogBody(body, "application/json")

	assert.NotContains(t, sanitized, "sk-secret-value")
	assert.NotContains(t, sanitized, "hidden-token")
	assert.NotContains(t, sanitized, "access_token=hidden")
	assert.Contains(t, sanitized, `"prompt":"draw a cat"`)
	assert.Contains(t, sanitized, `"api_key":"***masked***"`)
	assert.Contains(t, sanitized, `"Authorization":"***masked***"`)
	assert.Contains(t, sanitized, "[data URI omitted:")
	assert.Contains(t, sanitized, `"image_base64":"[binary omitted]"`)
}

func TestSanitizeTaskLogBodyKeepsMultipartFieldsWithoutFileContent(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "a short video"))
	require.NoError(t, writer.WriteField("token", "secret-token"))
	file, err := writer.CreateFormFile("image", "reference.png")
	require.NoError(t, err)
	_, err = file.Write([]byte("binary-image-content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	sanitized := SanitizeTaskLogBody(body.Bytes(), writer.FormDataContentType())
	var decoded map[string]any
	require.NoError(t, common.Unmarshal([]byte(sanitized), &decoded))

	assert.Equal(t, "a short video", decoded["prompt"])
	assert.Equal(t, "***masked***", decoded["token"])
	fileInfo, ok := decoded["image"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "reference.png", fileInfo["filename"])
	assert.Equal(t, "[binary omitted]", fileInfo["content"])
	assert.Equal(t, float64(len("binary-image-content")), fileInfo["size"])
}
