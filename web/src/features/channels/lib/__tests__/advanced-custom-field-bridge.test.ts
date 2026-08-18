import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  normalizeAdvancedCustomConfig,
  parseAdvancedCustomFieldMappings,
  parseAdvancedCustomPassthroughFields,
  stringifyAdvancedCustomConfig,
  validateAdvancedCustomConfig,
} from '../advanced-custom'

describe('advanced custom route field bridge', () => {
  test('normalizes passthrough fields and field mappings on a route', () => {
    const config = normalizeAdvancedCustomConfig({
      advanced_routes: [
        {
          incoming_path: '/v1/chat/completions',
          upstream_path: '/v1beta/models/{model}:generateContent',
          converter: 'openai_chat_completions_to_gemini_generate_content',
          passthrough_fields: [' seed ', 'seed', 'vendor.camera'],
          field_mappings: {
            ' aspect_ratio ': ' generationConfig.imageConfig.aspectRatio ',
          },
        },
      ],
    })

    assert.deepEqual(config.advanced_routes?.[0]?.passthrough_fields, [
      'seed',
      'vendor.camera',
    ])
    assert.deepEqual(config.advanced_routes?.[0]?.field_mappings, {
      aspect_ratio: 'generationConfig.imageConfig.aspectRatio',
    })
    assert.equal(validateAdvancedCustomConfig(config), null)
    assert.match(
      stringifyAdvancedCustomConfig(config),
      /"passthrough_fields": \[\s*"seed",\s*"vendor.camera"\s*\]/
    )
  })

  test('rejects protected mapping targets and model list field rules', () => {
    assert.equal(
      validateAdvancedCustomConfig({
        advanced_routes: [
          {
            incoming_path: '/v1/chat/completions',
            upstream_path: '/v1/chat/completions',
            converter: 'none',
            field_mappings: { token: 'authorization' },
          },
        ],
      })?.message,
      'Custom field replacement target is protected'
    )
    assert.equal(
      validateAdvancedCustomConfig({
        advanced_routes: [
          {
            incoming_path: '/v1/models',
            upstream_path: '/v1/models',
            converter: 'none',
            passthrough_fields: ['seed'],
          },
        ],
      })?.message,
      'OpenAI Models route does not support custom field rules'
    )
  })

  test('parses visual editor drafts into route field values', () => {
    assert.deepEqual(
      parseAdvancedCustomPassthroughFields('seed\ngenerate_audio\nseed'),
      ['seed', 'generate_audio']
    )
    assert.deepEqual(
      parseAdvancedCustomFieldMappings(
        '{\n  "aspect_ratio": "generationConfig.imageConfig.aspectRatio"\n}'
      ),
      {
        mappings: {
          aspect_ratio: 'generationConfig.imageConfig.aspectRatio',
        },
        error: null,
      }
    )
    assert.equal(
      parseAdvancedCustomFieldMappings('{').error,
      'Custom field replacements must be valid JSON'
    )
  })
})
