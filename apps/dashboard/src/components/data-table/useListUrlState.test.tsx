import { useEffect } from 'react'
import { act, render } from '@testing-library/react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ListUrlState } from './useListUrlState'
import { useListUrlState } from './useListUrlState'

// Every list page in the dashboard reads its state from here, so the invariants worth pinning are
// the ones a URL has to satisfy: pasteable, hand-editable without blanking the page, and free of
// the params that only restate a default
const { state } = vi.hoisted(() => ({ state: { accountSize: 20 } }))
vi.mock('./useTablePrefs', () => ({ useDefaultPageSize: () => state.accountSize }))

let api: ListUrlState
let search = ''

// Captured in an effect rather than during render - render must not write to anything outside it,
// and testing-library flushes effects inside act, so both are current when a test reads them
function Probe({ opts }: { opts?: { key?: string; pageSize?: number } }) {
  const current = useListUrlState(opts)
  const { search: url } = useLocation()
  useEffect(() => {
    api = current
    search = url
  })
  return null
}

function mount(url = '/orders', opts?: { key?: string; pageSize?: number }) {
  render(
    <MemoryRouter initialEntries={[url]}>
      <Probe opts={opts} />
    </MemoryRouter>,
  )
}

const params = () => new URLSearchParams(search)
const act1 = (fn: () => void) => act(() => fn())

beforeEach(() => {
  state.accountSize = 20
  search = ''
})

describe('reading state out of the url', () => {
  it('starts empty on a bare url', () => {
    mount()
    expect(api.globalFilter).toBe('')
    expect(api.columnFilters).toEqual([])
    expect(api.sorting).toEqual([])
    expect(api.pagination).toEqual({ pageIndex: 0, pageSize: 20 })
  })

  it('reads the page as 1-based, because a human reads it too', () => {
    mount('/orders?page=3')
    expect(api.pagination.pageIndex).toBe(2)
  })

  it('never lets a nonsense page index go negative', () => {
    mount('/orders?page=0')
    expect(api.pagination.pageIndex).toBe(0)
  })

  it('decodes filters and sorting', () => {
    mount('/orders?q=acme&f=' + encodeURIComponent('[{"id":"status","value":["OPEN"]}]') + '&s=' + encodeURIComponent('[{"id":"total","desc":true}]'))
    expect(api.globalFilter).toBe('acme')
    expect(api.columnFilters).toEqual([{ id: 'status', value: ['OPEN'] }])
    expect(api.sorting).toEqual([{ id: 'total', desc: true }])
  })

  // a link someone truncated in chat must not take the page down with it
  it('falls back rather than throwing on a hand-mangled param', () => {
    mount('/orders?f=[{"id":&s=notjson')
    expect(api.columnFilters).toEqual([])
    expect(api.sorting).toEqual([])
  })

  it('follows the reader’s own page-size default when the url says nothing', () => {
    state.accountSize = 50
    mount()
    expect(api.pagination.pageSize).toBe(50)
  })

  it('lets a page insist on a size over the account default', () => {
    state.accountSize = 50
    mount('/orders', { pageSize: 10 })
    expect(api.pagination.pageSize).toBe(10)
  })
})

describe('writing state back', () => {
  it('puts the search term in the url and takes it out again when cleared', () => {
    mount()
    act1(() => api.setGlobalFilter('acme'))
    expect(params().get('q')).toBe('acme')
    act1(() => api.setGlobalFilter(''))
    expect(params().has('q')).toBe(false)
  })

  it('sends you back to the first page when the result set changes under you', () => {
    mount('/orders?page=4')
    act1(() => api.setGlobalFilter('acme'))
    expect(params().has('page')).toBe(false)

    mount('/orders?page=4')
    act1(() => api.setColumnFilters([{ id: 'status', value: ['OPEN'] }]))
    expect(params().has('page')).toBe(false)
  })

  // re-sorting reorders the same rows, so the page you are on still means something
  it('leaves the page alone when only the sort changes', () => {
    mount('/orders?page=4')
    act1(() => api.setSorting([{ id: 'total', desc: true }]))
    expect(params().get('page')).toBe('4')
  })

  it('writes the page 1-based, and omits page one entirely', () => {
    mount()
    act1(() => api.setPagination({ pageIndex: 2, pageSize: 20 }))
    expect(params().get('page')).toBe('3')
    act1(() => api.setPagination({ pageIndex: 0, pageSize: 20 }))
    expect(params().has('page')).toBe(false)
  })

  // The default is both the fallback and the omit sentinel, deliberately. Writing it would make a
  // link read as "this exact size" to someone whose own default differs
  it('omits a page size that only restates the default, and keeps one that does not', () => {
    mount()
    act1(() => api.setPagination({ pageIndex: 0, pageSize: 20 }))
    expect(params().has('size')).toBe(false)
    act1(() => api.setPagination({ pageIndex: 0, pageSize: 100 }))
    expect(params().get('size')).toBe('100')
  })

  it('drops an emptied filter list instead of writing []', () => {
    mount()
    act1(() => api.setColumnFilters([{ id: 'status', value: ['OPEN'] }]))
    expect(params().has('f')).toBe(true)
    act1(() => api.setColumnFilters([]))
    expect(params().has('f')).toBe(false)
  })

  it('reset clears everything it owns', () => {
    mount('/orders?q=acme&page=3&size=100&f=' + encodeURIComponent('[{"id":"a","value":1}]'))
    act1(() => api.reset())
    expect(search === '' || [...params().keys()].length === 0).toBe(true)
  })

  it('leaves params it does not own alone', () => {
    mount('/orders?tab=archived')
    act1(() => api.setGlobalFilter('acme'))
    expect(params().get('tab')).toBe('archived')
  })

  // what the pending ref exists for: a table filters and resets the page in one tick, and composing
  // the second write against the pre-navigation url would wipe the first
  it('keeps both writes when two land in the same tick', () => {
    mount()
    act1(() => {
      api.setGlobalFilter('acme')
      api.setSorting([{ id: 'total', desc: true }])
    })
    expect(params().get('q')).toBe('acme')
    expect(params().get('s')).toBe('[{"id":"total","desc":true}]')
  })
})

describe('two tables on one route', () => {
  it('namespaces every param it owns', () => {
    mount('/orders', { key: 'inbox' })
    act1(() => api.setGlobalFilter('acme'))
    act1(() => api.setPagination({ pageIndex: 1, pageSize: 20 }))
    expect(params().get('inbox_q')).toBe('acme')
    expect(params().get('inbox_page')).toBe('2')
    expect(params().has('q')).toBe(false)
  })

  it('reads only its own namespace, so the neighbour’s filter is not its own', () => {
    mount('/orders?q=theirs&inbox_q=mine', { key: 'inbox' })
    expect(api.globalFilter).toBe('mine')
    expect(api.paramKey).toBe('inbox')
  })
})
