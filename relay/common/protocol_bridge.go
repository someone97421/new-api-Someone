package common

import (
	"fmt"
	"strings"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ApplyProtocolBridgeConfig copies explicitly allowed fields from the original
// downstream JSON request into an already converted upstream JSON body.
func ApplyProtocolBridgeConfig(c *gin.Context, target []byte, info *RelayInfo) ([]byte, error) {
	if info == nil || info.ChannelMeta == nil {
		return target, nil
	}
	return ApplyProtocolBridgeFields(c, target, info.ChannelOtherSettings.ProtocolBridge)
}

// ApplyProtocolBridgeFields copies explicitly allowed fields from the original
// downstream JSON request into an already converted upstream JSON body.
func ApplyProtocolBridgeFields(c *gin.Context, target []byte, config *dto.ProtocolBridgeConfig) ([]byte, error) {
	if config == nil || (len(config.PassthroughFields) == 0 && len(config.FieldMappings) == 0) {
		return target, nil
	}
	if c == nil {
		return target, nil
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	storage, err := rootcommon.GetBodyStorage(c)
	if err != nil {
		return nil, fmt.Errorf("read protocol bridge source request: %w", err)
	}
	source, err := storage.Bytes()
	if err != nil {
		return nil, fmt.Errorf("read protocol bridge source request: %w", err)
	}
	if !gjson.ValidBytes(source) {
		return target, nil
	}

	result := target
	for sourcePath, targetPath := range config.FieldMappings {
		result, err = copyProtocolBridgeJSONPath(result, source, sourcePath, targetPath)
		if err != nil {
			return nil, err
		}
	}
	for _, path := range config.PassthroughFields {
		result, err = copyProtocolBridgeJSONPath(result, source, path, path)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func copyProtocolBridgeJSONPath(target []byte, source []byte, sourcePath string, targetPath string) ([]byte, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	targetPath = strings.TrimSpace(targetPath)
	value := gjson.GetBytes(source, sourcePath)
	if !value.Exists() && value.Raw == "" {
		return target, nil
	}
	raw := []byte(value.Raw)
	if len(raw) == 0 {
		var err error
		raw, err = rootcommon.Marshal(value.Value())
		if err != nil {
			return nil, fmt.Errorf("marshal protocol bridge field %s: %w", sourcePath, err)
		}
	}
	result, err := sjson.SetRawBytes(target, targetPath, raw)
	if err != nil {
		return nil, fmt.Errorf("copy protocol bridge field %s to %s: %w", sourcePath, targetPath, err)
	}
	return result, nil
}

func MarshalAndApplyProtocolBridge(c *gin.Context, value any, info *RelayInfo) (map[string]any, error) {
	if info == nil || info.ChannelMeta == nil {
		return marshalJSONObject(value)
	}
	return MarshalAndApplyProtocolBridgeFields(c, value, info.ChannelOtherSettings.ProtocolBridge)
}

func MarshalAndApplyProtocolBridgeFields(c *gin.Context, value any, config *dto.ProtocolBridgeConfig) (map[string]any, error) {
	data, err := rootcommon.Marshal(value)
	if err != nil {
		return nil, err
	}
	data, err = ApplyProtocolBridgeFields(c, data, config)
	if err != nil {
		return nil, err
	}
	return unmarshalJSONObject(data)
}

func marshalJSONObject(value any) (map[string]any, error) {
	data, err := rootcommon.Marshal(value)
	if err != nil {
		return nil, err
	}
	return unmarshalJSONObject(data)
}

func unmarshalJSONObject(data []byte) (map[string]any, error) {
	var result map[string]any
	if err := rootcommon.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
