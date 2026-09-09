import path from 'node:path'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'

const pkg = JSON.parse(readFileSync(path.resolve(__dirname, 'package.json'), 'utf8')) as {
  version: string
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
  ],
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
    __BUILD_DATE__: JSON.stringify(new Date().toISOString().slice(0, 10)),
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  optimizeDeps: {
    // rrweb (the Replayer) is dynamic-imported only on the session-replay route. Pre-bundle it deterministically so the
    // first visit doesn't hit a lazy on-demand optimize + reload
    include: ['rrweb'],
    // The playground's Monaco worker registration is a bare `?worker` chain the dep optimizer can't pre-bundle - optimizing it serves a 504 and the whole route fails to load
    exclude: ['graphiql/setup-workers/vite', '@graphiql/react/setup-workers/vite'],
  },
  server: {
    // host:true binds to 0.0.0.0 so the dev server is reachable from outside the container. Harmless on host runs
    // (still maps to all local interfaces)
    host: true,
    proxy: {
      // API_PROXY is set to http://app:8080 inside the dashboard
      // container; on the host it defaults to localhost:8080. Not
      // VITE_-prefixed on purpose - read here at config time, never
      // surfaced to the browser bundle. ws:true forwards the WebSocket
      // upgrade for GraphQL subscriptions (live notifications).
      // Ahead of /api because vite matches in order, and the Signals API is its own service (docker compose --profile signals, SIGNALS_PORT 8090)
      '/api/signals': {
        target: process.env.SIGNALS_PROXY ?? 'http://localhost:8090',
      },
      '/api': {
        target: process.env.API_PROXY ?? 'http://localhost:8080',
        ws: true,
      },
    },
  },
})
