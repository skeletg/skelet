import { act, fireEvent, screen } from '@testing-library/react'
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, renderPage } from '@/test/page-harness'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { AdminSearchDocument } from './AdminSearch.gql'

// What these pin: ⌘K finds records the server matched - an order by a number its row doesn't spell the way the
// palette's own filter would - never shows an earlier term's rows under what is typed now, says when the lookup failed,
// asks nothing under three characters, and starts empty however it was closed
vi.mock('@/nav/use-visible-pages', () => ({ useVisiblePages: () => ({ pages: [{ path: '/orders', title: 'All orders' }], loading: false }) }))
vi.mock('@/shared/auth/auth', () => ({ useAuth: () => ({ auth: null }) }))

const { AdminSearch } = await import('./admin-search')

// cmdk scrolls the highlighted row into view, which jsdom doesn't implement
const scrollIntoView = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, writable: true, value: vi.fn() })
})
afterAll(() => {
  if (scrollIntoView) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', scrollIntoView)
  else delete (HTMLElement.prototype as { scrollIntoView?: unknown }).scrollIntoView
})

type Found = {
  orders?: Array<{ id: string; number: string; email: string; name: string }>
  customers?: Array<{ id: string; email: string; name: string }>
  products?: Array<{ id: string; sku: string; name: string }>
}

function found(term: string, f: Found, extra: Partial<MockLink.MockedResponse> = {}): MockLink.MockedResponse {
  return {
    request: { query: AdminSearchDocument, variables: { term } },
    result: {
      data: {
        adminSearch: {
          __typename: 'AdminSearchResult',
          orders: (f.orders ?? []).map((o) => ({ __typename: 'AdminSearchOrder', ...o })),
          customers: (f.customers ?? []).map((c) => ({ __typename: 'AdminSearchCustomer', ...c })),
          products: (f.products ?? []).map((p) => ({ __typename: 'AdminSearchProduct', ...p })),
        },
      },
    },
    ...extra,
  }
}

const sections: MockLink.MockedResponse = {
  request: { query: ConfigSectionsDocument, variables: () => true },
  result: { data: { configSections: [] } },
  maxUsageCount: 10,
}

async function openAndType(mocks: MockLink.MockedResponse[], text: string) {
  renderPage({ path: '/', routes: { '/': <AdminSearch />, '/orders/:id': <p>order page</p> }, mocks: [sections, ...mocks] })
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
  const input = await screen.findByPlaceholderText(/Search orders, customers, products and pages/)
  fireEvent.change(input, { target: { value: text } })
  return input as HTMLInputElement
}

describe('admin search', () => {
  it('finds an order by its number and opens it', async () => {
    await openAndType([found('ORD-00310', { orders: [{ id: 'o1', number: 'ORD-003103', email: 'ada@example.com', name: 'Ada Astro' }] })], 'ORD-00310')
    expect(await screen.findByText('ORD-003103', {}, SLOW)).toBeDefined()
    expect(screen.getByText('Ada Astro')).toBeDefined()
    fireEvent.click(screen.getByText('ORD-003103'))
    expect(await screen.findByText('order page', {}, SLOW)).toBeDefined()
  })

  it('names a customer by email and a product by SKU when they have no name', async () => {
    await openAndType(
      [found('nameless', { customers: [{ id: 'c1', email: 'x@shop.test', name: '' }], products: [{ id: 'p1', sku: 'SKU-NAMELESS', name: '' }] })],
      'nameless',
    )
    expect(await screen.findByText('x@shop.test', {}, SLOW)).toBeDefined()
    expect(screen.getByText('SKU-NAMELESS')).toBeDefined()
  })

  it('says when records could not be searched', async () => {
    await openAndType([{ request: { query: AdminSearchDocument, variables: { term: 'broken' } }, error: new Error('network down') }], 'broken')
    expect((await screen.findByRole('alert', {}, SLOW)).textContent).toMatch(/network down/)
  })

  it('asks nothing under three characters', async () => {
    const asked = vi.fn(() => true)
    await openAndType([{ request: { query: AdminSearchDocument, variables: asked }, result: { data: { adminSearch: null } } }], 'OR')
    await act(() => new Promise((r) => setTimeout(r, 400)))
    expect(asked).not.toHaveBeenCalled()
  })

  it("never shows an earlier term's rows under what is typed now", async () => {
    const input = await openAndType(
      [
        found('ada', { orders: [{ id: 'oa', number: 'ORD-ADA', email: 'ada@example.com', name: 'Ada' }] }),
        found('bob', { orders: [{ id: 'ob', number: 'ORD-BOB', email: 'bob@example.com', name: 'Bob' }] }, { delay: 600 }),
      ],
      'ada',
    )
    expect(await screen.findByText('ORD-ADA', {}, SLOW)).toBeDefined()
    // closed and reopened, then a new term pasted - until its answer lands there is nothing for Enter to open by mistake
    fireEvent.keyDown(input, { key: 'Escape' })
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
    fireEvent.change(await screen.findByPlaceholderText(/Search orders/), { target: { value: 'bob' } })
    await act(() => new Promise((r) => setTimeout(r, 50)))
    expect(screen.queryByText('ORD-ADA')).toBeNull()
    await act(() => new Promise((r) => setTimeout(r, 300)))
    expect(screen.queryByText('ORD-ADA')).toBeNull()
    expect(await screen.findByText('ORD-BOB', {}, SLOW)).toBeDefined()
  })

  it('starts empty after ⌘K closes it', async () => {
    await openAndType([], 'abc')
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true })
    expect(((await screen.findByPlaceholderText(/Search orders/)) as HTMLInputElement).value).toBe('')
  })
})
