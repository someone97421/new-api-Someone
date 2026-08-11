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
import { stringifyAdvancedCustomConfig } from './advanced-custom'

export const BANANA_PUBLIC_MODEL = 'nano-banana-2'
export const BANANA_GEMINI_UPSTREAM_MODEL = 'gemini-3.1-flash-image'

export type BananaChannelPreset = {
  models: string[]
  modelMapping: string
  priority: number
  testModel: string
  advancedCustom?: string
  imageTaskQueryPath?: string
}

export function getGeminiBananaChannelPreset(): BananaChannelPreset {
  return {
    models: [BANANA_PUBLIC_MODEL],
    modelMapping: JSON.stringify(
      { [BANANA_PUBLIC_MODEL]: BANANA_GEMINI_UPSTREAM_MODEL },
      null,
      2
    ),
    priority: 100,
    testModel: BANANA_PUBLIC_MODEL,
  }
}

export function getWhaleBananaChannelPreset(): BananaChannelPreset {
  return {
    models: [BANANA_PUBLIC_MODEL],
    modelMapping: '',
    priority: 0,
    testModel: BANANA_PUBLIC_MODEL,
    advancedCustom: stringifyAdvancedCustomConfig({
      advanced_routes: [
        {
          incoming_path: '/v1/images/generations',
          upstream_path: '/v1/image/generations',
          converter: 'none',
          models: [BANANA_PUBLIC_MODEL],
          auth: {
            type: 'header',
            name: 'Authorization',
            value: 'Bearer {api_key}',
          },
        },
      ],
    }),
    imageTaskQueryPath: '/v1/image/generations/{task_id}',
  }
}
