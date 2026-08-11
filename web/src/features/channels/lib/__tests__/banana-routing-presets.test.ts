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

import { parseAdvancedCustomConfig } from '../advanced-custom'
import {
  BANANA_GEMINI_UPSTREAM_MODEL,
  BANANA_PUBLIC_MODEL,
  getGeminiBananaChannelPreset,
  getWhaleBananaChannelPreset,
} from '../banana-routing-presets'

describe('banana routing presets', () => {
  test('Gemini preset exposes the public model and maps it upstream', () => {
    const preset = getGeminiBananaChannelPreset()

    assert.deepEqual(preset.models, [BANANA_PUBLIC_MODEL])
    assert.deepEqual(JSON.parse(preset.modelMapping), {
      [BANANA_PUBLIC_MODEL]: BANANA_GEMINI_UPSTREAM_MODEL,
    })
    assert.equal(preset.priority, 100)
  })

  test('Whale preset fills the singular submit and query paths', () => {
    const preset = getWhaleBananaChannelPreset()
    const config = parseAdvancedCustomConfig(preset.advancedCustom || '')

    assert.equal(
      config?.advanced_routes?.[0]?.upstream_path,
      '/v1/image/generations'
    )
    assert.equal(preset.imageTaskQueryPath, '/v1/image/generations/{task_id}')
    assert.equal(preset.priority, 0)
  })
})
