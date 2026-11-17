import path from 'node:path'
import { readFileSync } from 'node:fs'
import { defineConfig } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'

// The published @skelet/dashboard: this app compiled once, macros and the React compiler applied, every package it
// uses left as an import - so a project's build resolves one React, one Apollo and one copy of each registry, and
// bundles what a plugin brings beside it. package/build.mjs runs this, then adds the styles, types and preset
const version = process.env.SKELET_DASHBOARD_VERSION ?? (JSON.parse(readFileSync(path.resolve(__dirname, 'package.json'), 'utf8')) as { version: string }).version

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
  define: {
    __APP_VERSION__: JSON.stringify(version),
    __BUILD_DATE__: JSON.stringify(new Date().toISOString().slice(0, 10)),
  },
  resolve: {
    alias: {
      '@skelet/dashboard/sdk': path.resolve(__dirname, './src/sdk/index.ts'),
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: 'package/out/dist',
    emptyOutDir: true,
    minify: false, // the project's build minifies, and a readable library is easier to debug from
    sourcemap: true,
    lib: {
      entry: {
        index: path.resolve(__dirname, 'src/lib/package/mount.tsx'),
        sdk: path.resolve(__dirname, 'src/sdk/index.ts'),
        config: path.resolve(__dirname, 'src/lib/package/config.ts'),
      },
      formats: ['es'],
    },
    rollupOptions: { external },
  },
})
