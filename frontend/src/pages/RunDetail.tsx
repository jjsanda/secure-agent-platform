import { AlertTriangle, ArrowLeft, Radio, Sparkles } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { Link, useParams } from 'react-router-dom'

import { ApiError } from '@/api/client'
import { useRun } from '@/api/hooks'
import { useRunEventStream } from '@/api/useRunEventStream'
import { PageHeader } from '@/components/PageHeader'
import { RunEventTimeline } from '@/components/RunEventTimeline'
import { StatusBadge } from '@/components/StatusBadge'
import { ErrorState } from '@/components/States'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime, formatRelative } from '@/lib/format'
import {
  TERMINAL_STATUSES,
  type ErrorPayload,
  type FinalPayload,
  type RunDTO,
  type RunEventDTO,
  type RunStatus,
  type StatusPayload,
} from '@/types'

/** The payload of the last event of a given kind, scanning from the end. */
function lastPayload<T>(events: RunEventDTO[], kind: string): T | undefined {
  for (let i = events.length - 1; i >= 0; i--) {
    if (events[i].kind === kind) return events[i].payload as T
  }
  return undefined
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 py-2.5">
      <span className="text-sm text-muted-foreground">{label}</span>
      <div className="text-right text-sm">{children}</div>
    </div>
  )
}

function RunFacts({ run, status }: { run: RunDTO; status: RunStatus }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Run details</CardTitle>
      </CardHeader>
      <CardContent className="divide-y pt-0">
        <DetailRow label="Status">
          <StatusBadge status={status} />
        </DetailRow>
        <DetailRow label="Variant">
          <span className="font-mono text-xs">{run.variant}</span>
        </DetailRow>
        <DetailRow label="Allowed tools">
          <div className="flex flex-wrap justify-end gap-1.5">
            {run.allowedTools.map((tool) => (
              <code
                key={tool}
                className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground"
              >
                {tool}
              </code>
            ))}
          </div>
        </DetailRow>
        <DetailRow label="Created">
          <span title={formatDateTime(run.createdAt)}>{formatRelative(run.createdAt)}</span>
        </DetailRow>
        <DetailRow label="Updated">
          <span title={formatDateTime(run.updatedAt)}>{formatRelative(run.updatedAt)}</span>
        </DetailRow>
        <DetailRow label="Run ID">
          <span className="font-mono text-xs text-muted-foreground">{run.id}</span>
        </DetailRow>
      </CardContent>
    </Card>
  )
}

export function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const run = useRun(id)
  const stream = useRunEventStream(id)

  // Auto-scroll the timeline to the newest event as frames arrive.
  const listRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const el = listRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [stream.events.length])

  if (run.isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <div className="grid gap-6 lg:grid-cols-3">
          <Skeleton className="h-64 lg:col-span-2" />
          <Skeleton className="h-64" />
        </div>
      </div>
    )
  }

  if (run.isError || !run.data) {
    const notFound = run.error instanceof ApiError && run.error.status === 404
    return (
      <div className="space-y-6">
        <Button asChild variant="ghost" size="sm" className="-ml-2">
          <Link to="/runs">
            <ArrowLeft className="size-4" />
            Back to runs
          </Link>
        </Button>
        <ErrorState
          error={notFound ? new Error('This run does not exist in your tenant.') : run.error}
          onRetry={notFound ? undefined : () => void run.refetch()}
        />
      </div>
    )
  }

  const data = run.data
  const liveStatus = lastPayload<StatusPayload>(stream.events, 'status')?.status ?? data.status
  const runActive = !TERMINAL_STATUSES.includes(liveStatus) && stream.status !== 'done'

  const finalAnswer =
    data.finalAnswer ?? lastPayload<FinalPayload>(stream.events, 'final')?.answer ?? null
  const errorMessage =
    data.error ?? lastPayload<ErrorPayload>(stream.events, 'error')?.message ?? null

  return (
    <div className="space-y-6">
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-2 mb-3">
          <Link to="/runs">
            <ArrowLeft className="size-4" />
            Back to runs
          </Link>
        </Button>
        <PageHeader
          title={data.objective}
          actions={<StatusBadge status={liveStatus} className="text-sm" />}
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-6 lg:col-span-2">
          {finalAnswer && (
            <Card className="border-success/30 bg-success/5">
              <CardHeader className="flex-row items-center gap-2 space-y-0 pb-3">
                <Sparkles className="size-5 text-success" aria-hidden />
                <CardTitle className="text-base">Final answer</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="whitespace-pre-wrap text-sm leading-relaxed">{finalAnswer}</p>
              </CardContent>
            </Card>
          )}

          {errorMessage && (
            <Card className="border-destructive/40 bg-destructive/5">
              <CardHeader className="flex-row items-center gap-2 space-y-0 pb-3">
                <AlertTriangle className="size-5 text-destructive" aria-hidden />
                <CardTitle className="text-base text-destructive">Run failed</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="whitespace-pre-wrap text-sm leading-relaxed text-destructive">
                  {errorMessage}
                </p>
              </CardContent>
            </Card>
          )}

          <Card>
            <CardHeader className="flex-row items-center justify-between space-y-0">
              <CardTitle className="text-base">Event timeline</CardTitle>
              {runActive ? (
                <Badge variant="success" className="animate-pulse-ring">
                  <Radio className="size-3.5" aria-hidden />
                  Live
                </Badge>
              ) : (
                <Badge variant="muted">Completed</Badge>
              )}
            </CardHeader>
            <CardContent>
              {stream.status === 'error' ? (
                <ErrorState
                  error={stream.error ?? new Error('The event stream disconnected.')}
                  className="py-8"
                />
              ) : (
                <div ref={listRef} className="scrollbar-thin max-h-[65vh] overflow-y-auto pr-1">
                  <RunEventTimeline
                    events={stream.events}
                    streamStatus={stream.status}
                    runActive={runActive}
                  />
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <div className="lg:sticky lg:top-24 lg:h-fit">
          <RunFacts run={data} status={liveStatus} />
        </div>
      </div>
    </div>
  )
}
