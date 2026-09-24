package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// AsyncMediaJobStatus is the lifecycle of one explicit asynchronous media job.
// A job wraps exactly one replayed client request: the worker replays the
// original request through the gateway, and the official task/billing chain
// inside that replay owns generation, settlement and refunds. The job row only
// records acceptance, the stored response and the delivery state.
type AsyncMediaJobStatus string

const (
	AsyncMediaJobStatusQueued    AsyncMediaJobStatus = "queued"
	AsyncMediaJobStatusRunning   AsyncMediaJobStatus = "running"
	AsyncMediaJobStatusSucceeded AsyncMediaJobStatus = "succeeded"
	AsyncMediaJobStatusFailed    AsyncMediaJobStatus = "failed"
)

// Billing states recorded on the job. `not_charged` means the replayed request
// never reached the upstream call, so the official chain refunded it.
// `reconciliation_pending` means the worker was interrupted after the upstream
// call may have started: the job is never replayed automatically and an
// administrator must reconcile it, exactly like an interrupted synchronous
// submission.
const (
	AsyncMediaBillingNotCharged            = "not_charged"
	AsyncMediaBillingSettled               = "settled"
	AsyncMediaBillingReconciliationPending = "reconciliation_pending"
)

// AsyncMediaJob is the durable acceptance record of an explicit asynchronous
// media request (`POST /v1/images/generations?async=true`). It reuses the
// official Task row created by the replay for generation, polling, billing and
// artifacts; OriginTaskID links the two when the replay reported one.
type AsyncMediaJob struct {
	ID                  int64               `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	JobID               string              `json:"job_id" gorm:"type:varchar(64);uniqueIndex"`
	UserId              int                 `json:"user_id" gorm:"index"`
	TokenId             int                 `json:"token_id" gorm:"index"`
	UserGroup           string              `json:"user_group" gorm:"type:varchar(50)"`
	ModelName           string              `json:"model_name" gorm:"type:varchar(191);index"`
	RequestPath         string              `json:"request_path" gorm:"type:varchar(191)"`
	RawQuery            string              `json:"raw_query" gorm:"type:text"`
	RequestFile         string              `json:"-" gorm:"type:text"`
	RequestSize         int64               `json:"request_size"`
	ResponseFile        string              `json:"-" gorm:"type:text"`
	ResponseContentType string              `json:"response_content_type" gorm:"type:varchar(128)"`
	Status              AsyncMediaJobStatus `json:"status" gorm:"type:varchar(20);index"`
	BillingStatus       string              `json:"billing_status" gorm:"type:varchar(32);index"`
	OriginTaskID        string              `json:"origin_task_id" gorm:"type:varchar(191);index"`
	HTTPStatus          int                 `json:"http_status"`
	Error               string              `json:"error" gorm:"type:text"`
	CreatedAt           int64               `json:"created_at" gorm:"index"`
	StartedAt           int64               `json:"started_at"`
	CompletedAt         int64               `json:"completed_at" gorm:"index"`
	UpdatedAt           int64               `json:"updated_at" gorm:"index"`
	ExpiresAt           int64               `json:"expires_at" gorm:"index"`
}

// GenerateAsyncMediaJobID returns the client-facing job identifier.
func GenerateAsyncMediaJobID() (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return "job_" + key, nil
}

// CreateAsyncMediaJob persists one accepted job.
func CreateAsyncMediaJob(job *AsyncMediaJob) error {
	if job == nil || job.JobID == "" {
		return errors.New("async media job requires an id")
	}
	if job.CreatedAt == 0 {
		job.CreatedAt = common.GetTimestamp()
	}
	job.UpdatedAt = job.CreatedAt
	if job.Status == "" {
		job.Status = AsyncMediaJobStatusQueued
	}
	return DB.Create(job).Error
}

// GetAsyncMediaJob returns one job by its public id.
func GetAsyncMediaJob(jobID string) (*AsyncMediaJob, error) {
	if jobID == "" {
		return nil, errors.New("async media job id is required")
	}
	var job AsyncMediaJob
	err := DB.Where("job_id = ?", jobID).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetAsyncMediaJobForUser returns one job owned by the caller.
func GetAsyncMediaJobForUser(jobID string, userId int) (*AsyncMediaJob, error) {
	job, err := GetAsyncMediaJob(jobID)
	if err != nil {
		return nil, err
	}
	if job.UserId != userId {
		return nil, gorm.ErrRecordNotFound
	}
	return job, nil
}

// ClaimQueuedAsyncMediaJobs claims up to limit queued jobs for this worker. The
// conditional update is the mutual exclusion: a job is executed by exactly one
// worker, and a second scheduler pass cannot pick up a claimed job.
func ClaimQueuedAsyncMediaJobs(now int64, limit int) []*AsyncMediaJob {
	if limit <= 0 {
		return nil
	}
	var candidates []*AsyncMediaJob
	if err := DB.Where("status = ?", AsyncMediaJobStatusQueued).
		Order("id").
		Limit(limit).
		Find(&candidates).Error; err != nil {
		common.SysError("query async media jobs failed: " + err.Error())
		return nil
	}
	claimed := make([]*AsyncMediaJob, 0, len(candidates))
	for _, candidate := range candidates {
		result := DB.Model(&AsyncMediaJob{}).
			Where("job_id = ? AND status = ?", candidate.JobID, AsyncMediaJobStatusQueued).
			Updates(map[string]any{
				"status":     AsyncMediaJobStatusRunning,
				"started_at": now,
				"updated_at": now,
			})
		if result.Error != nil {
			common.SysError("claim async media job failed: " + result.Error.Error())
			continue
		}
		if result.RowsAffected != 1 {
			continue
		}
		candidate.Status = AsyncMediaJobStatusRunning
		candidate.StartedAt = now
		claimed = append(claimed, candidate)
	}
	return claimed
}

// RecoverStaleAsyncMediaJobs fails jobs whose worker disappeared. They are never
// replayed: the upstream call may already have started, and a silent retry could
// generate and bill twice. The operator reconciles them like an interrupted
// synchronous submission.
func RecoverStaleAsyncMediaJobs(staleBefore int64, now int64, expiresAt int64) int64 {
	result := DB.Model(&AsyncMediaJob{}).
		Where("status = ?", AsyncMediaJobStatusRunning).
		Where("started_at > 0 AND started_at < ?", staleBefore).
		Updates(map[string]any{
			"status":         AsyncMediaJobStatusFailed,
			"billing_status": AsyncMediaBillingReconciliationPending,
			"error":          "worker interrupted after the upstream call may have started; the job is not replayed automatically",
			"completed_at":   now,
			"updated_at":     now,
			// A recovered job keeps its accepted request for diagnosis, so the
			// retention deadline has to start here as well.
			"expires_at": expiresAt,
		})
	if result.Error != nil {
		common.SysError("recover stale async media jobs failed: " + result.Error.Error())
		return 0
	}
	return result.RowsAffected
}

// CompleteAsyncMediaJob records the terminal delivery state of one job.
func CompleteAsyncMediaJob(job *AsyncMediaJob, status AsyncMediaJobStatus, billingStatus string, httpStatus int, originTaskID, responseFile, responseContentType, errMessage string, now, expiresAt int64) error {
	if job == nil {
		return errors.New("async media job is required")
	}
	fields := map[string]any{
		"status":                status,
		"billing_status":        billingStatus,
		"http_status":           httpStatus,
		"origin_task_id":        originTaskID,
		"response_file":         responseFile,
		"response_content_type": responseContentType,
		"error":                 errMessage,
		"completed_at":          now,
		"updated_at":            now,
		"expires_at":            expiresAt,
	}
	return DB.Model(&AsyncMediaJob{}).Where("job_id = ?", job.JobID).Updates(fields).Error
}

// ListAsyncMediaJobs returns one page of jobs for administration, newest first.
func ListAsyncMediaJobs(offset, limit int, status string) ([]*AsyncMediaJob, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	query := DB.Model(&AsyncMediaJob{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var jobs []*AsyncMediaJob
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&jobs).Error; err != nil {
		return nil, 0, err
	}
	return jobs, total, nil
}

// ListExpiredAsyncMediaJobs returns jobs whose stored response passed its
// retention deadline and cannot be delivered anymore.
func ListExpiredAsyncMediaJobs(now int64, limit int) []*AsyncMediaJob {
	if limit <= 0 {
		return nil
	}
	var jobs []*AsyncMediaJob
	if err := DB.Where("expires_at > 0 AND expires_at < ? AND (response_file <> '' OR request_file <> '')", now).
		Order("expires_at").
		Limit(limit).
		Find(&jobs).Error; err != nil {
		common.SysError("query expired async media jobs failed: " + err.Error())
		return nil
	}
	return jobs
}

// ClearAsyncMediaJobFiles forgets the stored files of one job. The row stays as
// an audit record of the accepted request and its terminal state.
func ClearAsyncMediaJobFiles(jobID string, now int64) error {
	return DB.Model(&AsyncMediaJob{}).Where("job_id = ?", jobID).Updates(map[string]any{
		"request_file":  "",
		"response_file": "",
		"updated_at":    now,
	}).Error
}

// ClearAsyncMediaJobRequestFile forgets the accepted request file. It runs when a
// job reaches a terminal state: the request is never replayed again, while the
// stored response stays until its retention deadline.
func ClearAsyncMediaJobRequestFile(jobID string, now int64) error {
	return DB.Model(&AsyncMediaJob{}).Where("job_id = ?", jobID).Updates(map[string]any{
		"request_file": "",
		"updated_at":   now,
	}).Error
}

// AsyncMediaJobExpiry returns the retention deadline of a stored response.
func AsyncMediaJobExpiry(now int64, retentionHours int) int64 {
	if retentionHours <= 0 {
		return 0
	}
	return now + int64(retentionHours)*int64(time.Hour/time.Second)
}
