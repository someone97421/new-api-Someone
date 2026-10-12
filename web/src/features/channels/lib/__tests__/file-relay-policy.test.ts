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
  buildSettingJSON,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'

const channel = channelSchema.parse({
  id: 42,
  name: 'Image channel',
  type: 1,
  key: '',
  status: 1,
  created_time: 1,
  test_time: 0,
  response_time: 0,
  balance_updated_time: 0,
  models: 'image-model',
  group: 'default',
})

test.each([
  { policy: undefined, enabled: false, strict: 'inherit' },
  { policy: {}, enabled: false, strict: 'inherit' },
  { policy: { enabled: true }, enabled: true, strict: 'inherit' },
  { policy: { enabled: false }, enabled: false, strict: 'inherit' },
  { policy: { strict: true }, enabled: false, strict: 'on' },
  { policy: { strict: false }, enabled: false, strict: 'off' },
  { policy: { enabled: true, strict: false }, enabled: true, strict: 'off' },
  { policy: { enabled: false, strict: true }, enabled: false, strict: 'on' },
])(
  'Saved file relay $policy survives form parsing and create/update serialization',
  ({ policy, enabled, strict }) => {
    const defaults = transformChannelToFormDefaults({
      ...channel,
      setting: JSON.stringify({ file_relay: policy }),
    })
    expect(defaults).toMatchObject({
      file_relay_enabled: enabled,
      file_relay_strict: strict,
    })
    const parsed = channelFormSchema.parse(defaults)
    const create = transformFormDataToCreatePayload(parsed)
    const update = transformFormDataToUpdatePayload(parsed, channel.id)
    const expected = { ...policy, enabled }
    expect(JSON.parse(create.channel.setting ?? '{}').file_relay).toEqual(
      expected
    )
    expect(JSON.parse(update.setting ?? '{}').file_relay).toEqual(expected)
  }
)

test('Inheriting strict clears its override while channel relay remains explicitly disabled', () => {
  const defaults = transformChannelToFormDefaults({
    ...channel,
    setting: JSON.stringify({ file_relay: { enabled: true, strict: true } }),
  })
  const setting = JSON.parse(
    buildSettingJSON({
      ...defaults,
      file_relay_enabled: false,
      file_relay_strict: 'inherit',
    })
  )
  expect(setting.file_relay).toEqual({ enabled: false })
  expect(
    JSON.parse(buildSettingJSON(CHANNEL_FORM_DEFAULT_VALUES)).file_relay
  ).toEqual({ enabled: false })
})

test.each([
  { enabled: false, strict: 'inherit' as const, expected: 'idle' },
  { enabled: true, strict: 'inherit' as const, expected: 'configured' },
  { enabled: false, strict: 'off' as const, expected: 'configured' },
  { enabled: false, strict: 'on' as const, expected: 'configured' },
])(
  'Channel relay $enabled with strict $strict marks extra settings as $expected',
  ({ enabled, strict, expected }) => {
    const state = getChannelConfigurationState(
      {
        ...CHANNEL_FORM_DEFAULT_VALUES,
        file_relay_enabled: enabled,
        file_relay_strict: strict,
      },
      {},
      true
    )
    expect(state.blocks.extraSettings).toBe(expected)
    expect(state.sections.other).toBe(expected)
  }
)
