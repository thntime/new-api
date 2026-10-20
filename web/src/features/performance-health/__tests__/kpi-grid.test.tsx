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
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { PerformanceKpiGrid } from '../components/kpi-grid'
import type { PerformanceSummary } from '../types'

function buildSummary(
  patch: Partial<PerformanceSummary> = {}
): PerformanceSummary {
  return {
    requests: 0,
    successes: 0,
    failures: 0,
    success_rate: null,
    avg_tps: null,
    max_tps: null,
    min_tps: null,
    avg_latency_ms: null,
    p95_latency_ms: null,
    prompt_tokens: 0,
    completion_tokens: 0,
    total_tokens: 0,
    cache_tokens: 0,
    ...patch,
  }
}

describe('PerformanceKpiGrid', () => {
  it('renders empty placeholder for null metrics instead of 0', () => {
    render(<PerformanceKpiGrid summary={buildSummary()} />)
    const grid = screen.getByTestId('kpi-grid')
    expect(grid).toBeInTheDocument()

    expect(screen.getByTestId('kpi-success-rate').textContent).toContain('-')
    expect(screen.getByTestId('kpi-avg-tps').textContent).toContain('-')
    expect(screen.getByTestId('kpi-max-tps').textContent).toContain('-')
    expect(screen.getByTestId('kpi-min-tps').textContent).toContain('-')
    expect(screen.getByTestId('kpi-avg-latency').textContent).toContain('-')
    expect(screen.getByTestId('kpi-p95-latency').textContent).toContain('-')
    expect(screen.queryByTestId('kpi-success-rate')?.textContent).not.toContain(
      '0%'
    )
  })

  it('renders values when metrics are available', () => {
    render(
      <PerformanceKpiGrid
        summary={buildSummary({
          requests: 1500,
          failures: 3,
          success_rate: 99.8,
          avg_tps: 25.34,
          avg_latency_ms: 1009,
          p95_latency_ms: 4320,
          prompt_tokens: 12000,
          completion_tokens: 34000,
          total_tokens: 46000,
          cache_tokens: 9000,
        })}
      />
    )
    expect(screen.getByTestId('kpi-requests').textContent).toContain('1.5K')
    expect(screen.getByTestId('kpi-success-rate').textContent).toContain(
      '99.80%'
    )
    expect(screen.getByTestId('kpi-failures').textContent).toContain('3')
    expect(screen.getByTestId('kpi-avg-tps').textContent).toContain('25.34')
    expect(screen.getByTestId('kpi-avg-latency').textContent).toContain('1.01s')
    expect(screen.getByTestId('kpi-p95-latency').textContent).toContain('4.32s')
    expect(screen.getByTestId('kpi-total-tokens').textContent).toContain('46K')
    expect(screen.getByTestId('kpi-cache-tokens').textContent).toContain(
      '9K | 75.00%'
    )
  })
})
