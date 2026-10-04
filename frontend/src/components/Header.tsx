import { Building2, LogOut, ScrollText, ShieldCheck, Sparkles, SquareStack } from 'lucide-react'
import { useAuth } from 'react-oidc-context'
import { NavLink } from 'react-router-dom'

import { useMe, useTenants } from '@/api/hooks'
import { ThemeToggle } from '@/components/ThemeToggle'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

const NAV = [
  { to: '/', label: 'Overview', icon: SquareStack, end: true },
  { to: '/runs', label: 'Runs', icon: Sparkles, end: false },
  { to: '/audit', label: 'Audit', icon: ScrollText, end: false },
]

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

function TenantBadge() {
  const me = useMe()
  const tenants = useTenants()
  if (me.isLoading) return <Skeleton className="h-6 w-24 rounded-full" />
  if (!me.data) return null
  const tenant = tenants.data?.find((t) => t.id === me.data.tenantId)
  const label = tenant?.name ?? tenant?.slug ?? `${me.data.tenantId.slice(0, 8)}…`
  return (
    <Badge variant="outline" className="gap-1.5" title={`Tenant ${me.data.tenantId}`}>
      <Building2 className="size-3.5 text-primary" aria-hidden />
      {label}
    </Badge>
  )
}

function UserChip() {
  const me = useMe()
  if (me.isLoading) return <Skeleton className="h-9 w-40" />
  if (!me.data) return null
  return (
    <div className="flex items-center gap-2.5">
      <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary ring-1 ring-primary/25">
        {initials(me.data.name || me.data.email || me.data.subject)}
      </div>
      <div className="hidden leading-tight sm:block">
        <div className="text-sm font-medium">{me.data.name || me.data.subject}</div>
        <div className="text-xs text-muted-foreground">{me.data.email}</div>
      </div>
    </div>
  )
}

export function Header() {
  const auth = useAuth()

  return (
    <header className="sticky top-0 z-40 border-b border-border/60 bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="container flex h-16 items-center gap-4">
        <NavLink to="/" className="flex items-center gap-2.5">
          <span className="flex size-9 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
            <ShieldCheck className="size-5" aria-hidden />
          </span>
          <span className="hidden flex-col leading-none md:flex">
            <span className="text-sm font-semibold tracking-tight">Secure Agent Platform</span>
            <span className="text-[11px] text-muted-foreground">multi-tenant agent runtime</span>
          </span>
        </NavLink>

        <nav className="ml-2 hidden items-center gap-1 md:flex">
          {NAV.map(({ to, label, icon: Icon, end }) => (
            <NavLink
              key={to}
              to={to}
              end={end}
              className={({ isActive }) =>
                cn(
                  'flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                  isActive
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
                )
              }
            >
              <Icon className="size-4" aria-hidden />
              {label}
            </NavLink>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-2 sm:gap-3">
          <TenantBadge />
          <div className="hidden h-8 w-px bg-border sm:block" />
          <UserChip />
          <ThemeToggle />
          <Button
            variant="outline"
            size="sm"
            onClick={() => void auth.signoutRedirect()}
            title="Sign out"
          >
            <LogOut className="size-4" />
            <span className="hidden sm:inline">Sign out</span>
          </Button>
        </div>
      </div>

      {/* Compact nav for small screens */}
      <nav className="container flex items-center gap-1 pb-2 md:hidden">
        {NAV.map(({ to, label, icon: Icon, end }) => (
          <NavLink
            key={to}
            to={to}
            end={end}
            className={({ isActive }) =>
              cn(
                'flex flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
              )
            }
          >
            <Icon className="size-4" aria-hidden />
            {label}
          </NavLink>
        ))}
      </nav>
    </header>
  )
}
