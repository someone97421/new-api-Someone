package jsplugin

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// submitEventStream is how one SSE submission is read: client, when set, is
// the client of a native route, which receives every upstream event as it
// arrives, and frameLimit bounds each event.
type submitEventStream struct {
	client     *gin.Context
	frameLimit int64
}

// readSubmitEvents owns only SSE framing and bounded JSON state. The plugin
// interprets data, accumulates provider output and decides when it is
// complete. Native clients receive frames before the plugin reads them.
// Once a frame reaches the client, the submission settles instead of failing.
// When the plugin reports completion or the upstream closes the stream, the
// last result settles as usual. Other errors stop the stream and keep the
// reserved charge because the plugin did not see the end of the vendor's work.
// A result that invalid changes left half applied is never published.
func (a *TaskAdaptor) readSubmitEvents(parent context.Context, resp *http.Response, driverContext map[string]any, stream submitEventStream) (_ any, stopped bool, _ error) {
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/event-stream" {
		return nil, false, fmt.Errorf("expected a text/event-stream submit response")
	}
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	stopClose := context.AfterFunc(ctx, func() { _ = resp.Body.Close() })
	defer stopClose()
	idleTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Second
	}
	timer := time.AfterFunc(idleTimeout, func() { cancel(fmt.Errorf("task submit stream idle timeout")) })
	defer timer.Stop()

	scanner := helper.NewStreamScanner(resp.Body, int(stream.frameLimit)+1)
	hook := "parseSubmitEvent"
	var accumulated *pluginruntime.JSONState
	if pluginruntime.UsesSubmitEventDelta(a.plugin.Meta, a.plugin.Engine) {
		hook = "parseSubmitEventDelta"
		accumulated = pluginruntime.NewJSONState(maxTaskPluginPersistedJSONBytes)
	}
	rewritePublicID := stream.client != nil && slices.Contains(a.plugin.Meta.RequiredCapabilities, pluginruntime.CapabilitySubmitSSEPublicID)
	publicTaskID, _ := driverContext["publicTaskId"].(string)
	var state any
	var data []string
	var eventName, lastID string
	var frame strings.Builder
	relayed, completed := false, false
	var readErr error
	frameBytes := 0
	firstLine := true
	for scanner.Scan() {
		if readErr = context.Cause(ctx); readErr != nil {
			break
		}
		timer.Reset(idleTimeout)
		line := scanner.Text()
		if firstLine {
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		// The scanner handles LF and CRLF. A remaining CR could create a new
		// data line for the client that the host never inspected.
		if rewritePublicID && strings.ContainsRune(line, '\r') {
			readErr = fmt.Errorf("unsupported line ending in native task submit stream")
			break
		}
		frameBytes += len(line) + 1
		if int64(frameBytes) > stream.frameLimit {
			readErr = fmt.Errorf("task submit SSE event exceeds size limit")
			break
		}
		if stream.client != nil {
			frame.WriteString(line)
			frame.WriteByte('\n')
		}
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			value, _ = strings.CutPrefix(value, " ")
			switch field {
			case "data":
				data = append(data, value)
			case "event":
				eventName = value
			case "id":
				if !strings.ContainsRune(value, '\x00') {
					lastID = value
				}
			}
			continue
		}
		frameBytes = 0
		payload := strings.Join(data, "\n")
		if stream.client != nil {
			clientFrame := frame.String()
			if rewritePublicID && len(data) != 0 {
				clientFrame, readErr = rewriteSubmitEventPublicID(clientFrame, payload, publicTaskID)
				if readErr != nil {
					break
				}
			}
			relaySubmitEvent(stream.client, clientFrame)
			frame.Reset()
			relayed = true
		}
		if len(data) == 0 {
			eventName = ""
			continue
		}
		if eventName == "" {
			eventName = "message"
		}
		event := map[string]any{"event": eventName, "id": lastID, "data": payload}
		value, err := a.plugin.Engine.Call(ctx, hook, driverContext, event, state)
		if err != nil {
			readErr = err
			break
		}
		result, ok := value.(map[string]any)
		if !ok {
			readErr = fmt.Errorf("%s must return an object", hook)
			break
		}
		nextState, hasState := result["state"]
		done, hasDone := result["done"].(bool)
		if !hasState || !hasDone {
			readErr = fmt.Errorf("%s must return state and a boolean done", hook)
			break
		}
		encoded, err := common.Marshal(nextState)
		if err != nil {
			readErr = fmt.Errorf("invalid submit stream state: %w", err)
			break
		}
		if len(encoded) > maxTaskPluginPersistedJSONBytes {
			readErr = fmt.Errorf("task submit stream state exceeds size limit")
			break
		}
		if accumulated != nil {
			if len(result) != 3 {
				readErr = fmt.Errorf("parseSubmitEventDelta must return only changes, state and done")
				break
			}
			// Control state is separate from the full result and is the only
			// snapshot passed back to JS on the next event.
			if len(encoded) > 64<<10 {
				readErr = fmt.Errorf("task submit stream control state exceeds size limit")
				break
			}
			if err = accumulated.Apply(ctx, result["changes"]); err != nil {
				readErr = fmt.Errorf("invalid submit stream changes: %w", err)
				accumulated, state = nil, nil
				break
			}
		}
		// Keep the exact encoded-byte limit above. Plain JSON state goes back
		// to the next event as is; codec-specific values (for example exported
		// typed arrays) retain the old normalization.
		if isPlainJSONValue(nextState, 0) {
			state = nextState
		} else {
			var decoded any
			if err = common.Unmarshal(encoded, &decoded); err != nil {
				readErr = err
				break
			}
			state = decoded
		}
		if done {
			completed = true
			break
		}
		data = nil
		eventName = ""
	}
	if !completed && readErr == nil {
		readErr = context.Cause(ctx)
	}
	if !completed && readErr == nil && scanner.Err() != nil {
		readErr = fmt.Errorf("read task submit stream: %w", scanner.Err())
	}
	result := state
	if accumulated != nil {
		var valueErr error
		if result, valueErr = accumulated.Value(); valueErr != nil && readErr == nil {
			readErr = valueErr
		}
	}
	switch {
	case readErr == nil && completed:
		return result, false, nil
	case !relayed:
		if readErr == nil {
			readErr = fmt.Errorf("task submit stream ended before the plugin reported completion")
		}
		return nil, false, readErr
	case readErr == nil:
		logger.LogDebug(stream.client, "task_plugin subsystem=adaptor event=relayed_stream_closed plugin=%q reason=eof_before_done", a.plugin.Meta.Key)
		return result, false, nil
	default:
		logger.LogWarn(stream.client, common.LogText("task plugin %s settles a relayed submit stream that stopped early at the reserved quota: %s", a.plugin.Meta.Key, readErr.Error()))
		// The client's stream is cut off rather than closed, so a downstream
		// gateway relaying it keeps its reservation too.
		_ = http.NewResponseController(stream.client.Writer).SetWriteDeadline(time.Now())
		return result, true, nil
	}
}

// rewriteSubmitEventPublicID replaces only the JSON string token in its
// original SSE data line. A valid string token cannot span joined data lines.
func rewriteSubmitEventPublicID(frame, payload, publicTaskID string) (string, error) {
	if !strings.HasPrefix(strings.TrimLeft(payload, " \t\r\n"), "{") {
		return frame, nil
	}
	if err := common.UnmarshalJsonStr(payload, &struct{}{}); err != nil {
		return "", fmt.Errorf("invalid JSON object in native task submit stream")
	}
	var upstreamID gjson.Result
	idCount := 0
	gjson.Parse(payload).ForEach(func(key, value gjson.Result) bool {
		if key.Str == "id" {
			idCount++
			if value.Type == gjson.String {
				upstreamID = value
			}
		}
		return true
	})
	if !upstreamID.Exists() {
		return frame, nil
	}
	if idCount != 1 || publicTaskID == "" {
		return "", fmt.Errorf("cannot replace native task submit stream ID")
	}
	encoded, err := common.Marshal(publicTaskID)
	if err != nil {
		return "", fmt.Errorf("cannot encode native task submit stream public ID: %w", err)
	}
	frameOffset, payloadOffset := 0, 0
	for line := range strings.SplitAfterSeq(frame, "\n") {
		lineText, _ := strings.CutSuffix(line, "\n")
		field, value, _ := strings.Cut(lineText, ":")
		if field == "data" {
			value, _ = strings.CutPrefix(value, " ")
			start := upstreamID.Index - payloadOffset
			if end := start + len(upstreamID.Raw); start >= 0 && end <= len(value) && value[start:end] == upstreamID.Raw {
				start += frameOffset + len(lineText) - len(value)
				return frame[:start] + string(encoded) + frame[start+len(upstreamID.Raw):], nil
			}
			payloadOffset += len(value) + 1
		}
		frameOffset += len(line)
	}
	return "", fmt.Errorf("cannot locate native task submit stream ID in SSE data")
}

// relaySubmitEvent writes one SSE event to a native client. The first event
// commits a 200 event-stream response.
func relaySubmitEvent(c *gin.Context, frame string) {
	helper.SetEventStreamHeaders(c)
	_, _ = c.Writer.WriteString(frame)
	c.Writer.Flush()
}
