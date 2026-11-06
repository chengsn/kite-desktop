'use client'

import React from 'react'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from 'recharts'

import { MetricUnit, UsageDataPoint } from '@/types/api'
import { formatChartXTicks, formatDate } from '@/lib/utils'

import { Alert, AlertDescription } from '../ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card'
import {
  ChartConfig,
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '../ui/chart'
import { Skeleton } from '../ui/skeleton'

interface CpuUsageChartProps {
  data: UsageDataPoint[]
  isLoading?: boolean
  error?: Error | null
  syncId?: string
  /** How to read the series: a 0-100 utilization, or absolute cores. */
  unit?: MetricUnit
}

const CPUUsageChart = React.memo((prop: CpuUsageChartProps) => {
  const { t } = useTranslation()
  const { data, isLoading, error, syncId, unit } = prop
  // The backend reports utilization when the pod declares a limit or request,
  // and absolute cores otherwise. Until the unit arrives, keep the neutral
  // title instead of guessing "absolute".
  const isPercent = unit === 'percent'
  const title =
    isPercent || unit === undefined
      ? t('monitoring.cpuUsage')
      : t('monitoring.cpuUsageAbsolute')

  const cpuChartConfig = React.useMemo(
    () =>
      ({
        cpu: {
          label: isPercent ? 'CPU (%)' : 'CPU (cores)',
          theme: {
            light: 'hsl(220, 70%, 50%)',
            dark: 'hsl(210, 80%, 60%)',
          },
        },
      }) satisfies ChartConfig,
    [isPercent]
  )

  const cpuChartData = React.useMemo(() => {
    if (!data) return []

    return data
      .map((point) => ({
        timestamp: point.timestamp,
        time: new Date(point.timestamp).getTime(),
        cpu: Math.max(0, point.value),
      }))
      .sort((a, b) => a.time - b.time)
  }, [data])

  // Utilization is normally 0-100, but a container can burst past its request,
  // so widen the axis rather than clipping the series.
  const yDomain = React.useMemo<[number, number] | undefined>(() => {
    if (!isPercent) return undefined
    const max = Math.max(0, ...cpuChartData.map((point) => point.cpu))
    return [0, max > 100 ? Math.ceil(max / 10) * 10 : 100]
  }, [cpuChartData, isPercent])

  const isSameDay = React.useMemo(() => {
    if (cpuChartData.length < 2) return true
    const first = new Date(cpuChartData[0].timestamp)
    const last = new Date(cpuChartData[cpuChartData.length - 1].timestamp)
    return first.toDateString() === last.toDateString()
  }, [cpuChartData])

  // Show loading skeleton
  if (isLoading) {
    return (
      <Card className="@container/card">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            {title}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <Skeleton className="h-[250px] w-full" />
          </div>
        </CardContent>
      </Card>
    )
  }

  // Show error state
  if (error) {
    return (
      <Card className="@container/card">
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <Alert variant="destructive">
            <AlertTriangle className="h-4 w-4" />
            <AlertDescription>{error.message}</AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    )
  }

  // Show empty state
  if (!data || data.length === 0) {
    return (
      <Card className="@container/card">
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex h-[250px] w-full items-center justify-center text-muted-foreground">
            <p>{t('charts.noCpuUsageData')}</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card className="@container/card">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer config={cpuChartConfig} className="h-[250px] w-full">
          <AreaChart data={cpuChartData} syncId={syncId}>
            <CartesianGrid vertical={false} />
            <XAxis
              dataKey="timestamp"
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              minTickGap={30}
              allowDataOverflow={true}
              tickFormatter={(value) => formatChartXTicks(value, isSameDay)}
            />
            <YAxis
              tickLine={false}
              axisLine={false}
              tickMargin={8}
              domain={yDomain}
              tickFormatter={(value) =>
                isPercent ? `${Math.round(value)}%` : value.toFixed(3)
              }
            />
            <ChartTooltip
              content={
                <ChartTooltipContent
                  labelFormatter={(value) => formatDate(value)}
                />
              }
            />
            <Area
              isAnimationActive={false}
              dataKey="cpu"
              type="monotone"
              fill="var(--color-cpu)"
              stroke="var(--color-cpu)"
            />
          </AreaChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
})

export default CPUUsageChart
