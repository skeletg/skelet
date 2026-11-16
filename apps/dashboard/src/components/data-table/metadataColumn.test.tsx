import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DataTable } from './DataTable'
import { metadataColumn } from './columns'
import { __resetListExtensionsForTests, hasListColumns, registerListColumn, registerListRowAction } from './extensions'

vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(_key: string, fallback: T) => [fallback, () => {}],
  useOptionalPreference: <T,>(_key: string, fallback: T) => fallback,
}))

type Row = { id: string; name: string; metadata: { key: string; value: string }[] }

afterEach(__resetListExtensionsForTests)

describe('metadataColumn', () => {
  it('puts a plugin’s metadata key on the list it targets, ahead of the list’s own menu, a dash where unset', () => {
    registerListColumn<Row>({ id: 'acme:tier', target: 'customers', column: metadataColumn<Row>('tier', 'Tier', (v) => v.toUpperCase()) })
    const rows: Row[] = [
      { id: '1', name: 'Ada', metadata: [{ key: 'tier', value: 'gold' }] },
      { id: '2', name: 'Grace', metadata: [] },
    ]
    const menu = { id: 'actions', header: 'Menu', cell: () => '…' }
    render(<DataTable<Row> target="customers" columns={[{ accessorKey: 'name', header: 'Name' }, menu]} data={rows} />)
    expect(screen.getAllByRole('columnheader').map((h) => h.textContent)).toEqual(['Name', 'Tier', 'Menu'])
    expect(screen.getByText('GOLD')).toBeDefined()
    expect(screen.getByText('-')).toBeDefined()
  })
})

describe('hasListColumns', () => {
  it('tells a list whether to load what plugin columns read', () => {
    expect(hasListColumns('customers')).toBe(false)
    registerListColumn<Row>({ id: 'acme:tier', target: 'customers', column: metadataColumn<Row>('tier', 'Tier') })
    expect(hasListColumns('customers')).toBe(true)
    expect(hasListColumns('catalog.products')).toBe(false)
  })
})

describe('a list with its own menu and a plugin row action', () => {
  it('keeps one column per id, the list’s own menu last', () => {
    registerListRowAction<Row>({ id: 'acme:flag', target: 'customers', action: { label: 'Flag', onSelect: () => {} } })
    const menu = { id: 'actions', header: 'Menu', cell: () => '…' }
    render(<DataTable<Row> target="customers" columns={[{ accessorKey: 'name', header: 'Name' }, menu]} data={[{ id: '1', name: 'Ada', metadata: [] }]} />)
    const headers = screen.getAllByRole('columnheader')
    expect(headers.at(-1)?.textContent).toBe('Menu')
    expect(headers).toHaveLength(3)
  })
})
