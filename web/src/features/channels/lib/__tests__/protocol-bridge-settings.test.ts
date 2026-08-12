import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

describe('protocol bridge channel settings', () => {
  test('serializes passthrough paths and field mappings for GPT2Gemini', () => {
    const result = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'bridge',
      type: 62,
      base_url: 'https://gemini.example',
      key: 'secret',
      models: 'public-model',
      protocol_bridge_passthrough_fields: 'seed\ngenerate_audio\nseed',
      protocol_bridge_field_mappings: JSON.stringify({
        aspect_ratio: 'generationConfig.imageConfig.aspectRatio',
      }),
    })

    const settings = JSON.parse(String(result.channel.settings))
    assert.deepEqual(settings.protocol_bridge, {
      passthrough_fields: ['seed', 'generate_audio'],
      field_mappings: {
        aspect_ratio: 'generationConfig.imageConfig.aspectRatio',
      },
    })
  })

  test('restores protocol bridge settings for editing', () => {
    const values = transformChannelToFormDefaults({
      id: 1,
      name: 'bridge',
      type: 61,
      key: '',
      status: 1,
      models: 'public-model',
      group: 'default',
      settings: JSON.stringify({
        protocol_bridge: {
          passthrough_fields: ['seed', 'references'],
          field_mappings: { duration: 'parameters.durationSeconds' },
        },
      }),
      channel_info: { multi_key_mode: 'random' },
    } as never)

    assert.equal(values.protocol_bridge_passthrough_fields, 'seed\nreferences')
    assert.deepEqual(
      JSON.parse(values.protocol_bridge_field_mappings || '{}'),
      {
        duration: 'parameters.durationSeconds',
      }
    )
  })
})
