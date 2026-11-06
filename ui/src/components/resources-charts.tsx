import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { OverviewData } from '@/types/api'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

function toPercentage(value: number, total: number) {
  if (!Number.isFinite(value) || !Number.isFinite(total) || total <= 0) {
    return 0
  }

  return (value / total) * 100
}

export interface ResourceChartsProps {
  data?: OverviewData['resource']
  isLoading: boolean
  error: Error | null
  isError: boolean
}

export function ResourceCharts(props: ResourceChartsProps) {
  const { t } = useTranslation()
  const { isLoading, error, isError } = props
  const chartData = useMemo(() => {
    const { cpu, memory } = props.data || {
      cpu: { requested: 0, allocatable: 0, limited: 0 },
      memory: { requested: 0, allocatable: 0, limited: 0 },
    }
    return [
      {
        name: t('monitoring.cpuUsage'),
        request: cpu.requested / 1000,
        limit: cpu.limited / 1000,
        total: cpu.allocatable / 1000,
        requestPercentage: toPercentage(cpu.requested, cpu.allocatable),
        limitPercentage: toPercentage(cpu.limited, cpu.allocatable),
        unit: 'cores',
      },
      {
        name: t('monitoring.memoryUsage'),
        request: memory.requested / 1024 / 1024 / 1024 / 1024,
        limit: memory.limited / 1024 / 1024 / 1024 / 1024,
        total: memory.allocatable / 1024 / 1024 / 1024 / 1024,
        requestPercentage: toPercentage(memory.requested, memory.allocatable),
        limitPercentage: toPercentage(memory.limited, memory.allocatable),
        unit: 'GiB',
      },
    ]
  }, [props, t])

  if (isLoading) {
    return (
      <div className="grid grid-cols-1 gap-4">
        {Array.from({ length: 2 }).map((_, i) => (
          <Card key={i} className="animate-pulse">
            <CardHeader>
              <div className="h-4  bg-muted rounded w-1/3 mb-2"></div>
              <div className="h-6  bg-muted rounded w-1/2"></div>
            </CardHeader>
            <CardContent>
              <div className="h-32  bg-muted rounded"></div>
            </CardContent>
          </Card>
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span>{t('charts.resourcesRequest')}</span>
          </CardTitle>
        </CardHeader>
        <div className="flex flex-col items-center justify-center h-64 gap-2">
          <p className="text-sm text-muted-foreground">
            {error instanceof Error
              ? error.message
              : t('overview.unknownError')}
          </p>
        </div>
      </Card>
    )
  }

  return (
    <div className="grid grid-cols-1 gap-4 ">
      {chartData.map((resource) => {
        const requestIsHigh = resource.requestPercentage > 90
        const requestIsMedium = resource.requestPercentage > 60
        const limitIsHigh = resource.limitPercentage > 90

        return (
          <Card key={resource.name}>
            <CardHeader>
              <CardTitle className="flex items-center justify-between">
                <span>{resource.name}</span>
              </CardTitle>
              <CardDescription>
                {t('monitoring.requests')}: {resource.request.toFixed(1)} /{' '}
                {t('monitoring.limits')}: {resource.limit.toFixed(1)} /{' '}
                {t('monitoring.total')}: {resource.total.toFixed(2)}{' '}
                {resource.unit}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
                  <div>
                    <div className="flex justify-between text-xs text-muted-foreground mb-1">
                      <span className="font-medium text-info">
                        {t('monitoring.requests')}
                      </span>
                      <span>
                        {resource.request.toFixed(1)} {resource.unit}
                      </span>
                    </div>
                    <div className="w-full bg-muted rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all duration-300 ${
                          requestIsHigh
                            ? 'bg-destructive'
                            : requestIsMedium
                              ? 'bg-warning'
                              : 'bg-info'
                        }`}
                        style={{
                          width: `${Math.min(resource.requestPercentage, 100)}%`,
                        }}
                      />
                    </div>
                    <div className="text-xs text-muted-foreground mt-1">
                      {t('charts.ofCapacity', {
                        value: resource.requestPercentage.toFixed(1),
                      })}
                    </div>
                  </div>

                  <div>
                    <div className="flex justify-between text-xs text-muted-foreground mb-1">
                      <span className="font-medium text-warning">
                        {t('monitoring.limits')}
                      </span>
                      <span>
                        {resource.limit.toFixed(1)} {resource.unit}
                      </span>
                    </div>
                    <div className="w-full bg-muted rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all duration-300 ${
                          limitIsHigh ? 'bg-destructive' : 'bg-warning'
                        }`}
                        style={{
                          width: `${Math.min(resource.limitPercentage, 100)}%`,
                        }}
                      />
                    </div>
                    <div className="text-xs text-muted-foreground mt-1">
                      {t('charts.ofCapacity', {
                        value: resource.limitPercentage.toFixed(1),
                      })}
                    </div>
                  </div>
                </div>

                <div className="text-xs text-muted-foreground ">
                  {t('charts.available', {
                    value: (resource.total - resource.request).toFixed(1),
                    unit: resource.unit,
                  })}
                </div>
              </div>
            </CardContent>
          </Card>
        )
      })}
    </div>
  )
}
