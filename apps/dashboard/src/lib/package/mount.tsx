import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from '@/App.tsx'
import { AppErrorBoundary } from '@/shared/errors/AppErrorBoundary'
import { bootLocale } from '@/lib/i18n'
import { runtimeConfig } from '@/lib/runtime-config'

// @skelet/dashboard's entry, and this repo's own - starts the dashboard in el. A plugin registers by being imported
// before this runs, and the styles are the page's to load: a project compiles @skelet/dashboard/styles.css with its
// plugins' sources, so their class names get styled too
export function mount(el: HTMLElement) {
  // Before the first render, so the shell doesn't flash English for admins who work in another
  // language - the catalog fetch is async, but the activate lands before most routes finish loading
  bootLocale()

  // Error monitoring is opt-in - with no DSN, the default in dev, the SDK is never downloaded, and
  // the dynamic import keeps @sentry/react out of the main bundle and the dev compile path
  if (runtimeConfig.sentryDsn) {
    void import('@sentry/react').then((Sentry) => {
      Sentry.init({
        dsn: runtimeConfig.sentryDsn,
        environment: runtimeConfig.environment,
        integrations: [Sentry.browserTracingIntegration()],
        tracesSampleRate: 0.05,
      })
    })
  }

  createRoot(el).render(
    <StrictMode>
      <AppErrorBoundary>
        <App />
      </AppErrorBoundary>
    </StrictMode>,
  )
}
