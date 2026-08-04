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

import { evalExprLocally, type ExtraTokenValues } from '../tier-expr'

const emptyExtras: ExtraTokenValues = {
  cacheReadTokens: 0,
  cacheCreateTokens: 0,
  cacheCreate1hTokens: 0,
  imageTokens: 0,
  imageOutputTokens: 0,
  audioInputTokens: 0,
  audioOutputTokens: 0,
}

describe('request-aware billing expression estimator', () => {
  test('evaluates custom nested params and numeric string durations', () => {
    const result = evalExprLocally(
      'param("video.resolution") == "720p" ? tier("720p", number(param("video.duration")) * 0.1 * 1000000) : tier("other", 0)',
      0,
      0,
      emptyExtras,
      '{"video":{"duration":"10","resolution":"720p"}}'
    )

    assert.equal(result.error, null)
    assert.equal(result.cost, 1_000_000)
    assert.equal(result.matchedTier, '720p')
  })

  test('treats a missing request field as nil', () => {
    const result = evalExprLocally(
      'param("duration") == nil ? tier("default", 5) : tier("provided", 10)',
      0,
      0,
      emptyExtras,
      '{}'
    )

    assert.equal(result.error, null)
    assert.equal(result.cost, 5)
    assert.equal(result.matchedTier, 'default')
  })
})
