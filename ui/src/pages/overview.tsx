import { useTranslation } from 'react-i18next'

import { useOverview, useResourceUsageHistory } from '@/lib/api'
import { useCluster } from '@/hooks/use-cluster'
import NetworkUsageChart from '@/components/chart/network-usage-chart'
import ResourceUtilizationChart from '@/components/chart/resource-utilization'
import { ClusterStatsCards } from '@/components/cluster-stats-cards'
import { NoClusterState } from '@/components/no-cluster-state'
import { PageHeader } from '@/components/page-header'
import { RecentEvents } from '@/components/recent-events'
import { ErrorState } from '@/components/resource-state'
import { ResourceCharts } from '@/components/resources-charts'

export function Overview() {
  const { t } = useTranslation()
  const timeRange = '30m'
  const { currentCluster } = useCluster()
  const {
    data: overview,
    isLoading,
    error,
    isError,
    refetch: refetchOverview,
  } = useOverview()

  const {
    data: resourceUsage,
    isLoading: isLoadingResourceUsage,
    error: errorResourceUsage,
  } = useResourceUsageHistory(timeRange, {
    enabled: overview?.prometheusEnabled ?? false,
  })

  if (!currentCluster) {
    return <NoClusterState />
  }

  if (error) {
    return (
      <ErrorState
        className="h-64"
        title={t('overview.failedToLoad')}
        description={
          error instanceof Error ? error.message : t('overview.unknownError')
        }
        onRetry={() => void refetchOverview()}
      />
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('overview.title')} />

      <ClusterStatsCards stats={overview} isLoading={isLoading} />

      <div className="grid grid-cols-1 gap-4 @5xl/main:grid-cols-2">
        <ResourceCharts
          data={overview?.resource}
          isLoading={isLoading}
          error={error}
          isError={isError}
        />
        <RecentEvents />
      </div>

      {overview?.prometheusEnabled && (
        <div className="grid grid-cols-1 gap-4 @5xl/main:grid-cols-2">
          <ResourceUtilizationChart
            cpu={resourceUsage?.cpu || []}
            memory={resourceUsage?.memory || []}
            isLoading={isLoadingResourceUsage}
            error={errorResourceUsage}
          />

          <NetworkUsageChart
            networkIn={resourceUsage?.networkIn || []}
            networkOut={resourceUsage?.networkOut || []}
            isLoading={isLoadingResourceUsage}
            error={errorResourceUsage}
          />
        </div>
      )}
    </div>
  )
}
