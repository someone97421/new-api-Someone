package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestTaskArtifactLocal_PersistResolveServe(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalArtifactStore(dir, 168)

	task := &model.Task{TaskID: "task_test_001"}
	payload := []byte("content payload bytes for video artifact 1234567890")
	artifact := types.TaskArtifact{
		Key:      "video",
		Type:     "video",
		MimeType: "video/mp4",
	}

	// 1. Persist
	ref, err := store.Persist(context.Background(), task, artifact, bytes.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, ref)
	assert.Equal(t, "local", ref.Backend)
	assert.Equal(t, "task_test_001", filepath.Dir(ref.ObjectKey))
	assert.Equal(t, "video/mp4", ref.MimeType)
	assert.Equal(t, int64(len(payload)), ref.Size)

	// Verify sidecar file
	sidecarPath := filepath.Join(dir, filepath.FromSlash(ref.ObjectKey)) + ".meta.json"
	sidecarData, err := os.ReadFile(sidecarPath)
	require.NoError(t, err)
	var meta LocalArtifactMeta
	err = common.Unmarshal(sidecarData, &meta)
	require.NoError(t, err)
	assert.Equal(t, "video/mp4", meta.MimeType)
	assert.Equal(t, int64(len(payload)), meta.Size)
	assert.NotEmpty(t, meta.SHA256)
	assert.Greater(t, meta.ArchivedAt, int64(0))

	// 2. Resolve
	resolved, err := store.Resolve(task, "video")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, ref.Backend, resolved.Backend)
	assert.Equal(t, ref.ObjectKey, resolved.ObjectKey)
	assert.Equal(t, ref.MimeType, resolved.MimeType)
	assert.Equal(t, ref.Size, resolved.Size)

	// 3. Serve - standard GET
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/test/video", nil)

		err := store.Serve(c, task, resolved)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, payload, w.Body.Bytes())
		assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
		assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, "sandbox; default-src 'none'", w.Header().Get("Content-Security-Policy"))
		assert.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	}

	// 4. Serve - HEAD request
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodHead, "/test/video", nil)

		err := store.Serve(c, task, resolved)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, w.Body.Bytes())
		assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
		assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, "sandbox; default-src 'none'", w.Header().Get("Content-Security-Policy"))
		assert.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	}

	// 5. Serve - Range request
	{
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, "/test/video", nil)
		req.Header.Set("Range", "bytes=0-6")
		c.Request = req

		err := store.Serve(c, task, resolved)
		require.NoError(t, err)
		assert.Equal(t, http.StatusPartialContent, w.Code)
		assert.Equal(t, payload[:7], w.Body.Bytes())
		assert.Equal(t, "bytes 0-6/"+strconv.Itoa(len(payload)), w.Header().Get("Content-Range"))
	}
}

func TestTaskArtifactLocal_MimeSniffing(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalArtifactStore(dir, 168)

	task := &model.Task{TaskID: "task_test_png"}
	pngMagic := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01")
	artifact := types.TaskArtifact{
		Key:      "image",
		Type:     "image",
		MimeType: "", // omit MIME type so DetectContentType is used
	}

	ref, err := store.Persist(context.Background(), task, artifact, bytes.NewReader(pngMagic))
	require.NoError(t, err)
	require.NotNil(t, ref)
	assert.Equal(t, "image/png", ref.MimeType)

	resolved, err := store.Resolve(task, "image")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, "image/png", resolved.MimeType)
}

func TestTaskArtifactLocal_SidecarMissingFallback(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalArtifactStore(dir, 168)

	task := &model.Task{TaskID: "task_test_fallback"}
	payload := []byte("hello plain text content fallback test")
	artifact := types.TaskArtifact{
		Key:      "info",
		Type:     "file",
		MimeType: "text/plain",
	}

	ref, err := store.Persist(context.Background(), task, artifact, bytes.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, ref)

	// Remove sidecar
	sidecarPath := filepath.Join(dir, filepath.FromSlash(ref.ObjectKey)) + ".meta.json"
	err = os.Remove(sidecarPath)
	require.NoError(t, err)

	// Resolve should succeed with mtime/stat fallback
	resolved, err := store.Resolve(task, "info")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, ref.ObjectKey, resolved.ObjectKey)
	assert.Equal(t, int64(len(payload)), resolved.Size)
	assert.NotEmpty(t, resolved.MimeType)

	// Serve should work normally
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test/info", nil)

	err = store.Serve(c, task, resolved)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, payload, w.Body.Bytes())
}

func TestTaskArtifactLocal_Expiration(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalArtifactStore(dir, 24) // 24 hours retention

	baseTime := time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)
	currentTime := baseTime
	store.SetNowFunc(func() time.Time { return currentTime })

	task := &model.Task{TaskID: "task_exp_01"}
	payload := []byte("sample bytes to expire")
	artifact := types.TaskArtifact{
		Key:      "sample",
		Type:     "file",
		MimeType: "text/plain",
	}

	ref, err := store.Persist(context.Background(), task, artifact, bytes.NewReader(payload))
	require.NoError(t, err)
	require.NotNil(t, ref)

	// Before retention window ends: 23 hours later -> should be resolved
	currentTime = baseTime.Add(23 * time.Hour)
	resolved, err := store.Resolve(task, "sample")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	// At or after retention window: 24h 1s later -> should return nil, nil
	currentTime = baseTime.Add(24*time.Hour + time.Second)
	expiredResolved, err := store.Resolve(task, "sample")
	require.NoError(t, err)
	assert.Nil(t, expiredResolved, "expired artifact must resolve to nil")
}

func TestTaskArtifactLocal_SecurityAndPathTraversal(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalArtifactStore(dir, 168)

	validTask := &model.Task{TaskID: "valid_task_123"}
	validArtifact := types.TaskArtifact{Key: "video", Type: "video", MimeType: "video/mp4"}
	dummyPayload := []byte("dummy")

	// Invalid artifact keys in Persist
	invalidKeys := []string{
		"../escape",
		"../../etc/passwd",
		"sub/folder",
		"sub\\folder",
		"..",
		".",
		"foo*bar",
		"foo?bar",
		"foo;rm",
		" ",
	}

	for _, invalidKey := range invalidKeys {
		badArtifact := validArtifact
		badArtifact.Key = invalidKey
		_, err := store.Persist(context.Background(), validTask, badArtifact, bytes.NewReader(dummyPayload))
		assert.Error(t, err, "Persist must reject invalid artifact key %q", invalidKey)

		_, err = store.Resolve(validTask, invalidKey)
		assert.Error(t, err, "Resolve must reject invalid artifact key %q", invalidKey)
	}

	// Invalid task ID in Persist and Resolve
	invalidTaskIDs := []string{
		"../task",
		"task/1",
		"..",
		".",
		"",
	}
	for _, badTaskID := range invalidTaskIDs {
		badTask := &model.Task{TaskID: badTaskID}
		_, err := store.Persist(context.Background(), badTask, validArtifact, bytes.NewReader(dummyPayload))
		assert.Error(t, err, "Persist must reject invalid task ID %q", badTaskID)

		_, err = store.Resolve(badTask, "video")
		assert.Error(t, err, "Resolve must reject invalid task ID %q", badTaskID)
	}

	// Serve with path traversal in ref
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)
	err := store.Serve(c, validTask, &StoredArtifactRef{
		Backend:   "local",
		ObjectKey: "../outside/file",
	})
	assert.Error(t, err, "Serve must reject traversing object key")
}

func TestTaskArtifactLocal_CleanExpired(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalArtifactStore(dir, 10) // 10 hours retention

	baseTime := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime
	store.SetNowFunc(func() time.Time { return currentTime })

	taskOld := &model.Task{TaskID: "task_old_terminal"}
	taskNew := &model.Task{TaskID: "task_new_terminal"}

	// Persist taskOld artifact at baseTime
	_, err := store.Persist(context.Background(), taskOld, types.TaskArtifact{
		Key: "output", Type: "video", MimeType: "video/mp4",
	}, bytes.NewReader([]byte("old video content")))
	require.NoError(t, err)

	// Advance time by 15 hours (now taskOld is older than 10h retention)
	currentTime = baseTime.Add(15 * time.Hour)

	// Persist taskNew artifact at baseTime + 15h
	_, err = store.Persist(context.Background(), taskNew, types.TaskArtifact{
		Key: "output", Type: "video", MimeType: "video/mp4",
	}, bytes.NewReader([]byte("new video content")))
	require.NoError(t, err)

	// Clean expired artifacts
	cleaned, err := store.CleanExpired(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, cleaned, "expected 1 expired artifact to be cleaned")

	// Verify taskOld is gone
	oldResolved, err := store.Resolve(taskOld, "output")
	require.NoError(t, err)
	assert.Nil(t, oldResolved, "taskOld artifact should no longer exist")
	_, err = os.Stat(filepath.Join(dir, "task_old_terminal", localArtifactFilename("output")))
	assert.True(t, os.IsNotExist(err), "taskOld file must be deleted")
	_, err = os.Stat(filepath.Join(dir, "task_old_terminal", localArtifactFilename("output")+".meta.json"))
	assert.True(t, os.IsNotExist(err), "taskOld sidecar must be deleted")

	// Verify taskNew is preserved
	newResolved, err := store.Resolve(taskNew, "output")
	require.NoError(t, err)
	require.NotNil(t, newResolved, "taskNew artifact must be preserved")
	_, err = os.Stat(filepath.Join(dir, filepath.FromSlash(newResolved.ObjectKey)))
	assert.NoError(t, err, "taskNew file must still exist")
	_, err = os.Stat(filepath.Join(dir, filepath.FromSlash(newResolved.ObjectKey)) + ".meta.json")
	assert.NoError(t, err, "taskNew sidecar must still exist")
}

func TestTaskArtifactLocal_KeysCannotOverwriteMetadata(t *testing.T) {
	store := NewLocalArtifactStore(t.TempDir(), 1)
	archivedAt := time.Now()
	store.SetNowFunc(func() time.Time { return archivedAt })
	task := &model.Task{TaskID: "task_keys"}
	keys := []string{"video", "video.meta.json", ".tmp-video", "VIDEO", "artifact-custom.data"}
	for _, key := range keys {
		_, err := store.Persist(context.Background(), task, types.TaskArtifact{Key: key}, strings.NewReader("content:"+key))
		require.NoError(t, err)
	}
	for _, key := range keys {
		ref, err := store.Resolve(task, key)
		require.NoError(t, err)
		require.NotNil(t, ref)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/content", nil)
		require.NoError(t, store.Serve(c, task, ref))
		assert.Equal(t, "content:"+key, w.Body.String())
	}
	archivedAt = archivedAt.Add(2 * time.Hour)
	cleaned, err := store.CleanExpired(context.Background())
	require.NoError(t, err)
	assert.Equal(t, len(keys), cleaned)
	entries, err := os.ReadDir(store.LocalDir())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTaskArtifactLocal_LegacyFilesAndSidecars(t *testing.T) {
	store := NewLocalArtifactStore(t.TempDir(), 24)
	task := &model.Task{TaskID: "task_legacy"}
	dir := filepath.Join(store.LocalDir(), task.TaskID)
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "video"), []byte("legacy video"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "video.meta.json"), []byte(`{"mime_type":"video/mp4"}`), 0600))
	ref, err := store.Resolve(task, "video")
	require.NoError(t, err)
	require.NotNil(t, ref)
	assert.Equal(t, "task_legacy/video", ref.ObjectKey)
	for _, key := range []string{"video.meta.json", "video.META.JSON", "video.meta.json."} {
		ref, err = store.Resolve(task, key)
		require.NoError(t, err)
		assert.Nil(t, ref)
	}
}
