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
import { expect, test } from 'vitest'

import { channelSchema } from '../../types'
import { getChannelConfigurationState } from '../channel-configuration'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const endpoints = {
  submit_path: '/vendor/videos',
  query_path: '/vendor/videos/{task_id}',
  content_path: '/vendor/videos/{task_id}/content',
  remix_path: '/vendor/videos/{video_id}/remix',
}

const channel = channelSchema.parse({
  id: 42,
  name: 'Video channel',
  type: 1,
  key: '',
  status: 1,
  created_time: 1,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  models: 'video-model',
  group: 'default',
  settings: JSON.stringify({
    video_task_endpoints: endpoints,
    vendor_extension: { enabled: false },
  }),
})

test.each([1, 55])(
  'Channel type %i preserves custom video endpoints through create and update',
  (type) => {
    const defaults = transformChannelToFormDefaults({ ...channel, type })
    const values = channelFormSchema.parse(defaults)
    const create = transformFormDataToCreatePayload(values)
    const update = transformFormDataToUpdatePayload(values, channel.id)
    for (const settings of [create.channel.settings, update.settings]) {
      expect(JSON.parse(settings ?? '{}')).toMatchObject({
        video_task_endpoints: endpoints,
        vendor_extension: { enabled: false },
      })
    }
    const state = getChannelConfigurationState(values, {}, true)
    expect(state.blocks.requestProcessing).toBe('configured')
    expect(state.sections.request).toBe('configured')
  }
)

test.each([
  { label: 'cleared', type: 1, value: '' },
  { label: 'provider changed', type: 14, value: JSON.stringify(endpoints) },
])('Video endpoint overrides are removed when $label', ({ type, value }) => {
  const defaults = transformChannelToFormDefaults(channel)
  const payload = transformFormDataToUpdatePayload(
    { ...defaults, type, video_task_endpoints: value },
    channel.id
  )
  expect(JSON.parse(payload.settings ?? '{}')).not.toHaveProperty(
    'video_task_endpoints'
  )
})

test.each([
  '[]',
  'null',
  '{',
  JSON.stringify({ submit_path: 'https://vendor.example/videos' }),
  JSON.stringify({ submit_path: '//vendor.example/videos' }),
  JSON.stringify({ submit_path: '/videos/{task_id}' }),
  JSON.stringify({ query_path: '/videos' }),
  JSON.stringify({ query_path: '/videos/{task_id}/{task_id}' }),
  JSON.stringify({ content_path: '/videos/{task_id}#content' }),
  JSON.stringify({ remix_path: '/videos/{task_id}/remix' }),
  JSON.stringify({ submit_path: false }),
])(
  'Invalid video endpoint input %s is rejected before saving',
  (video_task_endpoints) => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Video',
      models: 'video-model',
      video_task_endpoints,
    })
    expect(result.success).toBe(false)
    if (!result.success) {
      expect(result.error.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ path: ['video_task_endpoints'] }),
        ])
      )
    }
  }
)
