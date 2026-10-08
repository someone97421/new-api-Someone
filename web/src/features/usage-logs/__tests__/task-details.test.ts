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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createElement } from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { TaskDetailsDialog } from '../components/dialogs/task-details-dialog'
import { resolveTaskDetailAccess } from '../lib/task-details'
import type { TaskLog } from '../types'

const task: TaskLog = {
  id: 1,
  user_id: 7,
  platform: 'document-parser',
  task_id: 'task_public',
  action: 'GENERATE',
  channel_id: 3,
  group: 'default',
  quota: 100,
  submit_time: 1,
  status: 'SUCCESS',
  admin_info: {
    task_plugin: {
      key: 'document-parser',
      name: 'Document Parser',
      version: '1.2.3',
      author: {
        name: 'Community Maintainer',
        url: 'https://plugins.example.com/maintainers/community',
      },
    },
  },
  root_info: {
    task_plugin: {
      key: 'document-parser',
      version: '1.2.3',
      api_version: 1,
      generation: 42,
    },
    upstream_task_id: 'upstream-private',
    node_name: 'node-a',
  },
}

describe('task detail access', () => {
  test('does not expose elevated fields in a self view', () => {
    expect(resolveTaskDetailAccess(task, false, false)).toEqual({})
  })

  test('gives admins plugin identity without root diagnostics', () => {
    expect(resolveTaskDetailAccess(task, true, false)).toEqual({
      plugin: task.admin_info?.task_plugin,
    })
  })

  test('adds runtime and upstream diagnostics for root', () => {
    expect(resolveTaskDetailAccess(task, true, true)).toEqual({
      plugin: task.admin_info?.task_plugin,
      runtime: task.root_info?.task_plugin,
      upstreamTaskId: 'upstream-private',
      nodeName: 'node-a',
    })
  })
})

afterEach(() => vi.restoreAllMocks())

function renderTask(
  overrides: Partial<Parameters<typeof TaskDetailsDialog>[0]> = {}
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const props = {
    log: task,
    isAdmin: true,
    isRoot: false,
    open: true,
    onOpenChange: vi.fn(),
    ...overrides,
  }
  const view = render(
    createElement(
      QueryClientProvider,
      { client },
      createElement(TaskDetailsDialog, props)
    )
  )
  return {
    ...view,
    update: (changes: Partial<typeof props>) =>
      view.rerender(
        createElement(
          QueryClientProvider,
          { client },
          createElement(TaskDetailsDialog, { ...props, ...changes })
        )
      ),
  }
}

const billing = {
  expression: 'tier("standard", u("seconds") * 0.1)',
  usage_facts: {
    seconds: 5,
    invalid: { nested: true },
    flag: true,
    infinite: Infinity,
  },
  tier: 'standard',
  estimated: true,
}

test('an administrator loads raw task data only after expanding it and sees only the result payload', async () => {
  const request = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        task_id: task.task_id,
        data: { result: 'raw-result' },
        billing,
      },
    },
  })
  renderTask()
  expect(request).not.toHaveBeenCalled()
  await userEvent.click(
    await screen.findByRole('button', { name: 'View Task Data' })
  )
  expect(await screen.findByText(/raw-result/)).toBeVisible()
  expect(screen.queryByText(/"estimated"/)).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Hide Task Data' })
  ).toHaveAttribute('aria-expanded', 'true')
  expect(request).toHaveBeenCalledWith(
    `/api/task/${task.task_id}/data`,
    expect.anything()
  )
})

test('an administrator expands an estimate without raw data and sees the expression fallback and explicit estimate notice', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        task_id: task.task_id,
        data: { result: 'hidden-result' },
        billing,
      },
    },
  })
  renderTask()
  await userEvent.click(
    await screen.findByRole('button', { name: 'View Billing Estimate' })
  )
  expect(
    await screen.findByText('This estimate is not the final bill.')
  ).toBeVisible()
  expect(screen.getByText(billing.expression)).toBeVisible()
  expect(screen.getByText('standard')).toBeVisible()
  expect(screen.getByText(/"seconds": 5/)).toBeVisible()
  expect(screen.getByText(/"flag": "true"/)).toBeVisible()
  expect(screen.queryByText(/hidden-result/)).not.toBeInTheDocument()
  expect(screen.queryByText(/"invalid"|"infinite"/)).not.toBeInTheDocument()
})

test('billing metadata is optional and a successful raw response reports no estimate', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { task_id: task.task_id, data: null } },
  })
  renderTask()
  await userEvent.click(
    await screen.findByRole('button', { name: 'View Billing Estimate' })
  )
  expect(
    await screen.findByText('No billing estimate is available.')
  ).toBeVisible()
})

test.each([
  { kind: 'network', error: new Error('connection unavailable') },
  {
    kind: 'business',
    response: { data: { success: false, message: 'result is unavailable' } },
  },
])(
  '$kind failure is displayed in the expanded task section',
  async ({ error, response }) => {
    const request = vi.spyOn(api, 'get')
    if (error) request.mockRejectedValue(error)
    else request.mockResolvedValue(response)
    renderTask()
    await userEvent.click(
      await screen.findByRole('button', { name: 'View Task Data' })
    )
    expect(
      await screen.findByText(error?.message ?? 'result is unavailable')
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeVisible()
  }
)

test('a discarded result offers no result or estimate fetch', async () => {
  const request = vi.spyOn(api, 'get')
  renderTask({ log: { ...task, result_discarded: true } })
  await screen.findByText('Task Details')
  expect(
    screen.queryByRole('button', { name: 'View Task Data' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'View Billing Estimate' })
  ).not.toBeInTheDocument()
  expect(request).not.toHaveBeenCalled()
})

test('a closed dialog or non-admin view never fetches administrator task data', async () => {
  const request = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { task_id: task.task_id, data: { result: 'admin-result' } },
    },
  })
  const view = renderTask({ open: false })
  expect(request).not.toHaveBeenCalled()
  view.update({ open: true })
  await userEvent.click(
    await screen.findByRole('button', { name: 'View Task Data' })
  )
  await screen.findByText(/admin-result/)
  request.mockClear()
  view.update({ isAdmin: false, log: { ...task, task_id: 'another-task' } })
  await waitFor(() =>
    expect(
      screen.queryByRole('button', { name: 'View Task Data' })
    ).not.toBeInTheDocument()
  )
  expect(screen.queryByText(/admin-result/)).not.toBeInTheDocument()
  expect(request).not.toHaveBeenCalled()
})

test('switching tasks resets expansion and requires a new explicit request', async () => {
  const request = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: { task_id: task.task_id, data: { result: 'first-result' } },
    },
  })
  const view = renderTask()
  await userEvent.click(
    await screen.findByRole('button', { name: 'View Task Data' })
  )
  await screen.findByText(/first-result/)
  request.mockClear()
  view.update({ log: { ...task, task_id: 'another-task' } })
  expect(
    await screen.findByRole('button', { name: 'View Task Data' })
  ).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByText(/first-result/)).not.toBeInTheDocument()
  expect(request).not.toHaveBeenCalled()
})

test('a supplied usage schema renders task unit pricing while retaining the estimate notice', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        task_id: task.task_id,
        data: null,
        billing: {
          ...billing,
          usage_schema: {
            seconds: {
              type: 'number',
              unit: 'second',
              description: 'Video seconds',
            },
          },
        },
      },
    },
  })
  renderTask()
  await userEvent.click(
    await screen.findByRole('button', { name: 'View Billing Estimate' })
  )
  expect(
    await screen.findByText('This estimate is not the final bill.')
  ).toBeVisible()
  expect(screen.getAllByText('$0.1/s').length).toBeGreaterThan(0)
  expect(
    screen.queryByText(
      'Task usage metadata is unavailable. Pricing details cannot be displayed.'
    )
  ).not.toBeInTheDocument()
})
