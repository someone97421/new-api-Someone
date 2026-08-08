package service

import (
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreAsyncMediaResultsDecodesBase64WithoutDatabaseBlob(t *testing.T) {
	job := &model.AsyncMediaJob{JobID: "job_base64_result", UserID: 7}
	responsePath, responseFile, err := CreateAsyncMediaResponseFile(job.JobID)
	require.NoError(t, err)
	pngBytes := []byte("meaningful-image-bytes")
	payload, err := common.Marshal(map[string]any{
		"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(pngBytes)}},
	})
	require.NoError(t, err)
	_, err = responseFile.Write(payload)
	require.NoError(t, err)
	require.NoError(t, responseFile.Close())
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(responsePath) })

	files, err := StoreAsyncMediaResults(job, responsePath, "application/json")
	require.NoError(t, err)
	require.Len(t, files, 1)
	t.Cleanup(func() { _ = DeleteAsyncMediaPath(files[0].Path) })

	absolute, err := ResolveAsyncMediaPath(files[0].Path)
	require.NoError(t, err)
	stored, err := os.ReadFile(absolute)
	require.NoError(t, err)
	assert.Equal(t, pngBytes, stored)
	assert.Equal(t, int64(len(pngBytes)), files[0].Size)
	assert.Greater(t, files[0].ExpiresAt, time.Now().Unix())
}

func TestAsyncMediaFileSignatureRejectsTamperingAndExpiry(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour).Unix()
	signature := AsyncMediaFileSignature("file_contract", expiresAt)
	assert.True(t, ValidateAsyncMediaFileSignature("file_contract", expiresAt, signature))
	assert.False(t, ValidateAsyncMediaFileSignature("file_other", expiresAt, signature))
	assert.False(t, ValidateAsyncMediaFileSignature("file_contract", time.Now().Add(-time.Second).Unix(), signature))
}

func TestAsyncMediaInternalSignatureCannotBeForgedWithJobIDAlone(t *testing.T) {
	signature := AsyncMediaInternalSignature("job_internal_contract")
	assert.True(t, ValidateAsyncMediaInternalRequest("job_internal_contract", signature))
	assert.False(t, ValidateAsyncMediaInternalRequest("job_other", signature))
	assert.False(t, ValidateAsyncMediaInternalRequest("job_internal_contract", "job_internal_contract"))
}
