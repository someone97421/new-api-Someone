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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { FileRelaySection } from '../file-relay-section'
import { createFileRelaySchema, fileRelayDefaults } from '../lib/file-relay'

const clients: QueryClient[] = []
afterEach(() => {
  clients.forEach((client) => client.clear())
  clients.length = 0
  vi.restoreAllMocks()
})

function RelayFixture(props: { value: string }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <FileRelaySection value={props.value} />
      </SettingsPageProvider>
    </>
  )
}

function renderRelay(value = '') {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  clients.push(client)
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => <RelayFixture value={value} />,
    }),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

test('Missing configuration shows disabled defaults and enabling unlocks settings', async () => {
  renderRelay()
  const toggle = await screen.findByRole('switch', {
    name: 'Enable file relay',
  })
  expect(toggle).toHaveAttribute('aria-checked', 'false')
  expect(screen.getByLabelText('Storage directory')).toHaveValue(
    './data/file-relay'
  )
  expect(screen.getByLabelText('Storage directory')).toBeDisabled()
  expect(
    screen.getByRole('switch', { name: 'Require local hosting' })
  ).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
  await userEvent.click(toggle)
  expect(screen.getByLabelText('Storage directory')).toBeEnabled()
  expect(
    screen.getByRole('switch', { name: 'Require local hosting' })
  ).not.toHaveAttribute('aria-disabled', 'true')
})

test('Saving sends one JSON option, preserves zero values and omits server-managed directories', async () => {
  const update = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true } })
  renderRelay(
    JSON.stringify({
      ...fileRelayDefaults,
      enabled: true,
      previous_directories: ['./old'],
    })
  )
  const retention = await screen.findByLabelText('File retention (hours)')
  await userEvent.clear(retention)
  await userEvent.type(retention, '0')
  const retries = screen.getByLabelText('Download retries')
  await userEvent.clear(retries)
  await userEvent.type(retries, '0')
  await userEvent.click(
    screen.getByRole('switch', { name: 'Require local hosting' })
  )
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() =>
    expect(update).toHaveBeenCalledWith('/api/option/', {
      key: 'FileRelaySettings',
      value: JSON.stringify({
        ...fileRelayDefaults,
        enabled: true,
        retention_hours: 0,
        retry_count: 0,
        strict: true,
      }),
    })
  )
  expect(update).toHaveBeenCalledTimes(1)
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
  )
})

test.each(['not json', '{"enabled":true}', 'null'])(
  'Invalid saved configuration %s shows an error without a save action',
  async (value) => {
    renderRelay(value)
    expect(
      await screen.findByText('Invalid file relay configuration')
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Save Changes' })
    ).not.toBeInTheDocument()
  }
)

test.each([
  ['retention_hours', -1],
  ['retention_hours', 0.5],
  ['retention_hours', 87601],
  ['cleanup_interval_minutes', 0],
  ['cleanup_interval_minutes', 1441],
  ['download_timeout_seconds', 0],
  ['download_timeout_seconds', 3601],
  ['retry_count', -1],
  ['retry_count', 6],
  ['retry_count', 1.5],
  ['retry_count', Number.NaN],
  ['directory', ' '],
  ['public_url', 'invalid'],
  ['public_url', 'https://user@example.com'],
  ['public_url', 'https://:password@example.com'],
  ['public_url', 'https://@example.com'],
  ['public_url', 'https://example.com?token=value'],
  ['public_url', 'https://example.com?'],
  ['public_url', 'https://example.com#section'],
  ['public_url', 'https://example.com#'],
])('Invalid %s=%s fails configuration validation', (key, value) => {
  expect(
    createFileRelaySchema((text) => text).safeParse({
      ...fileRelayDefaults,
      [key]: value,
    }).success
  ).toBe(false)
})

test.each(['business', 'network'])(
  'A %s save failure displays its reason and keeps edits available for retry',
  async (failure) => {
    const update = vi.spyOn(api, 'put')
    if (failure === 'business') {
      update.mockResolvedValue({
        data: { success: false, message: 'Disk is full' },
      })
    } else {
      update.mockRejectedValue(new Error('Disk is full'))
    }
    renderRelay(JSON.stringify({ ...fileRelayDefaults, enabled: true }))
    const directory = await screen.findByLabelText('Storage directory')
    await userEvent.clear(directory)
    await userEvent.type(directory, './changed')
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Disk is full')
    expect(directory).toHaveValue('./changed')
    expect(directory).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  }
)

test('Pending save disables controls until the response arrives', async () => {
  let complete!: (value: { data: { success: boolean } }) => void
  vi.spyOn(api, 'put').mockImplementation(
    () =>
      new Promise((resolve) => {
        complete = resolve
      })
  )
  renderRelay()
  const toggle = await screen.findByRole('switch', {
    name: 'Enable file relay',
  })
  await userEvent.click(toggle)
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(
    await screen.findByRole('button', { name: 'Saving...' })
  ).toBeDisabled()
  expect(toggle).toHaveAttribute('aria-disabled', 'true')
  expect(screen.getByLabelText('Storage directory')).toBeDisabled()
  complete({ data: { success: true } })
  await waitFor(() =>
    expect(toggle).not.toHaveAttribute('aria-disabled', 'true')
  )
})

test('Out-of-range retries show a field error and do not submit', async () => {
  const update = vi.spyOn(api, 'put')
  renderRelay(JSON.stringify({ ...fileRelayDefaults, enabled: true }))
  const retries = await screen.findByLabelText('Download retries')
  await userEvent.clear(retries)
  await userEvent.type(retries, '6')
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  expect(
    await screen.findByText('Enter a whole number within the allowed range')
  ).toBeVisible()
  expect(retries).toHaveAttribute('aria-invalid', 'true')
  expect(update).not.toHaveBeenCalled()
})

test('Reset restores saved values after a keyboard toggle', async () => {
  renderRelay()
  const toggle = await screen.findByRole('switch', {
    name: 'Enable file relay',
  })
  toggle.focus()
  await userEvent.keyboard(' ')
  expect(toggle).toHaveAttribute('aria-checked', 'true')
  await userEvent.click(screen.getByRole('button', { name: 'Reset' }))
  expect(toggle).toHaveAttribute('aria-checked', 'false')
  expect(screen.getByRole('button', { name: 'Save Changes' })).toBeDisabled()
})

test('Maximum numeric values and a public URL with a port and path are accepted', () => {
  const values = {
    ...fileRelayDefaults,
    retention_hours: 87600,
    cleanup_interval_minutes: 1440,
    download_timeout_seconds: 3600,
    retry_count: 5,
    public_url: 'https://example.com:8443/media',
  }
  expect(createFileRelaySchema((text) => text).parse(values)).toEqual(values)
})
