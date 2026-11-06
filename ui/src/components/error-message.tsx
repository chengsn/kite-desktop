import { ShieldX, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { isRBACError, translateError } from '@/lib/utils'

import { ErrorState } from './resource-state'

interface ErrorMessageProps {
  resourceName: string
  error: Error | unknown
  fallbackKey?: string
  className?: string
  refetch: () => void
}

export function ErrorMessage({
  resourceName,
  refetch,
  error,
  fallbackKey = 'common.error',
  className,
}: ErrorMessageProps) {
  const { t } = useTranslation()

  if (!error) {
    return null
  }

  const isRBAC = error instanceof Error && isRBACError(error.message)
  const message =
    error instanceof Error ? translateError(error, t) : t(fallbackKey)

  return (
    <ErrorState
      className={className}
      icon={isRBAC ? ShieldX : XCircle}
      title={t('resourceTable.errorLoading', {
        resourceName: resourceName.toLowerCase(),
      })}
      description={message}
      onRetry={() => refetch()}
      retryLabel={t('resourceTable.tryAgain')}
    />
  )
}
