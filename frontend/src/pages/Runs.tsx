import { ChevronRight, Sparkles, Wrench } from 'lucide-react'
import { Link } from 'react-router-dom'

import { useRuns } from '@/api/hooks'
import { NewRunForm } from '@/components/NewRunForm'
import { PageHeader } from '@/components/PageHeader'
import { StatusBadge } from '@/components/StatusBadge'
import { EmptyState, ErrorState } from '@/components/States'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime, formatRelative } from '@/lib/format'
import type { RunDTO } from '@/types'

function RunRow({ run }: { run: RunDTO }) {
  return (
    <Link
      to={`/runs/${run.id}`}
      className="flex items-center gap-4 px-5 py-4 transition-colors hover:bg-accent/50"
    >
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium">{run.objective}</p>
        <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span className="font-mono">{run.variant}</span>
          <span aria-hidden>·</span>
          <span className="inline-flex items-center gap-1">
            <Wrench className="size-3" aria-hidden />
            {run.allowedTools.length} {run.allowedTools.length === 1 ? 'tool' : 'tools'}
          </span>
          <span aria-hidden>·</span>
          <span title={formatDateTime(run.createdAt)}>{formatRelative(run.createdAt)}</span>
        </div>
      </div>
      <StatusBadge status={run.status} />
      <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />
    </Link>
  )
}

function RunsList() {
  const runs = useRuns()

  if (runs.isLoading) {
    return (
      <Card>
        <div className="divide-y">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="flex items-center gap-4 px-5 py-4">
              <div className="flex-1 space-y-2">
                <Skeleton className="h-4 w-2/3" />
                <Skeleton className="h-3 w-1/3" />
              </div>
              <Skeleton className="h-6 w-24 rounded-full" />
            </div>
          ))}
        </div>
      </Card>
    )
  }
  if (runs.isError) return <ErrorState error={runs.error} onRetry={() => void runs.refetch()} />

  const rows = runs.data ?? []
  if (rows.length === 0) {
    return (
      <EmptyState
        icon={Sparkles}
        title="No runs yet"
        description="Create your first run with the form to watch the agent plan, call tools, and stream events live."
      />
    )
  }

  return (
    <Card className="overflow-hidden">
      <div className="divide-y">
        {rows.map((run) => (
          <RunRow key={run.id} run={run} />
        ))}
      </div>
    </Card>
  )
}

export function Runs() {
  return (
    <div className="space-y-6">
      <PageHeader
        title="Runs"
        description="Launch agent runs and follow them from planning through to a final answer."
      />
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-3 lg:col-span-2">
          <RunsList />
        </div>
        <Card className="h-fit lg:sticky lg:top-24">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <Sparkles className="size-4 text-primary" aria-hidden />
              New run
            </CardTitle>
          </CardHeader>
          <CardContent>
            <NewRunForm />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
