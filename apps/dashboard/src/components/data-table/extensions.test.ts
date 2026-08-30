import { afterEach, describe, expect, it } from 'vitest'
import {
  __resetListExtensionsForTests,
  getListColumns,
  getListFilters,
  getListMassActions,
  getListRowActions,
  registerListColumn,
  registerListFilter,
  registerListMassAction,
  registerListRowAction,
} from './extensions'

type Row = { id: string }

afterEach(__resetListExtensionsForTests)

describe('list extensions', () => {
  it('only hands a list what was registered for its target', () => {
    registerListColumn<Row>({ id: 'a:col', target: 'catalog.products', column: { id: 'x' } })
    registerListColumn<Row>({ id: 'b:col', target: 'sales.orders', column: { id: 'y' } })

    expect(getListColumns('catalog.products').map((e) => e.column.id)).toEqual(['x'])
    expect(getListColumns('sales.orders').map((e) => e.column.id)).toEqual(['y'])
  })

  // A table with no target must stay inert, or an embedded sub-table inherits a page list's extras
  it('returns nothing for an untargeted table', () => {
    registerListColumn<Row>({ id: 'a:col', target: 'catalog.products', column: { id: 'x' } })
    expect(getListColumns(undefined)).toEqual([])
    expect(getListFilters(undefined)).toEqual([])
    expect(getListRowActions(undefined)).toEqual([])
    expect(getListMassActions(undefined)).toEqual([])
  })

  it('orders by `order`, defaulting to 100 so core sits ahead of late registrations', () => {
    registerListColumn<Row>({ id: 'late', target: 't', column: { id: 'late' }, order: 200 })
    registerListColumn<Row>({ id: 'default', target: 't', column: { id: 'default' } })
    registerListColumn<Row>({ id: 'early', target: 't', column: { id: 'early' }, order: 10 })

    expect(getListColumns('t').map((e) => e.column.id)).toEqual(['early', 'default', 'late'])
  })

  // Two plugins claiming one id would silently shadow each other, so registration is the place to fail
  it('refuses a duplicate id per kind', () => {
    registerListColumn<Row>({ id: 'dup', target: 't', column: { id: 'x' } })
    expect(() =>
      registerListColumn<Row>({ id: 'dup', target: 't', column: { id: 'y' } }),
    ).toThrow(/duplicate column id "dup"/)
  })

  it('keeps the four kinds in separate namespaces', () => {
    registerListColumn<Row>({ id: 'same', target: 't', column: { id: 'c' } })
    registerListFilter({ id: 'same', target: 't', filter: { column: 'c', title: 'C', options: [] } })
    registerListRowAction<Row>({ id: 'same', target: 't', action: { label: 'Go' } })
    registerListMassAction<Row>({
      id: 'same',
      target: 't',
      action: { label: 'Delete', perform: async () => {} },
    })

    expect(getListColumns('t')).toHaveLength(1)
    expect(getListFilters('t')).toHaveLength(1)
    expect(getListRowActions('t')).toHaveLength(1)
    expect(getListMassActions('t')).toHaveLength(1)
  })
})
