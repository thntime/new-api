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
import {
  formatLatency,
  formatUptimePct,
  getSuccessRateTextClass,
} from '@/features/performance-metrics/lib/format'
import { formatCompactNumber } from '@/lib/format'

import type { PerformanceSummary } from '../types'

export const EMPTY_PLACEHOLDER = '-'

// Charts and KPI cards keep two decimals so TPS values are comparable at a glance.
export function formatTps(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '—'
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K t/s`
  return `${value.toFixed(2)} t/s`
}

export interface MetricItem {
  id: string
  label: string
  value: string
  valueClassName?: string
}

export function buildMetricItems(
  summary: PerformanceSummary,
  t: (key: string) => string,
  locale?: string
): MetricItem[] {
  const formatNullablePercent = (value: number | null) =>
    value == null ? EMPTY_PLACEHOLDER : formatUptimePct(value)
  const formatNullableTps = (value: number | null) =>
    value == null ? EMPTY_PLACEHOLDER : formatTps(value)
  const formatNullableLatency = (value: number | null) =>
    value == null ? EMPTY_PLACEHOLDER : formatLatency(value)

  return [
    {
      id: 'requests',
      label: t('Requests'),
      value: formatCompactNumber(summary.requests, locale),
    },
    {
      id: 'success-rate',
      label: t('Success Rate'),
      value: formatNullablePercent(summary.success_rate),
      valueClassName:
        summary.success_rate == null
          ? 'text-muted-foreground'
          : getSuccessRateTextClass(summary.success_rate),
    },
    {
      id: 'failures',
      label: t('Errors'),
      value: formatCompactNumber(summary.failures, locale),
    },
    {
      id: 'avg-tps',
      label: t('Avg TPS'),
      value: formatNullableTps(summary.avg_tps),
    },
    {
      id: 'max-tps',
      label: t('Max TPS'),
      value: formatNullableTps(summary.max_tps),
    },
    {
      id: 'min-tps',
      label: t('Min TPS'),
      value: formatNullableTps(summary.min_tps),
    },
    {
      id: 'avg-latency',
      label: t('Avg Latency'),
      value: formatNullableLatency(summary.avg_latency_ms),
    },
    {
      id: 'p95-latency',
      label: t('P95 Latency'),
      value: formatNullableLatency(summary.p95_latency_ms),
    },
    {
      id: 'prompt-tokens',
      label: t('Input Tokens'),
      value: formatCompactNumber(summary.prompt_tokens, locale),
    },
    {
      id: 'completion-tokens',
      label: t('Output Tokens'),
      value: formatCompactNumber(summary.completion_tokens, locale),
    },
    {
      id: 'total-tokens',
      label: t('Total Tokens'),
      value: formatCompactNumber(summary.total_tokens, locale),
    },
    {
      id: 'cache-tokens',
      label: t('Cache Tokens'),
      value:
        summary.prompt_tokens > 0
          ? `${formatCompactNumber(summary.cache_tokens, locale)} | ${((summary.cache_tokens / summary.prompt_tokens) * 100).toFixed(2)}%`
          : formatCompactNumber(summary.cache_tokens, locale),
    },
  ]
}
