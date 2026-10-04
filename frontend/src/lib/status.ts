import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  Clock,
  Cpu,
  Eye,
  Flag,
  KeyRound,
  ListChecks,
  LogIn,
  MessageSquare,
  Sparkles,
  Terminal,
  Wrench,
  XCircle,
  type LucideIcon,
} from 'lucide-react'

import type { BadgeProps } from '@/components/ui/badge'
import type { RunEventKind, RunStatus } from '@/types'

type BadgeVariant = NonNullable<BadgeProps['variant']>

// --- Run status -------------------------------------------------------------

export interface StatusMeta {
  label: string
  variant: BadgeVariant
  icon: LucideIcon
  /** Live (non-terminal) states pulse to signal work in progress. */
  live?: boolean
}

const STATUS_META: Record<RunStatus, StatusMeta> = {
  pending: { label: 'Pending', variant: 'muted', icon: Clock },
  planning: { label: 'Planning', variant: 'default', icon: ListChecks, live: true },
  acting: { label: 'Acting', variant: 'warning', icon: Cpu, live: true },
  observing: { label: 'Observing', variant: 'default', icon: Eye, live: true },
  succeeded: { label: 'Succeeded', variant: 'success', icon: CheckCircle2 },
  failed: { label: 'Failed', variant: 'destructive', icon: XCircle },
}

export function statusMeta(status: RunStatus): StatusMeta {
  return STATUS_META[status] ?? { label: status, variant: 'muted', icon: CircleDashed }
}

// --- Run event kinds --------------------------------------------------------

/** Tailwind accent classes for a timeline node (icon chip + connector). */
export type Accent = 'muted' | 'primary' | 'warning' | 'success' | 'destructive'

export interface EventMeta {
  label: string
  icon: LucideIcon
  accent: Accent
}

const EVENT_META: Record<RunEventKind, EventMeta> = {
  status: { label: 'Status', icon: Activity, accent: 'muted' },
  plan: { label: 'Plan', icon: ListChecks, accent: 'primary' },
  llm_message: { label: 'LLM message', icon: MessageSquare, accent: 'primary' },
  tool_requested: { label: 'Tool requested', icon: Wrench, accent: 'warning' },
  tool_result: { label: 'Tool result', icon: Terminal, accent: 'success' },
  final: { label: 'Final answer', icon: Sparkles, accent: 'success' },
  error: { label: 'Error', icon: AlertTriangle, accent: 'destructive' },
}

export function eventMeta(kind: string): EventMeta {
  return EVENT_META[kind as RunEventKind] ?? { label: kind, icon: CircleDashed, accent: 'muted' }
}

export const ACCENT_CHIP: Record<Accent, string> = {
  muted: 'bg-muted text-muted-foreground ring-border',
  primary: 'bg-primary/15 text-primary ring-primary/30',
  warning: 'bg-warning/15 text-warning ring-warning/30',
  success: 'bg-success/15 text-success ring-success/30',
  destructive: 'bg-destructive/15 text-destructive ring-destructive/30',
}

// --- Audit actions ----------------------------------------------------------

export interface ActionMeta {
  variant: BadgeVariant
  icon: LucideIcon
  /** Small leading dot colour in the audit table. */
  dot: string
}

/**
 * Colour-code audit actions by their outcome suffix: denied is red, allowed /
 * executed is green, credential mint is emphasised, everything else is neutral.
 */
export function auditActionMeta(action: string): ActionMeta {
  if (action.endsWith('.denied'))
    return { variant: 'destructive', icon: XCircle, dot: 'bg-destructive' }
  if (action.endsWith('.allowed') || action.endsWith('.executed'))
    return { variant: 'success', icon: CheckCircle2, dot: 'bg-success' }
  if (action.endsWith('.requested')) return { variant: 'warning', icon: Wrench, dot: 'bg-warning' }
  if (action.startsWith('credential.'))
    return { variant: 'default', icon: KeyRound, dot: 'bg-primary' }
  if (action === 'auth.login') return { variant: 'muted', icon: LogIn, dot: 'bg-muted-foreground' }
  if (action === 'run.created') return { variant: 'default', icon: Sparkles, dot: 'bg-primary' }
  if (action === 'run.finished') return { variant: 'default', icon: Flag, dot: 'bg-primary' }
  return { variant: 'muted', icon: Activity, dot: 'bg-muted-foreground' }
}
