import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { statusMeta } from '@/lib/status'
import type { RunStatus } from '@/types'

export function StatusBadge({ status, className }: { status: RunStatus; className?: string }) {
  const meta = statusMeta(status)
  const Icon = meta.icon
  return (
    <Badge variant={meta.variant} className={className}>
      <Icon className={cn('size-3.5', meta.live && 'animate-pulse')} aria-hidden />
      {meta.label}
    </Badge>
  )
}
