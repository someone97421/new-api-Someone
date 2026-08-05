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

import {
  buildVideoPricingExpression,
  parseVideoPricingExpression,
} from '@/features/pricing/lib/video-pricing-expr'

describe('video pricing template', () => {
  test('builds request-aware pricing from custom field paths and options', () => {
    const expression = buildVideoPricingExpression(
      'video.seconds',
      'video.quality',
      '6.78',
      '5',
      '120',
      [
        { id: 'low', value: 'sd', pricePerSecond: '0.52' },
        { id: 'high', value: 'hd', pricePerSecond: '0.67' },
      ],
      'high'
    )

    assert.equal(
      expression,
      'param("video.quality") == "sd" ? tier("sd", min(max(param("video.seconds") == nil ? 5 : number(param("video.seconds")), 0), 120) * (0.52 / 6.78) * 1000000) : param("video.quality") == "hd" ? tier("hd", min(max(param("video.seconds") == nil ? 5 : number(param("video.seconds")), 0), 120) * (0.67 / 6.78) * 1000000) : tier("unknown_hd", min(max(param("video.seconds") == nil ? 5 : number(param("video.seconds")), 0), 120) * (0.67 / 6.78) * 1000000)'
    )
  })

  test('rejects duplicate option values and unsafe duration limits', () => {
    const items = [
      { id: 'a', value: '720p', pricePerSecond: '0.5' },
      { id: 'b', value: '720p', pricePerSecond: '0.8' },
    ]

    assert.equal(
      buildVideoPricingExpression(
        'duration',
        'resolution',
        '1',
        '5',
        '10',
        items,
        'a'
      ),
      null
    )
    assert.equal(
      buildVideoPricingExpression(
        'duration',
        'resolution',
        '1',
        '5',
        '3601',
        items.slice(0, 1),
        'a'
      ),
      null
    )
  })

  test('parses every configured option into USD per-second prices', () => {
    const expression = buildVideoPricingExpression(
      'video.seconds',
      'video.quality',
      '6.78',
      '5',
      '120',
      [
        { id: 'low', value: 'sd', pricePerSecond: '0.52' },
        { id: 'high', value: 'hd', pricePerSecond: '0.67' },
      ],
      'high'
    )

    assert.ok(expression)
    const parsed = parseVideoPricingExpression(expression)

    assert.deepEqual(parsed, {
      durationPath: 'video.seconds',
      optionPath: 'video.quality',
      defaultDuration: 5,
      maxDuration: 120,
      prices: [
        {
          optionValue: 'sd',
          tierLabel: 'sd',
          pricePerSecondUSD: 0.52 / 6.78,
        },
        {
          optionValue: 'hd',
          tierLabel: 'hd',
          pricePerSecondUSD: 0.67 / 6.78,
        },
      ],
    })
  })

  test('does not classify arbitrary request expressions as video pricing', () => {
    assert.equal(
      parseVideoPricingExpression(
        'param("quality") == "hd" ? tier("hd", p * 2) : tier("sd", p)'
      ),
      null
    )
  })
})
