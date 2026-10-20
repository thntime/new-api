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
import { describe, expect, it } from 'vitest'

import {
  DEFAULT_TIME_RANGE_PRESET,
  buildPerformanceParams,
  resolveTimeRange,
} from '../lib/search'

describe('buildPerformanceParams', () => {
  it('defaults to the 24h preset with auto granularity', () => {
    const params = buildPerformanceParams({})
    expect(params.granularity).toBe('auto')
    expect(params.model_name).toBeUndefined()
    expect(params.channel).toBeUndefined()
    expect(params.group).toBeUndefined()
    expect(params.token_name).toBeUndefined()
    expect(params.username).toBeUndefined()
    expect(params.is_stream).toBeUndefined()
    expect(params.end_timestamp - params.start_timestamp).toBe(24 * 3600)
  })

  it('maps filters to API params', () => {
    const params = buildPerformanceParams({
      startTime: 1_000_000,
      endTime: 1_360_000,
      granularity: 'hour',
      model: 'gpt-4o',
      channel: '12',
      group: 'vip',
      token: 'sk-test',
      username: 'alice',
      stream: 'stream',
    })
    expect(params.start_timestamp).toBe(1000)
    expect(params.end_timestamp).toBe(1360)
    expect(params.granularity).toBe('hour')
    expect(params.model_name).toBe('gpt-4o')
    expect(params.channel).toBe('12')
    expect(params.group).toBe('vip')
    expect(params.token_name).toBe('sk-test')
    expect(params.username).toBe('alice')
    expect(params.is_stream).toBe('true')
  })

  it('maps non-streaming filter to is_stream=false', () => {
    const params = buildPerformanceParams({ stream: 'non-stream' })
    expect(params.is_stream).toBe('false')
  })

  it('prefers custom start/end over preset', () => {
    const search = {
      preset: '7d' as const,
      startTime: 2_000_000,
      endTime: 5_000_000,
    }
    const range = resolveTimeRange(search)
    expect(range.start.getTime()).toBe(2_000_000)
    expect(range.end.getTime()).toBe(5_000_000)
  })

  it('uses the default preset when no search params are set', () => {
    const range = resolveTimeRange({})
    const now = Date.now()
    expect(now - range.start.getTime()).toBeGreaterThanOrEqual(23 * 3600 * 1000)
    expect(now - range.start.getTime()).toBeLessThanOrEqual(25 * 3600 * 1000)
    expect(DEFAULT_TIME_RANGE_PRESET).toBe('24h')
  })
})
