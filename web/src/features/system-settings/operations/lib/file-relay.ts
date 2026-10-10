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

export const fileRelayDefaults = {
  enabled: false,
  auto_relay_enabled: false,
  directory: './data/file-relay',
  public_url: '',
  retention_hours: 168,
  cleanup_interval_minutes: 10,
  download_timeout_seconds: 60,
  retry_count: 2,
  strict: false,
}

export function createFileRelaySchema(t: (key: string) => string) {
  const numberError = t('Enter a whole number within the allowed range')
  return z.object({
    enabled: z.boolean(),
    auto_relay_enabled: z.boolean().optional(),
    directory: z.string().trim().min(1, t('Storage directory is required')),
    public_url: z
      .string()
      .trim()
      .refine((value) => {
        if (!value) return true
        try {
          const url = new URL(value)
          return (
            (url.protocol === 'http:' || url.protocol === 'https:') &&
            !url.username &&
            !url.password &&
            !/^https?:\/\/[^/?#]*@/i.test(value) &&
            !value.includes('?') &&
            !value.includes('#')
          )
        } catch {
          return false
        }
      }, t('Enter an absolute HTTP(S) URL without credentials, query parameters, or fragments')),
    retention_hours: z
      .number({ error: numberError })
      .int(numberError)
      .min(0, numberError)
      .max(87600, numberError),
    cleanup_interval_minutes: z
      .number({ error: numberError })
      .int(numberError)
      .min(1, numberError)
      .max(1440, numberError),
    download_timeout_seconds: z
      .number({ error: numberError })
      .int(numberError)
      .min(1, numberError)
      .max(3600, numberError),
    retry_count: z
      .number({ error: numberError })
      .int(numberError)
      .min(0, numberError)
      .max(5, numberError),
    strict: z.boolean(),
  })
}

export type FileRelayValues = z.infer<ReturnType<typeof createFileRelaySchema>>
