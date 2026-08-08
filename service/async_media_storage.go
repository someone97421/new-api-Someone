package service

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

var (
	asyncMediaStorageRoot string
	asyncMediaStorageOnce sync.Once
	asyncMediaStorageErr  error
)

func InitAsyncMediaStorage() error {
	asyncMediaStorageOnce.Do(func() {
		root, err := filepath.Abs(strings.TrimSpace(constant.AsyncMediaStoragePath))
		if err != nil {
			asyncMediaStorageErr = err
			return
		}
		volumeRoot := filepath.Clean(filepath.VolumeName(root) + string(os.PathSeparator))
		if filepath.Clean(root) == volumeRoot {
			asyncMediaStorageErr = errors.New("ASYNC_MEDIA_STORAGE_PATH cannot be a filesystem root")
			return
		}
		for _, dir := range []string{"input", "response", "result"} {
			if err := os.MkdirAll(filepath.Join(root, dir), 0o750); err != nil {
				asyncMediaStorageErr = err
				return
			}
		}
		asyncMediaStorageRoot = root
	})
	return asyncMediaStorageErr
}

func asyncMediaPath(kind string, name string) (string, string, error) {
	if err := InitAsyncMediaStorage(); err != nil {
		return "", "", err
	}
	if name == "" || filepath.Base(name) != name {
		return "", "", errors.New("invalid async media filename")
	}
	relative := filepath.Join(kind, name[:min(2, len(name))], name)
	absolute := filepath.Join(asyncMediaStorageRoot, relative)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
		return "", "", err
	}
	return filepath.ToSlash(relative), absolute, nil
}

func ResolveAsyncMediaPath(relative string) (string, error) {
	if err := InitAsyncMediaStorage(); err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", errors.New("invalid async media relative path")
	}
	absolute := filepath.Join(asyncMediaStorageRoot, clean)
	rel, err := filepath.Rel(asyncMediaStorageRoot, absolute)
	if err != nil || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", errors.New("async media path escapes storage root")
	}
	return absolute, nil
}

func SaveAsyncMediaRequest(jobID string, reader io.Reader) (string, int64, error) {
	relative, absolute, err := asyncMediaPath("input", jobID+".request")
	if err != nil {
		return "", 0, err
	}
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", 0, err
	}
	maxBytes := int64(constant.MaxRequestBodyMB) * 1024 * 1024
	written, copyErr := io.Copy(file, io.LimitReader(reader, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil || written > maxBytes || closeErr != nil {
		_ = os.Remove(absolute)
		if written > maxBytes {
			return "", 0, common.ErrRequestBodyTooLarge
		}
		if copyErr != nil {
			return "", 0, copyErr
		}
		return "", 0, closeErr
	}
	return relative, written, nil
}

func CreateAsyncMediaResponseFile(jobID string) (string, *os.File, error) {
	relative, absolute, err := asyncMediaPath("response", jobID+".response")
	if err != nil {
		return "", nil, err
	}
	file, err := os.OpenFile(absolute, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o600)
	return relative, file, err
}

func DeleteAsyncMediaPath(relative string) error {
	if strings.TrimSpace(relative) == "" {
		return nil
	}
	absolute, err := ResolveAsyncMediaPath(relative)
	if err != nil {
		return err
	}
	err = os.Remove(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func StoreAsyncMediaResults(job *model.AsyncMediaJob, responseFile string, contentType string) ([]*model.AsyncMediaFile, error) {
	absolute, err := ResolveAsyncMediaPath(responseFile)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	mediaType, _, _ := mime.ParseMediaType(contentType)
	if strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "video/") || mediaType == "application/octet-stream" {
		stored, err := storeAsyncMediaReader(job, mediaType, file)
		if err != nil {
			return nil, err
		}
		return []*model.AsyncMediaFile{stored}, nil
	}

	jsonLimitMB := min(constant.AsyncMediaMaxFileMB, max(32, constant.MaxRequestBodyMB))
	maxBytes := int64(jsonLimitMB)*1024*1024 + 1
	body, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) >= maxBytes {
		return nil, errors.New("async media response exceeds ASYNC_MEDIA_MAX_FILE_MB")
	}
	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("async media response is neither media nor JSON: %w", err)
	}

	files := make([]*model.AsyncMediaFile, 0)
	seen := make(map[string]bool)
	if err := collectAsyncMediaValues(job, payload, "", seen, &files); err != nil {
		DeleteAsyncMediaFiles(files)
		return nil, err
	}
	return files, nil
}

func DeleteAsyncMediaFiles(files []*model.AsyncMediaFile) {
	for _, file := range files {
		if file != nil {
			_ = DeleteAsyncMediaPath(file.Path)
		}
	}
}

func StoreAsyncMediaURLResult(job *model.AsyncMediaJob, sourceURL string) (*model.AsyncMediaFile, error) {
	if strings.HasPrefix(sourceURL, "data:image/") || strings.HasPrefix(sourceURL, "data:video/") {
		return storeAsyncMediaDataURI(job, sourceURL)
	}
	return downloadAsyncMediaResult(job, sourceURL)
}

func collectAsyncMediaValues(job *model.AsyncMediaJob, value any, key string, seen map[string]bool, files *[]*model.AsyncMediaFile) error {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			if err := collectAsyncMediaValues(job, child, strings.ToLower(childKey), seen, files); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := collectAsyncMediaValues(job, child, key, seen, files); err != nil {
				return err
			}
		}
	case string:
		if typed == "" || seen[typed] {
			return nil
		}
		if strings.HasPrefix(typed, "data:image/") || strings.HasPrefix(typed, "data:video/") {
			stored, err := storeAsyncMediaDataURI(job, typed)
			if err != nil {
				return err
			}
			seen[typed] = true
			*files = append(*files, stored)
			return nil
		}
		if key == "b64_json" || key == "base64" || key == "binary_data_base64" {
			stored, err := storeAsyncMediaBase64(job, typed, "image/png")
			if err != nil {
				return err
			}
			seen[typed] = true
			*files = append(*files, stored)
			return nil
		}
		if isAsyncMediaURLKey(key) && (strings.HasPrefix(typed, "https://") || strings.HasPrefix(typed, "http://")) {
			stored, err := downloadAsyncMediaResult(job, typed)
			if err != nil {
				return err
			}
			seen[typed] = true
			*files = append(*files, stored)
		}
	}
	return nil
}

func isAsyncMediaURLKey(key string) bool {
	switch key {
	case "url", "image_url", "video_url", "download_url", "result_url":
		return true
	default:
		return false
	}
}

func storeAsyncMediaDataURI(job *model.AsyncMediaJob, dataURI string) (*model.AsyncMediaFile, error) {
	comma := strings.IndexByte(dataURI, ',')
	if comma <= 5 || !strings.Contains(dataURI[:comma], ";base64") {
		return nil, errors.New("unsupported media data URI")
	}
	mediaType := strings.TrimPrefix(strings.Split(dataURI[:comma], ";")[0], "data:")
	return storeAsyncMediaBase64(job, dataURI[comma+1:], mediaType)
}

func storeAsyncMediaBase64(job *model.AsyncMediaJob, encoded string, mediaType string) (*model.AsyncMediaFile, error) {
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded))
	stored, err := storeAsyncMediaReader(job, mediaType, decoder)
	if err == nil || !strings.Contains(err.Error(), "illegal base64") {
		return stored, err
	}
	decoder = base64.NewDecoder(base64.RawStdEncoding, strings.NewReader(encoded))
	return storeAsyncMediaReader(job, mediaType, decoder)
}

func downloadAsyncMediaResult(job *model.AsyncMediaJob, sourceURL string) (*model.AsyncMediaFile, error) {
	resp, err := DoDownloadRequest(sourceURL, "async media result")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("media download returned status %d", resp.StatusCode)
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(mediaType, "image/") && !strings.HasPrefix(mediaType, "video/") && mediaType != "application/octet-stream" {
		return nil, fmt.Errorf("downloaded result has unsupported content type %q", mediaType)
	}
	return storeAsyncMediaReader(job, mediaType, resp.Body)
}

func storeAsyncMediaReader(job *model.AsyncMediaJob, mediaType string, reader io.Reader) (*model.AsyncMediaFile, error) {
	fileID, err := model.GenerateAsyncMediaFileID()
	if err != nil {
		return nil, err
	}
	extension := ".bin"
	if extensions, _ := mime.ExtensionsByType(mediaType); len(extensions) > 0 {
		extension = extensions[0]
	}
	relative, absolute, err := asyncMediaPath("result", fileID+extension)
	if err != nil {
		return nil, err
	}
	destination, err := os.OpenFile(absolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	maxBytes := int64(constant.AsyncMediaMaxFileMB) * 1024 * 1024
	written, copyErr := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(reader, maxBytes+1))
	closeErr := destination.Close()
	if copyErr != nil || written > maxBytes || closeErr != nil {
		_ = os.Remove(absolute)
		if written > maxBytes {
			return nil, errors.New("async media file exceeds ASYNC_MEDIA_MAX_FILE_MB")
		}
		if copyErr != nil {
			return nil, copyErr
		}
		return nil, closeErr
	}
	if mediaType == "" || mediaType == "application/octet-stream" {
		probe, err := os.Open(absolute)
		if err == nil {
			buffer := make([]byte, 512)
			n, _ := probe.Read(buffer)
			_ = probe.Close()
			if n > 0 {
				mediaType = http.DetectContentType(buffer[:n])
			}
		}
	}
	now := time.Now().Unix()
	return &model.AsyncMediaFile{
		FileID:    fileID,
		JobID:     job.JobID,
		UserID:    job.UserID,
		Path:      relative,
		MimeType:  mediaType,
		Size:      written,
		SHA256:    hex.EncodeToString(hash.Sum(nil)),
		ExpiresAt: now + int64(constant.AsyncMediaRetentionHours)*3600,
		CreatedAt: now,
	}, nil
}
