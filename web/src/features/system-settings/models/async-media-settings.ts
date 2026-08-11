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
import * as z from 'zod'

export const asyncMediaSettingsSchema = z.object({
  AsyncMediaEnabled: z.boolean(),
  AsyncMediaStoragePath: z.string().trim().min(1),
  AsyncMediaRetentionHours: z.number().int().min(1).max(8760),
  AsyncMediaWorkers: z.number().int().min(1).max(128),
  AsyncMediaMaxFileMB: z.number().int().min(1).max(10240),
  AsyncMediaLeaseSeconds: z.number().int().min(30).max(3600),
  TaskTimeoutMinutes: z.number().int().min(0).max(525600),
  RetryTimes: z.number().int().min(0).max(10),
})

export type AsyncMediaSettingsFormValues = z.infer<
  typeof asyncMediaSettingsSchema
>

export function normalizeAsyncMediaSettings(
  values: AsyncMediaSettingsFormValues
): AsyncMediaSettingsFormValues {
  return {
    ...values,
    AsyncMediaStoragePath: values.AsyncMediaStoragePath.trim(),
  }
}
