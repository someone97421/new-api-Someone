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

import type { PricingModel } from '../../types'
import { getDynamicPricingSummary } from '../dynamic-price'
import { getDisplayGroupPricingContext } from '../model-helpers'
import { buildVideoPricingExpression } from '../video-pricing-expr'

function createVideoModel(billingExpr: string): PricingModel {
  return {
    id: 1,
    model_name: 'custom-video',
    quota_type: 0,
    model_ratio: 0,
    completion_ratio: 0,
    enable_groups: ['default', 'vip'],
    group_ratio: { default: 1, vip: 0.8 },
    billing_mode: 'tiered_expr',
    billing_expr: billingExpr,
  }
}

describe('video pricing summary', () => {
  test('formats every resolution with base and selected-group per-second prices', () => {
    const expression = buildVideoPricingExpression(
      'duration',
      'resolution',
      '1',
      '5',
      '60',
      [
        { id: 'sd', value: '480p', pricePerSecond: '0.5' },
        { id: 'hd', value: '720p', pricePerSecond: '0.75' },
      ],
      'hd'
    )
    assert.ok(expression)

    const summary = getDynamicPricingSummary(createVideoModel(expression), {
      tokenUnit: 'M',
      groupRatioMultiplier: 0.8,
    })

    assert.equal(summary?.isSpecialExpression, false)
    assert.deepEqual(
      summary?.videoPricing?.prices.map((price) => ({
        optionValue: price.optionValue,
        baseFormatted: price.baseFormatted,
        groupFormatted: price.groupFormatted,
      })),
      [
        {
          optionValue: '480p',
          baseFormatted: '$0.5',
          groupFormatted: '$0.4',
        },
        {
          optionValue: '720p',
          baseFormatted: '$0.75',
          groupFormatted: '$0.6',
        },
      ]
    )
  })

  test('identifies the group whose multiplier is used in the unfiltered price column', () => {
    const context = getDisplayGroupPricingContext(
      createVideoModel('tier("base", p)'),
      'all'
    )

    assert.deepEqual(context, { group: 'vip', ratio: 0.8 })
  })
})
