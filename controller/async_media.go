package controller

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
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
	if job.Status == model.AsyncMediaJobStatusAwaitingTask {
		reconcileAsyncMediaTask(c, job)
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

// reconcileAsyncMediaTask renders the terminal official task without re-submitting
// the upstream request or changing its billing. A nonterminal task remains pending.
func reconcileAsyncMediaTask(c *gin.Context, job *model.AsyncMediaJob) {
	task, exists, err := model.GetByTaskId(job.UserId, job.OriginTaskID)
	if err != nil || !exists || task == nil {
		return
	}
	if task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure {
		return
	}
	now := common.GetTimestamp()
	expires := model.AsyncMediaJobExpiry(now, constant.AsyncMediaRetentionHours)
	if task.Status == model.TaskStatusFailure {
		if err := model.CompleteAsyncMediaJob(job, model.AsyncMediaJobStatusFailed, model.AsyncMediaBillingNotCharged, http.StatusBadRequest, job.OriginTaskID, "", "", task.FailReason, now, expires); err != nil {
			return
		}
		service.DeleteAsyncMediaFile(job.RequestFile)
		_ = model.ClearAsyncMediaJobRequestFile(job.JobID, now)
		job.Status, job.BillingStatus, job.Error = model.AsyncMediaJobStatusFailed, model.AsyncMediaBillingNotCharged, task.FailReason
		return
	}
	body, err := service.ReadAsyncMediaRequest(job)
	if err != nil {
		return
	}
	var requestBody map[string]any
	if err := common.Unmarshal(body, &requestBody); err != nil {
		return
	}
	plugin, _, ok := resolveTaskPluginForProtocolRetrieve(task.Platform)
	if !ok || plugin == nil {
		return
	}
	view, err := service.BuildTaskPluginView(task)
	if err != nil {
		return
	}
	viewValue, err := taskPluginProtocolJSONValue(view)
	if err != nil {
		return
	}
	operation := "generate"
	if job.RequestPath == "/v1/images/edits" {
		operation = "edit"
	}
	protocolRequest := pluginruntime.ProtocolRequestContext{
		RouteRequestContext: pluginruntime.RouteRequestContext{Path: job.RequestPath, Method: http.MethodPost, Body: map[string]any{"kind": "json", "value": requestBody}},
		Protocol:            pluginruntime.ProtocolOpenAIImage, Operation: operation, Model: job.ModelName,
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), pluginruntime.DefaultCallTimeout)
	defer cancel()
	value, err := plugin.Engine.CallPathWithAdmissionTimeout(ctx, pluginruntime.DefaultCallTimeout, "protocols", []string{pluginruntime.ProtocolOpenAIImage, "render"}, protocolRequest.JSValue(), viewValue)
	if err != nil {
		return
	}
	result, ok := value.(map[string]any)
	if !ok {
		return
	}
	data, ok := result["data"].([]any)
	if !ok {
		return
	}
	if format, _ := requestBody["response_format"].(string); format == "b64_json" {
		for _, entry := range data {
			item, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			url, _ := item["url"].(string)
			encoded, _ := item["b64_json"].(string)
			if url == "" || encoded != "" {
				continue
			}
			_, encoded, err := service.GetImageFromUrl(url)
			if err == nil {
				item["b64_json"] = encoded
			}
		}
	}
	if _, present := result["created"]; !present {
		result["created"] = task.CreatedAt
	}
	payload, err := common.Marshal(result)
	if err != nil || len(payload) > 32<<20 {
		message := "rendered task result exceeds the stored response limit or cannot be encoded"
		if err := model.CompleteAsyncMediaJob(job, model.AsyncMediaJobStatusFailed, model.AsyncMediaBillingReconciliationPending, http.StatusInternalServerError, job.OriginTaskID, "", "", message, now, expires); err != nil {
			return
		}
		service.DeleteAsyncMediaFile(job.RequestFile)
		_ = model.ClearAsyncMediaJobRequestFile(job.JobID, now)
		job.Status, job.BillingStatus, job.HTTPStatus, job.Error = model.AsyncMediaJobStatusFailed, model.AsyncMediaBillingReconciliationPending, http.StatusInternalServerError, message
		return
	}
	responseFile, err := service.SaveAsyncMediaResponse(job.JobID, payload)
	if err != nil {
		return
	}
	if err := model.CompleteAsyncMediaJob(job, model.AsyncMediaJobStatusSucceeded, model.AsyncMediaBillingSettled, http.StatusOK, job.OriginTaskID, responseFile, "application/json", "", now, expires); err != nil {
		return
	}
	service.DeleteAsyncMediaFile(job.RequestFile)
	_ = model.ClearAsyncMediaJobRequestFile(job.JobID, now)
	job.Status, job.BillingStatus, job.HTTPStatus, job.ResponseFile = model.AsyncMediaJobStatusSucceeded, model.AsyncMediaBillingSettled, http.StatusOK, responseFile
	job.Error = ""
}
