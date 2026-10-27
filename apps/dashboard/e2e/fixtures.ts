import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, type Page } from '@playwright/test'
import { Kind, parse, print, visit, type DefinitionNode, type FragmentDefinitionNode, type OperationDefinitionNode } from 'graphql'
import { adminApi, startClean, test } from './helpers'

// What the form specs share: a run tag every record they create carries, the cleanup registry that
// deletes those records again, and the assertions that compare what the API held before and after a Save

/** Tags every record a spec creates, so a leftover from a crashed run is easy to find */
export const RUN = `e2e-forms-${Date.now()}`

const cleanups: (() => Promise<unknown>)[] = []

/** Deletes the record once the spec file finishes - newest first, so children go before parents */
export function cleanup(undo: () => Promise<unknown>) {
  cleanups.push(undo)
}

/** Call once at the top of a spec file that creates records */
export function registerCleanups() {
  test.afterAll(async () => {
    for (const undo of cleanups.splice(0).reverse()) await undo().catch(() => undefined)
  })
}

// A one-paragraph Lexical document, the shape the rich-text editor stores
export const lexical = (text: string) => ({
  root: {
    type: 'root',
    version: 1,
    direction: null,
    format: '',
    indent: 0,
    children: [
      {
        type: 'paragraph',
        version: 1,
        direction: null,
        format: '',
        indent: 0,
        textFormat: 0,
        textStyle: '',
        children: [{ type: 'text', version: 1, detail: 0, format: 0, mode: 'normal', style: '', text }],
      },
    ],
  },
})

export async function firstChannel() {
  const { channels } = await adminApi<{ channels: { id: string; name: string; currencyCode: string }[] }>('{ channels { id name currencyCode } }')
  if (!channels[0]) throw new Error('the dev stack has no channel')
  return channels[0]
}

// CI's database starts with no catalog, so a spec that needs a product type makes one for the run
export async function productTypeId(): Promise<string> {
  const { productTypes } = await adminApi<{ productTypes: { id: string }[] }>('{ productTypes { id } }')
  if (productTypes[0]) return productTypes[0].id
  const { createProductType } = await adminApi<{ createProductType: { id: string } }>('mutation($i:CreateProductTypeInput!){ createProductType(input:$i){ id } }', {
    i: { slug: `${RUN}-type`, name: `${RUN} type` },
  })
  cleanup(() => adminApi('mutation($id:ID!){ deleteProductType(id:$id) }', { id: createProductType.id }))
  return createProductType.id
}

export async function open(page: Page, path: string) {
  await startClean(page)
  await page.goto(path, { waitUntil: 'domcontentloaded' })
  // The roaming workspace and the header scope land a beat after the page - typing before they do can
  // land in a tab that is about to be rebuilt, or be validated against a page that hasn't loaded yet
  await page.waitForLoadState('networkidle')
}

// Pages that edit translations only once a channel + locale is picked in the header
export async function openScoped(page: Page, path: string) {
  const channel = await firstChannel()
  await page.addInitScript((scope) => localStorage.setItem('skelet-dash:config-scope', scope), JSON.stringify({ mode: 'CHANNEL', channelId: channel.id, locale: 'en' }))
  await open(page, path)
}

/** The stored drafts in this browser - `match` narrows to keys containing it */
export function draftKeys(page: Page, match = ''): Promise<string[]> {
  return page.evaluate((m) => Object.keys(localStorage).filter((k) => k.startsWith('skelet.draft.') && k.includes(m)), match)
}

export const hasDraft = async (page: Page, match = '') => (await draftKeys(page, match)).length > 0

const VOLATILE = new Set(['updatedAt', 'createdAt', 'lastLoginAt', 'lastUsedAt'])

/** Every leaf path where two API reads differ - `a.b[2].c` - ignoring the timestamps a write moves */
export function changedPaths(before: unknown, after: unknown, at = ''): string[] {
  if (Object.is(before, after)) return []
  const bothObjects = before !== null && after !== null && typeof before === 'object' && typeof after === 'object'
  if (!bothObjects || Array.isArray(before) !== Array.isArray(after)) return [at || '(root)']
  if (Array.isArray(before) && Array.isArray(after)) {
    const out: string[] = []
    for (let i = 0; i < Math.max(before.length, after.length); i++) out.push(...changedPaths(before[i], after[i], `${at}[${i}]`))
    return out
  }
  const a = before as Record<string, unknown>
  const b = after as Record<string, unknown>
  const out: string[] = []
  for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) {
    if (VOLATILE.has(key)) continue
    out.push(...changedPaths(a[key], b[key], at ? `${at}.${key}` : key))
  }
  return out
}

/**
 * The sweep's core assertion: a Save moved something, and nothing outside the paths it was allowed to. A
 * path listed here covers everything under it - `address` covers `address.line1` - and a record's
 * resolved-locale fields are listed beside its rows, since which of them moves depends on the locale
 */
export function expectOnlyChanged(before: unknown, after: unknown, expected: string[]) {
  const moved = changedPaths(before, after)
  const under = (p: string, e: string) => p === e || p.startsWith(`${e}.`) || p.startsWith(`${e}[`)
  const stray = moved.filter((p) => !expected.some((e) => under(p, e)))
  expect(stray, `the Save also changed ${stray.join(', ')}\nbefore: ${JSON.stringify(before)}\nafter:  ${JSON.stringify(after)}`).toEqual([])
  expect(moved.length, `the Save changed none of ${expected.join(', ')}`).toBeGreaterThan(0)
}

/**
 * A create page's header button writes straight through, then the page moves off `/new` - returns
 * the path it landed on, for the caller to pull an id out of
 */
export async function submitCreate(page: Page, label: string | RegExp): Promise<string> {
  const from = new URL(page.url()).pathname
  await page.getByRole('button', { name: label }).click()
  await expect.poll(() => new URL(page.url()).pathname, { message: `still on ${from} after ${label}` }).not.toBe(from)
  return new URL(page.url()).pathname
}

// ---------- The page's own queries ----------

// Every operation and fragment in the dashboard's documents, by name - read once per worker
let documents: { ops: Map<string, OperationDefinitionNode>; fragments: Map<string, FragmentDefinitionNode> } | null = null

function loadDocuments() {
  if (documents) return documents
  const ops = new Map<string, OperationDefinitionNode>()
  const fragments = new Map<string, FragmentDefinitionNode>()
  const add = (source: string) => {
    for (const def of parse(source).definitions as DefinitionNode[]) {
      if (def.kind === Kind.OPERATION_DEFINITION && def.name) ops.set(def.name.value, def)
      if (def.kind === Kind.FRAGMENT_DEFINITION) fragments.set(def.name.value, def)
    }
  }
  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const path = join(dir, entry.name)
      if (entry.isDirectory()) walk(path)
      else if (entry.name.endsWith('.graphql')) add(readFileSync(path, 'utf8'))
      // A few documents are still hand-written gql`` literals - a generated .gql.ts sits beside its
      // .graphql and repeats it, so it is skipped. Fragment interpolations are dropped: the fragments
      // themselves are picked up by name wherever they are defined
      else if (/\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) && !(entry.name.endsWith('.gql.ts') && existsSync(path.replace(/\.gql\.ts$/, '.graphql')))) {
        for (const m of readFileSync(path, 'utf8').matchAll(/\bgql`([^`]*)`/g)) {
          try {
            add(m[1].replace(/\$\{[^}]*\}/g, ''))
          } catch {
            // not a document this spec can use
          }
        }
      }
    }
  }
  walk(fileURLToPath(new URL('../src', import.meta.url)))
  documents = { ops, fragments }
  return documents
}

/**
 * A named operation from the dashboard's own .graphql documents, with every fragment it spreads.
 * Reading a record back through the query its page renders from means the before/after comparison
 * covers exactly the fields the form can see - and follows the page when the query grows
 */
export function pageQuery(name: string): string {
  const { ops, fragments } = loadDocuments()
  const op = ops.get(name)
  if (!op) throw new Error(`no operation named ${name} in src/**/*.graphql`)
  const needed = new Map<string, FragmentDefinitionNode>()
  const collect = (node: OperationDefinitionNode | FragmentDefinitionNode) =>
    visit(node, {
      FragmentSpread(spread) {
        const name = spread.name.value
        if (needed.has(name)) return
        const frag = fragments.get(name)
        if (!frag) throw new Error(`${name} is spread but not defined in src/**/*.graphql`)
        needed.set(name, frag)
        collect(frag)
      },
    })
  collect(op)
  return [op, ...needed.values()].map((d) => print(d)).join('\n')
}

export type DashboardQuery = {
  name: string
  /** The operation with its fragments, `__typename` added to every selection the way Apollo sends it */
  document: string
  /** Variables the operation can't run without - non-null and no default */
  required: string[]
  /** Every variable it declares */
  variables: string[]
}

/** Every query operation in the dashboard's documents, ready to send */
export function dashboardQueries(): DashboardQuery[] {
  const { ops } = loadDocuments()
  const out: DashboardQuery[] = []
  for (const [name, op] of ops) {
    if (op.operation !== 'query') continue
    const withTypename = parse(pageQuery(name))
    const document = print(
      visit(withTypename, {
        SelectionSet(set, _key, parent) {
          // The operation's own root selection has no __typename to add
          if (parent && !Array.isArray(parent) && (parent as { kind?: string }).kind === Kind.OPERATION_DEFINITION) return undefined
          if (set.selections.some((s) => s.kind === Kind.FIELD && s.name.value === '__typename')) return undefined
          return { ...set, selections: [...set.selections, { kind: Kind.FIELD, name: { kind: Kind.NAME, value: '__typename' } }] }
        },
      }),
    )
    const required = (op.variableDefinitions ?? []).filter((v) => v.type.kind === Kind.NON_NULL_TYPE && !v.defaultValue).map((v) => v.variable.name.value)
    const variables = (op.variableDefinitions ?? []).map((v) => v.variable.name.value)
    out.push({ name, document, required, variables })
  }
  return out
}

type MyScopes = { me: { isSuperuser: boolean; roles: { scopes: { channelId: string | null; permission: { slug: string } }[] }[] } }

/** Whether the e2e admin holds a permission - everywhere, or on the given channel */
export async function adminCan(slug: string, channelId?: string): Promise<boolean> {
  const { me } = await adminApi<MyScopes>('{ me { isSuperuser roles { scopes { channelId permission { slug } } } } }')
  if (me.isSuperuser) return true
  const [resource] = slug.split('.')
  // .manage covers .view, the way the server reads it
  const grants = slug.endsWith('.view') ? [slug, `${resource}.manage`] : [slug]
  return me.roles.some((r) => r.scopes.some((sc) => grants.includes(sc.permission.slug) && (sc.channelId === null || sc.channelId === channelId)))
}
