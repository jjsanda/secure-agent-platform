// Runtime configuration resolved from Vite env vars (baked at build time).
// Defaults match the default docker-compose profile.

export const config = {
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080',
  oidc: {
    issuer: import.meta.env.VITE_OIDC_ISSUER ?? 'http://localhost:9000',
    clientId: import.meta.env.VITE_OIDC_CLIENT_ID ?? 'sap-dashboard',
    redirectUri: import.meta.env.VITE_OIDC_REDIRECT_URI ?? 'http://localhost:5173/callback',
  },
} as const
