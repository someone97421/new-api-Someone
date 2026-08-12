package common

import (
	"fmt"
	"strings"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ApplyProtocolBridgeConfig copies explicitly allowed fields from the original
// downstream JSON request into an already converted upstream JSON body.
func ApplyProtocolBridgeConfig(c *gin.Context, target []byte, info *RelayInfo) ([]byte, error) {
	if c == nil || info == nil || info.ChannelMeta == nil {
		return target, nil
	}
	config := info.ChannelOtherSettings.ProtocolBridge
	if config == nil || (len(config.PassthroughFields) == 0 && len(config.FieldMappings) == 0) {
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
	data, err := rootcommon.Marshal(value)
	if err != nil {
		return nil, err
	}
	data, err = ApplyProtocolBridgeConfig(c, data, info)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := rootcommon.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
