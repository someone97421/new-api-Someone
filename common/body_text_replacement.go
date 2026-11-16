package common

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// ReplaceJSONBodyText preserves member order, keys and non-string tokens.
func ReplaceJSONBodyText(body []byte, policy *dto.BodyTextReplacement) ([]byte, error) {
	if policy == nil || !policy.Enabled || len(policy.Rules) == 0 {
		return body, nil
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("body text replacement requires valid JSON")
	}
	var out bytes.Buffer
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '"' {
			continue
		}
		end := i + 1
		for end < len(body) {
			if body[end] == '\\' {
				end += 2
				continue
			}
			if body[end] == '"' {
				break
			}
			end++
		}
		end++
		next := end
		for next < len(body) && strings.ContainsRune(" \t\r\n", rune(body[next])) {
			next++
		}
		if next == len(body) || body[next] != ':' {
			var value string
			if err := Unmarshal(body[i:end], &value); err != nil {
				return nil, err
			}
			replaced := policy.Replace(value)
			if replaced != value {
				encoded, err := Marshal(replaced)
				if err != nil {
					return nil, err
				}
				out.Write(body[start:i])
				out.Write(encoded)
				start = end
			}
		}
		i = end - 1
	}
	out.Write(body[start:])
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	if int64(out.Len()) > int64(maxMB)<<20 {
		return nil, ErrRequestBodyTooLarge
	}
	return out.Bytes(), nil
}

// ApplyChannelBodyTextReplacement installs an attempt-local body and restores
// the original request when the caller finishes the attempt.
func ApplyChannelBodyTextReplacement(c *gin.Context, policy *dto.BodyTextReplacement) (func(), error) {
	noop := func() {}
	if err := policy.Validate(); err != nil {
		return noop, err
	}
	original, err := GetBodyStorage(c)
	if err != nil {
		return noop, err
	}
	r := c.Request
	oldBody, oldLength, oldHeaders := r.Body, r.ContentLength, r.Header.Clone()
	oldForm, oldPostForm, oldMultipart := r.Form, r.PostForm, r.MultipartForm
	cachedBody, _ := c.Get(KeyRequestBody)
	cachedType, hadType := c.Get("_original_multipart_ct")
	var storage BodyStorage
	restore := func() {
		if r.MultipartForm != nil && r.MultipartForm != oldMultipart {
			_ = r.MultipartForm.RemoveAll()
		}
		if storage != nil {
			_ = storage.Close()
		}
		c.Set(KeyBodyStorage, original)
		c.Set(KeyRequestBody, cachedBody)
		if hadType {
			c.Set("_original_multipart_ct", cachedType)
		} else {
			c.Set("_original_multipart_ct", oldHeaders.Get("Content-Type"))
		}
		r.Body, r.ContentLength, r.Header = oldBody, oldLength, oldHeaders
		r.Form, r.PostForm, r.MultipartForm = oldForm, oldPostForm, oldMultipart
	}
	if policy == nil || !policy.Enabled || len(policy.Rules) == 0 {
		return restore, nil
	}
	body, err := original.Bytes()
	if err != nil {
		return noop, err
	}
	contentType := c.GetHeader("Content-Type")
	if hadType && strings.HasPrefix(contentType, "multipart/") {
		contentType, _ = cachedType.(string)
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return noop, err
	}
	var replaced []byte
	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		replaced, err = ReplaceJSONBodyText(body, policy)
	case mediaType == "multipart/form-data":
		var output bytes.Buffer
		writer := multipart.NewWriter(&output)
		reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
		for {
			part, readErr := reader.NextRawPart()
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return noop, readErr
			}
			destination, writeErr := writer.CreatePart(part.Header)
			if writeErr != nil {
				return noop, writeErr
			}
			_, disposition, parseErr := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			if parseErr != nil {
				return noop, parseErr
			}
			if _, file := disposition["filename"]; file {
				_, err = io.Copy(destination, part)
			} else {
				text, textErr := io.ReadAll(part)
				if textErr != nil {
					return noop, textErr
				}
				_, err = io.WriteString(destination, policy.Replace(string(text)))
			}
			if err != nil {
				return noop, err
			}
			part.Close()
		}
		err = writer.Close()
		replaced = output.Bytes()
		contentType = writer.FormDataContentType()
	case mediaType == "application/x-www-form-urlencoded":
		var values url.Values
		values, err = url.ParseQuery(string(body))
		if err == nil {
			for key, entries := range values {
				for i, entry := range entries {
					values[key][i] = policy.Replace(entry)
				}
			}
			replaced = []byte(values.Encode())
		}
	default:
		return noop, fmt.Errorf("body text replacement does not support content type %q", mediaType)
	}
	if err != nil {
		return noop, err
	}
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	if int64(len(replaced)) > int64(maxMB)<<20 {
		return noop, ErrRequestBodyTooLarge
	}
	storage, err = CreateBodyStorage(replaced)
	if err != nil {
		return noop, err
	}
	c.Set(KeyBodyStorage, storage)
	c.Set(KeyRequestBody, replaced)
	c.Set("_original_multipart_ct", contentType)
	r.Body = io.NopCloser(storage)
	r.ContentLength = int64(len(replaced))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Content-Length", strconv.Itoa(len(replaced)))
	r.Form, r.PostForm, r.MultipartForm = nil, nil, nil
	return restore, nil
}
