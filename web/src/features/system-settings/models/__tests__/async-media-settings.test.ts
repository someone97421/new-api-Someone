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
  asyncMediaSettingsSchema,
  normalizeAsyncMediaSettings,
} from '../async-media-settings'

const validSettings = {
  AsyncMediaEnabled: true,
  AsyncMediaStoragePath: './data/async-media',
  AsyncMediaRetentionHours: 24,
  AsyncMediaWorkers: 4,
  AsyncMediaMaxFileMB: 2048,
  AsyncMediaLeaseSeconds: 300,
  TaskTimeoutMinutes: 1440,
  RetryTimes: 1,
}

describe('async media settings', () => {
  test('accepts the documented defaults', () => {
    assert.equal(
      asyncMediaSettingsSchema.safeParse(validSettings).success,
      true
    )
  })

  test('rejects unsafe worker and lease bounds', () => {
    assert.equal(
      asyncMediaSettingsSchema.safeParse({
        ...validSettings,
        AsyncMediaWorkers: 0,
      }).success,
      false
    )
    assert.equal(
      asyncMediaSettingsSchema.safeParse({
        ...validSettings,
        AsyncMediaLeaseSeconds: 29,
      }).success,
      false
    )
  })

  test('allows disabling the task timeout', () => {
    assert.equal(
      asyncMediaSettingsSchema.safeParse({
        ...validSettings,
        TaskTimeoutMinutes: 0,
      }).success,
      true
    )
  })

  test('trims the persisted storage path', () => {
    assert.equal(
      normalizeAsyncMediaSettings({
        ...validSettings,
        AsyncMediaStoragePath: '  ./shared/async-media  ',
      }).AsyncMediaStoragePath,
      './shared/async-media'
    )
  })
})
