package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useAsyncMediaTestDB installs an in-memory database and a temporary storage
// directory for the duration of one test.
func useAsyncMediaTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousDir := constant.AsyncMediaDir
	previousRetention := constant.AsyncMediaRetentionHours
	previousStale := constant.AsyncMediaStaleMinutes
	t.Cleanup(func() {
		model.DB = previousDB
		constant.AsyncMediaDir = previousDir
		constant.AsyncMediaRetentionHours = previousRetention
		constant.AsyncMediaStaleMinutes = previousStale
	})
	// The process initializes these from the environment at startup; a test sets
	// them explicitly instead of relying on the ambient configuration.
	constant.AsyncMediaRetentionHours = 168
	constant.AsyncMediaStaleMinutes = 30
	constant.AsyncMediaDir = t.TempDir()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AsyncMediaJob{}))
	model.DB = db
}

func createAsyncMediaTestJob(t *testing.T, jobID, modelName string) *model.AsyncMediaJob {
	t.Helper()
	requestFile, size, err := SaveAsyncMediaRequest(jobID, strings.NewReader(`{"model":"`+modelName+`"}`), 1<<20)
	require.NoError(t, err)
	job := &model.AsyncMediaJob{
		JobID:       jobID,
		UserId:      1,
		TokenId:     2,
		UserGroup:   "default",
		ModelName:   modelName,
		RequestPath: "/v1/images/generations",
		RequestFile: requestFile,
		RequestSize: size,
		Status:      model.AsyncMediaJobStatusQueued,
	}
	require.NoError(t, model.CreateAsyncMediaJob(job))
	return job
}

func TestAsyncMediaJobStorageRoundTripAndGuards(t *testing.T) {
	useAsyncMediaTestDB(t)

	requestFile, size, err := SaveAsyncMediaRequest("job_storage", strings.NewReader("payload"), 64)
	require.NoError(t, err)
	assert.EqualValues(t, 7, size)
	assert.Equal(t, "payload", string(mustReadFile(t, requestFile)))

	responseFile, err := SaveAsyncMediaResponse("job_storage", []byte(`{"data":[]}`))
	require.NoError(t, err)
	job := &model.AsyncMediaJob{JobID: "job_storage", RequestFile: requestFile, ResponseFile: responseFile}
	payload, err := ReadAsyncMediaResponse(job)
	require.NoError(t, err)
	assert.Equal(t, `{"data":[]}`, string(payload))

	// A row pointing outside the storage directory is never read or deleted.
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("keep"), 0o600))
	DeleteAsyncMediaFile(outside)
	_, readErr := ReadAsyncMediaResponse(&model.AsyncMediaJob{ResponseFile: outside})
	require.Error(t, readErr)
	assert.Equal(t, "keep", string(mustReadFile(t, outside)))
	assert.FileExists(t, outside)

	databaseJob := &model.AsyncMediaJob{JobID: "job_other", ResponseFile: outside}
	_, tamperErr := ReadAsyncMediaRequest(databaseJob)
	require.ErrorContains(t, tamperErr, "stored request is gone")

	// An oversized body is rejected without leaving a file behind.
	_, _, tooLarge := SaveAsyncMediaRequest("job_large", strings.NewReader("0123456789"), 4)
	require.Error(t, tooLarge)
	_, statErr := os.Stat(filepath.Join(constant.AsyncMediaDir, asyncMediaRequestsDir, "job_large.request"))
	assert.True(t, os.IsNotExist(statErr))

	DeleteAsyncMediaFile(requestFile)
	_, goneErr := os.Stat(requestFile)
	assert.True(t, os.IsNotExist(goneErr))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	require.NoError(t, err)
	return payload
}

func TestAsyncMediaJobQueueLifecycle(t *testing.T) {
	useAsyncMediaTestDB(t)

	first := createAsyncMediaTestJob(t, "job_first", "nano-banana")
	second := createAsyncMediaTestJob(t, "job_second", "nano-banana")
	now := common.GetTimestamp()

	// One queued job is claimed exactly once, even if two scheduler passes race.
	claimed := model.ClaimQueuedAsyncMediaJobs(now, 1)
	require.Len(t, claimed, 1)
	assert.Equal(t, first.JobID, claimed[0].JobID)
	secondClaim := model.ClaimQueuedAsyncMediaJobs(now, 5)
	require.Len(t, secondClaim, 1)
	assert.Equal(t, second.JobID, secondClaim[0].JobID)
	assert.Empty(t, model.ClaimQueuedAsyncMediaJobs(now, 5))

	// A finished job stores its response, drops the request file and reports the
	// retention deadline.
	require.NoError(t, finishAsyncMediaJob(claimed[0], model.AsyncMediaJobStatusSucceeded, model.AsyncMediaBillingSettled, 200, "task_public", []byte(`{"created":1,"data":[]}`), "application/json", "", now))
	stored, err := model.GetAsyncMediaJob("job_first")
	require.NoError(t, err)
	assert.Equal(t, model.AsyncMediaJobStatusSucceeded, stored.Status)
	assert.Equal(t, model.AsyncMediaBillingSettled, stored.BillingStatus)
	assert.Equal(t, "task_public", stored.OriginTaskID)
	assert.EqualValues(t, 200, stored.HTTPStatus)
	assert.NotEmpty(t, stored.ResponseFile)
	assert.Empty(t, stored.RequestFile)
	assert.Greater(t, stored.ExpiresAt, now)
	assert.NoFileExists(t, first.RequestFile)

	// A timed-out image bridge keeps the request for rendering after the
	// official task finishes; it is not replayed and still counts as pending.
	require.NoError(t, finishAsyncMediaJob(secondClaim[0], model.AsyncMediaJobStatusAwaitingTask, model.AsyncMediaBillingReconciliationPending, 504, "task_pending", nil, "", "", now))
	waiting, err := model.GetAsyncMediaJob(second.JobID)
	require.NoError(t, err)
	assert.Equal(t, model.AsyncMediaJobStatusAwaitingTask, waiting.Status)
	assert.Equal(t, "task_pending", waiting.OriginTaskID)
	assert.FileExists(t, second.RequestFile)
	pending, err := model.CountPendingAsyncMediaJobs(second.UserId)
	require.NoError(t, err)
	assert.EqualValues(t, 1, pending)

	// A job whose worker disappeared is failed for reconciliation, never replayed.
	require.NoError(t, model.CompleteAsyncMediaJob(second, model.AsyncMediaJobStatusRunning, "", 0, "", "", "", "", now, 0))
	recovered := model.RecoverStaleAsyncMediaJobs(now+1, now+2, now+3)
	assert.EqualValues(t, 1, recovered)
	abandoned, err := model.GetAsyncMediaJob("job_second")
	require.NoError(t, err)
	assert.Equal(t, model.AsyncMediaJobStatusFailed, abandoned.Status)
	assert.Equal(t, model.AsyncMediaBillingReconciliationPending, abandoned.BillingStatus)
	assert.Contains(t, abandoned.Error, "not replayed automatically")
	assert.EqualValues(t, 0, model.RecoverStaleAsyncMediaJobs(now+1, now+2, now+3))

	// Expired responses are purged while the audit row stays.
	expired := &model.AsyncMediaJob{
		JobID:        "job_expired",
		ModelName:    "nano-banana",
		Status:       model.AsyncMediaJobStatusSucceeded,
		ResponseFile: stored.ResponseFile,
		ExpiresAt:    now - 1,
	}
	require.NoError(t, model.DB.Create(expired).Error)
	assert.EqualValues(t, 1, purgeExpiredAsyncMediaFiles(now))
	afterPurge, err := model.GetAsyncMediaJob("job_expired")
	require.NoError(t, err)
	assert.Empty(t, afterPurge.ResponseFile)
	assert.NoFileExists(t, stored.ResponseFile)
	assert.EqualValues(t, 0, purgeExpiredAsyncMediaFiles(now))
}

func TestAsyncMediaJobExpiryUsesRetentionHours(t *testing.T) {
	now := int64(1000)
	assert.EqualValues(t, 0, model.AsyncMediaJobExpiry(now, 0))
	assert.EqualValues(t, 1000+3600, model.AsyncMediaJobExpiry(now, 1))
}
