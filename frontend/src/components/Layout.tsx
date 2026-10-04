import { Outlet } from 'react-router-dom'

import { Header } from '@/components/Header'

export function Layout() {
  return (
    <div className="flex min-h-screen flex-col">
      <Header />
      <main className="container flex-1 py-8">
        <Outlet />
      </main>
      <footer className="border-t border-border/60 py-6">
        <div className="container flex flex-col items-center justify-between gap-2 text-xs text-muted-foreground sm:flex-row">
          <p>Secure Agent Platform — portfolio dashboard.</p>
          <p>Row-level tenant isolation · scoped credentials · audited tool calls.</p>
        </div>
      </footer>
    </div>
  )
}
