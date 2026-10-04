import { CheckCircle2, XCircle } from 'lucide-react'

import { StatusBadge } from '@/components/StatusBadge'
import { Badge } from '@/components/ui/badge'
import { Spinner } from '@/components/ui/spinner'
import { formatTime, prettyJson } from '@/lib/format'
import { ACCENT_CHIP, eventMeta } from '@/lib/status'
import { cn } from '@/lib/utils'
import type {
  ErrorPayload,
  FinalPayload,
  LlmMessagePayload,
  PlanPayload,
  RunEventDTO,
  RunStatus,
  StatusPayload,
  ToolRequestedPayload,
  ToolResultPayload,
} from '@/types'
import type { StreamStatus } from '@/api/useRunEventStream'

function JsonBlock({ value }: { value: unknown }) {
  if (value == null || (typeof value === 'object' && Object.keys(value).length === 0)) {
    return null
  }
  return (
    <pre className="scrollbar-thin mt-2 max-h-56 overflow-auto rounded-md bg-muted/60 p-3 font-mono text-xs leading-relaxed text-muted-foreground">
      {prettyJson(value)}
    </pre>
  )
}

function ToolName({ name }: { name: string }) {
  return (
    <code className="rounded bg-background/60 px-1.5 py-0.5 font-mono text-xs font-medium text-foreground ring-1 ring-border">
      {name}
    </code>
  )
}

function EventBody({ event }: { event: RunEventDTO }) {
  switch (event.kind) {
    case 'status': {
      const status = (event.payload as StatusPayload)?.status
      return status ? (
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>Status changed to</span>
          <StatusBadge status={status as RunStatus} />
        </div>
      ) : null
    }
    case 'plan': {
      const steps = (event.payload as PlanPayload)?.steps ?? []
      return (
        <ol className="mt-1 space-y-1.5">
          {steps.map((step, i) => (
            <li key={i} className="flex gap-2.5 text-sm">
              <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/15 text-[11px] font-semibold text-primary">
                {i + 1}
              </span>
              <span>{step}</span>
            </li>
          ))}
        </ol>
      )
    }
    case 'llm_message': {
      const msg = event.payload as LlmMessagePayload
      return (
        <div className="space-y-1.5">
          <Badge variant="muted" className="uppercase">
            {msg?.role ?? 'assistant'}
          </Badge>
          <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground/90">
            {msg?.content}
          </p>
        </div>
      )
    }
    case 'tool_requested': {
      const req = event.payload as ToolRequestedPayload
      return (
        <div className="space-y-1">
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">Requested tool</span>
            <ToolName name={req?.tool ?? 'unknown'} />
          </div>
          <JsonBlock value={req?.arguments} />
        </div>
      )
    }
    case 'tool_result': {
      const res = event.payload as ToolResultPayload
      return (
        <div className="space-y-1.5">
          <div className="flex items-center gap-2 text-sm">
            {res?.ok ? (
              <CheckCircle2 className="size-4 text-success" aria-hidden />
            ) : (
              <XCircle className="size-4 text-destructive" aria-hidden />
            )}
            <ToolName name={res?.tool ?? 'unknown'} />
            <span className={cn('font-medium', res?.ok ? 'text-success' : 'text-destructive')}>
              {res?.ok ? 'ok' : 'failed'}
            </span>
          </div>
          {res?.detail && (
            <p className="whitespace-pre-wrap text-sm text-muted-foreground">{res.detail}</p>
          )}
        </div>
      )
    }
    case 'final': {
      const answer = (event.payload as FinalPayload)?.answer
      return (
        <div className="rounded-lg border border-success/30 bg-success/5 p-3">
          <p className="whitespace-pre-wrap text-sm font-medium leading-relaxed">{answer}</p>
        </div>
      )
    }
    case 'error': {
      const message = (event.payload as ErrorPayload)?.message
      return <p className="text-sm font-medium text-destructive">{message}</p>
    }
    default:
      return <JsonBlock value={event.payload} />
  }
}

function TimelineItem({ event, last }: { event: RunEventDTO; last: boolean }) {
  const meta = eventMeta(event.kind)
  const Icon = meta.icon
  return (
    <li className="relative flex gap-4 pb-5 last:pb-0">
      {!last && <span className="absolute bottom-0 left-4 top-9 w-px -translate-x-1/2 bg-border" />}
      <span
        className={cn(
          'relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full ring-1',
          ACCENT_CHIP[meta.accent],
        )}
      >
        <Icon className="size-4" aria-hidden />
      </span>
      <div className="min-w-0 flex-1 animate-fade-in pt-1">
        <div className="mb-1 flex items-center gap-2">
          <span className="text-sm font-semibold">{meta.label}</span>
          <span className="text-xs text-muted-foreground">step {event.step}</span>
          <span className="ml-auto font-mono text-xs text-muted-foreground">
            {formatTime(event.at)}
          </span>
        </div>
        <EventBody event={event} />
      </div>
    </li>
  )
}

export function RunEventTimeline({
  events,
  streamStatus,
  runActive,
}: {
  events: RunEventDTO[]
  streamStatus: StreamStatus
  runActive: boolean
}) {
  const waiting = events.length === 0 && (streamStatus === 'connecting' || streamStatus === 'open')

  if (events.length === 0) {
    return (
      <div className="flex items-center justify-center gap-2 rounded-lg border border-dashed py-10 text-sm text-muted-foreground">
        {waiting ? (
          <>
            <Spinner /> Waiting for the agent to emit events…
          </>
        ) : (
          <span>No events recorded for this run.</span>
        )}
      </div>
    )
  }

  return (
    <ol className="relative">
      {events.map((event, i) => (
        <TimelineItem
          key={`${event.id}-${i}`}
          event={event}
          last={i === events.length - 1 && !runActive}
        />
      ))}
      {runActive && (
        <li className="relative flex items-center gap-4 pl-0.5 text-xs text-muted-foreground">
          <span className="flex size-7 items-center justify-center rounded-full bg-muted ring-1 ring-border">
            <Spinner className="size-3.5" />
          </span>
          <span>Streaming live…</span>
        </li>
      )}
    </ol>
  )
}
