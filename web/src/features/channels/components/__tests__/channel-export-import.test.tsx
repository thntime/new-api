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
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { toast } from 'sonner'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { channelsQueryKeys } from '../../lib'
import { ChannelsPrimaryButtons } from '../channels-primary-buttons'
import { ChannelsProvider } from '../channels-provider'
import { DataTableBulkActions } from '../data-table-bulk-actions'
import { ExportChannelsDialog } from '../dialogs/export-channels-dialog'
import { ImportChannelsDialog } from '../dialogs/import-channels-dialog'

function renderWithQueryClient(ui: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  render(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
  )
  return queryClient
}

beforeEach(() => {
  // jsdom does not implement the object URL API, so define it before spying.
  Object.defineProperty(URL, 'createObjectURL', {
    configurable: true,
    writable: true,
    value: () => '',
  })
  Object.defineProperty(URL, 'revokeObjectURL', {
    configurable: true,
    writable: true,
    value: () => undefined,
  })
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:channel-export')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
})

afterEach(() => {
  Reflect.deleteProperty(URL, 'createObjectURL')
  Reflect.deleteProperty(URL, 'revokeObjectURL')
  i18next.removeResourceBundle('en', 'translation')
  useAuthStore.getState().auth.reset()
  localStorage.clear()
})

test('export dialog posts the selected ids in order and downloads the encrypted file', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        filename: 'channels-20261123.nacx',
        content: '{"format":"new-api-channel-export"}',
      },
    },
  })
  let downloadedName: string | null = null
  let downloadedHref: string | null = null
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(
    function (this: HTMLAnchorElement) {
      downloadedName = this.download
      downloadedHref = this.href
    }
  )
  const success = vi.spyOn(toast, 'success')

  renderWithQueryClient(
    <ExportChannelsDialog open onOpenChange={() => {}} ids={[3, 1, 2]} />
  )

  await user.type(screen.getByLabelText('Password'), 'secret123')
  await user.click(screen.getByRole('button', { name: 'Export' }))

  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    '/api/channel/export',
    { ids: [3, 1, 2], password: 'secret123' },
    expect.objectContaining({ skipBusinessError: true, skipErrorHandler: true })
  )
  await waitFor(() =>
    expect(downloadedName).toBe('channels-20261123.nacx')
  )
  expect(downloadedHref).toBe('blob:channel-export')
  const blob = vi.mocked(URL.createObjectURL).mock.calls[0][0] as Blob
  expect(await blob.text()).toBe('{"format":"new-api-channel-export"}')
  expect(success).toHaveBeenCalledWith('Exported 3 channels')
})

test('import dialog reads the selected file and posts the password with its text', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, data: { count: 2, ids: [10, 11] } },
  })
  const success = vi.spyOn(toast, 'success')
  const queryClient = renderWithQueryClient(
    <ImportChannelsDialog open onOpenChange={() => {}} />
  )
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')

  await user.upload(
    screen.getByLabelText('Channel export file'),
    new File(['{"cipher":"abc"}'], 'channels.nacx', {
      type: 'application/json',
    })
  )
  await user.type(screen.getByLabelText('Password'), 'secret123')
  await user.click(screen.getByRole('button', { name: 'Import' }))

  await waitFor(() => expect(post).toHaveBeenCalledTimes(1))
  expect(post).toHaveBeenCalledWith(
    '/api/channel/import',
    { password: 'secret123', content: '{"cipher":"abc"}' },
    expect.objectContaining({ skipBusinessError: true, skipErrorHandler: true })
  )
  expect(success).toHaveBeenCalledWith('Imported 2 channels')
  expect(invalidate).toHaveBeenCalledWith({
    queryKey: channelsQueryKeys.lists(),
  })
})

test('a wrong password import shows the server error key', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: false,
      message: 'Wrong password or corrupted export file',
      message_key: 'Wrong password or corrupted export file',
    },
  })
  const error = vi.spyOn(toast, 'error')

  renderWithQueryClient(<ImportChannelsDialog open onOpenChange={() => {}} />)

  await user.upload(
    screen.getByLabelText('Channel export file'),
    new File(['{}'], 'channels.nacx', { type: 'application/json' })
  )
  await user.type(screen.getByLabelText('Password'), 'wrong-password')
  await user.click(screen.getByRole('button', { name: 'Import' }))

  await waitFor(() =>
    expect(error).toHaveBeenCalledWith('Wrong password or corrupted export file')
  )
})

test('translates the inner error of a rejected import', async () => {
  const user = userEvent.setup()
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: false,
      message_key: 'Channel {{index}} ({{name}}) is invalid: {{error}}',
      message_params: {
        index: 2,
        name: 'import-rollback-third',
        error:
          'Request plugin instance missing-request-plugin does not exist',
        error_key: 'Request plugin instance {{id}} does not exist',
        error_params: { id: 'missing-request-plugin' },
      },
    },
  })
  i18next.addResourceBundle(
    'en',
    'translation',
    {
      'Request plugin instance {{id}} does not exist':
        'Request plugin {{id}} is missing',
    },
    true,
    true
  )
  const error = vi.spyOn(toast, 'error')

  renderWithQueryClient(<ImportChannelsDialog open onOpenChange={() => {}} />)

  await user.upload(
    screen.getByLabelText('Channel export file'),
    new File(['{}'], 'channels.nacx', { type: 'application/json' })
  )
  await user.type(screen.getByLabelText('Password'), 'wrong-password')
  await user.click(screen.getByRole('button', { name: 'Import' }))

  await waitFor(() => expect(error).toHaveBeenCalled())
  const shown = String(error.mock.calls.at(-1)?.[0])
  expect(shown).toContain('Channel 2 (import-rollback-third) is invalid:')
  expect(shown).toContain('Request plugin missing-request-plugin is missing')
  expect(shown).not.toContain(
    'Request plugin instance missing-request-plugin does not exist'
  )
})

test('rejects a password shorter than 8 characters before sending the request', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post')
  const error = vi.spyOn(toast, 'error')

  renderWithQueryClient(
    <ExportChannelsDialog open onOpenChange={() => {}} ids={[1]} />
  )

  await user.type(screen.getByLabelText('Password'), 'short')
  await user.click(screen.getByRole('button', { name: 'Export' }))

  expect(error).toHaveBeenCalledWith(
    'Password must contain between 8 and 128 characters.'
  )
  expect(post).not.toHaveBeenCalled()
})

test('counts password code points so 4 emoji are rejected', async () => {
  const user = userEvent.setup()
  const post = vi.spyOn(api, 'post')
  const error = vi.spyOn(toast, 'error')

  renderWithQueryClient(
    <ExportChannelsDialog open onOpenChange={() => {}} ids={[1]} />
  )

  await user.type(screen.getByLabelText('Password'), '😀😀😀😀')
  await user.click(screen.getByRole('button', { name: 'Export' }))

  expect(error).toHaveBeenCalledWith(
    'Password must contain between 8 and 128 characters.'
  )
  expect(post).not.toHaveBeenCalled()
})

function BulkActionsFixture(props: { selected: boolean }) {
  const table = useReactTable({
    data: [{ id: 3 }, { id: 1 }, { id: 2 }],
    columns: [{ id: 'id', accessorKey: 'id' }],
    state: { rowSelection: props.selected ? { 0: true, 1: true } : {} },
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  })

  return <DataTableBulkActions table={table} />
}

test('hides the export entry when no channel is selected', () => {
  renderWithQueryClient(<BulkActionsFixture selected={false} />)

  expect(
    screen.queryByRole('button', { name: 'Export selected channels' })
  ).toBeNull()
})

test('offers the export entry once channels are selected', () => {
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'root',
    role: ROLE.SUPER_ADMIN,
    group: 'default',
  })
  renderWithQueryClient(<BulkActionsFixture selected />)

  expect(
    screen.getByRole('button', { name: 'Export selected channels' })
  ).toBeEnabled()
})

async function mountPrimaryButtons(
  role: number,
  adminPermissions?: Record<string, Record<string, boolean>>
) {
  useAuthStore.getState().auth.setUser({
    id: 2,
    username: 'operator',
    display_name: 'Operator',
    role,
    group: 'default',
    permissions: adminPermissions
      ? { admin_permissions: adminPermissions }
      : undefined,
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/channel/priority/order') {
      return {
        data: {
          success: true,
          data: { revision: 'test', items: [], changes: [] },
        },
      }
    }
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    return { data: { success: true, data: null } }
  })

  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const channels = createRoute({
    getParentRoute: () => authenticated,
    path: '/channels/',
    validateSearch: (search) => ({ page: 1, pageSize: 10, ...search }),
    component: () => (
      <ChannelsProvider>
        <ChannelsPrimaryButtons />
      </ChannelsProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([channels])]),
    history: createMemoryHistory({ initialEntries: ['/channels/'] }),
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <RouterProvider router={router} />
    </QueryClientProvider>
  )

  return screen.findByRole('button', { name: 'More actions' })
}

test('disables the import entry without channel sensitive write permission', async () => {
  const user = userEvent.setup()
  await mountPrimaryButtons(ROLE.ADMIN, { channel: { read: true } })

  await user.click(screen.getByRole('button', { name: 'More actions' }))

  expect(
    await screen.findByRole('menuitem', { name: 'Import Channels' })
  ).toHaveAttribute('aria-disabled', 'true')
})

test('enables the import entry with channel sensitive write permission', async () => {
  const user = userEvent.setup()
  await mountPrimaryButtons(ROLE.ADMIN, {
    channel: { read: true, sensitive_write: true },
  })

  await user.click(screen.getByRole('button', { name: 'More actions' }))

  expect(
    await screen.findByRole('menuitem', { name: 'Import Channels' })
  ).toBeEnabled()
})
