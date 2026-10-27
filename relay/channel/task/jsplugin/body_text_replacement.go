package jsplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

func replaceTaskRequestBody(request pluginruntime.RouteRequestContext, policy *dto.BodyTextReplacement) (pluginruntime.RouteRequestContext, error) {
	if policy == nil || !policy.Enabled || len(policy.Rules) == 0 {
		return request, nil
	}
	if err := policy.Validate(); err != nil {
		return request, err
	}
	body, ok := request.Body.(map[string]any)
	if !ok {
		return request, fmt.Errorf("task request body is unavailable for text replacement")
	}
	body = maps.Clone(body)
	switch body["kind"] {
	case string(pluginruntime.BodyJSON):
		text := request.BodyText
		if len(text) == 0 {
			var err error
			text, err = common.Marshal(body["value"])
			if err != nil {
				return request, err
			}
		}
		replaced, err := common.ReplaceJSONBodyText(text, policy)
		if err != nil {
			return request, err
		}
		var value any
		if err := common.Unmarshal(replaced, &value); err != nil {
			return request, err
		}
		body["value"] = value
		request.BodyText = replaced
		request.RequestBody = value
	case string(pluginruntime.BodyMultipart), string(pluginruntime.BodyForm):
		fields, ok := body["fields"].(map[string][]string)
		if !ok {
			return request, fmt.Errorf("task text fields are unavailable for replacement")
		}
		replaced := make(map[string][]string, len(fields))
		for key, values := range fields {
			entries := make([]string, len(values))
			for i, value := range values {
				entries[i] = policy.Replace(value)
			}
			replaced[key] = entries
		}
		body["fields"] = replaced
		request.RequestBody = replaced
	default:
		return request, fmt.Errorf("task request body does not support text replacement")
	}
	request.Body = body
	return request, nil
}

func (a *TaskAdaptor) decodeChannelRequest(c *gin.Context, info *relaycommon.RelayInfo) error {
	var (
		request pluginruntime.RouteRequestContext
		hook    string
		path    []string
		ctx     map[string]any
		err     error
	)
	policy := info.ChannelSetting.BodyTextReplacement
	if value, exists := c.Get(pluginruntime.ContextKeyPinnedEndpoint); exists {
		pinned, ok := value.(pluginruntime.PinnedEndpoint)
		if !ok || pinned.Plugin != a.plugin {
			return nil
		}
		value, exists = c.Get(pluginruntime.ContextKeyProtocolRequest)
		protocol, ok := value.(pluginruntime.ProtocolRequestContext)
		if !exists || !ok {
			return nil
		}
		request, err = replaceTaskRequestBody(protocol.RouteRequestContext, policy)
		if err != nil {
			return err
		}
		protocol.RouteRequestContext = request
		protocol.UpstreamModel = info.UpstreamModelName
		ctx = protocol.JSValueFor(a.plugin.Meta)
		hook, path = "protocols", []string{pinned.Protocol, "decodeRequest"}
	} else if value, exists := c.Get(pluginruntime.ContextKeyPinnedRoute); exists {
		pinned, ok := value.(pluginruntime.PinnedRoute)
		if !ok || pinned.Plugin != a.plugin {
			return nil
		}
		value, _ = c.Get(pluginruntime.ContextKeyRouteRequest)
		original, ok := value.(pluginruntime.RouteRequestContext)
		if !ok {
			return fmt.Errorf("task native context is unavailable")
		}
		request, err = replaceTaskRequestBody(original, policy)
		if err != nil {
			return err
		}
		ctx = request.JSValueFor(a.plugin.Meta)
		hook, path = "native", []string{pinned.Route.Decode}
	} else {
		return nil
	}
	value, text, err := a.plugin.Engine.CallPathWithMemberJSON(
		context.WithoutCancel(c.Request.Context()), 0, a.plugin.Meta.JSONTextMember("requestBody"), hook, path, ctx,
	)
	if err != nil {
		return err
	}
	resolved, ok := value.(map[string]any)
	if !ok || resolved["kind"] != "submit" {
		return fmt.Errorf("final task plugin decoder must return a submit intent")
	}
	model, ok := resolved["model"].(string)
	if !ok || (model != info.OriginModelName && model != info.ChannelRequestModel()) {
		return fmt.Errorf("final task plugin decoder rejected the pinned model")
	}
	if _, forbidden := resolved["renderer"]; forbidden {
		return fmt.Errorf("decoder must not return renderer")
	}
	if policy != nil && policy.Enabled {
		if ids, present := resolved["originTaskIds"]; present {
			encoded, err := common.Marshal(ids)
			if err != nil {
				return err
			}
			var taskIDs []string
			if err := common.Unmarshal(encoded, &taskIDs); err != nil {
				return err
			}
			for _, id := range taskIDs {
				owned := false
				if info.TaskRelayInfo != nil {
					for _, task := range info.OriginTasks {
						owned = owned || task.TaskID == id
					}
				}
				if !owned {
					return fmt.Errorf("body text replacement changed a pinned origin task")
				}
			}
		}
	}
	body := request.RequestBody
	if replacement, present := resolved["requestBody"]; present {
		body = replacement
	}
	c.Set("task_request", body)
	c.Set(pluginruntime.ContextKeyRequestBodyText, json.RawMessage(text))
	if action, ok := resolved["action"].(string); ok && strings.TrimSpace(action) != "" {
		c.Set("task_action", action)
		info.Action = action
	}
	return nil
}
