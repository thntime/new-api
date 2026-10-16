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
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { TagRow } from '../../lib'
import type { Channel, ChannelInfo } from '../../types'
import { FallbackCell, useChannelsColumns } from '../channels-columns'

vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
  }
})

vi.mock('../channels-provider', () => ({
  useChannels: () => ({
    setOpen: vi.fn(),
    setCurrentRow: vi.fn(),
    sensitiveVisible: true,
    upstream: {
      openModal: vi.fn(),
      detectChannelUpdates: vi.fn(),
    },
  }),
}))

vi.mock('sonner', async (importOriginal) => {
  const actual = await importOriginal<typeof import('sonner')>()
  return {
    ...actual,
    toast: {
      ...actual.toast,
      success: vi.fn(),
      error: vi.fn(),
    },
  }
})

const invalidateQueriesMock = vi.fn()

function createChannelInfo(overrides: Partial<ChannelInfo> = {}): ChannelInfo {
  return {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
    is_fallback: false,
    ...overrides,
  } as ChannelInfo
}

function createChannel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: 7,
    name: 'Test Channel',
    type: 1,
    status: 1,
    channel_info: createChannelInfo(),
    ...overrides,
  } as Channel
}

function collectColumns() {
  let columns: ReturnType<typeof useChannelsColumns>
  function CaptureColumns() {
    columns = useChannelsColumns()
    return null
  }
  render(<CaptureColumns />)
  return () => columns
}

beforeEach(() => {
  invalidateQueriesMock.mockClear()
  vi.spyOn(api, 'put').mockResolvedValue({
    data: { success: true },
  } as never)
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('fallback column definition', () => {
  test('places the fallback column right before the priority column', () => {
    const getColumns = collectColumns()
    const columns = getColumns()

    const fallbackIndex = columns.findIndex(
      (column) => 'id' in column && column.id === 'fallback'
    )
    const priorityIndex = columns.findIndex(
      (column) => 'accessorKey' in column && column.accessorKey === 'priority'
    )

    expect(fallbackIndex).toBeGreaterThanOrEqual(0)
    expect(priorityIndex).toBe(fallbackIndex + 1)
  })

  test('configures small size, no sorting and a hideable header', () => {
    const getColumns = collectColumns()
    const fallbackColumn = getColumns().find(
      (column) => 'id' in column && column.id === 'fallback'
    ) as
      | {
          id?: string
          header?: unknown
          size?: unknown
          minSize?: unknown
          enableSorting?: unknown
        }
      | undefined

    expect(fallbackColumn?.header).toBe('Fallback')
    expect(fallbackColumn?.size).toBe(72)
    expect(fallbackColumn?.minSize).toBe(56)
    expect(fallbackColumn?.enableSorting).toBe(false)
  })
})

describe('FallbackCell', () => {
  test('renders an unpressed toggle for a non-fallback channel and presses it after marking', async () => {
    const user = userEvent.setup()
    const channel = createChannel({
      id: 11,
      name: 'Primary Channel',
    })
    render(<FallbackCell channel={channel} />)

    const toggle = screen.getByRole('button', {
      name: 'Not a fallback channel: Primary Channel',
    })
    expect(toggle).toHaveAttribute('aria-pressed', 'false')

    await user.click(toggle)

    await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
    expect(api.put).toHaveBeenCalledWith(
      '/api/channel/',
      {
        id: 11,
        channel_info: createChannelInfo({ is_fallback: true }),
      },
      expect.anything()
    )
    await waitFor(() =>
      expect(invalidateQueriesMock).toHaveBeenCalledWith({
        queryKey: ['channels', 'list'],
      })
    )
  })

  test('sends is_fallback false when unmarking an existing fallback channel', async () => {
    const user = userEvent.setup()
    const channel = createChannel({
      id: 12,
      name: 'Backup Channel',
      channel_info: createChannelInfo({ is_fallback: true }),
    })
    render(<FallbackCell channel={channel} />)

    const toggle = screen.getByRole('button', {
      name: 'Fallback channel: Backup Channel',
    })
    expect(toggle).toHaveAttribute('aria-pressed', 'true')

    await user.click(toggle)

    await waitFor(() => expect(api.put).toHaveBeenCalledTimes(1))
    expect(api.put).toHaveBeenCalledWith(
      '/api/channel/',
      {
        id: 12,
        channel_info: createChannelInfo({ is_fallback: false }),
      },
      expect.anything()
    )
  })

  test('keeps the current state and shows a toast when the update fails', async () => {
    const user = userEvent.setup()
    vi.spyOn(api, 'put').mockResolvedValue({
      data: { success: false, message: 'save failed' },
    } as never)
    const channel = createChannel({ id: 13, name: 'Failing Channel' })
    render(<FallbackCell channel={channel} />)

    const toggle = screen.getByRole('button', {
      name: 'Not a fallback channel: Failing Channel',
    })
    await user.click(toggle)

    await waitFor(() => expect(toast.error).toHaveBeenCalled())
    // No optimistic update: the toggle still reflects the original state.
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
    expect(invalidateQueriesMock).not.toHaveBeenCalled()
  })

  test('renders nothing for a tag aggregate row', () => {
    const tagRow = {
      ...createChannel(),
      tag: 'test-tag',
      children: [],
    } as TagRow
    const { container } = render(<FallbackCell channel={tagRow} />)

    expect(container).toBeEmptyDOMElement()
  })
})
