import { config } from '@/config'

/** Error thrown for any non-2xx API response, carrying the {error, detail} envelope. */
export class ApiError extends Error {
  readonly status: number
  readonly detail?: string

  constructor(status: number, message: string, detail?: string) {
    super(detail ? `${message}: ${detail}` : message)
    this.name = 'ApiError'
    this.status = status
    this.detail = detail
  }
}

interface ErrorEnvelope {
  error?: string
  detail?: string
}

/**
 * Authenticated JSON fetch against the control-plane REST surface. Sends the
 * access token as `Authorization: Bearer <token>` and unwraps the API's
 * `{error, detail}` envelope into an {@link ApiError} on failure.
 */
export async function apiFetch<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set('Authorization', `Bearer ${token}`)
  headers.set('Accept', 'application/json')
  if (init?.body != null && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const res = await fetch(`${config.apiBaseUrl}/api/v1${path}`, { ...init, headers })

  if (!res.ok) {
    let envelope: ErrorEnvelope = {}
    try {
      envelope = (await res.json()) as ErrorEnvelope
    } catch {
      // Non-JSON error body — fall back to the status text.
    }
    throw new ApiError(res.status, envelope.error ?? res.statusText, envelope.detail)
  }

  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}
