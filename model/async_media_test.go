package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncMediaJobClaimUsesConditionalLease(t *testing.T) {
	truncateTables(t)
	job := &AsyncMediaJob{
		JobID:       "job_claim_contract",
		UserID:      11,
		TokenID:     22,
		Method:      "POST",
		RequestPath: "/v1/images/generations",
		RequestFile: "input/job_claim_contract.request",
	}
	require.NoError(t, CreateAsyncMediaJob(job))

	claimed, won, err := ClaimNextAsyncMediaJob("worker-a", time.Now().Unix()+300)
	require.NoError(t, err)
	require.True(t, won)
	require.NotNil(t, claimed)
	assert.Equal(t, AsyncMediaJobStatusQueued, claimed.Status)
	assert.Equal(t, job.JobID, claimed.JobID)

	var persisted AsyncMediaJob
	require.NoError(t, DB.Where("job_id = ?", job.JobID).First(&persisted).Error)
	assert.Equal(t, AsyncMediaJobStatusRunning, persisted.Status)
	assert.Equal(t, "worker-a", persisted.LeaseOwner)

	_, won, err = ClaimNextAsyncMediaJob("worker-b", time.Now().Unix()+300)
	require.NoError(t, err)
	assert.False(t, won)
}

func TestExpiredRunningAsyncMediaJobIsNotAutomaticallyReplayed(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	job := &AsyncMediaJob{
		JobID:         "job_ambiguous_dispatch",
		UserID:        11,
		TokenID:       22,
		Method:        "POST",
		RequestPath:   "/v1/images/generations",
		RequestFile:   "input/job_ambiguous_dispatch.request",
		Status:        AsyncMediaJobStatusRunning,
		BillingStatus: AsyncMediaBillingPending,
		LeaseOwner:    "dead-worker",
		LeaseUntil:    now - 1,
	}
	require.NoError(t, CreateAsyncMediaJob(job))
	require.NoError(t, MarkExpiredRunningAsyncMediaJobsFailed(now))

	var persisted AsyncMediaJob
	require.NoError(t, DB.Where("job_id = ?", job.JobID).First(&persisted).Error)
	assert.Equal(t, AsyncMediaJobStatusFailed, persisted.Status)
	assert.Equal(t, AsyncMediaBillingReconciliationPending, persisted.BillingStatus)
	assert.Contains(t, persisted.Error, "automatic replay disabled")
}
