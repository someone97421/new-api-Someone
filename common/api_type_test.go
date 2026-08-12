package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelType2APITypeRegistersProtocolBridgeChannels(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeGemini2GPT, constant.ChannelTypeGPT2Gemini} {
		apiType, ok := ChannelType2APIType(channelType)
		require.True(t, ok)
		assert.Equal(t, constant.APITypeProtocolBridge, apiType)
	}
}
