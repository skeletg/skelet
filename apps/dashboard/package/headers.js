// The _headers and _redirects a static host serves the dashboard's build with - this repo's own build and a project's
// on @skelet/dashboard write them the same way, from the header template skelet gen keeps in step with core's
import { readFileSync } from 'node:fs'
import { loadEnv } from 'vite'

const origin = (url) => {
  try {
    return /^https?:\/\//.test(url ?? '') ? new URL(url).origin : ''
  } catch {
    return ''
  }
}
const unique = (values) => [...new Set(values.filter(Boolean))]

// What the build's settings make the page reach: the API when it's on another origin, with its websocket for
// subscriptions, the storefront's API for the playground, error monitoring, the upload hosts - and the storefront
// itself in a frame, for the CMS and blog previews
export function originsFrom(env) {
  const api = origin(env.VITE_GRAPHQL_URL)
  return {
    connect: unique([
      api,
      api.replace(/^http/, 'ws'),
      origin(env.VITE_FRONT_GRAPHQL_URL),
      origin(env.VITE_SENTRY_DSN),
      ...String(env.UPLOAD_ORIGINS ?? '').split(/[\s,]+/),
    ]),
    frame: unique([origin(env.VITE_STOREFRONT_ORIGIN)]),
  }
}

const CONNECT = ' {CONNECT_ORIGINS}'
const NO_FRAMES = "frame-src 'none'"

export function renderHeaders(template, connect, frame) {
  // A template that stopped carrying either would be written out unchanged, and nobody would know why the API or
  // a preview was blocked
  for (const marker of [CONNECT, NO_FRAMES]) {
    if (!template.includes(marker)) throw new Error(`the header template no longer holds "${marker.trim()}" - the security headers writer needs updating`)
  }
  return template
    .replace(CONNECT, connect.length ? ' ' + connect.join(' ') : '')
    .replace(NO_FRAMES, frame.length ? 'frame-src ' + frame.join(' ') : NO_FRAMES)
}

/** @param {{ template: string, connect?: string[], frame?: string[] }} options template is the header template's path */
export function securityHeaders({ template, connect = [], frame = [] }) {
  let origins = { connect: [], frame: [] }
  return {
    name: 'skelet-security-headers',
    apply: 'build',
    configResolved(config) {
      origins = originsFrom(loadEnv(config.mode, config.envDir || config.root, ''))
    },
    generateBundle() {
      const source = renderHeaders(readFileSync(template, 'utf8'), unique([...origins.connect, ...connect]), unique([...origins.frame, ...frame]))
      this.emitFile({ type: 'asset', fileName: '_headers', source })
      this.emitFile({ type: 'asset', fileName: '_redirects', source: '/* /index.html 200\n' })
    },
  }
}
