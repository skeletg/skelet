import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from '@/App.tsx'
import { AppErrorBoundary } from '@/shared/errors/AppErrorBoundary'
import { bootLocale } from '@/lib/i18n'
import { runtimeConfig } from '@/lib/runtime-config'

export type MountOptions = {
  // Loads the project's plugins, by importing them: () => import('./plugins'). They load here, after the dashboard's
  // own menu and registries exist - a plugin adding a page under a core menu group needs the group to be there - and
  // before the first render
  plugins?: () => Promise<unknown>
}

// @skelet/dashboard's entry, and this repo's own - starts the dashboard in el. The styles are the page's to load: a
// project compiles @skelet/dashboard/styles.css with its plugins' sources, so their class names get styled too
export async function mount(el: HTMLElement, options: MountOptions = {}) {
  // A plugin that fails as it loads costs its own pages, not the dashboard - without the catch nothing ever renders
  try {
    await options.plugins?.()
  } catch (err) {
    console.error('@skelet/dashboard: a plugin failed to load, and the dashboard is starting without what it had left to register', err)
  }

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
