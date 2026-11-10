import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { HybridSearch } from './HybridSearch'
import { EmbeddingCoverageDocument, SearchOverviewPreviewDocument, SearchPreviewDocument, StorefrontSearchPreviewDocument } from './HybridSearch.gql'

const store = {
  ...globalScope,
  scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' },
  scopeInput: { kind: 'CHANNEL' as const, channelId: 'ch1' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'ch1',
  selectedChannel: { id: 'ch1', name: 'Main' },
  effectiveLocale: 'en',
  availableLocales: ['en'],
} as typeof globalScope

const option = ([value, label]: [string, string]) => ({ __typename: 'ConfigOption' as const, label, value })

const field = (path: string, label: string, value: string, options: [string, string][]) => ({
  __typename: 'ConfigField' as const,
  path,
  section: 'search',
  group: 'products',
  field: path.split('/').pop(),
  label,
  description: '',
  type: 'SELECT',
  value,
  isSet: false,
  maxLength: null,
  options: options.map(option),
  isDefault: true,
  public: false,
  scoping: 'CHANNEL' as const,
  scope: { __typename: 'ConfigScope' as const, kind: 'DEFAULT' as const, channelId: null, locale: null },
  updatedAt: null,
})

const sections = {
  request: { query: ConfigSectionsDocument, variables: { scope: { kind: 'CHANNEL', channelId: 'ch1' } } },
  maxUsageCount: 3,
  result: {
    data: {
      configSections: [
        {
          __typename: 'ConfigSection' as const,
          slug: 'search',
          label: 'Search',
          groups: [
            {
              __typename: 'ConfigGroup' as const,
              slug: 'products',
              label: 'Product Search',
              fields: [
                field('search/products/meaning', 'Search by meaning', 'natural', [
                  ['off', 'Off - words only'],
                  ['natural', 'For sentences, and when words find nothing'],
                  ['always', 'Every search'],
                ]),
                field('search/products/meaning_strictness', 'Search by meaning strictness', 'balanced', [
                  ['strict', 'Strict'],
                  ['balanced', 'Balanced'],
                  ['broad', 'Broad'],
                ]),
                field('search/products/ai_overview', 'AI overview', 'off', [
                  ['off', 'Off'],
                  ['meaning', 'When meaning answers'],
                ]),
              ],
            },
          ],
        },
      ],
    },
  },
}

// The same sections with the store's AI overview on
const overviewOn = structuredClone(sections)
overviewOn.result.data.configSections[0].groups[0].fields[2].value = 'meaning'

const coverage = {
  request: { query: EmbeddingCoverageDocument, variables: { channelId: 'ch1' } },
  maxUsageCount: 3,
  result: {
    data: { embeddingCoverage: { __typename: 'EmbeddingCoverage', model: 'voyage-3.5', products: 10, embeddedProducts: 10, pages: 2, embeddedPages: 2, queued: 0, failed: '' } },
  },
}

const product = (id: string, name: string) => ({ __typename: 'SearchPreviewProduct' as const, id, name, sku: id.toUpperCase(), url: `https://shop.example/${id}`, price: 9800, currencyCode: 'USD', inStock: true })

const preview = (strictness: 'BALANCED' | 'STRICT' | null, total: number, mode: 'NATURAL' | null = 'NATURAL') => ({
  request: {
    query: StorefrontSearchPreviewDocument,
    variables: { channelId: 'ch1', query: 'an oversized shirt for summer', locale: 'en', mode, strictness },
  },
  result: {
    data: {
      storefrontSearchPreview: {
        __typename: 'StorefrontSearchPreview',
        reads: 'SENTENCE',
        answeredBy: 'MEANING',
        margin: strictness === 'STRICT' ? 0.08 : 0.15,
        total,
        wordsTotal: 3,
        meaningFailed: '',
        listed: [{ __typename: 'SearchPreviewShown', byWords: false, byMeaning: true, product: product('pop', 'Poplin Overshirt') }],
        byWords: [product('jns', 'High-Rise Wide Jeans')],
        byMeaning: [
          { __typename: 'SearchPreviewNear', distance: 0.385, counted: true, product: product('pop', 'Poplin Overshirt') },
          { __typename: 'SearchPreviewNear', distance: 0.61, counted: false, product: product('jns', 'High-Rise Wide Jeans') },
        ],
      },
    },
  },
})

const assistantPreview = (strictness: 'BALANCED' | 'STRICT', over: object = {}, query = 'an oversized shirt for summer') => ({
  request: { query: SearchPreviewDocument, variables: { channelId: 'ch1', query, locale: 'en', mode: 'NATURAL', strictness } },
  result: {
    data: {
      searchPreview: {
        __typename: 'SearchPreview',
        locales: ['en'],
        wordsTotal: 1,
        byWords: [product('wsk', 'Warm socks')],
        byMeaning: [
          { __typename: 'SearchPreviewNear', distance: 0.2134, counted: true, product: { ...product('dj', 'Down jacket'), inStock: false } },
          { __typename: 'SearchPreviewNear', distance: 0.71, counted: false, product: product('bt', 'Beach towel') },
        ],
        shown: [
          { __typename: 'SearchPreviewShown', byWords: true, byMeaning: false, product: product('wsk', 'Warm socks') },
          { __typename: 'SearchPreviewShown', byWords: false, byMeaning: true, product: { ...product('dj', 'Down jacket'), inStock: false } },
        ],
        productsText: `${strictness === 'STRICT' ? 1 : 2} products match:\n- Warm socks`,
        pagesByWords: [],
        passages: [{ __typename: 'SearchPreviewPassage', title: 'Care guide', url: 'https://shop.example/care', text: 'Wool keeps you warm when wet.', distance: 0.3, counted: true }],
        pagesText: 'No store page mentions that.',
        meaningFailed: '',
        ...over,
      },
    },
  },
})

const QUERY = 'an oversized shirt for summer'

const asked = vi.fn()
const overview = (status: 'WRITTEN' | 'KEYWORDS' | 'UNAVAILABLE' | 'NOTHING', text = '') => ({
  request: {
    query: SearchOverviewPreviewDocument,
    variables: (vars: Record<string, unknown>) => {
      asked(vars)
      return true
    },
  },
  result: {
    data: {
      searchOverviewPreview: {
        __typename: 'SearchOverviewPreview',
        status,
        text,
        products: [
          { __typename: 'SearchOverviewPreviewProduct', id: 'pop', name: 'Poplin Overshirt', url: 'https://shop.example/poplin-overshirt' },
          { __typename: 'SearchOverviewPreviewProduct', id: 'lin', name: 'Linen Camp Shirt', url: 'https://shop.example/linen-camp-shirt' },
        ],
      },
    },
  },
})

const searchFor = async (query = QUERY) => {
  fireEvent.change(await screen.findByLabelText('What a shopper types', {}, SLOW), { target: { value: query } })
  fireEvent.click(screen.getByRole('button', { name: 'Search' }))
}

const writes = vi.fn()
const save = {
  request: {
    query: SetConfigValueDocument,
    variables: (vars: Record<string, unknown>) => {
      writes(vars)
      return true
    },
  },
  result: {
    data: {
      setConfigValue: {
        __typename: 'ConfigValue',
        path: 'search/products/meaning_strictness',
        value: 'strict',
        isDefault: false,
        scope: { __typename: 'ConfigScope', kind: 'CHANNEL', channelId: 'ch1', locale: null },
        updatedAt: null,
      },
    },
  },
}

afterEach(() => {
  localStorage.clear()
  writes.mockReset()
  asked.mockReset()
})

describe('HybridSearch', () => {
  it('tries a search with the choices as they stand, and follows a change before it is saved', async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage, preview('BALANCED', 39), preview('STRICT', 14), save],
    })

    expect(await screen.findByText('Keywords by their words, sentences by their meaning', {}, SLOW)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('What a shopper types'), { target: { value: '  an oversized shirt for summer ' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText(/lists the 39 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
    expect(screen.getByText('By meaning - counts within 0.15 of the closest')).toBeInTheDocument()
    expect(screen.getByText('Too far')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('radio', { name: 'Strict' }))
    expect(await screen.findByText(/lists the 14 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
    expect(writes).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(writes).toHaveBeenCalledTimes(1), SLOW)
    expect(writes).toHaveBeenCalledWith(expect.objectContaining({ path: 'search/products/meaning_strictness', value: 'strict', scope: { kind: 'CHANNEL', channelId: 'ch1' } }))
  })

  it("opens on the assistant from the old preview's address, and shows what each search found and what the model is handed", async () => {
    renderPage({
      path: '/agentic/hybrid-search?tab=assistant',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage, assistantPreview('BALANCED')],
    })
    fireEvent.change(await screen.findByLabelText('What a shopper types', {}, SLOW), { target: { value: 'an oversized shirt for summer' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))

    const section = (await screen.findByRole('heading', { name: /What the assistant is shown/ }, SLOW)).closest('section')
    const rows = within(section ?? document.body).getAllByRole('listitem')
    expect(rows.map((r) => r.textContent)).toEqual([expect.stringMatching(/^1Warm socks.*Words$/), expect.stringMatching(/^2Down jacket.*Out of stock.*Meaning$/)])
    expect(screen.getByText('0.213')).toBeInTheDocument()
    expect(screen.getAllByText('Counts')).toHaveLength(2)
    expect(screen.getByText('Too far')).toBeInTheDocument()
    expect(screen.getByText('Wool keeps you warm when wet.')).toBeInTheDocument()
    expect(screen.queryByText('What the shopper sees')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'search_products' }))
    expect(screen.getByText(/2 products match/)).toBeInTheDocument()
  })

  it('runs both tabs once opened, and an unsaved reach reaches the assistant too', async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage, preview('BALANCED', 39), assistantPreview('BALANCED'), preview('STRICT', 14), assistantPreview('STRICT')],
    })
    fireEvent.change(await screen.findByLabelText('What a shopper types', {}, SLOW), { target: { value: 'an oversized shirt for summer' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText(/lists the 39 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
    expect(screen.getByText(/A shopper scans a grid, so it keeps to close matches/)).toBeInTheDocument()

    fireEvent.mouseDown(screen.getByRole('tab', { name: 'Assistant' }), { button: 0, ctrlKey: false })
    expect(screen.getByText(/Words and meaning are always ranked together, as equals, on one short page/)).toBeInTheDocument()
    fireEvent.click(await screen.findByRole('button', { name: 'search_products' }, SLOW))
    expect(screen.getByText(/2 products match/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('radio', { name: 'Strict' }))
    expect(await screen.findByText(/1 products match/, {}, SLOW)).toBeInTheDocument()

    fireEvent.mouseDown(screen.getByRole('tab', { name: 'Search page' }), { button: 0, ctrlKey: false })
    expect(await screen.findByText(/lists the 14 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
  })

  it('says when meaning failed and the assistant answered on its words alone', async () => {
    const noModel = structuredClone(coverage)
    noModel.result.data.embeddingCoverage.model = ''
    renderPage({
      path: '/agentic/hybrid-search?tab=assistant',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, noModel, assistantPreview('BALANCED', { meaningFailed: 'ai: embeddings 401 from api.example' })],
    })
    expect(await screen.findByRole('link', { name: 'Settings, AI, Embeddings' }, SLOW)).toHaveAttribute('href', '/settings/ai/embeddings')
    fireEvent.change(screen.getByLabelText('What a shopper types'), { target: { value: 'an oversized shirt for summer' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText('ai: embeddings 401 from api.example', {}, SLOW)).toBeInTheDocument()
    expect(screen.getAllByText('Search by meaning failed.')).toHaveLength(2)
  })

  it('leaves the reach alone while meaning is off', async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage],
    })
    fireEvent.click(await screen.findByRole('radio', { name: 'Off - words only' }, SLOW))
    expect(screen.getByText('Every search by its words alone')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: 'Strict' })).toBeDisabled()
    expect(screen.getByRole('radio', { name: 'When meaning answers' })).toBeDisabled()
    expect(screen.getByText('Unsaved')).toBeInTheDocument()
  })

  it('searches again when the same query is asked twice', async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage, preview('BALANCED', 39), preview('BALANCED', 41)],
    })
    const box = await screen.findByLabelText('What a shopper types', {}, SLOW)
    fireEvent.change(box, { target: { value: 'an oversized shirt for summer' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText(/lists the 39 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
    await vi.waitFor(() => expect(screen.getByRole('button', { name: 'Search' })).toBeEnabled(), SLOW)
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText(/lists the 41 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
  })

  it("lets a catalog-only admin try the store's saved search, and says changing it needs settings access", async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock({ superuser: false, permissions: ['catalog.view'] }), coverage, preview(null, 12, null)],
    })
    expect(await screen.findByText(/needs access to settings on this store/, {}, SLOW)).toBeInTheDocument()
    expect(screen.queryByText(/Couldn't load the search settings/)).toBeNull()
    expect(screen.queryByRole('radiogroup')).toBeNull()
    fireEvent.change(screen.getByLabelText('What a shopper types'), { target: { value: 'an oversized shirt for summer' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(await screen.findByText(/lists the 12 products closest in meaning/, {}, SLOW)).toBeInTheDocument()
    expect(screen.getByText(/Previewing an overview needs permission to manage settings on this store/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Preview the AI overview' })).toBeNull()
    expect(asked).not.toHaveBeenCalled()
  })

  it('shows a read-only admin the choices without a way to reset them', async () => {
    const own = structuredClone(sections)
    const reach = own.result.data.configSections[0].groups[0].fields[1]
    Object.assign(reach, { isDefault: false, scope: { __typename: 'ConfigScope', kind: 'CHANNEL', channelId: 'ch1', locale: null } })
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock({ superuser: false, permissions: ['catalog.view', 'config.view'] }), own, coverage],
    })
    expect(await screen.findByText('Set for this store', {}, SLOW)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^Reset/ })).toBeNull()
    expect(screen.getByRole('radio', { name: 'Strict' })).toBeDisabled()
    expect(screen.getByText(/changing them needs permission to manage settings/)).toBeInTheDocument()
  })

  it('stages the AI overview, writes one with it before it is saved, and saves it through the dialog', async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage, preview('BALANCED', 39), overview('WRITTEN', 'Light and breezy: [[1]] works over anything.'), save],
    })
    const on = await screen.findByRole('radio', { name: 'When meaning answers' }, SLOW)
    expect(screen.getByText(/The search page shows its results with nothing written above them/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Settings, Sales, AI shopping agents' })).toHaveAttribute('href', '/settings/sales/agents')
    expect(screen.getByRole('link', { name: 'its voice' })).toHaveAttribute('href', '/agentic/voice')
    fireEvent.click(on)
    expect(screen.getByText(/a few sentences from the products listed sit above them/)).toBeInTheDocument()
    expect(screen.getByText('Unsaved')).toBeInTheDocument()

    await searchFor()
    const section = (await screen.findByRole('heading', { name: /AI overview/ }, SLOW)).closest('section')!
    expect(await within(section).findByRole('link', { name: 'Poplin Overshirt' }, SLOW)).toHaveAttribute('href', 'https://shop.example/poplin-overshirt')
    expect(asked).toHaveBeenCalledWith({ channelId: 'ch1', query: QUERY, locale: 'en', mode: 'NATURAL', strictness: 'BALANCED' })
    expect(writes).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText(/Search · AI overview/)).toBeInTheDocument()
    expect(within(dialog).getByText('When meaning answers')).toBeInTheDocument()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(writes).toHaveBeenCalledTimes(1), SLOW)
    expect(writes).toHaveBeenCalledWith(expect.objectContaining({ path: 'search/products/ai_overview', value: 'meaning', scope: { kind: 'CHANNEL', channelId: 'ch1' } }))
  })

  it("writes the overview on its own while it's on, above what the shopper sees, each product it names a link", async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), overviewOn, coverage, preview('BALANCED', 39), overview('WRITTEN', 'For summer, [[1]] keeps it loose and [[2]] breathes. [[7]]Both pack light.')],
    })
    await searchFor()
    const heading = await screen.findByRole('heading', { name: /AI overview/ }, SLOW)
    const section = heading.closest('section')!
    const shirt = await within(section).findByRole('link', { name: 'Poplin Overshirt' }, SLOW)
    expect(shirt).toHaveAttribute('target', '_blank')
    expect(within(section).getByRole('link', { name: 'Linen Camp Shirt' })).toHaveAttribute('href', 'https://shop.example/linen-camp-shirt')
    expect(shirt.closest('p')).toHaveTextContent('For summer, Poplin Overshirt keeps it loose and Linen Camp Shirt breathes. Both pack light.')
    expect(within(section).getByText(/Written above these results while it's on/)).toBeInTheDocument()
    // busy until the last word is in, so a screen reader reads it once, whole
    await vi.waitFor(() => expect(shirt.closest('[aria-live]')).toHaveAttribute('aria-busy', 'false'), SLOW)
    expect(screen.queryByRole('button', { name: 'Preview the AI overview' })).toBeNull()
    const order = screen.getAllByRole('heading').map((h) => h.textContent)
    expect(order.findIndex((t) => t?.includes('AI overview'))).toBeLessThan(order.findIndex((t) => t?.includes('What the shopper sees')))
  })

  it("waits to be asked while it's off, since writing one spends the AI budget, and says when the assistant has to be on first", async () => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), sections, coverage, preview('BALANCED', 39), overview('UNAVAILABLE')],
    })
    await searchFor()
    const ask = await screen.findByRole('button', { name: 'Preview the AI overview' }, SLOW)
    expect(screen.getByText(/Off for this store, so the search page writes nothing above these results/)).toBeInTheDocument()
    expect(asked).not.toHaveBeenCalled()

    fireEvent.click(ask)
    expect(await screen.findByText(/isn't answering on this store/, {}, SLOW)).toBeInTheDocument()
    expect(asked).toHaveBeenCalledTimes(1)
    const section = screen.getByRole('heading', { name: /AI overview/ }).closest('section')!
    expect(within(section).getByRole('link', { name: 'Settings, Sales, AI shopping agents' })).toHaveAttribute('href', '/settings/sales/agents')
    expect(within(section).getByRole('link', { name: 'Settings, AI, LLM provider' })).toHaveAttribute('href', '/settings/ai/llm')
  })

  it.each([
    ['KEYWORDS', 'The words answered this search, so the search page writes nothing above it.'],
    ['NOTHING', /the model had nothing to say about them, or the store's AI budget for today is spent/],
  ] as const)('says why there is no overview when it comes back %s', async (status, says) => {
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), overviewOn, coverage, preview('BALANCED', 39), overview(status)],
    })
    await searchFor()
    expect(await screen.findByText(says, {}, SLOW)).toBeInTheDocument()
  })

  it('asks for no overview when the words answered the search', async () => {
    const byWords = preview('BALANCED', 3)
    Object.assign(byWords.result.data.storefrontSearchPreview, { reads: 'KEYWORDS', answeredBy: 'WORDS' })
    renderPage({
      path: '/agentic/hybrid-search',
      adminId: 'admin-1',
      scope: store,
      routes: { '/agentic/hybrid-search': <HybridSearch /> },
      mocks: [meMock(), overviewOn, coverage, byWords],
    })
    await searchFor()
    expect(await screen.findByText(/Read as keywords, and they match 3 products/, {}, SLOW)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /AI overview/ })).toBeNull()
    expect(asked).not.toHaveBeenCalled()
  })
})
