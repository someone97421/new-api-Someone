package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// RelayImageResponse transforms only the explicit OpenAI image payload fields.
// RawMessage preserves vendor metadata, large numeric values and usage exactly.
func RelayImageResponse(ctx context.Context, cfg system_setting.FileRelaySettings, body []byte) ([]byte, error) {
	if !cfg.Enabled {
		return body, nil
	}
	var response map[string]json.RawMessage
	if common.Unmarshal(body, &response) != nil {
		return body, nil
	}
	data := bytes.TrimSpace(response["data"])
	if len(data) == 0 {
		return body, nil
	}
	object := data[0] == '{'
	var items []map[string]json.RawMessage
	if object {
		var item map[string]json.RawMessage
		if common.Unmarshal(data, &item) != nil {
			return body, nil
		}
		items = append(items, item)
	} else if common.Unmarshal(data, &items) != nil {
		return body, nil
	}
	changed := false
	for _, item := range items {
		var source FileRelaySource
		_ = common.Unmarshal(item["url"], &source.URL)
		_ = common.Unmarshal(item["b64_json"], &source.Base64)
		if source.URL == "" && source.Base64 == "" {
			continue
		}
		if strings.HasPrefix(source.URL, cfg.PublicURL+FileRelayContentPath) {
			continue
		}
		ref, err := RelayFile(ctx, cfg, source)
		if err != nil {
			if cfg.Strict {
				return nil, err
			}
			common.SysError("image file relay failed; preserving upstream result")
			continue
		}
		item["url"], _ = common.Marshal(ref.URL)
		// Keep explicitly requested Base64 output compatible while adding a local URL.
		changed = true
	}
	if !changed {
		return body, nil
	}
	var err error
	if object {
		response["data"], err = common.Marshal(items[0])
	} else {
		response["data"], err = common.Marshal(items)
	}
	if err != nil {
		return nil, err
	}
	return common.Marshal(response)
}

// ImageResponseCapture defers ordinary image writes until the adaptor has
// parsed the original usage. Streaming requests keep their existing writer.
type ImageResponseCapture struct {
	gin.ResponseWriter
	body    bytes.Buffer
	status  int
	written bool
}

func CaptureImageResponse(writer gin.ResponseWriter) *ImageResponseCapture {
	return &ImageResponseCapture{ResponseWriter: writer, status: http.StatusOK}
}
func (w *ImageResponseCapture) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}
func (w *ImageResponseCapture) WriteHeaderNow()                   { w.written = true }
func (w *ImageResponseCapture) Write(p []byte) (int, error)       { w.written = true; return w.body.Write(p) }
func (w *ImageResponseCapture) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *ImageResponseCapture) Status() int                       { return w.status }
func (w *ImageResponseCapture) Size() int {
	if !w.written {
		return -1
	}
	return w.body.Len()
}
func (w *ImageResponseCapture) Written() bool { return w.written }
func (w *ImageResponseCapture) Flush()        { w.WriteHeaderNow() }
func (w *ImageResponseCapture) Send(c *gin.Context, cfg system_setting.FileRelaySettings) {
	body := w.body.Bytes()
	var err error
	if w.status >= 200 && w.status < 300 {
		body, err = RelayImageResponse(c.Request.Context(), cfg, body)
	}
	// Upstream generation succeeded and must settle once even in strict mode.
	// Writing the delivery error here keeps it outside the channel retry loop.
	w.Header().Del("Content-Length")
	w.Header().Del("ETag")
	if err != nil {
		w.Header().Del("Content-Encoding")
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"code": "file_relay_failed", "message": "File relay failed after generation completed"}})
		return
	}
	w.ResponseWriter.WriteHeader(w.status)
	_, _ = w.ResponseWriter.Write(body)
}
