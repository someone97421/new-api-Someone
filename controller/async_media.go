package controller

import (
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetAsyncMediaJob answers the client query of one accepted asynchronous media
// job. A succeeded job returns the stored response body the synchronous
// endpoint would have produced; the job row itself never exposes credentials or
// stored file paths.
func GetAsyncMediaJob(c *gin.Context) {
	job, err := model.GetAsyncMediaJobForUser(c.Param("job_id"), c.GetInt("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": "asynchronous media job not found",
			"type":    "invalid_request_error",
			"code":    "job_not_found",
		}})
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, asyncMediaJobResponse(job))
}

// ListAsyncMediaJobs lists accepted jobs for administrators.
func ListAsyncMediaJobs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	jobs, total, err := model.ListAsyncMediaJobs(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), c.Query("status"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]gin.H, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, asyncMediaJobSummary(job))
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// asyncMediaJobSummary describes one job without its stored payload.
func asyncMediaJobSummary(job *model.AsyncMediaJob) gin.H {
	return gin.H{
		"id":             job.JobID,
		"object":         "async_media_job",
		"status":         string(job.Status),
		"billing_status": job.BillingStatus,
		"model":          job.ModelName,
		"user_id":        job.UserId,
		"token_id":       job.TokenId,
		"request_path":   job.RequestPath,
		"http_status":    job.HTTPStatus,
		"task_id":        job.OriginTaskID,
		"error":          job.Error,
		"created_at":     job.CreatedAt,
		"started_at":     job.StartedAt,
		"completed_at":   job.CompletedAt,
		"expires_at":     job.ExpiresAt,
		"status_url":     "/v1/async/tasks/" + job.JobID,
	}
}

// asyncMediaJobResponse adds the stored result of a terminal job. The stored body
// is the replayed response, so a JSON answer is delivered inline exactly as the
// synchronous endpoint returned it; a non-JSON answer is reported by content type
// only, because this endpoint never streams media bytes.
func asyncMediaJobResponse(job *model.AsyncMediaJob) gin.H {
	response := asyncMediaJobSummary(job)
	if job.Status != model.AsyncMediaJobStatusSucceeded || job.ResponseFile == "" {
		return response
	}
	payload, err := service.ReadAsyncMediaResponse(job)
	if err != nil {
		response["error"] = "the stored result is no longer available"
		return response
	}
	response["content_type"] = job.ResponseContentType
	if json.Valid(payload) {
		response["data"] = json.RawMessage(payload)
	}
	return response
}
