package system_setting

import (
	"errors"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const FileRelaySettingsKey = "FileRelaySettings"

type FileRelaySettings struct {
	Enabled                bool   `json:"enabled"`
	Directory              string `json:"directory"`
	PublicURL              string `json:"public_url"`
	RetentionHours         int    `json:"retention_hours"`
	CleanupIntervalMinutes int    `json:"cleanup_interval_minutes"`
	DownloadTimeoutSeconds int    `json:"download_timeout_seconds"`
	RetryCount             int    `json:"retry_count"`
	Strict                 bool   `json:"strict"`
}

func DefaultFileRelaySettings() FileRelaySettings {
	return FileRelaySettings{Directory: "./data/file-relay", RetentionHours: 168, CleanupIntervalMinutes: 10, DownloadTimeoutSeconds: 60, RetryCount: 2}
}

func ParseFileRelaySettings(raw string) (FileRelaySettings, error) {
	cfg := DefaultFileRelaySettings()
	if raw != "" {
		if err := common.UnmarshalJsonStr(raw, &cfg); err != nil {
			return cfg, errors.New("invalid file relay settings JSON")
		}
	}
	cfg.Directory = strings.TrimSpace(cfg.Directory)
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	if cfg.Directory == "" {
		return cfg, errors.New("file relay directory is required")
	}
	if cfg.RetentionHours < 0 || cfg.RetentionHours > 87600 || cfg.CleanupIntervalMinutes < 1 || cfg.CleanupIntervalMinutes > 1440 || cfg.DownloadTimeoutSeconds < 1 || cfg.DownloadTimeoutSeconds > 3600 || cfg.RetryCount < 0 || cfg.RetryCount > 5 {
		return cfg, errors.New("invalid file relay retention, cleanup interval, timeout or retry count")
	}
	if cfg.PublicURL != "" {
		u, err := url.Parse(cfg.PublicURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || strings.ContainsAny(cfg.PublicURL, "?#") {
			return cfg, errors.New("file relay public URL must be an HTTP(S) site URL")
		}
	}
	return cfg, nil
}

func GetFileRelaySettings() FileRelaySettings {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[FileRelaySettingsKey]
	serverAddress := common.OptionMap["ServerAddress"]
	common.OptionMapRWMutex.RUnlock()
	cfg, err := ParseFileRelaySettings(raw)
	if err != nil {
		return DefaultFileRelaySettings()
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = strings.TrimRight(strings.TrimSpace(serverAddress), "/")
	}
	return cfg
}
