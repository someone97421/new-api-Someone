package gemini

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLDoesNotDuplicateGeminiVersionInBaseURL(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl:    "https://generativelanguage.googleapis.com/v1beta/",
		UpstreamModelName: "gemini-3.1-flash-image",
	}}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-image:generateContent", requestURL)
}

func TestGetRequestURLAddsGeminiVersionWhenBaseURLHasNoVersion(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelBaseUrl:    "https://generativelanguage.googleapis.com",
		UpstreamModelName: "gemini-3.1-flash-image",
	}}

	requestURL, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.1-flash-image:generateContent", requestURL)
}
