package middleware

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const asyncMediaOriginChannelContextKey = "async_media_origin_channel"

func AsyncMediaEnqueue() gin.HandlerFunc {
	return func(c *gin.Context) {
		internalJobID := c.GetHeader("X-New-API-Async-Job-ID")
		if service.ValidateAsyncMediaInternalRequest(internalJobID, c.GetHeader("X-New-API-Async-Worker-Signature")) {
			if channelID := strings.TrimSpace(c.GetHeader(service.AsyncMediaInternalChannelHeader)); channelID != "" {
				parsedChannelID, err := strconv.Atoi(channelID)
				if err != nil || parsedChannelID <= 0 {
					abortWithOpenAiMessage(c, http.StatusBadRequest, "无效的异步任务上游渠道")
					return
				}
				common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(parsedChannelID))
				c.Set(asyncMediaOriginChannelContextKey, true)
			}
			c.Next()
			return
		}
		if !constant.AsyncMediaEnabled || c.Request.Method != http.MethodPost || !isAsyncMediaRequest(c) {
			c.Next()
			return
		}

		jobID, err := model.GenerateAsyncMediaJobID()
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "创建异步媒体任务失败")
			return
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			status := http.StatusBadRequest
			if common.IsRequestBodyTooLargeError(err) {
				status = http.StatusRequestEntityTooLarge
			}
			abortWithOpenAiMessage(c, status, err.Error())
			return
		}
		if _, err := storage.Seek(0, io.SeekStart); err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "读取异步媒体请求失败")
			return
		}
		requestFile, requestSize, err := service.SaveAsyncMediaRequest(jobID, storage)
		if err != nil {
			status := http.StatusInternalServerError
			if common.IsRequestBodyTooLargeError(err) {
				status = http.StatusRequestEntityTooLarge
			}
			abortWithOpenAiMessage(c, status, err.Error())
			return
		}

		headers := make(http.Header)
		for name, values := range c.Request.Header {
			if shouldPersistAsyncMediaHeader(name) {
				headers[name] = append([]string(nil), values...)
			}
		}
		headerJSON, err := common.Marshal(headers)
		if err != nil {
			_ = service.DeleteAsyncMediaPath(requestFile)
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "保存异步媒体请求头失败")
			return
		}
		job := &model.AsyncMediaJob{
			JobID:          jobID,
			UserID:         c.GetInt("id"),
			TokenID:        c.GetInt("token_id"),
			Method:         c.Request.Method,
			RequestPath:    c.Request.URL.Path,
			RawQuery:       c.Request.URL.RawQuery,
			RequestHeaders: string(headerJSON),
			RequestFile:    requestFile,
			RequestSize:    requestSize,
		}
		if err := model.CreateAsyncMediaJob(job); err != nil {
			_ = service.DeleteAsyncMediaPath(requestFile)
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "持久化异步媒体任务失败")
			return
		}

		c.AbortWithStatusJSON(http.StatusAccepted, gin.H{
			"id":         job.JobID,
			"object":     "async_media_job",
			"status":     job.Status,
			"created_at": job.CreatedAt,
			"status_url": "/v1/async/tasks/" + job.JobID,
		})
	}
}

func isAsyncMediaRequest(c *gin.Context) bool {
	asyncValue := strings.TrimSpace(strings.ToLower(c.Query("async")))
	if asyncValue != "true" && asyncValue != "1" {
		return false
	}
	path := c.Request.URL.Path
	switch {
	case path == "/v1/images/generations":
		return true
	case path == "/v1/images/edits" || path == "/v1/edits":
		return true
	case path == "/v1/videos" || path == "/v1/video/generations":
		return true
	case strings.HasPrefix(path, "/kling/v1/videos/"):
		return true
	case path == "/jimeng" || path == "/jimeng/":
		return true
	default:
		return false
	}
}

func shouldPersistAsyncMediaHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "cookie", "content-length", "content-encoding", "connection", "host", "x-new-api-async-job-id", "x-new-api-async-worker-signature":
		return false
	default:
		return true
	}
}
