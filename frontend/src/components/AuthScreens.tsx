import { AlertTriangle, KeyRound, LogIn, ScrollText, ShieldCheck, Users } from 'lucide-react'
import { useAuth } from 'react-oidc-context'

import { ThemeToggle } from '@/components/ThemeToggle'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative flex min-h-screen items-center justify-center p-6">
      <div className="absolute right-4 top-4">
        <ThemeToggle />
      </div>
      {children}
    </div>
  )
}

const FEATURES = [
  { icon: Users, text: 'Row-level-security tenant isolation' },
  { icon: KeyRound, text: 'Short-lived, tool-scoped credentials' },
  { icon: ScrollText, text: 'Every agent action is audited' },
]

/** Full-screen sign-in / landing shown when the user is not authenticated. */
export function SignIn() {
  const auth = useAuth()
  return (
    <Shell>
      <div className="w-full max-w-md animate-fade-in">
        <div className="rounded-2xl border bg-card/80 p-8 shadow-lg backdrop-blur">
          <div className="mb-6 flex flex-col items-center text-center">
            <span className="mb-4 flex size-14 items-center justify-center rounded-2xl bg-primary text-primary-foreground shadow-md">
              <ShieldCheck className="size-7" aria-hidden />
            </span>
            <h1 className="text-xl font-semibold tracking-tight">Secure Agent Platform</h1>
            <p className="mt-1.5 text-sm text-muted-foreground">
              A multi-tenant runtime for running AI agents securely. Sign in to view your tenant,
              launch runs, and watch them stream live.
            </p>
          </div>

          <Button size="lg" className="w-full" onClick={() => void auth.signinRedirect()}>
            <LogIn className="size-4" />
            Sign in
          </Button>

          <div className="mt-6 rounded-lg border border-dashed bg-muted/40 p-3 text-center text-xs text-muted-foreground">
            Demo identities: pick{' '}
            <span className="font-medium text-foreground">Alice (Tenant A)</span> or{' '}
            <span className="font-medium text-foreground">Bob (Tenant B)</span> on the next screen —
            each sees only their own tenant&apos;s data.
          </div>

          <ul className="mt-6 space-y-2.5">
            {FEATURES.map(({ icon: Icon, text }) => (
              <li key={text} className="flex items-center gap-2.5 text-sm text-muted-foreground">
                <Icon className="size-4 shrink-0 text-primary" aria-hidden />
                {text}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </Shell>
  )
}

/** Full-screen loader for auth transitions (initial load, callback exchange). */
export function AuthLoading({ label = 'Loading…' }: { label?: string }) {
  return (
    <Shell>
      <div className="flex flex-col items-center gap-3 text-muted-foreground">
        <Spinner className="size-6 text-primary" />
        <p className="text-sm">{label}</p>
      </div>
    </Shell>
  )
}

/** Full-screen auth error with a recovery action. */
export function AuthErrorScreen({ error }: { error: Error }) {
  const auth = useAuth()
  return (
    <Shell>
      <div className="w-full max-w-md animate-fade-in rounded-2xl border border-destructive/40 bg-card p-8 text-center shadow-lg">
        <span className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-destructive/15 text-destructive">
          <AlertTriangle className="size-6" aria-hidden />
        </span>
        <h1 className="text-lg font-semibold">Sign-in failed</h1>
        <p className="mt-1.5 text-sm text-muted-foreground">{error.message}</p>
        <div className="mt-6 flex justify-center gap-2">
          <Button onClick={() => void auth.signinRedirect()}>
            <LogIn className="size-4" />
            Try again
          </Button>
          <Button variant="outline" onClick={() => window.location.assign('/')}>
            Back to start
          </Button>
        </div>
      </div>
    </Shell>
  )
}
