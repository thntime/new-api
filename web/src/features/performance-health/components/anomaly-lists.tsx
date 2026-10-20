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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import { formatTimestampToDate, formatUseTime } from '@/lib/format'

import type { ErrorLogRow, SlowRequestRow } from '../types'

const EMPTY_PLACEHOLDER = '-'

function RequestIdLink(props: { requestId: string }) {
  const { t } = useTranslation()
  if (!props.requestId) return <span>{EMPTY_PLACEHOLDER}</span>
  return (
    <Link
      to='/usage-logs/$section'
      params={{ section: 'common' }}
      search={{ requestId: props.requestId }}
      className='text-primary text-xs underline-offset-2 hover:underline'
    >
      {t('View')}
    </Link>
  )
}

function SlowRequestsTable(props: { rows: SlowRequestRow[] }) {
  const { t } = useTranslation()
  const columns: StaticDataTableColumn<SlowRequestRow>[] = [
    {
      id: 'time',
      header: t('Time'),
      cell: (row) => formatTimestampToDate(row.created_at),
    },
    { id: 'model', header: t('Model Name'), cell: (row) => row.model_name },
    {
      id: 'channel',
      header: t('Channel'),
      cell: (row) => row.channel_name || `#${row.channel_id}`,
    },
    {
      id: 'token',
      header: t('Token Name'),
      cell: (row) => row.token_name || EMPTY_PLACEHOLDER,
    },
    {
      id: 'use_time',
      header: t('Use Time'),
      cell: (row) => formatUseTime(row.use_time),
    },
    {
      id: 'completion_tokens',
      header: t('Output Tokens'),
      cell: (row) => row.completion_tokens,
    },
    {
      id: 'request_id',
      header: t('Request ID'),
      cell: (row) => <RequestIdLink requestId={row.request_id} />,
    },
  ]

  return (
    <PanelWrapper
      title={t('Slowest Requests')}
      loading={false}
      empty={props.rows.length === 0}
      emptyMessage={t('No data available')}
      height='h-32'
    >
      <StaticDataTable columns={columns} data={props.rows} />
    </PanelWrapper>
  )
}

function ErrorLogsTable(props: { rows: ErrorLogRow[] }) {
  const { t } = useTranslation()
  const columns: StaticDataTableColumn<ErrorLogRow>[] = [
    {
      id: 'time',
      header: t('Time'),
      cell: (row) => formatTimestampToDate(row.created_at),
    },
    { id: 'model', header: t('Model Name'), cell: (row) => row.model_name },
    {
      id: 'channel',
      header: t('Channel'),
      cell: (row) => row.channel_name || `#${row.channel_id}`,
    },
    {
      id: 'token',
      header: t('Token Name'),
      cell: (row) => row.token_name || EMPTY_PLACEHOLDER,
    },
    {
      id: 'content',
      header: t('Error'),
      cell: (row) => (
        <span className='line-clamp-1 max-w-64' title={row.content}>
          {row.content}
        </span>
      ),
    },
    {
      id: 'request_id',
      header: t('Request ID'),
      cell: (row) => <RequestIdLink requestId={row.request_id} />,
    },
  ]

  return (
    <PanelWrapper
      title={t('Recent Errors')}
      loading={false}
      empty={props.rows.length === 0}
      emptyMessage={t('No data available')}
      height='h-32'
    >
      <StaticDataTable columns={columns} data={props.rows} />
    </PanelWrapper>
  )
}

export function PerformanceAnomalyLists(props: {
  slowRequests: SlowRequestRow[]
  errors: ErrorLogRow[]
}) {
  return (
    <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
      <SlowRequestsTable rows={props.slowRequests} />
      <ErrorLogsTable rows={props.errors} />
    </div>
  )
}
