import { config } from '@/config'
import type { RunEventDTO } from '@/types'

export interface StreamHandlers {
  onEvent: (event: RunEventDTO) => void
  onDone?: () => void
  onError?: (error: unknown) => void
}

/**
 * Read a run's live event stream over Server-Sent Events using `fetch` — the
 * native `EventSource` cannot attach the `Authorization` header. We read the
 * response body as a stream and parse SSE frames ourselves: `id:` / `event:` /
 * `data:` lines separated by a blank line, with an `event: done` frame ending
 * the stream. Pass an `AbortSignal` to cancel on unmount / navigation.
 */
export async function streamRunEvents(
  runId: string,
  token: string,
  signal: AbortSignal,
  handlers: StreamHandlers,
): Promise<void> {
  const { onDone, onError } = handlers
  try {
    const res = await fetch(`${config.apiBaseUrl}/api/v1/runs/${runId}/events`, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: 'text/event-stream',
      },
      signal,
    })
    if (!res.ok || !res.body) {
      throw new Error(`event stream failed (HTTP ${res.status})`)
    }

    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      // Normalise CRLF so frame splitting on a blank line is reliable.
      buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n')

      let boundary = buffer.indexOf('\n\n')
      while (boundary !== -1) {
        const frame = buffer.slice(0, boundary)
        buffer = buffer.slice(boundary + 2)
        if (handleFrame(frame, handlers)) {
          return // saw `event: done`
        }
        boundary = buffer.indexOf('\n\n')
      }
    }
    // Server closed the stream without an explicit done frame.
    onDone?.()
  } catch (err) {
    if (signal.aborted || (err instanceof DOMException && err.name === 'AbortError')) {
      return
    }
    onError?.(err)
  }
}

/** Parse one SSE frame. Returns true when the frame signals end-of-stream. */
function handleFrame(frame: string, handlers: StreamHandlers): boolean {
  let eventName = 'message'
  const dataLines: string[] = []

  for (const line of frame.split('\n')) {
    if (line === '' || line.startsWith(':')) continue // blank or keep-alive comment
    const colon = line.indexOf(':')
    const field = colon === -1 ? line : line.slice(0, colon)
    let value = colon === -1 ? '' : line.slice(colon + 1)
    if (value.startsWith(' ')) value = value.slice(1)
    if (field === 'event') eventName = value
    else if (field === 'data') dataLines.push(value)
    // `id:` is ignored; each RunEventDTO already carries its own id.
  }

  if (eventName === 'done') {
    handlers.onDone?.()
    return true
  }
  if (dataLines.length === 0) return false
  try {
    handlers.onEvent(JSON.parse(dataLines.join('\n')) as RunEventDTO)
  } catch {
    // Skip a malformed frame rather than tearing down the whole stream.
  }
  return false
}
