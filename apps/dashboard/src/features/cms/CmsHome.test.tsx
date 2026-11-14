import { fireEvent, screen, within } from '@testing-library/react'
import type { MockLink } from '@apollo/client/testing'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { CmsHome } from './CmsHome'
import { CmsHomeBlogDocument, CmsHomeDocument, CmsHomeFormsDocument } from './home/CmsHome.gql'

type Row = { id: string; urlPath: string; title: string; type?: string; isPublished?: boolean; workflowState?: string; outline?: string[]; home?: string }

const row = ({ id, urlPath, title, type = 't1', isPublished = true, workflowState = 'PUBLISHED', outline = [], home }: Row) => ({
  __typename: 'CMSPage',
  id,
  pageTypeId: type,
  urlPath,
  isPublished,
  scheduledAt: null,
  workflowState,
  updatedAt: '2026-10-01T00:00:00Z',
  content: { __typename: 'CMSPageContent', locale: 'en', title, outline },
  channelListings: home ? [{ __typename: 'CMSPageChannelListing', channelId: home, isHomepage: true }] : [],
})

const ROWS = [
  row({ id: 'p1', urlPath: 'about', title: 'About us', outline: ['hero', 'rich_text'] }),
  row({ id: 'p2', urlPath: 'docs/start', title: 'Getting started', type: 't2', outline: ['product_grid', 'stamp'] }),
  row({ id: 'p3', urlPath: 'docs/faq', title: 'Questions', type: 't2', isPublished: false, workflowState: 'IN_REVIEW' }),
  row({ id: 'p4', urlPath: 'sale', title: 'Summer sale', isPublished: false, workflowState: 'DRAFT', outline: ['hero'] }),
]

const ids = (n: number, typename: string) => Array.from({ length: n }, (_, i) => ({ __typename: typename, id: `${typename}-${i}` }))
const pageType = (id: string, slug: string, name: string) => ({ __typename: 'CMSPageType', id, slug, content: { __typename: 'CMSPageTypeContent', locale: 'en', name } })

const site = (rows: object[], filter: object = { onlyPublished: false }, total = rows.length): MockLink.MockedResponse => ({
  request: { query: CmsHomeDocument, variables: { filter, limit: 500 } },
  result: {
    data: {
      cmsPageCount: total,
      cmsPages: rows,
      cmsPageTypes: [pageType('t1', 'landing', 'Landing page'), pageType('t2', 'doc', 'Help article')],
      cmsAttributes: ids(3, 'CMSAttribute'),
      cmsSnippets: [],
      cmsBlockTypes: [{ __typename: 'CMSBlockType', id: 'b1', key: 'stamp', name: 'Trust stamp' }],
      menus: ids(4, 'Menu'),
    },
  },
})

const blog: MockLink.MockedResponse = {
  request: { query: CmsHomeBlogDocument },
  result: { data: { blogPosts: { __typename: 'BlogPostList', totalCount: 7 } } },
}

const home = (mocks: MockLink.MockedResponse[], scope?: ConfigScopeContextValue) => renderPage({ path: '/cms', scope, routes: { '/cms': <CmsHome /> }, mocks })

const lane = (name: string) => screen.getByRole('region', { name })
const titles = (el: HTMLElement) => within(el).getAllByRole('link').map((a) => a.getAttribute('aria-label')?.split(',')[0] ?? a.textContent)

describe('CmsHome', () => {
  it('lays the site out by address, each page drawn by its blocks', async () => {
    home([meMock(), site(ROWS), blog])

    const about = await screen.findByRole('link', { name: 'About us, /about, live, 2 blocks' }, SLOW)
    expect(about.getAttribute('href')).toBe('/cms/pages/p1')
    expect(within(about).getByText('Hero')).toBeDefined()
    expect(within(about).getByText('Rich text')).toBeDefined()
    expect(titles(lane('Top of the site'))).toEqual(['About us', 'Summer sale', 'Add a page'])
    expect(titles(lane('/docs'))).toEqual(['Questions', 'Getting started'])

    const start = screen.getByRole('link', { name: 'Getting started, /docs/start, live, 2 blocks' })
    expect(within(start).getByText('Trust stamp')).toBeDefined()
    expect(within(screen.getByRole('link', { name: 'Questions, /docs/faq, in review, 0 blocks' })).getByText('No blocks')).toBeDefined()
  })

  it('narrows the board to a state or to what was typed, and says when nothing is left', async () => {
    home([meMock(), site(ROWS), blog])

    const all = await screen.findByRole('radio', { name: /^All pages/ }, SLOW)
    expect(all.textContent).toContain('4')
    expect(screen.getByRole('radio', { name: /^Live/ }).textContent).toContain('2')
    expect(screen.queryByRole('radio', { name: /^Scheduled/ })).toBeNull()

    fireEvent.click(screen.getByRole('radio', { name: /^Draft/ }))
    expect(titles(screen.getByRole('region', { name: 'Site map' }))).toEqual(['Summer sale'])

    fireEvent.click(all)
    fireEvent.change(screen.getByRole('searchbox', { name: 'Find a page' }), { target: { value: '/docs' } })
    expect(titles(lane('/docs'))).toEqual(['Questions', 'Getting started'])
    expect(screen.queryByRole('region', { name: 'Top of the site' })).toBeNull()

    fireEvent.change(screen.getByRole('searchbox', { name: 'Find a page' }), { target: { value: 'nothing like this' } })
    expect(screen.getByText('No page matches that.')).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Show every page' }))
    expect(titles(lane('Top of the site'))).toContain('About us')
  })

  it('arranges the pages by page type instead', async () => {
    home([meMock(), site(ROWS), blog])

    fireEvent.click(await screen.findByRole('radio', { name: 'By page type' }, SLOW))
    expect(titles(lane('Landing page'))).toEqual(['About us', 'Summer sale', 'Add a page'])
    expect(titles(lane('Help article'))).toEqual(['Questions', 'Getting started'])
    expect(screen.queryByRole('region', { name: '/docs' })).toBeNull()
  })

  it('opens a long lane on its first dozen pages, and the rest on request', async () => {
    const many = Array.from({ length: 14 }, (_, i) => row({ id: `d${i}`, urlPath: `docs/page-${String(i).padStart(2, '0')}`, title: `Doc ${i}` }))
    home([meMock(), site(many), blog])

    await screen.findByRole('link', { name: /^Doc 0,/ }, SLOW)
    const drawn = () => within(lane('/docs')).getAllByRole('link', { name: /^Doc / }).length
    expect(drawn()).toBe(12)
    fireEvent.click(within(lane('/docs')).getByRole('button', { name: 'Show all 14 in /docs' }))
    expect(drawn()).toBe(14)
    expect(within(lane('/docs')).queryByRole('button', { name: /Show all/ })).toBeNull()
  })

  it('says so when the site has more pages than it was handed, and draws their states as one bar', async () => {
    const full = Array.from({ length: 500 }, (_, i) => row({ id: `f${i}`, urlPath: `p${i}`, title: `Page ${i}`, isPublished: i % 2 === 0, workflowState: i % 2 === 0 ? 'PUBLISHED' : 'DRAFT' }))
    home([meMock(), site(full, undefined, 1240), blog])

    const pages = await screen.findByRole('link', { name: 'Pages, 1,240' }, SLOW)
    const bar = within(pages).getByRole('img', { name: '250 live, 250 draft' })
    expect(bar.children.length).toBe(2)
    expect(within(pages).getByText('of the first 500')).toBeDefined()
    expect(within(screen.getByRole('region', { name: 'Site map' })).getByText(/These are the first 500 of 1,240 pages/)).toBeDefined()
  })

  it('picks out one kind of block across the board', async () => {
    home([meMock(), site(ROWS), blog])

    const kind = await screen.findByRole('button', { name: 'Catalog & blog' }, SLOW)
    const dimmed = () => screen.getByText('Rich text').parentElement?.className.includes('opacity-30')
    expect(dimmed()).toBe(false)
    fireEvent.click(kind)
    expect(kind.getAttribute('aria-pressed')).toBe('true')
    expect(dimmed()).toBe(true)
    expect(screen.getByText('Product grid').parentElement?.className.includes('opacity-30')).toBe(false)
    fireEvent.click(kind)
    expect(dimmed()).toBe(false)
  })

  it('counts what sits behind each tool', async () => {
    home([meMock(), site(ROWS), blog])

    const tools = await screen.findByRole('navigation', { name: 'Everything in the CMS' }, SLOW)
    const tool = (name: string) => within(tools).findByRole('link', { name }, SLOW)
    const pages = await tool('Pages, 4')
    expect(pages.getAttribute('href')).toBe('/cms/pages')
    expect(within(pages).getByRole('img', { name: '2 live, 1 draft, 1 in review' })).toBeDefined()
    expect((await tool('Blog posts, 7')).getAttribute('href')).toBe('/blog/posts')
    expect((await tool('Page types, 2')).getAttribute('href')).toBe('/cms/page-types')
    expect((await tool('Menus, 4')).getAttribute('href')).toBe('/menus')
    expect((await tool('Forms')).getAttribute('href')).toBe('/cms/forms')
    expect(within(await tool('Page types, 2')).getByText('For pages')).toBeDefined()
    expect(within(await tool('Blog posts, 7')).getByText('For the blog')).toBeDefined()
    expect(within(await tool('Categories, tags and authors')).getByText('For the blog')).toBeDefined()
    expect(within(await tool('Menus, 4')).getByText('For the storefront')).toBeDefined()
    expect(tools.compareDocumentPosition(screen.getByRole('region', { name: 'Site map' })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(screen.getByRole('link', { name: 'New page' }).getAttribute('href')).toBe('/cms/pages/new')
    fireEvent.click(screen.getByRole('button', { name: 'New post' }))
    expect(await within(await screen.findByRole('dialog')).findByLabelText('Title')).toBeDefined()
  })

  it('says a count could not be had, and the box still opens', async () => {
    home([meMock(), site(ROWS), { request: { query: CmsHomeBlogDocument }, error: new Error('the blog is away') }])

    const posts = await screen.findByRole('link', { name: 'Blog posts, count unavailable' }, SLOW)
    expect(posts.getAttribute('href')).toBe('/blog/posts')
    expect(screen.getByRole('link', { name: 'Page types, 2' })).toBeDefined()
  })

  it('asks for the scoped store only, puts its homepage first at the root and counts its forms', async () => {
    const store = { id: 'ch1', name: 'Demo Store', defaultLocale: 'en', locales: [{ code: 'en' }], isActive: true }
    const inStore: ConfigScopeContextValue = { ...globalScope, scope: { mode: 'CHANNEL', channelId: 'ch1', locale: '' }, channels: [store], selectedChannel: store, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1' }
    const forms = { request: { query: CmsHomeFormsDocument, variables: { channelId: 'ch1' } }, result: { data: { webForms: ids(2, 'WebForm') } } }
    home([meMock(), site([ROWS[0]!, row({ id: 'h', urlPath: 'docs/home', title: 'Home', home: 'ch1' })], { onlyPublished: false, channelId: 'ch1' }), blog, forms], inStore)

    const front = await screen.findByRole('link', { name: 'Home, /, live, 0 blocks' }, SLOW)
    expect(front.getAttribute('href')).toBe('/cms/pages/h')
    expect(titles(lane('Top of the site'))).toEqual(['Home', 'About us', 'Add a page'])
    expect((await screen.findByRole('link', { name: 'Forms, 2' }, SLOW)).getAttribute('href')).toBe('/cms/forms')
  })

  it('leaves out what the admin may not open', async () => {
    home([meMock({ superuser: false, permissions: ['cms.view'] }), site(ROWS)])

    await screen.findByRole('link', { name: /^About us,/ }, SLOW)
    expect(screen.getByRole('link', { name: 'Page types, 2' })).toBeDefined()
    expect(screen.queryByRole('link', { name: /^Blog posts/ })).toBeNull()
    expect(screen.queryByRole('link', { name: /^Categories/ })).toBeNull()
    expect(screen.queryByRole('link', { name: /^Forms/ })).toBeNull()
    expect(screen.queryByRole('link', { name: 'New page' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Add a page' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'New post' })).toBeNull()
  })

  it('points an empty site at where to start', async () => {
    home([meMock(), site([]), blog])

    expect(await screen.findByText('No pages yet', {}, SLOW)).toBeDefined()
    expect(within(screen.getByRole('region', { name: 'Site map' })).getByRole('link', { name: 'Page types' }).getAttribute('href')).toBe('/cms/page-types')
    expect(screen.getByRole('link', { name: 'Pages, 0' })).toBeDefined()
    expect(within(screen.getByRole('region', { name: 'Site map' })).getByRole('link', { name: 'New page' }).getAttribute('href')).toBe('/cms/pages/new')
  })

  it('says why when the pages fail to load, and keeps the tools within reach', async () => {
    home([meMock(), { request: { query: CmsHomeDocument, variables: { filter: { onlyPublished: false }, limit: 500 } }, error: new Error('the database is away') }, blog])

    expect(await screen.findByText("The pages couldn't be loaded", {}, SLOW)).toBeDefined()
    expect(screen.getByText('the database is away')).toBeDefined()
    expect(screen.queryByRole('status', { name: 'Counting the pages' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Try again' })).toBeDefined()
    expect(screen.getByRole('link', { name: 'Page types' }).getAttribute('href')).toBe('/cms/page-types')
    expect(screen.getByRole('link', { name: 'Pages' }).getAttribute('href')).toBe('/cms/pages')
  })
})
