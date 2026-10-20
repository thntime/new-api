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
import { getRouteApi } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'

import { getPerformanceHealth } from './api'
import { PerformanceAnomalyLists } from './components/anomaly-lists'
import { PerformanceBreakdownTables } from './components/breakdown-tables'
import { PerformanceHealthFilterBar } from './components/filter-bar'
import { PerformanceKpiGrid } from './components/kpi-grid'
import { PerformanceTrendCharts } from './components/trend-charts'
import { buildPerformanceParams } from './lib/search'

const route = getRouteApi('/_authenticated/usage-logs/performance')

export function PerformanceHealth() {
  const { t } = useTranslation()
  const search = route.useSearch()
  const params = useMemo(() => buildPerformanceParams(search), [search])

  const query = useQuery({
    queryKey: ['performance-health', params],
    queryFn: () => getPerformanceHealth(params),
  })

  const data = query.data
  const hasData =
    data != null &&
    (data.summary.requests > 0 ||
      data.series.length > 0 ||
      data.groups.length > 0 ||
      data.models.length > 0 ||
      data.channels.length > 0)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Performance Health')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='flex flex-col gap-3'>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Latency is calculated from per-request logs at second precision; the success rate counts each attempt, so a retried request is counted once per attempt.'
            )}
          </p>

          <PerformanceHealthFilterBar searching={query.isFetching} />

          {query.isPending && <LoadingState className='min-h-[320px]' />}

          {query.isError && (
            <ErrorState
              description={
                query.error instanceof Error && query.error.message
                  ? t(query.error.message)
                  : t(
                      'Failed to load performance metrics. Adjust the filters or retry.'
                    )
              }
              onRetry={() => void query.refetch()}
            />
          )}

          {data && !data.error_log_enabled && (
            <Alert>
              <AlertDescription>
                {t(
                  'Error logging is disabled: failure counts and success rate may be higher than actual availability.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {data && (
            <p className='text-muted-foreground text-xs'>
              {t('Granularity')}: {data.granularity}
            </p>
          )}

          {data && !hasData && (
            <EmptyState
              title={t('No Data')}
              description={t(
                'No requests matched the current filters in the selected time range.'
              )}
            />
          )}

          {data && hasData && (
            <>
              <PerformanceKpiGrid summary={data.summary} />
              <PerformanceTrendCharts
                series={data.series}
                granularity={data.granularity}
              />
              <PerformanceBreakdownTables
                groups={data.groups}
                models={data.models}
                channels={data.channels}
              />
              <PerformanceAnomalyLists
                slowRequests={data.slow_requests}
                errors={data.errors}
              />
            </>
          )}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
