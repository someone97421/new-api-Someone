package controller

import (
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

// CreateFileRelay accepts exactly one URL, Base64 payload or multipart file.
func CreateFileRelay(c *gin.Context) {
	cfg := system_setting.GetFileRelaySettings()
	if !cfg.Enabled {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "File relay is disabled"})
		return
	}
	var source service.FileRelaySource
	contentType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if contentType == "multipart/form-data" {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid multipart file"})
			return
		}
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A file field is required"})
			return
		}
		defer file.Close()
		source = service.FileRelaySource{Reader: file, Name: header.Filename, ContentType: header.Header.Get("Content-Type")}
	} else {
		var input struct {
			URL         string `json:"url"`
			Base64      string `json:"base64"`
			Name        string `json:"name"`
			ContentType string `json:"content_type"`
		}
		if err := common.DecodeJson(c.Request.Body, &input); err != nil || (input.URL == "") == (input.Base64 == "") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Provide exactly one URL or Base64 payload"})
			return
		}
		source = service.FileRelaySource{URL: input.URL, Base64: input.Base64, Name: input.Name, ContentType: input.ContentType}
	}
	ref, err := service.RelayFile(c.Request.Context(), cfg, source)
	if err != nil {
		if !cfg.Strict && (strings.HasPrefix(source.URL, "https://") || strings.HasPrefix(source.URL, "http://")) {
			c.JSON(http.StatusOK, gin.H{"url": source.URL, "relayed": false})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "File relay failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": ref.ID, "url": ref.URL, "name": ref.Name, "content_type": ref.ContentType, "size": ref.Size, "expires_at": ref.ExpiresAt, "relayed": true})
}

func FileRelayContent(c *gin.Context) {
	if err := serveRelayedFile(c, c.Param("id")); err != nil {
		c.Status(http.StatusNotFound)
	}
}

func serveRelayedFile(c *gin.Context, id string) error {
	cfg := system_setting.GetFileRelaySettings()
	ref, file, err := service.OpenRelayedFile(cfg, id)
	if err != nil {
		return err
	}
	defer file.Close()
	c.Header("Content-Type", ref.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "private, no-store")
	disposition := "attachment"
	if strings.HasPrefix(ref.ContentType, "image/") && ref.ContentType != "image/svg+xml" || strings.HasPrefix(ref.ContentType, "video/") || strings.HasPrefix(ref.ContentType, "audio/") {
		disposition = "inline"
	}
	c.Header("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": ref.Name}))
	http.ServeContent(c.Writer, c.Request, ref.Name, time.Time{}, file)
	return nil
}

func relayTaskFile(c *gin.Context, cfg system_setting.FileRelaySettings, key string, source service.FileRelaySource) (bool, error) {
	ref, err := service.RelayCachedFile(c.Request.Context(), cfg, key, source)
	if err == nil {
		err = serveRelayedFile(c, ref.ID)
		if err == nil {
			return true, nil
		}
	}
	if cfg.Strict {
		return false, &taskMediaProxyError{status: http.StatusBadGateway, code: "file_relay_failed", message: "Failed to store task artifact"}
	}
	common.SysError("task file relay failed; falling back to upstream content")
	return false, nil
}
