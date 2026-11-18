import path from 'node:path'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'

// The published @skelet/dashboard: this app compiled once, macros and the React compiler applied, every package it
// uses left as an import - so a project's build resolves one React, one Apollo and one copy of each registry, and
// bundles what a plugin brings beside it. package/build.mjs runs this, then adds the styles, types and preset
const version = process.env.SKELET_DASHBOARD_VERSION ?? (JSON.parse(readFileSync(path.resolve(import.meta.dirname, 'package.json'), 'utf8')) as { version: string }).version

// Anything not this app's own source - a package, by name
const external = (id: string) => !id.startsWith('.') && !id.startsWith('/') && !id.startsWith('@/') && !id.startsWith('\0') && !path.isAbsolute(id)

export default defineConfig({
  plugins: [
    react(),
    babel({
      plugins: [['@lingui/babel-plugin-lingui-macro', { stripMessageField: false }]],
      presets: [reactCompilerPreset()],
    }),
  ],
  // Nothing of the machine this builds on goes into the package: no .env is read, and no variable in its shell is a
  // setting. Left to Vite's defaults a VITE_GRAPHQL_URL there became every project's fallback endpoint - package/build.mjs
  // builds with canaries in those variables and fails if one turns up in the output
  envDir: false,
  envPrefix: 'SKELET_DASHBOARD_PACKAGE_BUILD_',
  define: {
    __APP_VERSION__: JSON.stringify(version),
    __BUILD_DATE__: JSON.stringify(new Date().toISOString().slice(0, 10)),
    // src/lib/runtime-config.ts: configured through configure(), or not started
    __SKELET_PACKAGE__: 'true',
  },
  resolve: {
    alias: { '@': path.resolve(import.meta.dirname, './src') },
  },
  build: {
    outDir: 'package/out/dist',
    emptyOutDir: true,
    copyPublicDir: false, // public/ ships beside dist, where the preset points a project's build at it
    minify: false, // the project's build minifies, and a readable library is easier to debug from
    sourcemap: false, // readable as it is - maps with their sources were more than half the package
    lib: {
      entry: {
        index: path.resolve(import.meta.dirname, 'src/lib/package/mount.tsx'),
        sdk: path.resolve(import.meta.dirname, 'src/sdk/index.ts'),
        config: path.resolve(import.meta.dirname, 'src/lib/package/config.ts'),
      },
      formats: ['es'],
    },
    rollupOptions: { external },
  },
})
