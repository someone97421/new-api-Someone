package common

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyTextReplacementJSON(t *testing.T) {
	policy := &dto.BodyTextReplacement{Enabled: true, Rules: []dto.BodyTextReplacementRule{
		{From: "敏感", To: "替代"}, {From: "替代词", To: ""}, {From: "old", To: "new\"\n"},
	}}
	body := `{"敏感":"敏感词","z":["old",{"u":"https://old/path","v":"\u654f\u611f"}],"n":1e+09,"zero":0,"b":false}`
	got, err := ReplaceJSONBodyText([]byte(body), policy)
	require.NoError(t, err)
	assert.Equal(t, `{"敏感":"","z":["new\"\n",{"u":"https://new\"\n/path","v":"替代"}],"n":1e+09,"zero":0,"b":false}`, string(got))
	policy.Enabled = false
	got, err = ReplaceJSONBodyText([]byte(body), policy)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	policy.Enabled = true
	_, err = ReplaceJSONBodyText([]byte(`{"broken":`), policy)
	require.Error(t, err)
	policy.Rules = []dto.BodyTextReplacementRule{{From: ""}}
	require.Error(t, policy.Validate())
}

func TestBodyTextReplacementAttempts(t *testing.T) {
	for _, multipartBody := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "multipart"}[multipartBody], func(t *testing.T) {
			body := []byte(`{"model":"old-model","prompt":"old"}`)
			contentType := "application/json"
			if multipartBody {
				var buffer bytes.Buffer
				writer := multipart.NewWriter(&buffer)
				require.NoError(t, writer.WriteField("old-key", "old"))
				file, err := writer.CreateFormFile("image", "old.png")
				require.NoError(t, err)
				_, err = io.Copy(file, strings.NewReader("old\x00file bytes"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				body, contentType = buffer.Bytes(), writer.FormDataContentType()
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", contentType)
			original, err := GetBodyStorage(c)
			require.NoError(t, err)
			defer original.Close()
			for _, target := range []string{"first", "second"} {
				policy := &dto.BodyTextReplacement{Enabled: true, Rules: []dto.BodyTextReplacementRule{{From: "old", To: target}}}
				restore, err := ApplyChannelBodyTextReplacement(c, policy)
				require.NoError(t, err)
				storage, err := GetBodyStorage(c)
				require.NoError(t, err)
				got, err := storage.Bytes()
				require.NoError(t, err)
				assert.Equal(t, int64(len(got)), c.Request.ContentLength)
				if multipartBody {
					_, params, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
					require.NoError(t, err)
					reader := multipart.NewReader(bytes.NewReader(got), params["boundary"])
					field, err := reader.NextPart()
					require.NoError(t, err)
					assert.Equal(t, "old-key", field.FormName())
					text, err := io.ReadAll(field)
					require.NoError(t, err)
					assert.Equal(t, target, string(text))
					file, err := reader.NextPart()
					require.NoError(t, err)
					assert.Equal(t, "old.png", file.FileName())
					payload, err := io.ReadAll(file)
					require.NoError(t, err)
					assert.Equal(t, "old\x00file bytes", string(payload))
				} else {
					assert.JSONEq(t, `{"model":"`+target+`-model","prompt":"`+target+`"}`, string(got))
				}
				restore()
				assert.Equal(t, contentType, c.GetHeader("Content-Type"))
				restored, err := GetBodyStorage(c)
				require.NoError(t, err)
				originalBytes, err := restored.Bytes()
				require.NoError(t, err)
				assert.Equal(t, body, originalBytes)
			}
		})
	}
}
