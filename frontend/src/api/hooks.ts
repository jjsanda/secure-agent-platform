import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuth } from 'react-oidc-context'

import type { AuditDTO, CreateRunRequest, MeDTO, RunDTO, TenantDTO } from '@/types'
import { apiFetch } from './client'

/** The current access token, or undefined when not signed in. */
export function useAccessToken(): string | undefined {
  const auth = useAuth()
  return auth.user?.access_token
}

/** GET /me — the caller's verified identity + tenant. */
export function useMe() {
  const token = useAccessToken()
  return useQuery({
    queryKey: ['me'],
    enabled: !!token,
    staleTime: 5 * 60_000,
    queryFn: () => apiFetch<MeDTO>('/me', token!),
  })
}

/** GET /tenants — RLS-scoped; typically just the caller's own tenant. */
export function useTenants() {
  const token = useAccessToken()
  return useQuery({
    queryKey: ['tenants'],
    enabled: !!token,
    staleTime: 60_000,
    queryFn: () => apiFetch<TenantDTO[]>('/tenants', token!),
  })
}

/** GET /runs — polled so run statuses stay fresh. */
export function useRuns() {
  const token = useAccessToken()
  return useQuery({
    queryKey: ['runs'],
    enabled: !!token,
    refetchInterval: 5_000,
    queryFn: () => apiFetch<RunDTO[]>('/runs', token!),
  })
}

/** GET /runs/:id — a single run. */
export function useRun(id: string | undefined) {
  const token = useAccessToken()
  return useQuery({
    queryKey: ['run', id],
    enabled: !!token && !!id,
    queryFn: () => apiFetch<RunDTO>(`/runs/${id}`, token!),
  })
}

/** GET /audit — the security audit log, gently polled. */
export function useAudit() {
  const token = useAccessToken()
  return useQuery({
    queryKey: ['audit'],
    enabled: !!token,
    refetchInterval: 8_000,
    queryFn: () => apiFetch<AuditDTO[]>('/audit', token!),
  })
}

/** POST /runs — create + dispatch a run, then prime its cache entry. */
export function useCreateRun() {
  const token = useAccessToken()
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateRunRequest) =>
      apiFetch<RunDTO>('/runs', token!, { method: 'POST', body: JSON.stringify(body) }),
    onSuccess: (run) => {
      qc.setQueryData(['run', run.id], run)
      void qc.invalidateQueries({ queryKey: ['runs'] })
      void qc.invalidateQueries({ queryKey: ['audit'] })
    },
  })
}
