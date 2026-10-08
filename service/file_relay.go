package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"golang.org/x/sync/singleflight"
)

const FileRelayContentPath = "/v1/file-relay/content/"

var fileRelayIDPattern = regexp.MustCompile(`^[0-9a-f]{48}$`)
var fileRelayCachePattern = regexp.MustCompile(`^[0-9a-f]{64}\.ref$`)
var fileRelayCleanupOnce sync.Once
var fileRelayDownloads singleflight.Group

// FileRelaySource is shared by uploads, image responses and task artifacts.
// Reader ownership stays with the caller. Only GET downloads are retried.
type FileRelaySource struct {
	URL         string
	Base64      string
	Reader      io.Reader
	Name        string
	ContentType string
	// Supplied only by the already-validated task media proxy, never client JSON.
	Request *http.Request
	Client  *http.Client
}

type RelayedFile struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	ExpiresAt   int64  `json:"expires_at"`
}

func RelayFile(ctx context.Context, cfg system_setting.FileRelaySettings, source FileRelaySource) (*RelayedFile, error) {
	if !cfg.Enabled {
		return nil, errors.New("file relay is disabled")
	}
	if source.Request != nil {
		source.URL = source.Request.URL.String()
	}
	if source.Reader != nil {
		return storeRelayedFile(ctx, cfg, source, source.Reader)
	}
	encoded := source.Base64
	if strings.HasPrefix(source.URL, "data:") {
		encoded = source.URL
	}
	if encoded != "" {
		if strings.HasPrefix(encoded, "data:") {
			header, payload, ok := strings.Cut(encoded, ",")
			if !ok || !strings.HasSuffix(header, ";base64") {
				return nil, errors.New("invalid base64 data URL")
			}
			source.ContentType = strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
			encoded = payload
		}
		encoding := base64.StdEncoding
		if !strings.Contains(encoded, "=") {
			encoding = base64.RawStdEncoding
		}
		return storeRelayedFile(ctx, cfg, source, base64.NewDecoder(encoding, strings.NewReader(encoded)))
	}
	u, err := url.Parse(source.URL)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("invalid file URL")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.DownloadTimeoutSeconds)*time.Second)
	defer cancel()
	client := GetSSRFProtectedHTTPClient()
	if source.Client != nil {
		client = source.Client
	}
	if client == nil {
		return nil, errors.New("file relay HTTP client unavailable")
	}
	attempts := cfg.RetryCount + 1
	if source.Request != nil && source.Request.Method != http.MethodGet {
		attempts = 1
	}
	var lastErr error
	for range attempts {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
		if source.Request != nil {
			req = source.Request.Clone(ctx)
			if source.Request.GetBody != nil {
				req.Body, err = source.Request.GetBody()
			}
		}
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = errors.New("file download failed")
		} else {
			if resp.StatusCode == http.StatusOK {
				downloaded := source
				if downloaded.Name == "" {
					downloaded.Name = filepath.Base(u.Path)
				}
				if downloaded.ContentType == "" {
					downloaded.ContentType = resp.Header.Get("Content-Type")
				}
				result, storeErr := storeRelayedFile(ctx, cfg, downloaded, resp.Body)
				resp.Body.Close()
				if storeErr == nil {
					return result, nil
				}
				lastErr = storeErr
			} else {
				resp.Body.Close()
				lastErr = fmt.Errorf("file download returned HTTP %d", resp.StatusCode)
				if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
					break
				}
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

func storeRelayedFile(ctx context.Context, cfg system_setting.FileRelaySettings, source FileRelaySource, reader io.Reader) (*RelayedFile, error) {
	if err := os.MkdirAll(cfg.Directory, 0750); err != nil {
		return nil, err
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(random)
	temp, err := os.CreateTemp(cfg.Directory, ".upload-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temp.Name())
	reader = &fileRelayContextReader{ctx: ctx, reader: reader}
	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(reader, prefix)
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		temp.Close()
		return nil, readErr
	}
	prefix = prefix[:n]
	if source.ContentType == "" {
		source.ContentType = http.DetectContentType(prefix)
	}
	mediaType, _, err := mime.ParseMediaType(source.ContentType)
	if err != nil {
		mediaType = "application/octet-stream"
	}
	size, err := io.Copy(temp, io.MultiReader(bytes.NewReader(prefix), reader))
	closeErr := temp.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if size == 0 {
		return nil, errors.New("empty file")
	}
	ref := &RelayedFile{ID: id, Name: filepath.Base(source.Name), ContentType: mediaType, Size: size}
	if cfg.RetentionHours != 0 {
		ref.ExpiresAt = time.Now().Add(time.Duration(cfg.RetentionHours) * time.Hour).Unix()
	}
	if ref.Name == "." || ref.Name == "/" || ref.Name == "" {
		ref.Name = "file"
	}
	ref.URL = cfg.PublicURL + FileRelayContentPath + id
	bodyPath := filepath.Join(cfg.Directory, id+".bin")
	if err := os.Rename(temp.Name(), bodyPath); err != nil {
		return nil, err
	}
	metadata, err := common.Marshal(ref)
	if err == nil {
		err = os.WriteFile(filepath.Join(cfg.Directory, id+".json"), metadata, 0600)
	}
	if err != nil {
		os.Remove(bodyPath)
		os.Remove(filepath.Join(cfg.Directory, id+".json"))
		return nil, err
	}
	return ref, nil
}

type fileRelayContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *fileRelayContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func OpenRelayedFile(cfg system_setting.FileRelaySettings, id string) (*RelayedFile, *os.File, error) {
	if !fileRelayIDPattern.MatchString(id) {
		return nil, nil, os.ErrNotExist
	}
	raw, err := os.ReadFile(filepath.Join(cfg.Directory, id+".json"))
	if err != nil {
		return nil, nil, err
	}
	var ref RelayedFile
	if err := common.Unmarshal(raw, &ref); err != nil {
		return nil, nil, err
	}
	if ref.ID != id || ref.ExpiresAt != 0 && ref.ExpiresAt <= time.Now().Unix() {
		return nil, nil, os.ErrNotExist
	}
	file, err := os.Open(filepath.Join(cfg.Directory, id+".bin"))
	return &ref, file, err
}

func FindCachedRelayFile(cfg system_setting.FileRelaySettings, key string) (*RelayedFile, error) {
	digest := sha256.Sum256([]byte(key))
	raw, err := os.ReadFile(filepath.Join(cfg.Directory, hex.EncodeToString(digest[:])+".ref"))
	if err != nil {
		return nil, err
	}
	var ref RelayedFile
	if err := common.Unmarshal(raw, &ref); err != nil {
		return nil, err
	}
	actual, file, err := OpenRelayedFile(cfg, ref.ID)
	if err != nil {
		return nil, err
	}
	file.Close()
	return actual, nil
}

// Persist the cache index beside the files so a restart does not redownload tasks.
func RelayCachedFile(ctx context.Context, cfg system_setting.FileRelaySettings, key string, source FileRelaySource) (*RelayedFile, error) {
	digest := sha256.Sum256([]byte(key))
	indexPath := filepath.Join(cfg.Directory, hex.EncodeToString(digest[:])+".ref")
	result, err, _ := fileRelayDownloads.Do(indexPath, func() (any, error) {
		if ref, err := FindCachedRelayFile(cfg, key); err == nil {
			return ref, nil
		}
		ref, err := RelayFile(ctx, cfg, source)
		if err != nil {
			return nil, err
		}
		raw, err := common.Marshal(ref)
		if err != nil {
			return nil, err
		}
		temp, err := os.CreateTemp(cfg.Directory, ".upload-index-*")
		if err != nil {
			return nil, err
		}
		defer os.Remove(temp.Name())
		_, err = temp.Write(raw)
		closeErr := temp.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if err := os.Rename(temp.Name(), indexPath); err != nil {
			return nil, err
		}
		return ref, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*RelayedFile), nil
}

// Reads enforce expiry immediately; cleanup only handles physical deletion.
func CleanupRelayedFiles(cfg system_setting.FileRelaySettings, now time.Time) error {
	entries, err := os.ReadDir(cfg.Directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		id := strings.TrimSuffix(name, ".json")
		metadata := strings.HasSuffix(name, ".json") && fileRelayIDPattern.MatchString(id)
		if metadata || fileRelayCachePattern.MatchString(name) {
			raw, err := os.ReadFile(filepath.Join(cfg.Directory, name))
			if err != nil {
				continue
			}
			var ref RelayedFile
			if common.Unmarshal(raw, &ref) != nil || ref.ExpiresAt == 0 || ref.ExpiresAt > now.Unix() {
				continue
			}
			if metadata {
				if err := os.Remove(filepath.Join(cfg.Directory, id+".bin")); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			if err := os.Remove(filepath.Join(cfg.Directory, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		} else if strings.HasPrefix(name, ".upload-") {
			info, err := entry.Info()
			if err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
				_ = os.Remove(filepath.Join(cfg.Directory, name))
			}
		} else if id, ok := strings.CutSuffix(name, ".bin"); ok && fileRelayIDPattern.MatchString(id) {
			// Recover bodies left behind by a process crash before metadata was committed.
			info, err := entry.Info()
			if err == nil && now.Sub(info.ModTime()) > 24*time.Hour {
				if _, err := os.Stat(filepath.Join(cfg.Directory, id+".json")); os.IsNotExist(err) {
					_ = os.Remove(filepath.Join(cfg.Directory, name))
				}
			}
		}
	}
	return nil
}

func StartFileRelayCleanup(ctx context.Context) {
	fileRelayCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			var last time.Time
			for {
				cfg := system_setting.GetFileRelaySettings()
				if time.Since(last) >= time.Duration(cfg.CleanupIntervalMinutes)*time.Minute {
					if err := CleanupRelayedFiles(cfg, time.Now()); err != nil {
						common.SysError("file relay cleanup failed: " + err.Error())
					}
					last = time.Now()
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}
