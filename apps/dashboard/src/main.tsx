import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { AppErrorBoundary } from '@/shared/errors/AppErrorBoundary'
import { bootLocale } from '@/lib/i18n'

// Before the first render, so the shell doesn't flash English for admins who work in another
// language - the catalog fetch is async, but the activate lands before most routes finish loading
bootLocale()

// Error monitoring is opt-in - with no DSN, the default in dev, the SDK is never downloaded, and
// the dynamic import keeps @sentry/react out of the main bundle and the dev compile path
if (import.meta.env.VITE_SENTRY_DSN) {
  void import('@sentry/react').then((Sentry) => {
    Sentry.init({
      dsn: import.meta.env.VITE_SENTRY_DSN,
      environment: import.meta.env.MODE,
      integrations: [Sentry.browserTracingIntegration()],
      tracesSampleRate: 0.05,
    })
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppErrorBoundary>
      <App />
    </AppErrorBoundary>
  </StrictMode>,
)
