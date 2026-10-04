import { Building2, Mail, ShieldCheck, Sparkles, UserRound } from 'lucide-react'
import { Link } from 'react-router-dom'

import { useMe, useRuns, useTenants } from '@/api/hooks'
import { ErrorState } from '@/components/States'
import { PageHeader } from '@/components/PageHeader'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDateTime, formatRelative } from '@/lib/format'
import type { TenantDTO } from '@/types'

function IdentityCard() {
  const me = useMe()

  if (me.isLoading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-32" />
        </CardHeader>
        <CardContent className="space-y-3">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-4 w-24" />
        </CardContent>
      </Card>
    )
  }
  if (me.isError) return <ErrorState error={me.error} onRetry={() => void me.refetch()} />
  if (!me.data) return null

  return (
    <Card>
      <CardHeader className="flex-row items-center gap-3 space-y-0">
        <span className="flex size-10 items-center justify-center rounded-full bg-primary/15 text-primary">
          <UserRound className="size-5" aria-hidden />
        </span>
        <div>
          <CardTitle>{me.data.name || me.data.subject}</CardTitle>
          <p className="mt-0.5 flex items-center gap-1.5 text-sm text-muted-foreground">
            <Mail className="size-3.5" aria-hidden />
            {me.data.email}
          </p>
        </div>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <div className="flex justify-between gap-4">
          <span className="text-muted-foreground">Subject</span>
          <span className="font-mono text-xs">{me.data.subject}</span>
        </div>
        <div className="flex items-center justify-between gap-4">
          <span className="text-muted-foreground">Roles</span>
          <div className="flex flex-wrap justify-end gap-1.5">
            {me.data.roles.length > 0 ? (
              me.data.roles.map((role) => (
                <Badge key={role} variant="secondary">
                  {role}
                </Badge>
              ))
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </div>
        </div>
        <div className="flex items-center justify-between gap-4">
          <span className="text-muted-foreground">Tenant ID</span>
          <span className="truncate font-mono text-xs" title={me.data.tenantId}>
            {me.data.tenantId}
          </span>
        </div>
      </CardContent>
    </Card>
  )
}

function TenantCard({ tenant }: { tenant: TenantDTO }) {
  return (
    <Card>
      <CardHeader className="flex-row items-center gap-3 space-y-0">
        <span className="flex size-10 items-center justify-center rounded-lg bg-primary/15 text-primary">
          <Building2 className="size-5" aria-hidden />
        </span>
        <div>
          <CardTitle>{tenant.name}</CardTitle>
          <p className="mt-0.5 font-mono text-xs text-muted-foreground">{tenant.slug}</p>
        </div>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        <div className="flex justify-between gap-4">
          <span className="text-muted-foreground">Tenant ID</span>
          <span className="truncate font-mono text-xs" title={tenant.id}>
            {tenant.id}
          </span>
        </div>
        <div className="flex justify-between gap-4">
          <span className="text-muted-foreground">Created</span>
          <span title={formatDateTime(tenant.createdAt)}>{formatRelative(tenant.createdAt)}</span>
        </div>
      </CardContent>
    </Card>
  )
}

function TenantsSection() {
  const tenants = useTenants()

  if (tenants.isLoading) {
    return (
      <div className="grid gap-4 sm:grid-cols-2">
        <Skeleton className="h-40" />
        <Skeleton className="h-40" />
      </div>
    )
  }
  if (tenants.isError)
    return <ErrorState error={tenants.error} onRetry={() => void tenants.refetch()} />

  const rows = tenants.data ?? []
  if (rows.length === 0) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-sm text-muted-foreground">
          No tenants are visible to you.
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {rows.map((tenant) => (
        <TenantCard key={tenant.id} tenant={tenant} />
      ))}
    </div>
  )
}

export function Overview() {
  const runs = useRuns()

  return (
    <div className="space-y-8">
      <PageHeader
        title="Overview"
        description="Your identity, tenant, and a quick way into runs — scoped to you by row-level security."
      />

      <Card className="overflow-hidden border-primary/20 bg-gradient-to-br from-primary/5 to-transparent">
        <CardContent className="flex flex-col gap-4 py-6 sm:flex-row sm:items-center sm:justify-between">
          <div className="max-w-2xl space-y-1.5">
            <div className="flex items-center gap-2 text-primary">
              <ShieldCheck className="size-5" aria-hidden />
              <span className="text-sm font-semibold uppercase tracking-wide">What this shows</span>
            </div>
            <p className="text-sm text-muted-foreground">
              Sign in as different demo users to see multi-tenant isolation first-hand: each caller
              only ever sees their own tenant, runs, and audit trail. Launch a run to watch the
              agent plan, call tools through the scoped-credential proxy, and stream events live —
              every action recorded in the audit log.
            </p>
          </div>
          <Button asChild className="shrink-0">
            <Link to="/runs">
              <Sparkles className="size-4" />
              Start a run
            </Link>
          </Button>
        </CardContent>
      </Card>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Signed in as
        </h2>
        <div className="grid gap-4 lg:grid-cols-2">
          <IdentityCard />
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Activity</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-baseline gap-2">
                <span className="text-3xl font-semibold tabular-nums">
                  {runs.isLoading ? '—' : (runs.data?.length ?? 0)}
                </span>
                <span className="text-sm text-muted-foreground">runs in this tenant</span>
              </div>
              <Button asChild variant="outline" size="sm" className="mt-4">
                <Link to="/runs">View runs</Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </section>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
          Your tenant
        </h2>
        <TenantsSection />
      </section>
    </div>
  )
}
