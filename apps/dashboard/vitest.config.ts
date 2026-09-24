import path from 'node:path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

// Deliberately separate from vite.config.ts: tests don't need tailwind, the React Compiler pass, or the dev-server
// proxy - just JSX + the @ alias, in jsdom
export default defineConfig({
  plugins: [react()],
  resolve: {
    // The lingui entries stub the macros (no babel pass here) AND the react bindings (no
    // I18nProvider in test renders) - tests assert the English source strings either way.
    // Order matters: the /macro entries must outrank the '@lingui/react' prefix
    alias: [
      { find: '@lingui/react/macro', replacement: path.resolve(__dirname, './src/test-lingui-stub.tsx') },
      { find: '@lingui/core/macro', replacement: path.resolve(__dirname, './src/test-lingui-stub.tsx') },
      { find: '@lingui/react', replacement: path.resolve(__dirname, './src/test-lingui-stub.tsx') },
      { find: '@', replacement: path.resolve(__dirname, './src') },
    ],
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['./src/test-setup.ts'],
    // Page tests wait up to 5s for a save -> mutate -> navigate round trip (SLOW in
    // src/test/page-harness) - the test itself must outlast those waits on a loaded machine
    testTimeout: 15000,
  },
})
