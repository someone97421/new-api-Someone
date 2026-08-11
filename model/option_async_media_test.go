package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionMapAppliesAsyncMediaSettings(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	originalOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	originalEnabled := constant.AsyncMediaEnabled
	originalRetention := constant.AsyncMediaRetentionHours
	originalTimeout := constant.TaskTimeoutMinutes
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		constant.AsyncMediaEnabled = originalEnabled
		constant.AsyncMediaRetentionHours = originalRetention
		constant.TaskTimeoutMinutes = originalTimeout
	})

	require.NoError(t, updateOptionMap("AsyncMediaEnabled", "false"))
	require.NoError(t, updateOptionMap("AsyncMediaRetentionHours", "72"))
	require.NoError(t, updateOptionMap("TaskTimeoutMinutes", "0"))

	assert.False(t, constant.AsyncMediaEnabled)
	assert.Equal(t, 72, constant.AsyncMediaRetentionHours)
	assert.Zero(t, constant.TaskTimeoutMinutes)
}
