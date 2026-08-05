package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestFilterChannelsByRequestPathAllowsConfiguredImageTaskQuery(t *testing.T) {
	channelSyncLock.Lock()
	previousChannels := channelsIDM
	previousAdvancedConfigs := channel2advancedCustomConfig
	previousImageEndpoints := channel2imageTaskEndpoints
	t.Cleanup(func() {
		channelsIDM = previousChannels
		channel2advancedCustomConfig = previousAdvancedConfigs
		channel2imageTaskEndpoints = previousImageEndpoints
		channelSyncLock.Unlock()
	})

	channelsIDM = map[int]*Channel{
		1: {Id: 1, Type: constant.ChannelTypeAdvancedCustom},
		2: {Id: 2, Type: constant.ChannelTypeAdvancedCustom},
	}
	channel2advancedCustomConfig = map[int]*dto.AdvancedCustomConfig{
		1: {Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/images/generations",
			UpstreamPath: "/v1/image/generations",
			Converter:    "none",
		}}},
		2: {Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/images/generations",
			UpstreamPath: "/v1/images/generations",
			Converter:    "none",
		}}},
	}
	channel2imageTaskEndpoints = map[int]*dto.ImageTaskEndpoints{
		1: {QueryPath: "/v1/image/generations/{task_id}"},
	}

	filtered := filterChannelsByRequestPathAndModel(
		[]int{1, 2},
		"/v1/images/generations/task-1",
		"nano-banana-2",
	)

	assert.Equal(t, []int{1}, filtered)
}
