/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { Channel } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
} from '../channel-form'

const configuredEndpoint = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Async image upstream',
  type: 58,
  base_url: 'https://image.example',
  key: 'test-key',
  models: 'image-model',
  advanced_custom: JSON.stringify({
    advanced_routes: [
      {
        incoming_path: '/v1/images/generations',
        upstream_path: '/v1/image/generations',
        converter: 'none',
      },
    ],
  }),
  image_task_query_path: '/v1/image/generations/{task_id}',
}

describe('image task endpoint override', () => {
  test('persists and restores OpenAI-compatible image query path', () => {
    const payload = transformFormDataToCreatePayload(configuredEndpoint)
    const settings = JSON.parse(String(payload.channel.settings))

    assert.deepEqual(settings.image_task_endpoints, {
      query_path: '/v1/image/generations/{task_id}',
    })

    const restored = transformChannelToFormDefaults({
      ...payload.channel,
      id: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance_updated_time: 0,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
    } as Channel)
    assert.equal(
      restored.image_task_query_path,
      '/v1/image/generations/{task_id}'
    )
  })

  test('rejects unsafe or incomplete image task query paths', () => {
    const absolutePath = channelFormSchema.safeParse({
      ...configuredEndpoint,
      image_task_query_path: 'https://evil.example/tasks/{task_id}',
    })
    const missingPlaceholder = channelFormSchema.safeParse({
      ...configuredEndpoint,
      image_task_query_path: '/v1/image/generations',
    })

    assert.equal(absolutePath.success, false)
    assert.equal(missingPlaceholder.success, false)
  })

  test('does not persist image task endpoints for unsupported channel types', () => {
    const payload = transformFormDataToCreatePayload({
      ...configuredEndpoint,
      type: 24,
      advanced_custom: '',
    })
    const settings = JSON.parse(String(payload.channel.settings))

    assert.equal(settings.image_task_endpoints, undefined)
  })
})
