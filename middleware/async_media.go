package middleware

import (
	"bytes"
	"mime"
	"net"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// AsyncMediaEnqueue accepts an explicitly asynchronous media request as a
// durable job and answers 202 with the job id, so the client never holds a long
// generation connection. A request without the explicit flag is untouched and
// keeps the synchronous contract of the same endpoint.
//
// Only JSON bodies enter the queue: the accepted request is stored verbatim and
// replayed later, and multipart uploads are handled by the synchronous endpoint
// they belong to.
func AsyncMediaEnqueue() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isAsyncMediaRequest(c) {
			c.Next()
			return
		}
		if !constant.AsyncMediaEnabled {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "asynchronous media requests are disabled")
			return
		}
		mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
		if err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "asynchronous media requests must use a JSON body")
			return
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error())
			return
		}
		body, err := storage.Bytes()
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error())
			return
		}
		maxBytes := int64(constant.AsyncMediaMaxRequestMB) << 20
		if maxBytes > 0 && int64(len(body)) > maxBytes {
			abortWithOpenAiMessage(c, http.StatusRequestEntityTooLarge, "asynchronous media request body is too large")
			return
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := common.Unmarshal(body, &payload); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "request body must be a JSON object")
			return
		}
		modelName := strings.TrimSpace(payload.Model)
		if modelName == "" {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "field model is required")
			return
		}

		// The background worker replays the accepted request from this node, so a
		// token whose IP restrictions exclude loopback cannot use the queue. It is
		// rejected here instead of failing after a 202 was already returned.
		token, tokenErr := model.GetTokenById(common.GetContextKeyInt(c, constant.ContextKeyTokenId))
		if tokenErr != nil || token == nil {
			abortWithOpenAiMessage(c, http.StatusUnauthorized, "the accepting token is not available")
			return
		}
		if allowIps := token.GetIpLimits(); len(allowIps) > 0 && !common.IsIpInCIDRList(net.ParseIP("127.0.0.1"), allowIps) {
			abortWithOpenAiMessage(c, http.StatusForbidden, "asynchronous media jobs require a token that allows loopback requests")
			return
		}

		jobID, err := model.GenerateAsyncMediaJobID()
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to create the asynchronous media job")
			return
		}
		requestFile, requestSize, err := service.SaveAsyncMediaRequest(jobID, bytes.NewReader(body), maxBytes)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to store the asynchronous media request")
			return
		}
		// The replayed request must not re-enter this middleware, so the flag is
		// dropped from the stored query string.
		query := c.Request.URL.Query()
		query.Del("async")
		job := &model.AsyncMediaJob{
			JobID:       jobID,
			UserId:      common.GetContextKeyInt(c, constant.ContextKeyUserId),
			TokenId:     common.GetContextKeyInt(c, constant.ContextKeyTokenId),
			UserGroup:   common.GetContextKeyString(c, constant.ContextKeyUserGroup),
			ModelName:   modelName,
			RequestPath: c.Request.URL.Path,
			RawQuery:    query.Encode(),
			RequestFile: requestFile,
			RequestSize: requestSize,
			Status:      model.AsyncMediaJobStatusQueued,
		}
		if err := model.CreateAsyncMediaJob(job); err != nil {
			service.DeleteAsyncMediaFile(requestFile)
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "failed to accept the asynchronous media job")
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.AbortWithStatusJSON(http.StatusAccepted, gin.H{
			"id":         job.JobID,
			"object":     "async_media_job",
			"status":     string(job.Status),
			"model":      job.ModelName,
			"created_at": job.CreatedAt,
			"status_url": "/v1/async/tasks/" + job.JobID,
		})
	}
}

// isAsyncMediaRequest reports whether the request explicitly asked for the
// asynchronous image contract. `true` and `1` are accepted, matching the
// documented flag.
func isAsyncMediaRequest(c *gin.Context) bool {
	if c.Request.Method != http.MethodPost {
		return false
	}
	flag := strings.ToLower(strings.TrimSpace(c.Query("async")))
	if flag != "true" && flag != "1" {
		return false
	}
	switch c.Request.URL.Path {
	case "/v1/images/generations", "/v1/images/edits", "/v1/edits":
		return true
	default:
		return false
	}
}
