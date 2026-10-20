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
export const PERFORMANCE_GRANULARITIES = [
  'auto',
  'minute',
  '5min',
  '15min',
  'hour',
  'day',
] as const

export type PerformanceGranularity = (typeof PERFORMANCE_GRANULARITIES)[number]

export type PerformanceSummary = {
  requests: number
  successes: number
  failures: number
  success_rate: number | null
  avg_tps: number | null
  max_tps: number | null
  min_tps: number | null
  avg_latency_ms: number | null
  p95_latency_ms: number | null
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
}

export type PerformanceSeriesPoint = PerformanceSummary & {
  ts: number
}

export type PerformanceGroupRow = PerformanceSummary & {
  group: string
}

export type PerformanceModelRow = PerformanceSummary & {
  model_name: string
}

export type PerformanceChannelRow = PerformanceSummary & {
  channel_id: number
  channel_name: string
}

export type SlowRequestRow = {
  created_at: number
  model_name: string
  channel_id: number
  channel_name: string
  token_name: string
  use_time: number
  completion_tokens: number
  request_id: string
}

export type ErrorLogRow = {
  created_at: number
  model_name: string
  channel_id: number
  channel_name: string
  token_name: string
  content: string
  request_id: string
}

export type PerformanceData = {
  granularity: PerformanceGranularity
  error_log_enabled: boolean
  summary: PerformanceSummary
  series: PerformanceSeriesPoint[]
  groups: PerformanceGroupRow[]
  models: PerformanceModelRow[]
  channels: PerformanceChannelRow[]
  slow_requests: SlowRequestRow[]
  errors: ErrorLogRow[]
}

export type PerformanceQueryParams = {
  start_timestamp: number
  end_timestamp: number
  granularity: PerformanceGranularity
  model_name?: string
  channel?: string
  group?: string
  token_name?: string
  username?: string
  is_stream?: string
}
