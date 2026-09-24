package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var safeArtifactIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateSafeArtifactIdentifier(id string) bool {
	if id != strings.TrimSpace(id) || id == "" || len(id) > 128 {
		return false
	}
	if id == "." || id == ".." || strings.Contains(id, "..") || strings.ContainsAny(id, `/\`) {
		return false
	}
	return safeArtifactIdentifierPattern.MatchString(id)
}

// A fixed-size filename keeps provider keys separate from sidecars and temporary
// files, including on filesystems with case-insensitive names.
func localArtifactFilename(key string) string {
	digest := sha256.Sum256([]byte(key))
	return "artifact-" + hex.EncodeToString(digest[:]) + ".data"
}

// LocalArtifactMeta holds sidecar metadata for a locally stored artifact.
type LocalArtifactMeta struct {
	MimeType   string `json:"mime_type"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	ArchivedAt int64  `json:"archived_at"`
}

// LocalArtifactStore persists task artifacts to the local filesystem.
type LocalArtifactStore struct {
	localDir       string
	retentionHours int
	nowFunc        func() time.Time
}

// NewLocalArtifactStore creates a new LocalArtifactStore instance.
func NewLocalArtifactStore(localDir string, retentionHours int) *LocalArtifactStore {
	return &LocalArtifactStore{
		localDir:       localDir,
		retentionHours: retentionHours,
	}
}

// SetNowFunc overrides current time retrieval for deterministic testing.
func (s *LocalArtifactStore) SetNowFunc(fn func() time.Time) {
	s.nowFunc = fn
}

func (s *LocalArtifactStore) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

// LocalDir returns the root directory for persisted artifacts.
func (s *LocalArtifactStore) LocalDir() string {
	return s.localDir
}

// RetentionHours returns the retention duration in hours.
func (s *LocalArtifactStore) RetentionHours() int {
	return s.retentionHours
}

// Enabled reports whether the store is operational.
func (s *LocalArtifactStore) Enabled() bool {
	return true
}

// Persist writes an artifact's content to the local filesystem atomically.
func (s *LocalArtifactStore) Persist(ctx context.Context, task *model.Task, artifact types.TaskArtifact, content io.Reader) (*StoredArtifactRef, error) {
	if task == nil || !validateSafeArtifactIdentifier(task.TaskID) || !validateSafeArtifactIdentifier(artifact.Key) {
		return nil, errors.New("invalid task ID or artifact key")
	}
	if content == nil {
		return nil, errors.New("nil content reader")
	}

	taskDir := filepath.Join(s.localDir, task.TaskID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, err
	}

	tempFile, err := os.CreateTemp(taskDir, ".tmp-"+artifact.Key+"-*")
	if err != nil {
		return nil, err
	}
	tempName := tempFile.Name()
	cleaned := false
	defer func() {
		if !cleaned {
			_ = tempFile.Close()
			_ = os.Remove(tempName)
		}
	}()

	hasher := sha256.New()
	buf := make([]byte, 32*1024)
	var written int64
	var first512 []byte

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, rErr := content.Read(buf)
		if n > 0 {
			if len(first512) < 512 {
				need := 512 - len(first512)
				if n < need {
					first512 = append(first512, buf[:n]...)
				} else {
					first512 = append(first512, buf[:need]...)
				}
			}
			wn, wErr := tempFile.Write(buf[:n])
			if wErr != nil {
				return nil, wErr
			}
			hasher.Write(buf[:wn])
			written += int64(wn)
		}
		if rErr != nil {
			if errors.Is(rErr, io.EOF) {
				break
			}
			return nil, rErr
		}
	}

	if err := tempFile.Sync(); err != nil {
		return nil, err
	}
	if err := tempFile.Close(); err != nil {
		return nil, err
	}

	mimeType := strings.TrimSpace(artifact.MimeType)
	if mimeType == "" && len(first512) > 0 {
		mimeType = http.DetectContentType(first512)
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	meta := LocalArtifactMeta{
		MimeType:   mimeType,
		Size:       written,
		SHA256:     hex.EncodeToString(hasher.Sum(nil)),
		ArchivedAt: s.now().Unix(),
	}
	metaBytes, err := common.Marshal(meta)
	if err != nil {
		return nil, err
	}

	sidecarTemp, err := os.CreateTemp(taskDir, ".tmp-meta-"+artifact.Key+"-*")
	if err != nil {
		return nil, err
	}
	sidecarTempName := sidecarTemp.Name()
	defer func() {
		if !cleaned {
			_ = sidecarTemp.Close()
			_ = os.Remove(sidecarTempName)
		}
	}()

	if _, err := sidecarTemp.Write(metaBytes); err != nil {
		_ = sidecarTemp.Close()
		return nil, err
	}
	if err := sidecarTemp.Sync(); err != nil {
		_ = sidecarTemp.Close()
		return nil, err
	}
	if err := sidecarTemp.Close(); err != nil {
		return nil, err
	}

	targetPath := filepath.Join(taskDir, localArtifactFilename(artifact.Key))
	sidecarPath := targetPath + ".meta.json"

	if err := os.Rename(tempName, targetPath); err != nil {
		return nil, err
	}
	if err := os.Rename(sidecarTempName, sidecarPath); err != nil {
		_ = os.Remove(targetPath)
		return nil, err
	}
	cleaned = true

	relKey := filepath.ToSlash(filepath.Join(task.TaskID, filepath.Base(targetPath)))
	return &StoredArtifactRef{
		Backend:   "local",
		Bucket:    "",
		ObjectKey: relKey,
		MimeType:  mimeType,
		Size:      written,
	}, nil
}

// Resolve looks up a stored artifact. Missing or expired artifacts return (nil, nil)
// to trigger upstream fallback. Real I/O errors return an error.
func (s *LocalArtifactStore) Resolve(task *model.Task, artifactKey string) (*StoredArtifactRef, error) {
	if task == nil || !validateSafeArtifactIdentifier(task.TaskID) || !validateSafeArtifactIdentifier(artifactKey) {
		return nil, errors.New("invalid task ID or artifact key")
	}

	targetPath := filepath.Join(s.localDir, task.TaskID, localArtifactFilename(artifactKey))
	stat, err := os.Stat(targetPath)
	// Read files written before encoded filenames were introduced. Reserved
	// legacy names can denote metadata or temporary files, never safe content.
	legacyKey := strings.TrimRight(strings.ToLower(artifactKey), ".")
	if os.IsNotExist(err) && !strings.HasSuffix(legacyKey, ".meta.json") &&
		!strings.HasPrefix(legacyKey, ".tmp-") && !strings.HasPrefix(legacyKey, "artifact-") {
		targetPath = filepath.Join(s.localDir, task.TaskID, artifactKey)
		stat, err = os.Stat(targetPath)
	}
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if stat.IsDir() {
		return nil, nil
	}

	sidecarPath := targetPath + ".meta.json"
	metaBytes, mErr := os.ReadFile(sidecarPath)

	var archivedAt int64
	var mimeType string
	var size int64

	var meta LocalArtifactMeta
	if mErr == nil && common.Unmarshal(metaBytes, &meta) == nil && meta.ArchivedAt > 0 {
		archivedAt = meta.ArchivedAt
		mimeType = meta.MimeType
		size = meta.Size
	} else {
		// Fallback when sidecar is missing or corrupt: use file mtime and stat.
		archivedAt = stat.ModTime().Unix()
		size = stat.Size()
		if f, oErr := os.Open(targetPath); oErr == nil {
			buf := make([]byte, 512)
			n, _ := f.Read(buf)
			_ = f.Close()
			if n > 0 {
				mimeType = http.DetectContentType(buf[:n])
			}
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
	}

	now := s.now().Unix()
	retentionSeconds := int64(s.retentionHours) * 3600
	if retentionSeconds > 0 && now >= archivedAt+retentionSeconds {
		return nil, nil
	}

	relKey := filepath.ToSlash(filepath.Join(task.TaskID, filepath.Base(targetPath)))
	return &StoredArtifactRef{
		Backend:   "local",
		Bucket:    "",
		ObjectKey: relKey,
		MimeType:  mimeType,
		Size:      size,
	}, nil
}

// Serve streams the artifact using http.ServeContent, supporting Range, HEAD,
// and conditional headers, matching security headers from controller/video_proxy.go.
func (s *LocalArtifactStore) Serve(c *gin.Context, task *model.Task, ref *StoredArtifactRef) error {
	if ref == nil || strings.TrimSpace(ref.ObjectKey) == "" {
		return errors.New("invalid artifact ref")
	}

	rel := filepath.Clean(filepath.FromSlash(ref.ObjectKey))
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return errors.New("invalid artifact object key")
	}

	filePath := filepath.Join(s.localDir, rel)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	// Security headers match controller/video_proxy.go (setTaskMediaResponseSecurityHeaders).
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")

	if ref.MimeType != "" {
		c.Header("Content-Type", ref.MimeType)
	}

	http.ServeContent(c.Writer, c.Request, filepath.Base(filePath), stat.ModTime(), file)
	return nil
}

// CleanExpired removes artifacts and sidecars older than the retention period.
func (s *LocalArtifactStore) CleanExpired(ctx context.Context) (int, error) {
	entries, err := os.ReadDir(s.localDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	now := s.now().Unix()
	retentionSeconds := int64(s.retentionHours) * 3600
	cleaned := 0

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return cleaned, ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}
		taskID := entry.Name()
		if !validateSafeArtifactIdentifier(taskID) {
			continue
		}

		taskDir := filepath.Join(s.localDir, taskID)
		files, err := os.ReadDir(taskDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || strings.HasSuffix(f.Name(), ".meta.json") || strings.HasPrefix(f.Name(), ".tmp-") {
				continue
			}
			artifactKey := f.Name()
			if !validateSafeArtifactIdentifier(artifactKey) {
				continue
			}

			artPath := filepath.Join(taskDir, artifactKey)
			sidecarPath := artPath + ".meta.json"

			var archivedAt int64
			metaBytes, mErr := os.ReadFile(sidecarPath)
			var meta LocalArtifactMeta
			if mErr == nil && common.Unmarshal(metaBytes, &meta) == nil && meta.ArchivedAt > 0 {
				archivedAt = meta.ArchivedAt
			} else {
				if info, sErr := f.Info(); sErr == nil {
					archivedAt = info.ModTime().Unix()
				}
			}

			if retentionSeconds > 0 && archivedAt > 0 && now >= archivedAt+retentionSeconds {
				delErr := os.Remove(artPath)
				if delErr != nil && !os.IsNotExist(delErr) {
					logger.LogWarn(ctx, fmt.Sprintf("failed to clean expired artifact %s: %v", artPath, delErr))
					continue
				}
				_ = os.Remove(sidecarPath)
				cleaned++
			}
		}

		// Prune empty task directory if all artifacts were removed.
		if remaining, rErr := os.ReadDir(taskDir); rErr == nil && len(remaining) == 0 {
			_ = os.Remove(taskDir)
		}
	}

	return cleaned, nil
}
