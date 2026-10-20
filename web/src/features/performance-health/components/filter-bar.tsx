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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { Loader2 } from 'lucide-react'
import { useMemo, useState, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
} from '@/features/usage-logs/components/logs-filter-toolbar'
import { getGroups } from '@/features/users/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  DEFAULT_TIME_RANGE_PRESET,
  STREAM_FILTER_ALL,
  STREAM_FILTER_NON_STREAM,
  STREAM_FILTER_STREAM,
  TIME_RANGE_PRESETS,
  resolveTimeRange,
  type PerformanceSearch,
  type StreamFilter,
  type TimeRangePreset,
} from '../lib/search'
import {
  PERFORMANCE_GRANULARITIES,
  type PerformanceGranularity,
} from '../types'

const route = getRouteApi('/_authenticated/usage-logs/performance')

const GRANULARITY_LABEL_KEYS: Record<PerformanceGranularity, string> = {
  auto: 'Auto',
  minute: '1 minute',
  '5min': '5 minutes',
  '15min': '15 minutes',
  hour: '1 hour',
  day: '1 day',
}

const PRESET_LABEL_KEYS: Record<TimeRangePreset, string> = {
  '1h': 'Last 1 hour',
  '6h': 'Last 6 hours',
  '24h': 'Last 24 hours',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

export function PerformanceHealthFilterBar(props: { searching?: boolean }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const searchParams = route.useSearch()
  const { data: adminGroups } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
  })
  const groupOptions = useMemo(() => {
    return (adminGroups?.data ?? [])
      .filter((group) => group !== 'auto')
      .map((group) => ({ label: group, value: group }))
  }, [adminGroups])

  // Draft state is synced from URL whenever the URL search key changes.
  const searchKey = JSON.stringify(searchParams)
  const [draftState, setDraftState] = useState(() => ({
    key: searchKey,
    search: { ...searchParams },
  }))
  const draft = useMemo(
    () =>
      draftState.key === searchKey ? draftState.search : { ...searchParams },
    [draftState, searchKey, searchParams]
  )

  const updateDraft = useCallback(
    (patch: Partial<PerformanceSearch>) => {
      setDraftState((current) => {
        const base =
          current.key === searchKey ? current.search : { ...searchParams }
        return { key: searchKey, search: { ...base, ...patch } }
      })
    },
    [searchKey, searchParams]
  )

  const handleApply = useCallback(() => {
    void navigate({
      to: '/usage-logs/performance',
      search: { ...draft },
      replace: false,
    })
  }, [draft, navigate])

  const preset = draft.preset ?? DEFAULT_TIME_RANGE_PRESET
  const useCustomRange = draft.startTime != null && draft.endTime != null
  const range = resolveTimeRange(draft)

  const handleReset = useCallback(() => {
    const empty: PerformanceSearch = { preset: DEFAULT_TIME_RANGE_PRESET }
    setDraftState({ key: searchKey, search: empty })
    void navigate({ to: '/usage-logs/performance', search: empty })
  }, [navigate, searchKey])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const applyPreset = (next: TimeRangePreset) => {
    // Clear custom range so the preset drives resolution.
    updateDraft({ preset: next, startTime: undefined, endTime: undefined })
  }

  const granularityItems = useMemo(
    () =>
      PERFORMANCE_GRANULARITIES.map((g) => ({
        value: g,
        label: t(GRANULARITY_LABEL_KEYS[g]),
      })),
    [t]
  )

  const streamItems = useMemo(
    () => [
      { value: STREAM_FILTER_ALL, label: t('All') },
      { value: STREAM_FILTER_STREAM, label: t('Streaming') },
      { value: STREAM_FILTER_NON_STREAM, label: t('Non-streaming') },
    ],
    [t]
  )

  const streamValue =
    draft.stream === STREAM_FILTER_STREAM ||
    draft.stream === STREAM_FILTER_NON_STREAM
      ? draft.stream
      : STREAM_FILTER_ALL

  const hasActiveFilters = Boolean(
    draft.model ||
    draft.channel ||
    draft.group ||
    draft.token ||
    draft.username ||
    streamValue !== STREAM_FILTER_ALL ||
    useCustomRange ||
    preset !== DEFAULT_TIME_RANGE_PRESET ||
    (draft.granularity && draft.granularity !== 'auto')
  )

  return (
    <div className='bg-card/50 rounded-lg border p-2.5 sm:p-3'>
      <div className='flex flex-wrap items-center gap-1.5'>
        {TIME_RANGE_PRESETS.map((p) => (
          <Button
            key={p}
            type='button'
            size='sm'
            variant={!useCustomRange && preset === p ? 'default' : 'outline'}
            className='h-8 px-2.5 text-xs'
            onClick={() => applyPreset(p)}
          >
            {t(PRESET_LABEL_KEYS[p])}
          </Button>
        ))}
      </div>

      <div className='mt-2 grid grid-cols-1 gap-2 sm:grid-cols-[repeat(auto-fit,minmax(10rem,1fr))]'>
        <LogsFilterField wide>
          <CompactDateTimeRangePicker
            start={useCustomRange ? range.start : undefined}
            end={useCustomRange ? range.end : undefined}
            onChange={({ start, end }) => {
              updateDraft({
                startTime: start?.getTime(),
                endTime: end?.getTime(),
              })
            }}
          />
        </LogsFilterField>
        <LogsFilterField>
          <Select
            items={granularityItems}
            value={draft.granularity ?? 'auto'}
            onValueChange={(value) =>
              updateDraft({
                granularity: (value ?? 'auto') as PerformanceGranularity,
              })
            }
          >
            <SelectTrigger aria-label={t('Granularity')}>
              <SelectValue className='min-w-0'>
                <span className='truncate'>
                  {t(GRANULARITY_LABEL_KEYS[draft.granularity ?? 'auto'])}
                </span>
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {granularityItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </LogsFilterField>
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Model Name')}
            value={draft.model ?? ''}
            onChange={(e) => updateDraft({ model: e.target.value })}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Channel ID')}
            value={draft.channel ?? ''}
            onChange={(e) => updateDraft({ channel: e.target.value })}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
        <LogsFilterField>
          <Combobox
            options={groupOptions}
            allowCustomValue
            aria-label={t('Group')}
            emptyText={t('No group found.')}
            placeholder={t('Group')}
            className='h-8 min-w-0 text-sm leading-5'
            value={draft.group ?? ''}
            onValueChange={(value) => updateDraft({ group: value ?? '' })}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Token Name')}
            value={draft.token ?? ''}
            onChange={(e) => updateDraft({ token: e.target.value })}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Username')}
            value={draft.username ?? ''}
            onChange={(e) => updateDraft({ username: e.target.value })}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
        <LogsFilterField>
          <Select
            items={streamItems}
            value={streamValue}
            onValueChange={(value) =>
              updateDraft({
                stream: (value ?? STREAM_FILTER_ALL) as StreamFilter,
              })
            }
          >
            <SelectTrigger aria-label={t('Stream')}>
              <SelectValue className='min-w-0'>
                <span className='truncate'>
                  {streamItems.find((item) => item.value === streamValue)
                    ?.label ?? t('All')}
                </span>
              </SelectValue>
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {streamItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </LogsFilterField>
      </div>

      <div className='mt-2 flex flex-wrap items-center justify-end gap-1.5 sm:gap-2'>
        <Button
          type='button'
          variant='outline'
          onClick={handleReset}
          disabled={!hasActiveFilters}
        >
          {t('Reset')}
        </Button>
        <Button type='button' onClick={handleApply} disabled={props.searching}>
          {props.searching && <Loader2 className='animate-spin' />}
          {t('Search')}
        </Button>
      </div>
    </div>
  )
}
