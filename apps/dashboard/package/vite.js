// @skelet/dashboard/vite - a project's dashboard build. skeletDashboard() is the whole of its vite.config: React for
// the plugins' own components, Tailwind over the dashboard and the plugins, the dashboard's public files, the API
// proxy for dev, and the _headers and _redirects a static host serves the build with
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { securityHeaders } from './headers.js'
import { imports } from './optimize.js'
import { shared, unbundled, workerCommonJS } from './shared.js'

const here = path.dirname(fileURLToPath(import.meta.url))

/**
 * @param {{ plugins?: string[], connect?: string[], frame?: string[] }} [options] plugins are the npm packages of the
 * plugins' dashboard halves; connect and frame are origins they reach or frame, added to the CSP beside the ones the
 * project's .env already names (the API, the storefront, error monitoring, the upload hosts)
 */
export function skeletDashboard(options = {}) {
  const plugins = options.plugins ?? []
  return {
    plugins: [
      react(),
      tailwindcss(),
      securityHeaders({ template: path.join(here, 'headers.template'), connect: options.connect, frame: options.frame }),
    ],
    publicDir: path.join(here, 'public'),
    resolve: {
      // One copy of each, whoever asks - see shared.js
      dedupe: [...shared, '@skelet/dashboard'],
    },
    optimizeDeps: {
      // Dev serves the dashboard and the plugins as the modules they are, the way this repo's own dev serves its
      // source: one copy of each registry between them, React refresh in the plugins, and the playground's worker
      // setup left to Vite. Pre-bundled, the dashboard's chunk for the playground fails to load and takes the page
      exclude: ['@skelet/dashboard', ...unbundled, ...plugins],
      // What the dashboard imports is pre-bundled up front instead - found one request at a time, each is a reload
      include: [
        ...imports.filter((spec) => !unbundled.includes(spec)).map((spec) => (shared.includes(packageOf(spec)) ? spec : `@skelet/dashboard > ${spec}`)),
        ...workerCommonJS.map((chain) => `@skelet/dashboard > ${chain}`),
      ],
    },
    server: {
      proxy: { '/api': { target: process.env.API_PROXY ?? 'http://localhost:8080', ws: true } },
    },
  }
}

const packageOf = (spec) => (spec.startsWith('@') ? spec.split('/').slice(0, 2).join('/') : spec.split('/')[0])
