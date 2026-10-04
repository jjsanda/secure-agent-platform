import { ScrollText } from 'lucide-react'
import { Link } from 'react-router-dom'

import { useAudit } from '@/api/hooks'
import { PageHeader } from '@/components/PageHeader'
import { EmptyState, ErrorState } from '@/components/States'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime, formatRelative, prettyJson } from '@/lib/format'
import { auditActionMeta } from '@/lib/status'
import { cn } from '@/lib/utils'
import type { AuditDTO } from '@/types'

function DetailCell({ detail }: { detail: Record<string, unknown> }) {
  const hasDetail = detail && Object.keys(detail).length > 0
  if (!hasDetail) return <span className="text-muted-foreground">—</span>
  return (
    <details className="group max-w-md">
      <summary className="cursor-pointer list-none text-xs text-primary hover:underline">
        <span className="group-open:hidden">view</span>
        <span className="hidden group-open:inline">hide</span>
      </summary>
      <pre className="scrollbar-thin mt-1.5 max-h-48 overflow-auto rounded-md bg-muted/60 p-2.5 font-mono text-xs leading-relaxed text-muted-foreground">
        {prettyJson(detail)}
      </pre>
    </details>
  )
}

function AuditRow({ entry }: { entry: AuditDTO }) {
  const meta = auditActionMeta(entry.action)
  const Icon = meta.icon
  return (
    <tr className="border-t align-top transition-colors hover:bg-accent/40">
      <td className="py-3 pl-5 pr-3">
        <span className={cn('inline-block size-2 rounded-full', meta.dot)} aria-hidden />
      </td>
      <td className="whitespace-nowrap py-3 pr-4 text-sm" title={formatDateTime(entry.at)}>
        {formatRelative(entry.at)}
      </td>
      <td className="py-3 pr-4">
        <span className="font-mono text-xs">{entry.actor}</span>
      </td>
      <td className="py-3 pr-4">
        <Badge variant={meta.variant}>
          <Icon className="size-3.5" aria-hidden />
          {entry.action}
        </Badge>
      </td>
      <td className="py-3 pr-4">
        {entry.toolName ? (
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{entry.toolName}</code>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="py-3 pr-4">
        {entry.runId ? (
          <Link
            to={`/runs/${entry.runId}`}
            className="font-mono text-xs text-primary hover:underline"
            title={entry.runId}
          >
            {entry.runId.slice(0, 8)}…
          </Link>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </td>
      <td className="py-3 pr-5">
        <DetailCell detail={entry.detail} />
      </td>
    </tr>
  )
}

export function Audit() {
  const audit = useAudit()

  let body: React.ReactNode
  if (audit.isLoading) {
    body = (
      <Card className="p-5">
        <div className="space-y-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-6 w-full" />
          ))}
        </div>
      </Card>
    )
  } else if (audit.isError) {
    body = <ErrorState error={audit.error} onRetry={() => void audit.refetch()} />
  } else if ((audit.data ?? []).length === 0) {
    body = (
      <EmptyState
        icon={ScrollText}
        title="No audit entries yet"
        description="Signing in and creating runs will record entries here — logins, credential mints, and every tool decision."
      />
    )
  } else {
    body = (
      <Card className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-left">
            <thead>
              <tr className="text-xs uppercase tracking-wide text-muted-foreground">
                <th className="py-2.5 pl-5 pr-3 font-medium" aria-label="severity" />
                <th className="py-2.5 pr-4 font-medium">Time</th>
                <th className="py-2.5 pr-4 font-medium">Actor</th>
                <th className="py-2.5 pr-4 font-medium">Action</th>
                <th className="py-2.5 pr-4 font-medium">Tool</th>
                <th className="py-2.5 pr-4 font-medium">Run</th>
                <th className="py-2.5 pr-5 font-medium">Detail</th>
              </tr>
            </thead>
            <tbody>
              {(audit.data ?? []).map((entry) => (
                <AuditRow key={entry.id} entry={entry} />
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Audit log"
        description="An append-only record of security-relevant events: logins, credential mints, and every allowed or denied tool call."
      />
      {body}
    </div>
  )
}
