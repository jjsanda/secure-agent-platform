import { AlertCircle, Inbox, RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'

import { ApiError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

/** Empty state with an icon, title, and optional action. */
export function EmptyState({
  icon: Icon = Inbox,
  title,
  description,
  action,
  className,
}: {
  icon?: typeof Inbox
  title: string
  description?: string
  action?: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center rounded-xl border border-dashed p-12 text-center',
        className,
      )}
    >
      <div className="mb-4 flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Icon className="size-6" aria-hidden />
      </div>
      <h3 className="text-sm font-semibold">{title}</h3>
      {description && <p className="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 401) return 'Your session may have expired. Try signing in again.'
    if (error.status === 403) return 'You do not have access to this resource.'
    return error.message
  }
  if (error instanceof Error) return error.message
  return 'An unexpected error occurred.'
}

/** Error state with the API detail and an optional retry. */
export function ErrorState({
  error,
  onRetry,
  className,
}: {
  error: unknown
  onRetry?: () => void
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center rounded-xl border border-destructive/40 bg-destructive/5 p-12 text-center',
        className,
      )}
    >
      <div className="mb-4 flex size-12 items-center justify-center rounded-full bg-destructive/15 text-destructive">
        <AlertCircle className="size-6" aria-hidden />
      </div>
      <h3 className="text-sm font-semibold">Something went wrong</h3>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">{describeError(error)}</p>
      {onRetry && (
        <Button variant="outline" size="sm" className="mt-4" onClick={onRetry}>
          <RefreshCw className="size-4" />
          Retry
        </Button>
      )}
    </div>
  )
}
