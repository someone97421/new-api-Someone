package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRejectsInvalidHTTPTransport(t *testing.T) {
	tests := []struct {
		name    string
		setting dto.ChannelSettings
		wantErr string
	}{
		{
			name:    "auto with shards is valid",
			setting: dto.ChannelSettings{HTTPProtocol: "auto", HTTP2ConnectionShards: 4},
		},
		{
			name:    "http1 with shards greater than one rejected",
			setting: dto.ChannelSettings{HTTPProtocol: "http1", HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(tt.setting)
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestChannelValidateSettingsRejectsInvalidVideoTaskEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		endpoints   dto.VideoTaskEndpoints
		wantErr     string
	}{
		{
			name:        "openai accepts relative endpoint overrides",
			channelType: constant.ChannelTypeOpenAI,
			endpoints: dto.VideoTaskEndpoints{
				SubmitPath:  "/v1/video/generations",
				QueryPath:   "/v1/video/generations/{task_id}",
				ContentPath: "/v1/video/generations/{task_id}/content",
				RemixPath:   "/v1/video/generations/{video_id}/remix",
			},
		},
		{
			name:        "absolute submit URL rejected",
			channelType: constant.ChannelTypeOpenAI,
			endpoints:   dto.VideoTaskEndpoints{SubmitPath: "https://evil.example/videos"},
			wantErr:     "submit_path",
		},
		{
			name:        "query path requires task placeholder",
			channelType: constant.ChannelTypeSora,
			endpoints:   dto.VideoTaskEndpoints{QueryPath: "/v1/video/generations"},
			wantErr:     "{task_id}",
		},
		{
			name:        "unsupported channel type rejected",
			channelType: constant.ChannelTypeGemini,
			endpoints:   dto.VideoTaskEndpoints{SubmitPath: "/custom/videos"},
			wantErr:     "OpenAI or Sora",
		},
		{
			name:        "content path requires task placeholder",
			channelType: constant.ChannelTypeOpenAI,
			endpoints:   dto.VideoTaskEndpoints{ContentPath: "/v1/video/content"},
			wantErr:     "content_path",
		},
		{
			name:        "remix path requires video placeholder",
			channelType: constant.ChannelTypeSora,
			endpoints:   dto.VideoTaskEndpoints{RemixPath: "/v1/video/remix"},
			wantErr:     "remix_path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: tt.channelType}
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoTaskEndpoints: &tt.endpoints})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestChannelValidateSettingsRejectsInvalidImageTaskEndpoints(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		endpoints   dto.ImageTaskEndpoints
		wantErr     string
	}{
		{
			name:        "advanced custom accepts image query endpoint",
			channelType: constant.ChannelTypeAdvancedCustom,
			endpoints: dto.ImageTaskEndpoints{
				QueryPath: "/v1/image/generations/{task_id}",
			},
		},
		{
			name:        "openai accepts image query endpoint",
			channelType: constant.ChannelTypeOpenAI,
			endpoints: dto.ImageTaskEndpoints{
				QueryPath: "/v1/tasks/{task_id}",
			},
		},
		{
			name:        "query path requires task placeholder",
			channelType: constant.ChannelTypeAdvancedCustom,
			endpoints: dto.ImageTaskEndpoints{
				QueryPath: "/v1/image/generations",
			},
			wantErr: "{task_id}",
		},
		{
			name:        "absolute query URL rejected",
			channelType: constant.ChannelTypeAdvancedCustom,
			endpoints: dto.ImageTaskEndpoints{
				QueryPath: "https://evil.example/tasks/{task_id}",
			},
			wantErr: "query_path",
		},
		{
			name:        "unsupported channel type rejected",
			channelType: constant.ChannelTypeGemini,
			endpoints: dto.ImageTaskEndpoints{
				QueryPath: "/v1/tasks/{task_id}",
			},
			wantErr: "OpenAI or Advanced Custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: tt.channelType}
			otherSettings := dto.ChannelOtherSettings{ImageTaskEndpoints: &tt.endpoints}
			if tt.channelType == constant.ChannelTypeAdvancedCustom {
				otherSettings.AdvancedCustom = &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
					IncomingPath: "/v1/images/generations",
					UpstreamPath: "/v1/images/generations",
					Converter:    "none",
				}}}
			}
			channel.SetOtherSettings(otherSettings)

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
