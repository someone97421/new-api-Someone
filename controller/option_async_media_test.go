package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAsyncMediaOption(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{name: "relative storage path", key: "AsyncMediaStoragePath", value: "./data/async-media"},
		{name: "empty storage path", key: "AsyncMediaStoragePath", value: " ", wantErr: true},
		{name: "worker count lower bound", key: "AsyncMediaWorkers", value: "1"},
		{name: "worker count zero", key: "AsyncMediaWorkers", value: "0", wantErr: true},
		{name: "lease lower bound", key: "AsyncMediaLeaseSeconds", value: "30"},
		{name: "lease too short", key: "AsyncMediaLeaseSeconds", value: "29", wantErr: true},
		{name: "task timeout disabled", key: "TaskTimeoutMinutes", value: "0"},
		{name: "task timeout negative", key: "TaskTimeoutMinutes", value: "-1", wantErr: true},
		{name: "invalid integer", key: "AsyncMediaRetentionHours", value: "one", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAsyncMediaOption(tt.key, tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestValidateGeminiImageGenerationConfigMode(t *testing.T) {
	require.NoError(t, model_setting.ValidateGeminiImageGenerationConfigMode("official"))
	require.NoError(t, model_setting.ValidateGeminiImageGenerationConfigMode("legacy"))
	assert.Error(t, model_setting.ValidateGeminiImageGenerationConfigMode("auto"))
}

func TestValidateRetryTimesRange(t *testing.T) {
	require.NoError(t, validateBoundedIntegerOption("RetryTimes", "1", 0, 10))
	assert.Error(t, validateBoundedIntegerOption("RetryTimes", "11", 0, 10))
}
