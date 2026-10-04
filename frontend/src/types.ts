// Shared DTOs mirroring the control-plane REST + SSE contract
// (control-plane/internal/api/dto.go). All JSON is camelCase.

/** Run lifecycle states (control-plane run.status). */
export type RunStatus = 'pending' | 'planning' | 'acting' | 'observing' | 'succeeded' | 'failed'

/** The terminal states in which a run no longer emits live events. */
export const TERMINAL_STATUSES: readonly RunStatus[] = ['succeeded', 'failed']

/** Agent implementation selected for a run. */
export type RunVariant = 'custom' | 'langgraph'

/** GET /api/v1/me — the caller's identity, straight from the verified token. */
export interface MeDTO {
  subject: string
  tenantId: string
  email: string
  name: string
  roles: string[]
}

/** TenantDTO — the public shape of a tenant. */
export interface TenantDTO {
  id: string
  slug: string
  name: string
  createdAt: string
}

/** RunDTO — the public shape of an agent run. */
export interface RunDTO {
  id: string
  objective: string
  variant: string
  allowedTools: string[]
  status: RunStatus
  finalAnswer: string | null
  error: string | null
  createdAt: string
  updatedAt: string
}

/** Body for POST /api/v1/runs. */
export interface CreateRunRequest {
  objective: string
  variant?: RunVariant
  allowedTools?: string[]
}

/** AuditDTO — one append-only audit record. */
export interface AuditDTO {
  id: number
  at: string
  actor: string
  action: string
  runId: string | null
  toolName: string | null
  detail: Record<string, unknown>
}

/** The kind discriminator on a streamed run event. */
export type RunEventKind =
  'status' | 'plan' | 'llm_message' | 'tool_requested' | 'tool_result' | 'final' | 'error'

/**
 * RunEventDTO — one item in a run's event trace. Delivered as the JSON body of
 * each SSE `data:` frame. `payload` is discriminated by `kind` (see the payload
 * interfaces below).
 */
export interface RunEventDTO {
  id: number
  kind: RunEventKind | string
  step: number
  payload: unknown
  at: string
}

// --- Per-kind payload shapes (control-plane maps proto oneof -> these) -------

export interface StatusPayload {
  status: RunStatus
}
export interface PlanPayload {
  steps: string[]
}
export interface LlmMessagePayload {
  role: string
  content: string
}
export interface ToolRequestedPayload {
  tool: string
  arguments: Record<string, unknown>
}
export interface ToolResultPayload {
  tool: string
  ok: boolean
  detail: string
}
export interface FinalPayload {
  answer: string
}
export interface ErrorPayload {
  message: string
}
