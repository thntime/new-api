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
import { VChart } from '@visactor/react-vchart'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { PanelWrapper } from '@/features/dashboard/components/ui/panel-wrapper'
import {
  formatLatency,
  formatThroughput,
  formatUptimePct,
} from '@/features/performance-metrics/lib/format'
import { formatCompactNumber } from '@/lib/format'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import { formatBucketLabel } from '../lib/format-bucket'
import type { PerformanceGranularity, PerformanceSeriesPoint } from '../types'

function getChartThemeTokens(resolvedTheme: string) {
  return {
    textColor:
      resolvedTheme === 'dark'
        ? 'rgba(255, 255, 255, 0.68)'
        : 'rgba(15, 23, 42, 0.58)',
    gridColor:
      resolvedTheme === 'dark'
        ? 'rgba(255, 255, 255, 0.12)'
        : 'rgba(15, 23, 42, 0.12)',
  }
}

function ChartFrame(props: {
  resolvedTheme: string
  themeReady: boolean
  spec: unknown
  name: string
}) {
  if (!props.themeReady || !props.spec) return null
  return (
    <div className='h-56 sm:h-64'>
      <VChart
        key={`${props.name}-${props.resolvedTheme}`}
        spec={{
          ...(props.spec as Record<string, unknown>),
          theme: props.resolvedTheme === 'dark' ? 'dark' : 'light',
          background: 'transparent',
        }}
        option={VCHART_OPTION}
      />
    </div>
  )
}

function TpsChart(props: {
  series: PerformanceSeriesPoint[]
  granularity: PerformanceGranularity
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const spec = useMemo(() => {
    if (props.series.length === 0) return null
    const { textColor, gridColor } = getChartThemeTokens(resolvedTheme)
    const data = props.series.flatMap((point) => [
      {
        time: formatBucketLabel(point.ts, props.granularity),
        tps: point.avg_tps,
        kind: t('Avg TPS'),
      },
      {
        time: formatBucketLabel(point.ts, props.granularity),
        tps: point.max_tps,
        kind: t('Max TPS'),
      },
      {
        time: formatBucketLabel(point.ts, props.granularity),
        tps: point.min_tps,
        kind: t('Min TPS'),
      },
    ])
    return {
      type: 'line' as const,
      data: [{ id: 'tps-trend', values: data }],
      xField: 'time',
      yField: 'tps',
      seriesField: 'kind',
      smooth: true,
      point: { visible: false },
      line: { style: { lineWidth: 2 } },
      legends: { visible: true, position: 'top' },
      tooltip: {
        mark: {
          content: [
            {
              key: (d: { kind: string }) => d.kind,
              value: (d: { tps: number | null }) =>
                d.tps == null ? '-' : formatThroughput(d.tps),
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fill: textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          label: {
            formatMethod: (val: number | string) =>
              `${formatCompactNumber(Number(val))}`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
      ],
    }
  }, [props.series, props.granularity, resolvedTheme, t])

  return (
    <PanelWrapper
      title={t('TPS Trend')}
      loading={false}
      empty={props.series.length === 0}
      emptyMessage={t('No data available')}
      height='h-56 sm:h-64'
    >
      <ChartFrame
        resolvedTheme={resolvedTheme}
        themeReady={themeReady}
        spec={spec}
        name='tps-trend'
      />
    </PanelWrapper>
  )
}

function LatencyChart(props: {
  series: PerformanceSeriesPoint[]
  granularity: PerformanceGranularity
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const spec = useMemo(() => {
    if (props.series.length === 0) return null
    const { textColor, gridColor } = getChartThemeTokens(resolvedTheme)
    const data = props.series.flatMap((point) => [
      {
        time: formatBucketLabel(point.ts, props.granularity),
        latency: point.avg_latency_ms,
        kind: t('Avg Latency'),
      },
      {
        time: formatBucketLabel(point.ts, props.granularity),
        latency: point.p95_latency_ms,
        kind: t('P95 Latency'),
      },
    ])
    return {
      type: 'line' as const,
      data: [{ id: 'latency-trend', values: data }],
      xField: 'time',
      yField: 'latency',
      seriesField: 'kind',
      smooth: true,
      point: { visible: false },
      line: { style: { lineWidth: 2 } },
      legends: { visible: true, position: 'top' },
      tooltip: {
        mark: {
          content: [
            {
              key: (d: { kind: string }) => d.kind,
              value: (d: { latency: number | null }) =>
                d.latency == null ? '-' : formatLatency(d.latency),
            },
          ],
        },
      },
      axes: [
        {
          orient: 'bottom',
          label: { style: { fill: textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          label: {
            formatMethod: (val: number | string) =>
              `${formatCompactNumber(Number(val))}`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
      ],
    }
  }, [props.series, props.granularity, resolvedTheme, t])

  return (
    <PanelWrapper
      title={t('Latency Trend')}
      loading={false}
      empty={props.series.length === 0}
      emptyMessage={t('No data available')}
      height='h-56 sm:h-64'
    >
      <ChartFrame
        resolvedTheme={resolvedTheme}
        themeReady={themeReady}
        spec={spec}
        name='latency-trend'
      />
    </PanelWrapper>
  )
}

function AvailabilityChart(props: {
  series: PerformanceSeriesPoint[]
  granularity: PerformanceGranularity
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const spec = useMemo(() => {
    if (props.series.length === 0) return null
    const { textColor, gridColor } = getChartThemeTokens(resolvedTheme)
    const data = props.series.map((point) => ({
      time: formatBucketLabel(point.ts, props.granularity),
      success_rate: point.success_rate,
      failures: point.failures,
    }))
    return {
      type: 'common' as const,
      data: [{ id: 'availability-trend', values: data }],
      series: [
        {
          type: 'line' as const,
          xField: 'time',
          yField: 'success_rate',
          name: t('Success Rate'),
          smooth: true,
          point: { visible: false },
          line: { style: { lineWidth: 2, stroke: '#10b981' } },
        },
        {
          type: 'bar' as const,
          xField: 'time',
          yField: 'failures',
          name: t('Errors'),
          bar: { style: { fill: '#ef4444', opacity: 0.35 } },
        },
      ],
      axes: [
        {
          orient: 'bottom',
          label: { style: { fill: textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          seriesIndex: [0],
          min: 0,
          max: 100,
          label: {
            formatMethod: (val: number | string) => `${val}%`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
        {
          orient: 'right',
          seriesIndex: [1],
          label: { style: { fill: textColor, fontSize: 10 } },
        },
      ],
      legends: { visible: true, position: 'top' },
      tooltip: {
        mark: {
          content: [
            {
              key: t('Success Rate'),
              value: (d: { success_rate: number | null }) =>
                d.success_rate == null ? '-' : formatUptimePct(d.success_rate),
            },
            {
              key: t('Errors'),
              value: (d: { failures: number }) => `${d.failures}`,
            },
          ],
        },
      },
    }
  }, [props.series, props.granularity, resolvedTheme, t])

  return (
    <PanelWrapper
      title={t('Availability Trend')}
      loading={false}
      empty={props.series.length === 0}
      emptyMessage={t('No data available')}
      height='h-56 sm:h-64'
    >
      <ChartFrame
        resolvedTheme={resolvedTheme}
        themeReady={themeReady}
        spec={spec}
        name='availability-trend'
      />
    </PanelWrapper>
  )
}

function RequestsChart(props: {
  series: PerformanceSeriesPoint[]
  granularity: PerformanceGranularity
}) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const spec = useMemo(() => {
    if (props.series.length === 0) return null
    const { textColor, gridColor } = getChartThemeTokens(resolvedTheme)
    const data = props.series.map((point) => ({
      time: formatBucketLabel(point.ts, props.granularity),
      requests: point.requests,
      total_tokens: point.total_tokens,
    }))
    return {
      type: 'common' as const,
      data: [{ id: 'requests-trend', values: data }],
      series: [
        {
          type: 'bar' as const,
          xField: 'time',
          yField: 'requests',
          name: t('Requests'),
          bar: { style: { fill: '#6366f1', opacity: 0.8 } },
        },
        {
          type: 'line' as const,
          xField: 'time',
          yField: 'total_tokens',
          name: t('Total Tokens'),
          smooth: true,
          point: { visible: false },
          line: { style: { lineWidth: 2, stroke: '#f59e0b' } },
        },
      ],
      axes: [
        {
          orient: 'bottom',
          label: { style: { fill: textColor, fontSize: 10 } },
          tick: { visible: false },
        },
        {
          orient: 'left',
          seriesIndex: [0],
          label: {
            formatMethod: (val: number | string) =>
              `${formatCompactNumber(Number(val))}`,
            style: { fill: textColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: gridColor },
          },
        },
        {
          orient: 'right',
          seriesIndex: [1],
          label: {
            formatMethod: (val: number | string) =>
              `${formatCompactNumber(Number(val))}`,
            style: { fill: textColor, fontSize: 10 },
          },
        },
      ],
      legends: { visible: true, position: 'top' },
      tooltip: {
        mark: {
          content: [
            {
              key: t('Requests'),
              value: (d: { requests: number }) => `${d.requests}`,
            },
            {
              key: t('Total Tokens'),
              value: (d: { total_tokens: number }) =>
                formatCompactNumber(d.total_tokens),
            },
          ],
        },
      },
    }
  }, [props.series, props.granularity, resolvedTheme, t])

  return (
    <PanelWrapper
      title={t('Requests & Tokens Trend')}
      loading={false}
      empty={props.series.length === 0}
      emptyMessage={t('No data available')}
      height='h-56 sm:h-64'
    >
      <ChartFrame
        resolvedTheme={resolvedTheme}
        themeReady={themeReady}
        spec={spec}
        name='requests-trend'
      />
    </PanelWrapper>
  )
}

export function PerformanceTrendCharts(props: {
  series: PerformanceSeriesPoint[]
  granularity: PerformanceGranularity
}) {
  return (
    <div className='grid grid-cols-1 gap-3 lg:grid-cols-2'>
      <TpsChart series={props.series} granularity={props.granularity} />
      <AvailabilityChart
        series={props.series}
        granularity={props.granularity}
      />
      <LatencyChart series={props.series} granularity={props.granularity} />
      <RequestsChart series={props.series} granularity={props.granularity} />
    </div>
  )
}
