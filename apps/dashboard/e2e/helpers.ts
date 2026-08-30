import { expect, type Page } from '@playwright/test'

const API = process.env.E2E_API_URL ?? 'http://localhost:8080'
const EMAIL = process.env.E2E_ADMIN_EMAIL ?? 'dev-super@skelet.test'
const PASSWORD = process.env.E2E_ADMIN_PASSWORD ?? 'devpassword'

async function api(query: string, variables: unknown, token?: string) {
  const res = await fetch(`${API}/api/admin/graphql`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: JSON.stringify({ query, variables }),
  })
  return (await res.json()) as { data?: Record<string, unknown>; errors?: unknown }
}

// One API sign-in per worker: the login endpoint is rate limited, and a token per test trips it
let cachedToken: Promise<string> | null = null

export function adminToken(): Promise<string> {
  cachedToken ??= signIn()
  return cachedToken
}

async function signIn(): Promise<string> {
  const out = await api(
    'mutation($e:String!,$p:String!){ adminLogin(input:{email:$e,password:$p}){ accessToken } }',
    { e: EMAIL, p: PASSWORD },
  )
  const token = (out.data?.adminLogin as { accessToken?: string } | undefined)?.accessToken
  if (!token) throw new Error(`e2e cannot sign in as ${EMAIL} - is the stack up and seeded? ${JSON.stringify(out).slice(0, 200)}`)
  return token
}

/**
 * Workspaces roam, so they outlive the browser context AND the run - every spec starts from nothing
 * or it inherits the previous one's tabs
 */
export async function resetWorkspaces(): Promise<void> {
  const token = await adminToken()
  await api('mutation($k:String!,$v:JSON){ setMyPreference(key:$k, value:$v) }', { k: 'workspaces', v: null }, token)
}

export const AUTH_FILE = 'e2e/.auth/admin.json'

/**
 * Signs in through the UI. Only the setup project does this - every other spec inherits the saved
 * storage state, which keeps the run clear of the login rate limiter
 */
type ConfigField = { path: string; value: string; isDefault: boolean }

/** Reads one config field from the API, so a spec can capture exactly what it is about to disturb */
export async function configField(path: string): Promise<ConfigField> {
  const token = await adminToken()
  const out = await api('{ configSections { groups { fields { path value isDefault } } } }', {}, token)
  const sections = (out.data?.configSections ?? []) as { groups: { fields: ConfigField[] }[] }[]
  for (const section of sections) {
    for (const group of section.groups) {
      for (const field of group.fields) if (field.path === path) return field
    }
  }
  throw new Error(`no config field at ${path}`)
}

export const configValue = async (path: string) => (await configField(path)).value

export async function setConfigValue(path: string, value: string): Promise<void> {
  const token = await adminToken()
  await api('mutation($p:String!,$v:String!){ setConfigValue(path:$p, value:$v){ path } }', { p: path, v: value }, token)
}

/** Puts a field back exactly as it was found - an untouched field goes back to having no row at all */
export async function restoreConfig(field: ConfigField): Promise<void> {
  const token = await adminToken()
  if (field.isDefault) {
    await api('mutation($p:String!){ resetConfigValue(path:$p){ path } }', { p: field.path }, token)
    return
  }
  await setConfigValue(field.path, field.value)
}

export async function login(page: Page): Promise<void> {
  await page.goto('/login', { waitUntil: 'domcontentloaded' })
  await page.fill('#email', EMAIL)
  await page.fill('#password', PASSWORD)
  await page.click('button[type=submit]')
  await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 45_000 })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
}

/**
 * Starts a spec from an empty workspace. resetWorkspaces() clears the roaming copy, but the saved
 * storage state also carries the preference store's own device mirror, which would otherwise seed
 * the app before the server answers - the sessionStorage flag keeps a reload inside the test honest
 */
export async function startClean(page: Page): Promise<void> {
  await resetWorkspaces()
  await page.addInitScript(() => {
    if (sessionStorage.getItem('e2e:wiped')) return
    sessionStorage.setItem('e2e:wiped', '1')
    for (const key of Object.keys(localStorage)) {
      if (key.endsWith(':workspaces') || key === 'skelet-dash:workspace-tabs') localStorage.removeItem(key)
    }
  })
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  await expect(page.locator('[role=tab]').first()).toBeVisible()
}

/** Every pane's strip, as the tab labels a person would read off the screen */
export async function strips(page: Page): Promise<string[][]> {
  return page.$$eval('[role=tablist]', (lists) =>
    lists.map((l) => [...l.querySelectorAll('[role=tab]')].map((t) => (t.textContent ?? '').trim())),
  )
}

export const paneCount = async (page: Page) => (await strips(page)).length

/**
 * dnd-kit arms on an 8px pointer move, and reads the pointer on every move after that - one jump
 * from A to B fires a single event and lands nowhere, so the travel has to be stepped
 */
export async function dragTo(page: Page, from: { x: number; y: number }, to: { x: number; y: number }) {
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(from.x + 12, from.y, { steps: 4 })
  await page.mouse.move(to.x, to.y, { steps: 25 })
  await page.waitForTimeout(250)
  await page.mouse.up()
  await page.waitForTimeout(1200)
}

export async function tabCentre(page: Page, label: string) {
  const box = await page.locator(`[role=tab]:has-text("${label}")`).first().boundingBox()
  if (!box) throw new Error(`no tab labelled "${label}"`)
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

/** The workspace area. NOT `main` - SidebarInset is itself a <main>, with the app header inside it */
export async function workspaceBox(page: Page) {
  const box = await page.locator('main main, main').last().boundingBox()
  if (!box) throw new Error('workspace area not found')
  return box
}

export async function dragTabToEdge(page: Page, label: string, edge: 'left' | 'right' | 'top' | 'bottom') {
  const area = await workspaceBox(page)
  const inset = 60
  const target = {
    left: { x: area.x + inset, y: area.y + area.height / 2 },
    right: { x: area.x + area.width - inset, y: area.y + area.height / 2 },
    top: { x: area.x + area.width / 2, y: area.y + inset },
    bottom: { x: area.x + area.width / 2, y: area.y + area.height - inset },
  }[edge]
  await dragTo(page, await tabCentre(page, label), target)
}

/**
 * Radix hands focus back to the trigger as it closes, and a click that lands in that window is
 * swallowed - so this drives the trigger until it actually reports itself open
 */
export async function openWorkspaceMenu(page: Page) {
  const trigger = page.locator('button[title^="Workspace:"]')
  await expect
    .poll(async () => {
      if ((await trigger.getAttribute('aria-expanded')) === 'true') return 'true'
      await trigger.click()
      return await trigger.getAttribute('aria-expanded')
    })
    .toBe('true')
  await expect(page.locator('[role=menuitem]').first()).toBeVisible()
}

/** Radix animates the menu out, and clicking the trigger mid-animation hits a detaching item */
export async function closeMenu(page: Page) {
  await page.keyboard.press('Escape')
  await expect(page.locator('[role=menuitem]')).toHaveCount(0)
}

export const workspaceName = (page: Page) => page.locator('button[title^="Workspace:"]').innerText()

export async function menuAction(page: Page, item: string) {
  await openWorkspaceMenu(page)
  await page.click(`[role=menuitem]:has-text("${item}")`)
}

/** Opens a tab in the pane whose strip is at `index`, then waits for the strip to grow */
export async function newTab(page: Page, index = 0) {
  const before = (await strips(page))[index]?.length ?? 0
  await page.locator('button:has-text("New Tab")').nth(index).click()
  await expect
    .poll(async () => (await strips(page))[index]?.length ?? 0)
    .toBeGreaterThan(before)
}

/**
 * In-app navigation, so the tab keeps its mounted page instead of reloading the whole app. The
 * sidebar collapses whichever group you are not in, so a link is only sometimes on screen - the
 * search palette is the route that always works
 */
export async function navigate(page: Page, href: string, title: string) {
  const link = page.locator(`a[href="${href}"]`).first()
  if (await link.isVisible().catch(() => false)) {
    await link.click()
  } else {
    await page.keyboard.press('Control+k')
    const input = page.getByPlaceholder('Search pages and settings…')
    await expect(input).toBeVisible()
    await input.fill(title)
    await page.locator('[role=option]', { hasText: title }).first().click()
  }
  await expect(page.locator(`[role=tab]:has-text("${title}")`).first()).toBeVisible()
}

/** The split dividers are the only separators that span the whole workspace */
export async function dividers(page: Page) {
  return page.$$eval('main [role=separator]', (els) =>
    els
      .map((e) => {
        const r = e.getBoundingClientRect()
        return { orientation: e.getAttribute('aria-orientation'), x: Math.round(r.x), y: Math.round(r.y), w: r.width, h: r.height }
      })
      .filter((d) => d.h > 400 || d.w > 400),
  )
}
