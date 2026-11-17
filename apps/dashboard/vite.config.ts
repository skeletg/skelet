import path from 'node:path'
import { readFileSync } from 'node:fs'
import { defineConfig, loadEnv, type Plugin } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'

const pkg = JSON.parse(readFileSync(path.resolve(__dirname, 'package.json'), 'utf8')) as {
  version: string
}

// The npm packages holding enabled plugins' dashboard halves, read off the plugin set's decision file - served as
// source rather than pre-bundled, since a pre-bundled copy would carry registries of its own that nothing renders from
function pluginPackages(): string[] {
  try {
    const set = JSON.parse(readFileSync(path.resolve(__dirname, '../backend/core/plugins/plugins.json'), 'utf8')) as {
      plugins?: Record<string, { enabled?: boolean; dashboard?: string }>
    }
    // The recorded spec carries its version ("@acme/fraud-dashboard@^1.2.0") and an import never does
    return Object.values(set.plugins ?? {})
      .filter((p) => p.enabled && p.dashboard)
      .map((p) => p.dashboard as string)
      .map((spec) => (spec.lastIndexOf('@') > 0 ? spec.slice(0, spec.lastIndexOf('@')) : spec))
  } catch {
    return []
  }
}

function securityHeaders(): Plugin {
  let connect = ''
  return {
    name: 'skelet-security-headers',
    apply: 'build',
    configResolved(config) {
      const env = loadEnv(config.mode, config.envDir || __dirname, '')
      const api = /^https?:\/\//.test(env.VITE_GRAPHQL_URL ?? '') ? new URL(env.VITE_GRAPHQL_URL).origin : ''
      connect = [api, ...(env.UPLOAD_ORIGINS ?? '').split(/[\s,]+/)].filter(Boolean).join(' ')
    },
    generateBundle() {
      const template = readFileSync(path.resolve(__dirname, 'headers.template'), 'utf8')
      this.emitFile({ type: 'asset', fileName: '_headers', source: template.replace(' {CONNECT_ORIGINS}', connect ? ` ${connect}` : '') })
      this.emitFile({ type: 'asset', fileName: '_redirects', source: '/* /index.html 200\n' })
    },
  }
}

export default defineConfig({
  plugins: [
    react(),
    // React Compiler: automatic memoization for every component the compiler can prove safe (it silently skips ones it
    // can't). react-hooks v6 lint keeps the code within the rules it needs
    //
    // The Lingui macro runs in the same babel pass - plugins run before presets, so <Trans> is
    // plain code by the time the compiler sees it. stripMessageField stays false in prod on
    // purpose: the inline English IS the fallback catalog, which is what lets a language pack
    // built against an older dashboard degrade to partial coverage instead of raw message ids
    babel({
      plugins: [['@lingui/babel-plugin-lingui-macro', { stripMessageField: false }]],
      presets: [reactCompilerPreset()],
    }),
    tailwindcss(),
    securityHeaders(),
  ],
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
    __BUILD_DATE__: JSON.stringify(new Date().toISOString().slice(0, 10)),
  },
  resolve: {
    alias: {
      // What a plugin imports - see src/sdk/index.ts
      '@skelet/dashboard-sdk': path.resolve(__dirname, './src/sdk/index.ts'),
      '@': path.resolve(__dirname, './src'),
    },
    // A plugin package's own copy of these would be a second React, a second Apollo cache, a router and a toast
    // store the dashboard's own never hear from
    dedupe: ['react', 'react-dom', '@apollo/client', 'react-router-dom', '@lingui/core', '@lingui/react', 'sonner'],
  },
  optimizeDeps: {
    // rrweb (the Replayer) is dynamic-imported only on the session-replay route. Pre-bundle it deterministically so the
    // first visit doesn't hit a lazy on-demand optimize + reload
    // The two CommonJS packages inside the playground's GraphQL worker - served raw, `import x from` finds no default
    // export, the worker dies at load and Monaco runs the language service on the main thread, freezing the page
    include: [
      'rrweb',
      'graphiql > @graphiql/react > monaco-graphql > graphql-language-service > nullthrows',
      'graphiql > @graphiql/react > monaco-graphql > picomatch-browser',
    ],
    // The playground's Monaco worker registration is a bare `?worker` chain the dep optimizer can't pre-bundle - optimizing it serves a 504 and the whole route fails to load
    exclude: ['graphiql/setup-workers/vite', '@graphiql/react/setup-workers/vite', ...pluginPackages()],
  },
  server: {
    // host:true binds to 0.0.0.0 so the dev server is reachable from outside the container. Harmless on host runs
    // (still maps to all local interfaces)
    host: true,
    proxy: {
      '/api': {
        target: process.env.API_PROXY ?? 'http://localhost:8080',
        ws: true,
      },
    },
  },
})
