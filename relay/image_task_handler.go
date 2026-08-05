package relay

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ImageTaskFetch forwards an asynchronous image task query to the selected
// channel. The public route is stable while each channel configures its own
// upstream query path through image_task_endpoints.query_path.
func ImageTaskFetch(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil || info.ChannelMeta == nil {
		return types.NewErrorWithStatusCode(errors.New("channel metadata is required"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		return types.NewErrorWithStatusCode(errors.New("task_id is required"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	endpoints := info.ChannelOtherSettings.ImageTaskEndpoints
	if endpoints == nil || strings.TrimSpace(endpoints.QueryPath) == "" {
		return types.NewErrorWithStatusCode(errors.New("image_task_endpoints.query_path is required for image task queries"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if err := endpoints.Validate(); err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	baseURL := strings.TrimRight(strings.TrimSpace(info.ChannelBaseUrl), "/")
	if baseURL == "" {
		return types.NewErrorWithStatusCode(errors.New("channel base URL is required"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	queryPath := strings.Replace(strings.TrimSpace(endpoints.QueryPath), "{task_id}", url.PathEscape(taskID), 1)
	requestURL := baseURL + queryPath
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, requestURL, http.NoBody)
	if err != nil {
		return types.NewError(fmt.Errorf("create image task query request failed: %w", err), types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}

	channel.SetupApiRequestHeader(info, c, &req.Header)
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	headerOverride, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelHeaderOverrideInvalid, types.ErrOptionWithSkipRetry())
	}
	for key, value := range headerOverride {
		req.Header.Set(key, value)
		if strings.EqualFold(key, "Host") {
			req.Host = value
		}
	}

	resp, err := channel.DoRequest(c, req, info)
	if err != nil {
		return types.NewError(fmt.Errorf("query upstream image task failed: %w", err), types.ErrorCodeDoRequestFailed)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return service.RelayErrorHandler(c.Request.Context(), resp, false)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeReadResponseBodyFailed, types.ErrOptionWithSkipRetry())
	}
	info.SetFirstResponseTime()
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return nil
}
