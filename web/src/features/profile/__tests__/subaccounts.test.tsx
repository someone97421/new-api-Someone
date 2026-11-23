import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SubaccountsCard } from '../components/subaccounts-card'

const child = {
  id: 8,
  username: 'child',
  status: 1,
  used_quota: 10,
  request_count: 17,
}
const endpoint = '/api/user/self/subaccounts'

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  render(
    <QueryClientProvider client={client}>
      <SubaccountsCard parentId={1} />
    </QueryClientProvider>
  )
  return userEvent.setup()
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

it('creates a child from the empty list and shows the refreshed request count', async () => {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: true, data: [] } })
    .mockResolvedValue({ data: { success: true, data: [child] } })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: child } })
  const user = renderCard()
  await screen.findByText('No subaccounts yet')
  await user.click(screen.getByRole('button', { name: 'Create subaccount' }))
  await user.type(screen.getByLabelText('Username'), child.username)
  await user.type(
    screen.getByLabelText('Password', { exact: true }),
    'example-password'
  )
  await user.click(screen.getByRole('button', { name: 'Create subaccount' }))
  expect(await screen.findByText('17')).toBeTruthy()
  expect(post).toHaveBeenCalledWith(
    endpoint,
    { username: child.username, password: 'example-password' },
    expect.objectContaining({ singleUseAuthorization: true })
  )
})

it('requires confirmation to disable a child and removes its disable action on success', async () => {
  vi.spyOn(api, 'get')
    .mockResolvedValueOnce({ data: { success: true, data: [child] } })
    .mockResolvedValue({
      data: { success: true, data: [{ ...child, status: 2 }] },
    })
  const post = vi
    .spyOn(api, 'post')
    .mockResolvedValue({ data: { success: true, data: null } })
  const user = renderCard()
  await user.click(
    await screen.findByRole('button', { name: /Disable.*child/ })
  )
  expect(post).not.toHaveBeenCalled()
  await user.click(
    within(screen.getByRole('alertdialog')).getByRole('button', {
      name: 'Disable',
    })
  )
  expect(await screen.findByText('Disabled')).toBeTruthy()
  expect(screen.queryByRole('button', { name: /Disable.*child/ })).toBeNull()
  expect(post).toHaveBeenCalledWith(
    `${endpoint}/8/disable`,
    undefined,
    expect.anything()
  )
})

it('clears the new password and offers sign-out when the server requires a fresh login', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: [] } })
  const post = vi
    .spyOn(api, 'post')
    .mockRejectedValue({
      isAxiosError: true,
      response: {
        status: 403,
        data: {
          code: 'RECENT_LOGIN_REQUIRED',
          message: 'Please sign in again.',
        },
      },
    })
  const user = renderCard()
  await user.click(screen.getByRole('button', { name: 'Create subaccount' }))
  await user.type(screen.getByLabelText('Username'), 'child')
  const password = screen.getByLabelText<HTMLInputElement>('Password', {
    exact: true,
  })
  await user.type(password, 'example-password')
  await user.click(screen.getByRole('button', { name: 'Create subaccount' }))
  expect(await screen.findByText('Please sign in again.')).toBeTruthy()
  expect(password.value).toBe('')
  expect(screen.getByRole('button', { name: 'Sign out' })).toBeTruthy()
  expect(post).toHaveBeenCalledTimes(1)
})
