import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

interface PageHeaderProps {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  /**
   * Action content when using composition. `actions` takes precedence.
   */
  children?: ReactNode
  className?: string
  titleClassName?: string
}

/**
 * Shared page header. Keeps the title, supporting text and actions on one
 * consistent hierarchy across list, detail and settings pages.
 */
export function PageHeader({
  title,
  description,
  actions,
  children,
  className,
  titleClassName,
}: PageHeaderProps) {
  const actionContent = actions ?? children

  return (
    <header
      className={cn(
        'flex flex-col gap-3 md:flex-row md:items-start md:justify-between',
        className
      )}
    >
      <div className="min-w-0 flex-1 space-y-1">
        <h1
          className={cn(
            'truncate text-2xl font-semibold tracking-tight',
            titleClassName
          )}
        >
          {title}
        </h1>
        {description ? (
          <div className="text-sm text-muted-foreground">{description}</div>
        ) : null}
      </div>
      {actionContent ? (
        <div className="flex w-full flex-wrap items-center gap-2 md:w-auto md:shrink-0 md:justify-end">
          {actionContent}
        </div>
      ) : null}
    </header>
  )
}
