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

interface MemoryUsageChartProps {
  data: UsageDataPoint[]
  isLoading?: boolean
  error?: Error | null
  syncId?: string
  /** How to read the series: a 0-100 utilization, or absolute megabytes. */
  unit?: MetricUnit
}

const MemoryUsageChart = React.memo((prop: MemoryUsageChartProps) => {
  const { t } = useTranslation()
  const { data, isLoading, error, syncId, unit } = prop
  // The backend reports utilization when the pod declares a limit or request,
  // and absolute megabytes otherwise. Until the unit arrives, keep the neutral
  // title instead of guessing "absolute".
  const isPercent = unit === 'percent'
  const title =
    isPercent || unit === undefined
      ? t('monitoring.memoryUsage')
      : t('monitoring.memoryUsageAbsolute')

  const memoryChartData = React.useMemo(() => {
    if (!data) return []

    return data
      .map((point) => ({
        timestamp: point.timestamp,
        time: new Date(point.timestamp).getTime(),
        memory: Math.max(0, point.value), // Absolute values are already in MB
      }))
      .sort((a, b) => a.time - b.time)
  }, [data])

  const isSameDay = React.useMemo(() => {
    if (memoryChartData.length < 2) return true
    const first = new Date(memoryChartData[0].timestamp)
    const last = new Date(memoryChartData[memoryChartData.length - 1].timestamp)
    return first.toDateString() === last.toDateString()
  }, [memoryChartData])

  // Determine if we should use GB instead of MB. Percentages never rescale.
  const useGB = React.useMemo(() => {
    if (isPercent || !memoryChartData.length) return false
    const maxMemory = Math.max(...memoryChartData.map((point) => point.memory))
    return maxMemory > 900
  }, [memoryChartData, isPercent])

  // Utilization is normally 0-100, but a container can burst past its request,
  // so widen the axis rather than clipping the series. Absolute values keep the
  // existing headroom above the tallest point.
  const yDomain = React.useMemo<
    [number, number] | [number, (dataMax: number) => number]
  >(() => {
    if (!isPercent) return [0, (dataMax: number) => dataMax * 1.1]
    const max = Math.max(0, ...memoryChartData.map((point) => point.memory))
    return [0, max > 100 ? Math.ceil(max / 10) * 10 : 100]
  }, [memoryChartData, isPercent])

  // Convert memory data to GB if needed
  const processedMemoryChartData = React.useMemo(() => {
    if (!useGB) return memoryChartData
    return memoryChartData.map((point) => ({
      ...point,
      memory: point.memory / 1024, // Convert MB to GB
    }))
  }, [memoryChartData, useGB])

  const dynamicMemoryChartConfig = React.useMemo(
    () => ({
      memory: {
        label: isPercent ? 'Memory (%)' : `Memory (${useGB ? 'GB' : 'MB'})`,
        theme: {
          light: 'hsl(142, 70%, 50%)',
          dark: 'hsl(150, 80%, 60%)',
        },
      },
    }),
    [useGB, isPercent]
  ) satisfies ChartConfig

  // Show loading skeleton
  if (isLoading) {
    return (
      <Card>
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
      <Card>
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
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex h-[250px] w-full items-center justify-center text-muted-foreground">
            <p>{t('charts.noMemoryUsageData')}</p>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <ChartContainer
          config={dynamicMemoryChartConfig}
          className="h-[250px] w-full"
        >
          <AreaChart data={processedMemoryChartData} syncId={syncId}>
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
              tickFormatter={(value) => {
                if (isPercent) return `${Math.round(value)}%`
                return `${value.toFixed(useGB ? 2 : 1)}${useGB ? 'GB' : 'MB'}`
              }}
              domain={yDomain}
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
              dataKey="memory"
              type="monotone"
              fill="var(--color-memory)"
              stroke="var(--color-memory)"
            />
          </AreaChart>
        </ChartContainer>
      </CardContent>
    </Card>
  )
})

export default MemoryUsageChart
