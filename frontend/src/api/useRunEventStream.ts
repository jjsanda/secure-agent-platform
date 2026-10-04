import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'

import type { RunEventDTO } from '@/types'
import { useAccessToken } from './hooks'
import { streamRunEvents } from './sse'

export type StreamStatus = 'idle' | 'connecting' | 'open' | 'done' | 'error'

export interface RunEventStream {
  events: RunEventDTO[]
  status: StreamStatus
  error: unknown
}

/**
 * Subscribe to a run's live event stream. Handles connect / replay / live
 * frames, de-dupes by event id, and — on completion — refreshes the run, runs
 * list, and audit log so the final answer/status and new audit rows appear. The
 * reader is aborted on unmount or when the run id changes.
 */
export function useRunEventStream(runId: string | undefined): RunEventStream {
  const token = useAccessToken()
  const qc = useQueryClient()
  const [events, setEvents] = useState<RunEventDTO[]>([])
  const [status, setStatus] = useState<StreamStatus>('idle')
  const [error, setError] = useState<unknown>(null)
  const seen = useRef<Set<number>>(new Set())

  useEffect(() => {
    if (!runId || !token) return

    const controller = new AbortController()
    seen.current = new Set()
    setEvents([])
    setError(null)
    setStatus('connecting')

    void streamRunEvents(runId, token, controller.signal, {
      onEvent: (ev) => {
        setStatus('open')
        setEvents((prev) => {
          if (typeof ev.id === 'number') {
            if (seen.current.has(ev.id)) return prev
            seen.current.add(ev.id)
          }
          return [...prev, ev]
        })
      },
      onDone: () => {
        setStatus('done')
        void qc.invalidateQueries({ queryKey: ['run', runId] })
        void qc.invalidateQueries({ queryKey: ['runs'] })
        void qc.invalidateQueries({ queryKey: ['audit'] })
      },
      onError: (err) => {
        setStatus('error')
        setError(err)
      },
    })

    return () => controller.abort()
  }, [runId, token, qc])

  return { events, status, error }
}
