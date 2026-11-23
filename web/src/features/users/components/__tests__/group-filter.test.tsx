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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { SectionPageLayout } from '@/components/layout'
import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as UsersRoute } from '@/routes/_authenticated/users/index'
import { useAuthStore } from '@/stores/auth-store'

import type { User } from '../../types'
import { UsersProvider } from '../users-provider'
import { UsersTable } from '../users-table'

const clients: QueryClient[] = []
const premiumSearchUrl =
  '/api/user/search?keyword=&group=premium&p=1&page_size=20'
const premiumUser: User = {
  id: 2,
  username: 'premium-user',
  display_name: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'premium',
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'admin', role: 100 })
})

afterEach(() => {
  clients.splice(0).forEach((client) => client.clear())
  localStorage.clear()
  useAuthStore.getState().auth.reset()
})

async function renderUsers(
  initialEntry = '/users/',
  getPage: (url: string) => { items: User[]; total: number } = () => ({
    items: [premiumUser],
    total: 1,
  })
) {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default', 'premium'] } }
    }
    return { data: { success: true, data: getPage(url) } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const users = createRoute({
    getParentRoute: () => auth,
    path: 'users/',
    validateSearch: UsersRoute.options.validateSearch,
    component: () => (
      <TooltipProvider>
        <UsersProvider>
          <SectionPageLayout fixedContent>
            <SectionPageLayout.Content>
              <UsersTable />
            </SectionPageLayout.Content>
          </SectionPageLayout>
        </UsersProvider>
      </TooltipProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([users])]),
    history: createMemoryHistory({ initialEntries: [initialEntry] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { get, router }
}

// The User Group column header is a sort menu with the same name; the
// filter chip is the one that opens a dialog.
function getGroupFilterTrigger() {
  const trigger = screen
    .getAllByRole('button', { name: /^User Group/ })
    .find((button) => button.getAttribute('aria-haspopup') === 'dialog')
  expect(trigger).toBeDefined()
  return trigger as HTMLElement
}

it('searches only the selected user group and lists all users again after the filter is cleared', async () => {
  const user = userEvent.setup()
  const { get, router } = await renderUsers()
  await screen.findByText('premium-user')
  expect(get).toHaveBeenCalledWith('/api/user/', {
    params: { p: 1, page_size: 20 },
  })

  await user.click(getGroupFilterTrigger())
  await user.click(await screen.findByRole('option', { name: /^premium/ }))

  await waitFor(() => expect(get).toHaveBeenLastCalledWith(premiumSearchUrl))
  expect(router.state.location.search).toMatchObject({ group: ['premium'] })
  expect(getGroupFilterTrigger()).toHaveTextContent('premium')

  await user.click(screen.getByRole('option', { name: 'Clear filters' }))

  await waitFor(() =>
    expect(get).toHaveBeenLastCalledWith('/api/user/', {
      params: { p: 1, page_size: 20 },
    })
  )
  expect(router.state.location.search).not.toHaveProperty('group')
})

it('restores the user group filter from the page URL on load', async () => {
  const { get } = await renderUsers('/users/?group=%5B%22premium%22%5D')
  await screen.findByText('premium-user')

  expect(get).toHaveBeenCalledWith(premiumSearchUrl)
  expect(get).not.toHaveBeenCalledWith('/api/user/', expect.anything())
  expect(getGroupFilterTrigger()).toHaveTextContent('premium')
})

it('opens all direct subaccounts across pages from a filtered parent row', async () => {
  const user = userEvent.setup()
  const { get, router } = await renderUsers(
    '/users/?page=2&pageSize=1&filter=premium&group=%5B%22premium%22%5D&role=%5B%221%22%5D&status=%5B%221%22%5D',
    (url) => {
      const params = new URL(url, 'http://localhost').searchParams
      if (params.get('parent_id') !== '2') {
        return { items: [{ ...premiumUser, subaccount_count: 2 }], total: 2 }
      }
      if (params.get('p') === '2') {
        return {
          items: [
            {
              ...premiumUser,
              id: 4,
              username: 'premium-child-two',
              parent_id: 2,
            },
          ],
          total: 2,
        }
      }
      return {
        items: [
          {
            ...premiumUser,
            id: 3,
            username: 'premium-child-one',
            parent_id: 2,
            parent_username: 'premium-user',
          },
        ],
        total: 2,
      }
    }
  )

  const openSubaccounts = await screen.findByRole('link', {
    name: 'Subaccounts: 2',
  })
  openSubaccounts.focus()
  await user.keyboard('{Enter}')

  expect(await screen.findByText('premium-child-one')).toBeVisible()
  expect(screen.getByText('premium-user')).toBeVisible()
  expect(screen.getByText('#2')).toBeVisible()
  expect(router.state.location.search).toMatchObject({
    parentId: 2,
    pageSize: 1,
  })
  expect(get).toHaveBeenLastCalledWith(
    '/api/user/search?keyword=&group=&parent_id=2&p=1&page_size=1'
  )

  await user.click(screen.getByRole('button', { name: 'Go to next page' }))

  expect(await screen.findByText('premium-child-two')).toBeVisible()
  expect(screen.getByText('#2')).toBeVisible()
  expect(screen.queryByText('premium-user')).not.toBeInTheDocument()
  expect(get).toHaveBeenLastCalledWith(
    '/api/user/search?keyword=&group=&parent_id=2&p=2&page_size=1'
  )

  const clear = screen.getByRole('button', {
    name: 'Clear parent account filter',
  })
  expect(clear).toHaveTextContent('Parent account: #2')
  await user.click(clear)

  expect(
    await screen.findByRole('link', { name: 'Subaccounts: 2' })
  ).toBeVisible()
  expect(router.state.location.search).not.toHaveProperty('parentId')
  expect(get).toHaveBeenLastCalledWith('/api/user/', {
    params: { p: 1, page_size: 1 },
  })
})

it('restores a combined parent and group filter on mobile and clears only the parent', async () => {
  const matchMedia = window.matchMedia
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches: query === '(max-width: 640px)' || matchMedia(query).matches,
  }))
  const user = userEvent.setup()
  const { get } = await renderUsers(
    '/users/?parentId=2&group=%5B%22premium%22%5D',
    (url) => ({
      items: url.includes('parent_id=2')
        ? [{ ...premiumUser, id: 3, username: 'mobile-child', parent_id: 2 }]
        : [{ ...premiumUser, subaccount_count: 1 }],
      total: 1,
    })
  )
  expect(await screen.findByText('mobile-child')).toBeVisible()
  expect(screen.getByText('#2')).toBeVisible()
  expect(get).toHaveBeenLastCalledWith(
    '/api/user/search?keyword=&group=premium&parent_id=2&p=1&page_size=10'
  )

  await user.click(
    screen.getByRole('button', { name: 'Clear parent account filter' })
  )
  expect(
    await screen.findByRole('link', { name: 'Subaccounts: 1' })
  ).toBeVisible()
  expect(get).toHaveBeenLastCalledWith(
    '/api/user/search?keyword=&group=premium&p=1&page_size=10'
  )
})
