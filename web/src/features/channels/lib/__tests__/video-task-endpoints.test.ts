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

const configuredEndpoints = {
  ...CHANNEL_FORM_DEFAULT_VALUES,
  name: 'Video upstream',
  type: 1,
  key: 'test-key',
  models: 'video-model',
  video_task_submit_path: '/v1/video/generations',
  video_task_query_path: '/v1/video/generations/{task_id}',
  video_task_content_path: '/v1/video/generations/{task_id}/content',
  video_task_remix_path: '/v1/video/generations/{video_id}/remix',
}

describe('video task endpoint overrides', () => {
  test('persists and restores OpenAI video task endpoint paths', () => {
    const payload = transformFormDataToCreatePayload(configuredEndpoints)
    const settings = JSON.parse(String(payload.channel.settings))

    assert.deepEqual(settings.video_task_endpoints, {
      submit_path: '/v1/video/generations',
      query_path: '/v1/video/generations/{task_id}',
      content_path: '/v1/video/generations/{task_id}/content',
      remix_path: '/v1/video/generations/{video_id}/remix',
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
    assert.equal(restored.video_task_submit_path, '/v1/video/generations')
    assert.equal(
      restored.video_task_query_path,
      '/v1/video/generations/{task_id}'
    )
    assert.equal(
      restored.video_task_content_path,
      '/v1/video/generations/{task_id}/content'
    )
    assert.equal(
      restored.video_task_remix_path,
      '/v1/video/generations/{video_id}/remix'
    )
  })

  test('rejects unsafe or incomplete endpoint paths', () => {
    const absoluteSubmit = channelFormSchema.safeParse({
      ...configuredEndpoints,
      video_task_submit_path: 'https://evil.example/videos',
    })
    const missingTaskPlaceholder = channelFormSchema.safeParse({
      ...configuredEndpoints,
      video_task_query_path: '/v1/video/generations',
    })

    assert.equal(absoluteSubmit.success, false)
    assert.equal(missingTaskPlaceholder.success, false)
  })

  test('does not persist endpoint overrides for unsupported channel types', () => {
    const payload = transformFormDataToCreatePayload({
      ...configuredEndpoints,
      type: 24,
    })
    const settings = JSON.parse(String(payload.channel.settings))

    assert.equal(settings.video_task_endpoints, undefined)
  })
})
