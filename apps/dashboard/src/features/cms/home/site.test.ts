import { describe, expect, it } from 'vitest'
import { addressOf, lanes, lanesByType, matches, since, stateOf, tally, type SitePage } from './site'

const page = (urlPath: string, over: Partial<SitePage> = {}): SitePage => ({
  id: urlPath,
  urlPath,
  title: urlPath,
  isPublished: true,
  scheduledAt: null,
  workflowState: 'PUBLISHED',
  updatedAt: '2026-11-01T00:00:00Z',
  home: false,
  typeId: 't1',
  outline: [],
  ...over,
})

describe('page state', () => {
  it('reads published first, then a schedule, then the workflow', () => {
    expect(stateOf(page('a'))).toBe('live')
    expect(stateOf(page('a', { workflowState: 'APPROVED' }))).toBe('live')
    expect(stateOf(page('a', { isPublished: false, scheduledAt: '2031-01-01T00:00:00Z', workflowState: 'IN_REVIEW' }))).toBe('scheduled')
    expect(stateOf(page('a', { isPublished: false, workflowState: 'IN_REVIEW' }))).toBe('review')
    expect(stateOf(page('a', { isPublished: false, workflowState: 'APPROVED' }))).toBe('approved')
    expect(stateOf(page('a', { isPublished: false, workflowState: 'DRAFT' }))).toBe('draft')
  })

  it('counts the pages in each state', () => {
    const pages = [page('a'), page('b'), page('c', { isPublished: false, workflowState: 'DRAFT' }), page('d', { isPublished: false, workflowState: 'IN_REVIEW' })]
    expect(tally(pages)).toEqual({ live: 2, draft: 1, review: 1, approved: 0, scheduled: 0 })
  })
})

describe('narrowing the board', () => {
  const sale = page('sale/summer', { title: 'Summer Sale', isPublished: false, workflowState: 'DRAFT' })

  it('matches on state, and on the title or the address whatever the case', () => {
    expect(matches(sale, 'all', '')).toBe(true)
    expect(matches(sale, 'draft', '')).toBe(true)
    expect(matches(sale, 'live', '')).toBe(false)
    expect(matches(sale, 'all', ' summer s ')).toBe(true)
    expect(matches(sale, 'all', '/SALE/')).toBe(true)
    expect(matches(sale, 'all', 'winter')).toBe(false)
    expect(matches(sale, 'live', 'summer')).toBe(false)
  })

  it('shows the homepage at the root, and still finds it by the path it was saved under', () => {
    const front = page('docs/home', { title: 'Welcome', home: true })
    expect(addressOf(front)).toBe('/')
    expect(matches(front, 'all', 'docs/home')).toBe(true)
    expect(matches(front, 'all', 'shop')).toBe(false)
  })
})

describe('where the pages sit', () => {
  const pages = [page('docs/b'), page('about'), page('docs'), page('home', { home: true }), page('docs/a'), page('contact'), page('blog/x/y')]
  const drawn = (list: ReturnType<typeof lanes>) => list.map((l) => `${l.name}${l.path ? ' (address)' : ''}: ${l.pages.map((p) => p.urlPath).join(' ')}`)

  it('leads with the top of the site, homepage first, then a lane for each folder', () => {
    expect(drawn(lanes(pages))).toEqual(['Top of the site: home about contact', '/blog (address): blog/x/y', '/docs (address): docs docs/a docs/b'])
  })

  it('has no top lane when every page is in a folder, and none at all for an empty site', () => {
    expect(drawn(lanes([page('docs/a')]))).toEqual(['/docs (address): docs/a'])
    expect(lanes([])).toEqual([])
  })

  it('arranges by page type in the types own order, leaving out a type with no pages', () => {
    const types = [{ id: 't2', name: 'Landing page' }, { id: 't1', name: 'Article' }, { id: 't3', name: 'Unused' }]
    const typed = [page('b'), page('promo', { typeId: 't2' }), page('a'), page('stray', { typeId: 'gone' })]
    expect(drawn(lanesByType(typed, types))).toEqual(['Landing page: promo', 'Article: a b', 'Other pages: stray'])
  })
})

describe('since', () => {
  const now = new Date('2026-11-12T12:00:00Z')
  const said = (iso: string) => since(iso, now)
  const say = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

  it('picks the unit that keeps the number small', () => {
    expect(said('2026-11-12T11:40:00Z')).toBe(say.format(-20, 'minute'))
    expect(said('2026-11-12T07:00:00Z')).toBe(say.format(-5, 'hour'))
    expect(said('2026-11-06T12:00:00Z')).toBe(say.format(-6, 'day'))
    expect(said('2026-09-28T12:00:00Z')).toBe(say.format(-6, 'week'))
    expect(said('2026-06-27T12:00:00Z')).toBe(say.format(-5, 'month'))
    expect(said('2023-11-12T12:00:00Z')).toBe(say.format(-3, 'year'))
  })

  it('says nothing for a date it cannot read', () => {
    expect(said('not a date')).toBe('')
  })
})
