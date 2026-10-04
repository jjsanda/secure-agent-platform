import { InMemoryWebStorage, WebStorageStateStore } from 'oidc-client-ts'
import type { AuthProviderProps } from 'react-oidc-context'

import { config } from '@/config'

/**
 * After the IdP redirects back to `/callback?code=...&state=...`,
 * react-oidc-context completes the code exchange and calls this. We strip the
 * OAuth response params from the URL and return to the app root; the `/callback`
 * route additionally renders a redirect so the router lands on `/`.
 */
function onSigninCallback(): void {
  window.history.replaceState({}, document.title, '/')
}

/**
 * OIDC configuration for the authorization-code + PKCE flow against the bundled
 * mock provider (or Keycloak — only the issuer URL changes).
 *
 * The authenticated user is held in memory only, so no tokens are persisted to
 * localStorage. The transient auth/PKCE state keeps the default localStorage
 * store, which it must, to survive the full-page redirect to the IdP and back.
 */
export const oidcConfig: AuthProviderProps = {
  authority: config.oidc.issuer,
  client_id: config.oidc.clientId,
  redirect_uri: config.oidc.redirectUri,
  post_logout_redirect_uri: window.location.origin,
  response_type: 'code',
  scope: 'openid profile email',
  userStore: new WebStorageStateStore({ store: new InMemoryWebStorage() }),
  automaticSilentRenew: true,
  loadUserInfo: false,
  monitorSession: false,
  onSigninCallback,
}
