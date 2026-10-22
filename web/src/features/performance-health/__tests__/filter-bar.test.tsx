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
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { getGroups } from '@/features/users/api'

import { PerformanceHealthFilterBar } from '../components/filter-bar'

// 路由搜索参数由用例注入；useNavigate 在展开/收起交互中不会被调用。
const routeState = vi.hoisted(() => ({
  search: {} as Record<string, unknown>,
}))

vi.mock('@tanstack/react-router', () => ({
  getRouteApi: () => ({ useSearch: () => routeState.search }),
  useNavigate: () => vi.fn(),
}))

// Identity translation：按钮文本即 i18n 键，按键名定位元素。
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

vi.mock('@/features/users/api', () => ({
  getGroups: vi.fn(),
}))

// 时间范围选择器与筛选输入是纯展示协作者，本用例只保护展开/收起按钮契约。
vi.mock('@/features/usage-logs/components/compact-date-time-range-picker', () => ({
  CompactDateTimeRangePicker: () => <div data-testid='range-picker' />,
}))

vi.mock('@/features/usage-logs/components/logs-filter-toolbar', () => ({
  LogsFilterField: (props: { children?: React.ReactNode }) => (
    <div>{props.children}</div>
  ),
  LogsFilterInput: (props: { placeholder?: string }) => (
    <input placeholder={props.placeholder} />
  ),
}))

function renderFilterBar(search: Record<string, unknown>) {
  routeState.search = search
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <PerformanceHealthFilterBar />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  routeState.search = {}
  vi.mocked(getGroups).mockResolvedValue({ success: true, data: [] })
})

describe('performance health filter bar expand toggle', () => {
  test('collapsed with hidden filters renders the count badge and Expand label', () => {
    renderFilterBar({ group: 'vip', token: 'sk-test' })

    const toggle = screen.getByRole('button', { name: /Expand/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  test('expanding switches the label to Collapse and keeps the active count visible', () => {
    renderFilterBar({ group: 'vip', token: 'sk-test' })

    fireEvent.click(screen.getByRole('button', { name: /Expand/ }))

    const toggle = screen.getByRole('button', { name: /Collapse/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    // 筛选值仍然生效，计数不应因展开而消失。
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  test('no hidden filters renders no count badge', () => {
    renderFilterBar({})

    const toggle = screen.getByRole('button', { name: /Expand/ })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('2')).not.toBeInTheDocument()
  })
})
