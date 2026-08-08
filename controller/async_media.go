package controller

import (
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetAsyncMediaJob(c *gin.Context) {
	job, err := model.GetAsyncMediaJobForUser(c.Param("job_id"), c.GetInt("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "async_media_error"}})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "异步媒体任务不存在", "type": "async_media_not_found"}})
		return
	}
	for index := range job.Files {
		file := &job.Files[index]
		file.URL = "/v1/async/files/" + file.FileID +
			"?expires=" + strconv.FormatInt(file.ExpiresAt, 10) +
			"&signature=" + service.AsyncMediaFileSignature(file.FileID, file.ExpiresAt)
	}
	c.JSON(http.StatusOK, gin.H{
		"id":             job.JobID,
		"object":         "async_media_job",
		"status":         job.Status,
		"billing_status": job.BillingStatus,
		"http_status":    job.HTTPStatus,
		"error":          job.Error,
		"created_at":     job.CreatedAt,
		"started_at":     job.StartedAt,
		"completed_at":   job.CompletedAt,
		"expires_at":     job.ExpiresAt,
		"data":           job.Files,
	})
}

func DownloadAsyncMediaFile(c *gin.Context) {
	expiresAt, err := strconv.ParseInt(c.Query("expires"), 10, 64)
	if err != nil || !service.ValidateAsyncMediaFileSignature(c.Param("file_id"), expiresAt, c.Query("signature")) {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "文件链接无效或已过期", "type": "invalid_file_signature"}})
		return
	}
	file, err := model.GetAsyncMediaFile(c.Param("file_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "async_media_error"}})
		return
	}
	if file == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "文件不存在", "type": "async_media_not_found"}})
		return
	}
	if file.DeletedAt != 0 || file.ExpiresAt <= time.Now().Unix() || expiresAt > file.ExpiresAt {
		c.JSON(http.StatusGone, gin.H{"error": gin.H{"message": "文件已过期", "type": "async_media_expired"}})
		return
	}
	absolute, err := service.ResolveAsyncMediaPath(file.Path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "async_media_error"}})
		return
	}
	opened, err := os.Open(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusGone, gin.H{"error": gin.H{"message": "文件已被清理", "type": "async_media_expired"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "async_media_error"}})
		return
	}
	defer opened.Close()
	stat, err := opened.Stat()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error(), "type": "async_media_error"}})
		return
	}
	c.Header("Content-Type", file.MimeType)
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, file.FileID, stat.ModTime(), opened)
}
