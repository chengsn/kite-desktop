import type { ComponentType, ReactNode } from 'react'
import { AlertTriangle, Inbox, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'

interface EmptyStateProps {
  title: ReactNode
  description?: ReactNode
  icon?: ComponentType<{ className?: string }>
  action?: ReactNode
  className?: string
}

export function EmptyState({
  title,
  description,
  icon: Icon = Inbox,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-2 px-6 py-12 text-center',
        className
      )}
    >
      <div className="mb-1 flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Icon className="size-6" />
      </div>
      <h2 className="text-base font-medium">{title}</h2>
      {description ? (
        <p className="max-w-md text-sm text-muted-foreground">{description}</p>
      ) : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  )
}

interface ErrorStateProps {
  title: ReactNode
  description?: ReactNode
  onRetry?: () => void
  retryLabel?: ReactNode
  icon?: ComponentType<{ className?: string }>
  className?: string
}

export function ErrorState({
  title,
  description,
  onRetry,
  retryLabel,
  icon: Icon = AlertTriangle,
  className,
}: ErrorStateProps) {
  const { t } = useTranslation()

  return (
    <div
      role="alert"
      className={cn(
        'flex flex-col items-center justify-center gap-2 px-6 py-12 text-center',
        className
      )}
    >
      <Icon className="mb-1 size-10 text-destructive" />
      <h2 className="text-base font-medium text-destructive">{title}</h2>
      {description ? (
        <p className="max-w-md text-sm text-muted-foreground">{description}</p>
      ) : null}
      {onRetry ? (
        <Button variant="outline" className="mt-2" onClick={onRetry}>
          <RotateCcw className="mr-2 size-4" />
          {retryLabel ?? t('resourceTable.tryAgain')}
        </Button>
      ) : null}
    </div>
  )
}

interface TableSkeletonProps {
  rows?: number
  columns?: number
  className?: string
}

export function TableSkeleton({
  rows = 6,
  columns = 4,
  className,
}: TableSkeletonProps) {
  const { t } = useTranslation()

  return (
    <div
      className={cn('space-y-3 p-4', className)}
      aria-busy="true"
      aria-live="polite"
    >
      <span className="sr-only">{t('common.loading')}</span>
      {Array.from({ length: rows }).map((_, rowIndex) => (
        <div key={rowIndex} className="flex items-center gap-4">
          {Array.from({ length: columns }).map((_, columnIndex) => (
            <Skeleton
              key={columnIndex}
              className={cn('h-4', columnIndex === 0 ? 'w-1/4' : 'flex-1')}
            />
          ))}
        </div>
      ))}
    </div>
  )
}

interface DetailSkeletonProps {
  className?: string
}

/**
 * Shared loading shell for resource detail pages: title, metadata line and a
 * couple of content cards.
 */
export function DetailSkeleton({ className }: DetailSkeletonProps) {
  const { t } = useTranslation()

  return (
    <div className={cn('space-y-4', className)} role="status" aria-busy="true">
      <span className="sr-only">{t('common.loading')}</span>
      <div className="space-y-2">
        <Skeleton className="h-8 w-1/3" />
        <Skeleton className="h-4 w-1/4" />
      </div>
      <Skeleton className="h-10 w-full max-w-xl" />
      <div className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-48 w-full" />
        <Skeleton className="h-48 w-full" />
      </div>
    </div>
  )
}
