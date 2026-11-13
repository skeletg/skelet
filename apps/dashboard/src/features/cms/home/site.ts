export type SitePage = {
  id: string
  urlPath: string
  title: string
  isPublished: boolean
  scheduledAt: string | null
  workflowState: string
  updatedAt: string
  home: boolean
  typeId: string
  outline: string[]
}

export type State = 'live' | 'scheduled' | 'review' | 'approved' | 'draft'

export const STATES: State[] = ['live', 'draft', 'review', 'approved', 'scheduled']

export const STATE_LABEL: Record<State, string> = {
  live: 'Live',
  draft: 'Draft',
  review: 'In review',
  approved: 'Approved',
  scheduled: 'Scheduled',
}

// Workflow runs beside publishing, so a page that is out is live whatever its workflow says
export function stateOf(p: Pick<SitePage, 'isPublished' | 'scheduledAt' | 'workflowState'>): State {
  if (p.isPublished) return 'live'
  if (p.scheduledAt) return 'scheduled'
  if (p.workflowState === 'IN_REVIEW') return 'review'
  if (p.workflowState === 'APPROVED') return 'approved'
  return 'draft'
}

export type Tally = Record<State, number>

export function tally(pages: SitePage[]): Tally {
  const out: Tally = { live: 0, scheduled: 0, review: 0, approved: 0, draft: 0 }
  for (const p of pages) out[stateOf(p)] += 1
  return out
}

export const addressOf = (p: SitePage) => (p.home ? '/' : `/${p.urlPath}`)

const head = (p: SitePage) => p.urlPath.split('/')[0] ?? ''

export function matches(p: SitePage, show: State | 'all', query: string): boolean {
  if (show !== 'all' && stateOf(p) !== show) return false
  const q = query.trim().toLowerCase()
  return q === '' || p.title.toLowerCase().includes(q) || `/${p.urlPath}`.toLowerCase().includes(q)
}

export type Lane = { key: string; name: string; path: boolean; pages: SitePage[] }

const inOrder = (pages: SitePage[]) => [...pages].sort((a, b) => Number(b.home) - Number(a.home) || a.urlPath.localeCompare(b.urlPath))

// The top of the site first, homepage leading, then a lane for each first segment that has pages under it - the
// page sitting at that segment itself goes in with them, as the folder's own page
export function lanes(pages: SitePage[]): Lane[] {
  const sorted = inOrder(pages)
  const nested = new Set(sorted.filter((p) => !p.home && p.urlPath.includes('/')).map(head))
  const top: SitePage[] = []
  const folders = new Map<string, SitePage[]>()
  for (const p of sorted) {
    if (p.home || !nested.has(head(p))) {
      top.push(p)
      continue
    }
    const held = folders.get(head(p))
    if (held) held.push(p)
    else folders.set(head(p), [p])
  }
  const out: Lane[] = top.length > 0 ? [{ key: 'top', name: 'Top of the site', path: false, pages: top }] : []
  for (const [key, held] of folders) out.push({ key, name: `/${key}`, path: true, pages: held })
  return out
}

export function lanesByType(pages: SitePage[], types: Array<{ id: string; name: string }>): Lane[] {
  const sorted = inOrder(pages)
  const out: Lane[] = types
    .map((t) => ({ key: t.id, name: t.name, path: false, pages: sorted.filter((p) => p.typeId === t.id) }))
    .filter((l) => l.pages.length > 0)
  const known = new Set(types.map((t) => t.id))
  const rest = sorted.filter((p) => !known.has(p.typeId))
  return rest.length > 0 ? [...out, { key: 'other', name: 'Other pages', path: false, pages: rest }] : out
}

const STEPS: Array<[limit: number, size: number, unit: Intl.RelativeTimeFormatUnit]> = [
  [60, 1, 'minute'],
  [48 * 60, 60, 'hour'],
  [14 * 1440, 1440, 'day'],
  [9 * 7 * 1440, 7 * 1440, 'week'],
  [18 * 30 * 1440, 30 * 1440, 'month'],
  [Infinity, 365 * 1440, 'year'],
]

// One formatter for the board's every card, rather than one a card
const AGO = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function since(iso: string, now = new Date()): string {
  const minutes = Math.round((new Date(iso).getTime() - now.getTime()) / 60_000)
  if (Number.isNaN(minutes)) return ''
  const [, size, unit] = STEPS.find(([limit]) => Math.abs(minutes) < limit)!
  return AGO.format(Math.round(minutes / size), unit)
}
