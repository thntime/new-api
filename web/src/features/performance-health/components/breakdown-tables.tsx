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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber } from '@/lib/format'

import type {
  PerformanceChannelRow,
  PerformanceGroupRow,
  PerformanceModelRow,
  PerformanceSummary,
} from '../types'

const EMPTY_PLACEHOLDER = '-'

type SortableKey = 'requests' | 'avg_latency_ms' | 'total_tokens'

type BreakdownRow = PerformanceSummary & { name: string }

function compareNullable(a: number | null, b: number | null): number {
  if (a == null && b == null) return 0
  if (a == null) return 1
  if (b == null) return -1
  return b - a
}

function BreakdownTable(props: { title: string; rows: BreakdownRow[] }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [sortBy, setSortBy] = useState<SortableKey>('requests')

  const sorted = useMemo(() => {
    const rows = [...props.rows]
    rows.sort((a, b) => {
      const key = sortBy
      return compareNullable(a[key], b[key])
    })
    return rows
  }, [props.rows, sortBy])

  const sortableHeader = (id: SortableKey, label: string) => (
    <Button
      type='button'
      variant='ghost'
      size='sm'
      className='h-auto px-1 font-medium'
      onClick={() => setSortBy(id)}
      aria-pressed={sortBy === id}
    >
      {label}
      {sortBy === id ? ' ↓' : ''}
    </Button>
  )

  const columns: StaticDataTableColumn<BreakdownRow>[] = [
    { id: 'name', header: t('Name'), cell: (row) => row.name || '-' },
    {
      id: 'requests',
      header: sortableHeader('requests', t('Requests')),
      cell: (row) => formatCompactNumber(row.requests, locale),
    },
    {
      id: 'success_rate',
      header: t('Success Rate'),
      cell: (row) =>
        row.success_rate == null
          ? EMPTY_PLACEHOLDER
          : formatUptimePct(row.success_rate),
    },
    {
      id: 'avg_latency_ms',
      header: sortableHeader('avg_latency_ms', t('Avg Latency')),
      cell: (row) =>
        row.avg_latency_ms == null
          ? EMPTY_PLACEHOLDER
          : formatLatency(row.avg_latency_ms),
    },
    {
      id: 'avg_tps',
      header: t('Avg TPS'),
      cell: (row) =>
        row.avg_tps == null ? EMPTY_PLACEHOLDER : formatThroughput(row.avg_tps),
    },
    {
      id: 'max_tps',
      header: t('Max TPS'),
      cell: (row) =>
        row.max_tps == null ? EMPTY_PLACEHOLDER : formatThroughput(row.max_tps),
    },
    {
      id: 'min_tps',
      header: t('Min TPS'),
      cell: (row) =>
        row.min_tps == null ? EMPTY_PLACEHOLDER : formatThroughput(row.min_tps),
    },
    {
      id: 'total_tokens',
      header: sortableHeader('total_tokens', t('Total Tokens')),
      cell: (row) => formatCompactNumber(row.total_tokens, locale),
    },
  ]

  return (
    <PanelWrapper
      title={props.title}
      loading={false}
      empty={props.rows.length === 0}
      emptyMessage={t('No data available')}
      height='h-32'
    >
      <StaticDataTable columns={columns} data={sorted} />
    </PanelWrapper>
  )
}

export function PerformanceBreakdownTables(props: {
  groups: PerformanceGroupRow[]
  models: PerformanceModelRow[]
  channels: PerformanceChannelRow[]
}) {
  const { t } = useTranslation()
  const groupRows: BreakdownRow[] = props.groups.map((row) => ({
    ...row,
    name: row.group,
  }))
  const modelRows: BreakdownRow[] = props.models.map((row) => ({
    ...row,
    name: row.model_name,
  }))
  const channelRows: BreakdownRow[] = props.channels.map((row) => ({
    ...row,
    name: row.channel_name
      ? `${row.channel_name} (#${row.channel_id})`
      : `#${row.channel_id}`,
  }))

  return (
    <div className='flex flex-col gap-3'>
      <BreakdownTable title={t('By Group')} rows={groupRows} />
      <BreakdownTable title={t('By Model')} rows={modelRows} />
      <BreakdownTable title={t('By Channel')} rows={channelRows} />
    </div>
  )
}
