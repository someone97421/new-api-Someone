package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestProtocolBridgeAlwaysDisablesRawBodyPassthrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = true
	t.Cleanup(func() { settings.PassThroughRequestEnabled = original })

	for _, channelType := range []int{constant.ChannelTypeGemini2GPT, constant.ChannelTypeGPT2Gemini} {
		info := &RelayInfo{ChannelMeta: &ChannelMeta{
			ChannelType:    channelType,
			ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
		}}
		assert.False(t, ShouldPassThroughRequestBody(info))
	}
}

func TestNormalChannelKeepsConfiguredRawBodyPassthrough(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = false
	t.Cleanup(func() { settings.PassThroughRequestEnabled = original })

	info := &RelayInfo{ChannelMeta: &ChannelMeta{
		ChannelType:    constant.ChannelTypeOpenAI,
		ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true},
	}}
	assert.True(t, ShouldPassThroughRequestBody(info))
}
