package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type AsyncMediaJobStatus string

const (
	AsyncMediaJobStatusQueued          AsyncMediaJobStatus = "queued"
	AsyncMediaJobStatusRunning         AsyncMediaJobStatus = "running"
	AsyncMediaJobStatusWaitingUpstream AsyncMediaJobStatus = "waiting_upstream"
	AsyncMediaJobStatusSucceeded       AsyncMediaJobStatus = "succeeded"
	AsyncMediaJobStatusFailed          AsyncMediaJobStatus = "failed"
)

const (
	AsyncMediaBillingPending               = "pending"
	AsyncMediaBillingDelegated             = "delegated"
	AsyncMediaBillingSettled               = "settled"
	AsyncMediaBillingRefunded              = "refunded"
	AsyncMediaBillingReconciliationPending = "reconciliation_pending"
)

type AsyncMediaJob struct {
	ID                int64               `json:"-" gorm:"primaryKey"`
	JobID             string              `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	UserID            int                 `json:"user_id" gorm:"index"`
	TokenID           int                 `json:"token_id" gorm:"index"`
	Method            string              `json:"method" gorm:"type:varchar(16)"`
	RequestPath       string              `json:"request_path" gorm:"type:varchar(512)"`
	RawQuery          string              `json:"-" gorm:"type:text"`
	RequestHeaders    string              `json:"-" gorm:"type:text"`
	RequestFile       string              `json:"-" gorm:"type:text"`
	RequestSize       int64               `json:"request_size"`
	Status            AsyncMediaJobStatus `json:"status" gorm:"type:varchar(32);index"`
	BillingStatus     string              `json:"billing_status" gorm:"type:varchar(32);index"`
	OriginTaskID      string              `json:"origin_task_id,omitempty" gorm:"type:varchar(191);index"`
	UpstreamChannelID int                 `json:"-" gorm:"index"`
	UpstreamTaskID    string              `json:"upstream_task_id,omitempty" gorm:"type:varchar(191);index"`
	ModelName         string              `json:"model,omitempty" gorm:"type:varchar(191);index"`
	HTTPStatus        int                 `json:"http_status,omitempty"`
	ResponseHeaders   string              `json:"-" gorm:"type:text"`
	ResponseFile      string              `json:"-" gorm:"type:text"`
	Error             string              `json:"error,omitempty" gorm:"type:text"`
	LeaseOwner        string              `json:"-" gorm:"type:varchar(128);index"`
	LeaseUntil        int64               `json:"-" gorm:"bigint;index"`
	NextRunAt         int64               `json:"-" gorm:"bigint;index"`
	CreatedAt         int64               `json:"created_at" gorm:"bigint;index"`
	StartedAt         int64               `json:"started_at,omitempty" gorm:"bigint"`
	CompletedAt       int64               `json:"completed_at,omitempty" gorm:"bigint;index"`
	ExpiresAt         int64               `json:"expires_at,omitempty" gorm:"bigint;index"`
	UpdatedAt         int64               `json:"updated_at" gorm:"bigint;index"`
	Files             []AsyncMediaFile    `json:"data,omitempty" gorm:"foreignKey:JobID;references:JobID"`
}

type AsyncMediaFile struct {
	ID        int64  `json:"-" gorm:"primaryKey"`
	FileID    string `json:"id" gorm:"type:varchar(64);uniqueIndex"`
	JobID     string `json:"-" gorm:"type:varchar(64);index"`
	UserID    int    `json:"-" gorm:"index"`
	Path      string `json:"-" gorm:"type:text"`
	MimeType  string `json:"mime_type" gorm:"type:varchar(128)"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256" gorm:"type:varchar(64)"`
	ExpiresAt int64  `json:"expires_at" gorm:"bigint;index"`
	DeletedAt int64  `json:"-" gorm:"bigint;index"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index"`
	URL       string `json:"url" gorm:"-"`
}

func GenerateAsyncMediaJobID() (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return "job_" + key, nil
}

func GenerateAsyncMediaFileID() (string, error) {
	key, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return "", err
	}
	return "file_" + key, nil
}

func CreateAsyncMediaJob(job *AsyncMediaJob) error {
	now := time.Now().Unix()
	if job.CreatedAt == 0 {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	if job.NextRunAt == 0 {
		job.NextRunAt = now
	}
	if job.Status == "" {
		job.Status = AsyncMediaJobStatusQueued
	}
	if job.BillingStatus == "" {
		job.BillingStatus = AsyncMediaBillingPending
	}
	return DB.Create(job).Error
}

func GetAsyncMediaJobForUser(jobID string, userID int) (*AsyncMediaJob, error) {
	var job AsyncMediaJob
	err := DB.Where("job_id = ? AND user_id = ?", jobID, userID).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := DB.Where("job_id = ? AND deleted_at = 0 AND expires_at > ?", jobID, time.Now().Unix()).Order("id asc").Find(&job.Files).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func GetAsyncMediaFile(fileID string) (*AsyncMediaFile, error) {
	var file AsyncMediaFile
	err := DB.Where("file_id = ?", fileID).First(&file).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &file, err
}

func ClaimNextAsyncMediaJob(workerID string, leaseUntil int64) (*AsyncMediaJob, bool, error) {
	now := time.Now().Unix()
	var candidates []*AsyncMediaJob
	err := DB.Where("status IN ? AND next_run_at <= ?", []AsyncMediaJobStatus{
		AsyncMediaJobStatusQueued,
		AsyncMediaJobStatusWaitingUpstream,
	}, now).Order("id asc").Limit(10).Find(&candidates).Error
	if err != nil {
		return nil, false, err
	}
	for _, candidate := range candidates {
		previousStatus := candidate.Status
		updates := map[string]any{
			"status":      AsyncMediaJobStatusRunning,
			"lease_owner": workerID,
			"lease_until": leaseUntil,
			"updated_at":  now,
		}
		if candidate.StartedAt == 0 {
			updates["started_at"] = now
		}
		result := DB.Model(&AsyncMediaJob{}).
			Where("id = ? AND status = ?", candidate.ID, previousStatus).
			Where("next_run_at <= ?", now).
			Updates(updates)
		if result.Error != nil {
			return nil, false, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		if err := DB.Where("id = ?", candidate.ID).First(candidate).Error; err != nil {
			return nil, false, err
		}
		candidate.Status = previousStatus
		return candidate, true, nil
	}
	return nil, false, nil
}

func RenewAsyncMediaJobLease(jobID string, workerID string, leaseUntil int64) error {
	result := DB.Model(&AsyncMediaJob{}).
		Where("job_id = ? AND status = ? AND lease_owner = ?", jobID, AsyncMediaJobStatusRunning, workerID).
		Updates(map[string]any{"lease_until": leaseUntil, "updated_at": time.Now().Unix()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("async media job lease lost")
	}
	return nil
}

func UpdateAsyncMediaJob(jobID string, workerID string, updates map[string]any) error {
	updates["lease_owner"] = ""
	updates["lease_until"] = 0
	updates["updated_at"] = time.Now().Unix()
	result := DB.Model(&AsyncMediaJob{}).
		Where("job_id = ? AND status = ? AND lease_owner = ?", jobID, AsyncMediaJobStatusRunning, workerID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("async media job lease lost")
	}
	return nil
}

func CreateAsyncMediaFiles(files []*AsyncMediaFile) error {
	if len(files) == 0 {
		return nil
	}
	return DB.Create(&files).Error
}

func FindExpiredAsyncMediaFiles(now int64, limit int) ([]*AsyncMediaFile, error) {
	if limit <= 0 {
		limit = 100
	}
	var files []*AsyncMediaFile
	err := DB.Where("deleted_at = 0 AND expires_at > 0 AND expires_at <= ?", now).
		Order("id asc").Limit(limit).Find(&files).Error
	return files, err
}

func MarkAsyncMediaFileDeleted(fileID string, deletedAt int64) error {
	return DB.Model(&AsyncMediaFile{}).Where("file_id = ? AND deleted_at = 0").Update("deleted_at", deletedAt).Error
}

func MarkExpiredRunningAsyncMediaJobsFailed(now int64) error {
	return DB.Model(&AsyncMediaJob{}).
		Where("status = ? AND lease_until > 0 AND lease_until < ?", AsyncMediaJobStatusRunning, now).
		Updates(map[string]any{
			"status":         AsyncMediaJobStatusFailed,
			"billing_status": AsyncMediaBillingReconciliationPending,
			"error":          "worker lease expired after dispatch; automatic replay disabled to avoid duplicate upstream billing",
			"completed_at":   now,
			"updated_at":     now,
		}).Error
}
