// @skelet/dashboard/vite - a project's dashboard build. skeletDashboard() is the whole of its vite.config: React for
// the plugins' own components, Tailwind over the dashboard and the plugins, the dashboard's public files, the API
// proxy for dev, and the _headers and _redirects a static host serves the build with
import path from 'node:path'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const here = path.dirname(fileURLToPath(import.meta.url))

/** @param {{ plugins?: string[] }} [options] the npm packages of the plugins' dashboard halves */
export function skeletDashboard(options = {}) {
  const plugins = options.plugins ?? []
  return {
    plugins: [react(), tailwindcss(), securityHeaders()],
    publicDir: path.join(here, 'public'),
    resolve: {
      // A plugin package's own copy of these would be a second React, a second Apollo cache, a second set of
      // registries, a router and a toast store the dashboard's own never hear from
      dedupe: ['react', 'react-dom', '@apollo/client', '@skelet/dashboard', 'react-router-dom', '@lingui/core', '@lingui/react', 'sonner'],
    },
    optimizeDeps: {
      // The plugins are compiled as source, so React refresh reaches them - and the playground's Monaco worker setup
      // is a ?worker chain the dep optimizer can't pre-bundle
      exclude: ['graphiql/setup-workers/vite', '@graphiql/react/setup-workers/vite', ...plugins],
    },
    server: {
      proxy: { '/api': { target: process.env.API_PROXY ?? 'http://localhost:8080', ws: true } },
    },
  }
}

// The CSP the dashboard is served with, from the template skelet gen writes - the API's origin and the upload hosts
// filled in from the project's .env
function securityHeaders() {
  let connect = ''
  return {
    name: 'skelet-security-headers',
    apply: 'build',
    configResolved(config) {
      const env = loadEnv(config.mode, config.envDir || config.root, '')
      const api = /^https?:\/\//.test(env.VITE_GRAPHQL_URL ?? '') ? new URL(env.VITE_GRAPHQL_URL).origin : ''
      connect = [api, ...(env.UPLOAD_ORIGINS ?? '').split(/[\s,]+/)].filter(Boolean).join(' ')
    },
    generateBundle() {
      const template = readFileSync(path.join(here, 'headers.template'), 'utf8')
      this.emitFile({ type: 'asset', fileName: '_headers', source: template.replace(' {CONNECT_ORIGINS}', connect ? ` ${connect}` : '') })
      this.emitFile({ type: 'asset', fileName: '_redirects', source: '/* /index.html 200\n' })
    },
  }
}
