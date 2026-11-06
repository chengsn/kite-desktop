import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { usePageTitle } from '@/hooks/use-page-title'
import { ResponsiveTabs } from '@/components/ui/responsive-tabs'
import { PageHeader } from '@/components/page-header'
import { AboutManagement } from '@/components/settings/about-management'
import { ClusterManagement } from '@/components/settings/cluster-management'
import { DesktopManagement } from '@/components/settings/desktop-management'
import { GeneralManagement } from '@/components/settings/general-management'
import { TemplateManagement } from '@/components/settings/template-management'

export function SettingsPage() {
  const { t } = useTranslation()

  usePageTitle(t('settings.title', 'Settings'))

  const tabs: Array<{
    value: string
    label: string
    content: ReactNode
  }> = [
    {
      value: 'general',
      label: t('settings.tabs.general', 'General'),
      content: <GeneralManagement />,
    },
    {
      value: 'clusters',
      label: t('settings.tabs.clusters', 'Cluster'),
      content: <ClusterManagement />,
    },
    {
      value: 'templates',
      label: t('settings.tabs.templates', 'Templates'),
      content: <TemplateManagement />,
    },
    {
      value: 'desktop',
      label: t('settings.tabs.desktop', 'Desktop'),
      content: <DesktopManagement />,
    },
    {
      value: 'about',
      label: t('settings.tabs.about', 'About'),
      content: <AboutManagement />,
    },
  ]

  return (
    <div className="space-y-2">
      <PageHeader
        className="mb-4"
        title={t('settings.title', 'Settings')}
        description={t(
          'settings.descriptionLocal',
          'Manage local preferences, clusters and templates'
        )}
      />

      <ResponsiveTabs tabs={tabs} />
    </div>
  )
}
