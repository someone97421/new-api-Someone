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
import { SystemBehaviorSection } from '../system-behavior-section'

function BehaviorFixture(props: { enabled: boolean }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <SystemBehaviorSection
          defaultValues={{
            TaskEnabled: props.enabled,
            DefaultCollapseSidebar: false,
            DemoSiteEnabled: false,
            SelfUseModeEnabled: false,
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

afterEach(() => vi.restoreAllMocks())

test.each([true, false])(
  'TaskEnabled=%s renders its saved state and saves the toggled boolean',
  async (enabled) => {
    const update = vi
      .spyOn(api, 'put')
      .mockResolvedValue({ data: { success: true } })
    const client = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })
    const router = createRouter({
      routeTree: createRootRoute({
        component: () => <BehaviorFixture enabled={enabled} />,
      }),
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    render(
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )
    const toggle = await screen.findByRole('switch', {
      name: 'Task Processing',
    })
    expect(toggle).toHaveAttribute('aria-checked', String(enabled))
    await userEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-checked', String(!enabled))
    await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith('/api/option/', {
        key: 'TaskEnabled',
        value: !enabled,
      })
    )
    expect(update).toHaveBeenCalledTimes(1)
    client.clear()
  }
)
