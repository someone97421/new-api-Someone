package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncMediaTaskLogFollowsJobStatus(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	job := &AsyncMediaJob{
		JobID:     "job_task_log_contract",
		UserID:    7,
		ModelName: "gemini-3.1-flash-image",
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, CreateAsyncMediaTaskLog(job))
	originTask, err := GetTaskByAsyncJobID(job.JobID)
	require.NoError(t, err)
	assert.Nil(t, originTask)

	var task Task
	err = DB.Where("async_job_id = ? AND platform = ?", job.JobID, constant.TaskPlatformAsyncMedia).First(&task).Error
	require.NoError(t, err)
	assert.Equal(t, string(constant.TaskPlatformAsyncMedia), string(task.Platform))
	assert.Equal(t, string(constant.TaskActionAsyncMedia), string(task.Action))
	assert.Equal(t, string(TaskStatusQueued), string(task.Status))
	assert.Equal(t, "gemini-3.1-flash-image", task.Properties.OriginModelName)

	require.NoError(t, UpdateAsyncMediaTaskLog(job.JobID, AsyncMediaJobStatusWaitingUpstream, map[string]any{
		"upstream_channel_id": 6,
	}))
	require.NoError(t, DB.Where("async_job_id = ? AND platform = ?", job.JobID, constant.TaskPlatformAsyncMedia).First(&task).Error)
	assert.Equal(t, string(TaskStatusInProgress), string(task.Status))
	assert.Equal(t, "50%", task.Progress)
	assert.Equal(t, 6, task.ChannelId)

	require.NoError(t, UpdateAsyncMediaTaskLog(job.JobID, AsyncMediaJobStatusFailed, map[string]any{
		"error": "upstream failed",
	}))
	require.NoError(t, DB.Where("async_job_id = ? AND platform = ?", job.JobID, constant.TaskPlatformAsyncMedia).First(&task).Error)
	assert.Equal(t, string(TaskStatusFailure), string(task.Status))
	assert.Equal(t, "100%", task.Progress)
	assert.Equal(t, "upstream failed", task.FailReason)
}
