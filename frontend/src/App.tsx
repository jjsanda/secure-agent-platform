import { useAuth } from 'react-oidc-context'
import { Navigate, Route, Routes } from 'react-router-dom'

import { AuthErrorScreen, AuthLoading, SignIn } from '@/components/AuthScreens'
import { Layout } from '@/components/Layout'
import { Audit } from '@/pages/Audit'
import { NotFound } from '@/pages/NotFound'
import { Overview } from '@/pages/Overview'
import { RunDetail } from '@/pages/RunDetail'
import { Runs } from '@/pages/Runs'

export default function App() {
  const auth = useAuth()

  // Auth transitions (initial load, callback code-exchange, silent renew).
  if (auth.isLoading) {
    const onCallback = window.location.pathname.startsWith('/callback')
    return <AuthLoading label={onCallback ? 'Completing sign-in…' : 'Loading…'} />
  }
  if (auth.error) return <AuthErrorScreen error={auth.error} />
  if (!auth.isAuthenticated) return <SignIn />

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<Overview />} />
        <Route path="/runs" element={<Runs />} />
        <Route path="/runs/:id" element={<RunDetail />} />
        <Route path="/audit" element={<Audit />} />
        {/* The IdP redirects here; once authenticated we land on the overview. */}
        <Route path="/callback" element={<Navigate to="/" replace />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}
