import { describe, expect, it } from 'vitest'

// The product page shipped for a while with its own hand-rolled copy of the save registry, so it
// never declared unsaved work and silently dropped edits on a refresh - nothing failed
//
// This is the backstop. Any route-registered DETAIL page that stages edits and commits them has to
// declare that work through one of the three entry points below, or say here why it has none

// Read as text, not imported - this asserts on what the source says, like the backend's authz
// coverage test - vite's glob keeps it off node APIs the app tsconfig has no types for
// Plugins are globbed alongside features: a plugin page registers its own route and would otherwise
// fall outside this gate entirely, which is a worse hole than a stale entry
const sources = {
  ...(import.meta.glob('/src/features/**/*.{ts,tsx}', {
    query: '?raw',
    import: 'default',
    eager: true,
  }) as Record<string, string>),
  ...(import.meta.glob('/src/plugins/**/*.{ts,tsx}', {
    query: '?raw',
    import: 'default',
    eager: true,
  }) as Record<string, string>),
}

// Core's route table plus every plugin registration file - a plugin registers through the same nav
// registry, so its pages are routes by the same definition and belong under the same backstop
const routesSource = Object.values({
  ...(import.meta.glob('/src/nav/routes.tsx', {
    query: '?raw',
    import: 'default',
    eager: true,
  }) as Record<string, string>),
  ...(import.meta.glob('/src/plugins/*/plugin.{ts,tsx}', {
    query: '?raw',
    import: 'default',
    eager: true,
  }) as Record<string, string>),
}).join('\n')

// The sanctioned ways to declare unsaved work - see docs/conventions.md
const ENTRY_POINTS = ['useSaveRegistry', 'useDraft', 'useUnsavedFields', 'useUnsavedChangesGuard']

// The same, for pages a plugin registers - inert without that plugin, where a stale core entry is
// real drift, and kept here because the check runs over core's route table
const PLUGIN_NO_STAGED_EDITS: Record<string, string> = {
  '@/plugins/b2b/B2BQuotesPage': 'quote edits go through CRUD dialogs with their own Save',
  '@/plugins/b2b/CompaniesPage': 'list - the create form lives in a dialog with its own Save',
}

// Page module -> why it holds no unsaved work. Every entry is asserted to still be a real route, so
// a page that grows a staged form later shows up here rather than going quietly unguarded
const NO_STAGED_EDITS: Record<string, string> = {
  '@/features/audit/AuditLogPage': 'read-only log viewer - the only state is filters',
  '@/features/preferences/PreferencesPage':
    'preferences write immediately by convention - the control IS its own preview',
  '@/features/webhooks/WebhookDetail':
    'lifecycle actions (rotate, clear, delete) behind confirms - no staged form',
  '@/features/segments/SegmentsListPage': 'list only - the editor is SegmentDetailPage',

  '@/features/gift-cards/GiftCardsList': 'issue and adjust dialogs, each with its own Save',
  '@/features/qna/QuestionsList': 'moderation actions and answer dialogs',
  '@/features/remote-providers/RemoteProvidersList': 'provider config dialogs with their own Save',
  '@/features/scheduler/ScheduledJobsList': 'job create/edit dialogs with their own Save',

  '@/features/destinations/DestinationsPage': 'row edit dialogs with their own Save',
  '@/features/shipping/ShippingBoxesPage': 'row edit dialogs with their own Save',
  '@/features/orders/OrderDetail': 'order lifecycle actions and edit dialogs, no staged page form',
  '@/features/emails/EmailsHub': 'tab hub over three list tabs - the editors are their own routes',
  '@/features/searchmerch/SearchMerchPage':
    'synonym/pin/redirect/boost rows are add-and-delete CRUD, nothing staged',
}

function resolve(spec: string, fromKey: string): string | null {
  let base: string
  if (spec.startsWith('@/')) base = `/src/${spec.slice(2)}`
  else if (spec.startsWith('.')) {
    const dir = fromKey.slice(0, fromKey.lastIndexOf('/'))
    base = new URL(spec, `file://${dir}/`).pathname
  } else return null
  for (const ext of ['.tsx', '.ts']) if (base + ext in sources) return base + ext
  return null
}

/** A page plus the modules it pulls in directly - the form often lives one hop away */
function groupSources(pageKey: string): string[] {
  const own = sources[pageKey] ?? ''
  const out = [own]
  for (const m of own.matchAll(/from '((?:\.|@\/)[^']+)'/g)) {
    const key = resolve(m[1], pageKey)
    if (key && key !== pageKey) out.push(sources[key])
  }
  return out
}

function routePages(): Array<{ spec: string; key: string }> {
  const seen = new Set<string>()
  const out: Array<{ spec: string; key: string }> = []
  for (const m of routesSource.matchAll(/import\('(@\/(?:features|plugins)\/[^']+)'\)/g)) {
    if (seen.has(m[1])) continue
    seen.add(m[1])
    const key = resolve(m[1], '/src/nav/routes.tsx')
    if (key) out.push({ spec: m[1], key })
  }
  return out
}

describe('unsaved-work coverage', () => {
  const pages = routePages()

  it('finds the route table', () => {
    expect(pages.length).toBeGreaterThan(50)
  })

  it('every detail page that stages and commits edits declares its unsaved work', () => {
    const missing: string[] = []
    for (const page of pages) {
      if (page.spec in NO_STAGED_EDITS || page.spec in PLUGIN_NO_STAGED_EDITS) continue
      const group = groupSources(page.key)
      // DetailPageLayout is the detail/editor shell, and staged save is the rule on those - a list
      // page whose writes all live in CRUD dialogs is an allowed exception and never matches here
      const isDetailPage = group.some((s) => s.includes('<DetailPageLayout'))
      const stages = group.some((s) => /useState[<(]/.test(s))
      const commits = group.some((s) =>
        /onSubmit=\{|function onSubmit|function save\(|function onSave\(|function applyChanges\(/.test(
          s,
        ),
      )
      if (!isDetailPage || !stages || !commits) continue
      // Whole-word, or a renamed alias like `useSaveRegistryLegacy` would satisfy a substring match
      const declares = group.some((s) =>
        ENTRY_POINTS.some((e) => new RegExp(`\\b${e}\\b`).test(s)),
      )
      if (!declares) missing.push(page.spec)
    }
    expect(missing).toEqual([])
  })

  it('every listed exception is still a real route', () => {
    const specs = new Set(pages.map((p) => p.spec))
    // Plugin pages come and go with what is installed, so an entry for one is not stale here just
    // because this tree does not have that plugin - core entries are still held to it
    for (const spec of Object.keys(NO_STAGED_EDITS)) {
      if (spec.startsWith('@/plugins/')) continue
      expect(specs).toContain(spec)
    }
  })
})
