package service

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const (
	maxTaskLogBodyBytes  = 64 * 1024
	maxTaskLogStringSize = 16 * 1024
)

// SanitizeTaskLogBody returns a bounded, display-ready copy of an async task
// request or response body. Credentials and embedded binary payloads are
// removed before the text is persisted in the task record.
func SanitizeTaskLogBody(body []byte, contentType string) string {
	if len(body) == 0 {
		return ""
	}

	mediaType, params, _ := mime.ParseMediaType(contentType)
	var sanitized string
	switch {
	case mediaType == "multipart/form-data" && params["boundary"] != "":
		sanitized = sanitizeMultipartTaskBody(body, params["boundary"])
	case mediaType == "application/x-www-form-urlencoded":
		sanitized = sanitizeFormTaskBody(body)
	default:
		sanitized = sanitizeJSONTaskBody(body)
	}
	return truncateTaskLogBody(sanitized)
}

func sanitizeJSONTaskBody(body []byte) string {
	var value any
	if err := common.Unmarshal(body, &value); err != nil {
		return sanitizeTaskLogString(string(body))
	}
	value = sanitizeTaskLogValue("", value)
	sanitized, err := common.Marshal(value)
	if err != nil {
		return sanitizeTaskLogString(string(body))
	}
	return string(sanitized)
}

func sanitizeFormTaskBody(body []byte) string {
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return sanitizeTaskLogString(string(body))
	}
	for key, items := range values {
		for index, item := range items {
			if isSensitiveTaskLogKey(key) {
				values[key][index] = "***masked***"
			} else {
				values[key][index] = sanitizeTaskLogString(item)
			}
		}
	}
	return values.Encode()
}

func sanitizeMultipartTaskBody(body []byte, boundary string) string {
	reader := multipart.NewReader(strings.NewReader(string(body)), boundary)
	fields := make(map[string]any)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "[multipart body could not be decoded]"
		}

		name := part.FormName()
		if name == "" {
			name = "unnamed"
		}
		if filename := part.FileName(); filename != "" {
			size, _ := io.Copy(io.Discard, part)
			fields[name] = map[string]any{
				"filename":     sanitizeTaskLogFilename(filename),
				"content_type": part.Header.Get("Content-Type"),
				"size":         size,
				"content":      "[binary omitted]",
			}
			continue
		}

		data, _ := io.ReadAll(io.LimitReader(part, maxTaskLogStringSize+1))
		if isSensitiveTaskLogKey(name) {
			fields[name] = "***masked***"
		} else {
			fields[name] = sanitizeTaskLogString(string(data))
		}
	}
	sanitized, err := common.Marshal(fields)
	if err != nil {
		return "[multipart body could not be encoded]"
	}
	return string(sanitized)
}

func sanitizeTaskLogValue(key string, value any) any {
	if isSensitiveTaskLogKey(key) {
		return "***masked***"
	}
	if isBinaryTaskLogKey(key) {
		return "[binary omitted]"
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			typed[childKey] = sanitizeTaskLogValue(childKey, childValue)
		}
		return typed
	case []any:
		for index, item := range typed {
			typed[index] = sanitizeTaskLogValue(key, item)
		}
		return typed
	case string:
		return sanitizeTaskLogString(typed)
	default:
		return value
	}
}

func isBinaryTaskLogKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	return strings.Contains(normalized, "base64") ||
		normalized == "bytes" ||
		normalized == "binary"
}

func sanitizeTaskLogString(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return fmt.Sprintf("[data URI omitted: %d characters]", len(value))
	}
	value = relaycommon.SanitizeURLForLog(value)
	value = common.MaskSensitiveInfo(value)
	if len(value) > maxTaskLogStringSize {
		return value[:maxTaskLogStringSize] + "... [truncated]"
	}
	return value
}

func sanitizeTaskLogFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if len(value) > 255 {
		return value[:255] + "..."
	}
	return value
}

func isSensitiveTaskLogKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	switch normalized {
	case "key", "token", "auth", "authorization", "cookie", "setcookie",
		"password", "passwd", "apikey", "xapikey", "accesstoken",
		"refreshtoken", "idtoken", "clientsecret", "privatekey",
		"webhooksecret", "signature", "credential":
		return true
	}
	return strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "privatekey") ||
		strings.Contains(normalized, "accesstoken") ||
		strings.Contains(normalized, "refreshtoken") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "signature")
}

func truncateTaskLogBody(value string) string {
	if len(value) <= maxTaskLogBodyBytes {
		return value
	}
	return value[:maxTaskLogBodyBytes] + "\n... [truncated]"
}
