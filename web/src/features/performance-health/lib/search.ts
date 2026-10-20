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
import dayjs from '@/lib/dayjs'

import type { PerformanceGranularity, PerformanceQueryParams } from '../types'

export const TIME_RANGE_PRESETS = ['1h', '6h', '24h', '7d', '30d'] as const
export type TimeRangePreset = (typeof TIME_RANGE_PRESETS)[number]

export const DEFAULT_TIME_RANGE_PRESET: TimeRangePreset = '24h'

export const STREAM_FILTER_ALL = 'all'
export const STREAM_FILTER_STREAM = 'stream'
export const STREAM_FILTER_NON_STREAM = 'non-stream'

export const STREAM_FILTERS = [
  STREAM_FILTER_ALL,
  STREAM_FILTER_STREAM,
  STREAM_FILTER_NON_STREAM,
] as const
export type StreamFilter = (typeof STREAM_FILTERS)[number]

export type PerformanceSearch = {
  preset?: TimeRangePreset
  startTime?: number
  endTime?: number
  granularity?: PerformanceGranularity
  model?: string
  channel?: string
  group?: string
  token?: string
  username?: string
  stream?: StreamFilter
}

export function getPresetTimeRange(preset: TimeRangePreset): {
  start: Date
  end: Date
} {
  const end = dayjs().toDate()
  const hours = { '1h': 1, '6h': 6, '24h': 24 }[preset as '1h' | '6h' | '24h']
  const start =
    hours !== undefined
      ? dayjs().subtract(hours, 'hour').toDate()
      : dayjs()
          .subtract(preset === '7d' ? 7 : 30, 'day')
          .toDate()
  return { start, end }
}

/**
 * Resolve the effective time range from URL search params. When a custom
 * start/end pair exists it wins over the preset; otherwise the preset is
 * expanded against "now".
 */
export function resolveTimeRange(search: PerformanceSearch): {
  start: Date
  end: Date
} {
  if (search.startTime != null && search.endTime != null) {
    return { start: new Date(search.startTime), end: new Date(search.endTime) }
  }
  return getPresetTimeRange(search.preset ?? DEFAULT_TIME_RANGE_PRESET)
}

/**
 * Build the request params for GET /api/log/performance from search state.
 */
export function buildPerformanceParams(
  search: PerformanceSearch
): PerformanceQueryParams {
  const { start, end } = resolveTimeRange(search)
  const params: PerformanceQueryParams = {
    start_timestamp: Math.floor(start.getTime() / 1000),
    end_timestamp: Math.floor(end.getTime() / 1000),
    granularity: search.granularity ?? 'auto',
  }
  if (search.model) params.model_name = search.model
  if (search.channel) params.channel = search.channel
  if (search.group) params.group = search.group
  if (search.token) params.token_name = search.token
  if (search.username) params.username = search.username
  if (search.stream === STREAM_FILTER_STREAM) params.is_stream = 'true'
  if (search.stream === STREAM_FILTER_NON_STREAM) params.is_stream = 'false'
  return params
}
