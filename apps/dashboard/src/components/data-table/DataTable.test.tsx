import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DataTable } from './DataTable'
import { selectColumn } from './select-column'
import type { ColumnDef } from '@/components/data-table/tanstack'

// The table is the one piece of the dashboard every list page goes through, and it rides a vendor
// compatibility layer - typecheck can't tell us rows still render, so this does
vi.mock('@/lib/admin-prefs', () => ({
  usePreference: <T,>(_key: string, fallback: T) => [fallback, () => {}],
  useOptionalPreference: <T,>(_key: string, fallback: T) => fallback,
}))

type Widget = { id: string; name: string; qty: number }

const rows: Widget[] = [
  { id: '1', name: 'Anvil', qty: 3 },
  { id: '2', name: 'Bolt', qty: 7 },
  { id: '3', name: 'Cog', qty: 5 },
]

const columns: ColumnDef<Widget, unknown>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'qty', header: 'Qty' },
]

const table = () => screen.getByRole('table')
const body = () => within(table().querySelector('tbody') as HTMLElement)

describe('DataTable', () => {
  it('renders a header per column and a row per record', () => {
    render(<DataTable columns={columns} data={rows} />)
    const heads = Array.from(table().querySelectorAll('thead th')).map((h) => h.textContent?.trim())
    expect(heads).toEqual(expect.arrayContaining(['Name', 'Qty']))
    expect(body().getAllByRole('row')).toHaveLength(3)
  })

  it('reads cell values through the accessor', () => {
    render(<DataTable columns={columns} data={rows} />)
    const cells = body()
      .getAllByRole('cell')
      .map((c) => c.textContent)
    expect(cells).toEqual(expect.arrayContaining(['Anvil', '3', 'Bolt', '7', 'Cog', '5']))
  })

  it('pages client-side rather than rendering everything', () => {
    const many = Array.from({ length: 12 }, (_, i) => ({ id: String(i), name: `Row ${i}`, qty: i }))
    render(<DataTable columns={columns} data={many} pageSize={5} />)
    expect(body().getAllByRole('row')).toHaveLength(5)
  })

  // Both of these adjust during render rather than in an effect, which the react-hooks rules
  // reject - these pin the behaviour so it can't quietly regress every list
  it('clamps the page index when the row count shrinks under it', () => {
    const many = Array.from({ length: 12 }, (_, i) => ({ id: String(i), name: `Row ${i}`, qty: i }))
    const view = render(<DataTable columns={columns} data={many} pageSize={5} />)
    fireEvent.click(screen.getByLabelText('Next page'))
    fireEvent.click(screen.getByLabelText('Next page'))
    expect(body().getAllByRole('row')).toHaveLength(2) // page 3 of 12

    // deleting rows out from under page 3 must land on the last real page, never an empty body
    view.rerender(<DataTable columns={columns} data={many.slice(0, 6)} pageSize={5} />)
    expect(body().getAllByRole('row')).toHaveLength(1) // page 2 of 6
    expect(body().getByText('Row 5')).toBeTruthy()
  })

  it('keeps the page index when the data changes but still covers it', () => {
    const many = Array.from({ length: 12 }, (_, i) => ({ id: String(i), name: `Row ${i}`, qty: i }))
    const view = render(<DataTable columns={columns} data={many} pageSize={5} />)
    fireEvent.click(screen.getByLabelText('Next page'))
    expect(body().getByText('Row 5')).toBeTruthy()
    view.rerender(<DataTable columns={columns} data={many.slice(0, 11)} pageSize={5} />)
    expect(body().getByText('Row 5')).toBeTruthy() // still page 2, not reset
  })

  it('shows the empty state instead of a body row when there is no data', () => {
    render(<DataTable columns={columns} data={[]} emptyState={<span>Nothing here</span>} />)
    expect(screen.getByText('Nothing here')).toBeTruthy()
  })

  // The rest drive the feature slots the compatibility layer re-declares - each one is a
  // separate registration, so a miswired slot fails here rather than on a list page

  it('sorts client-side when a sortable header is clicked', () => {
    const sortable: ColumnDef<Widget, unknown>[] = [
      { accessorKey: 'name', header: 'Name' },
      { accessorKey: 'qty', header: 'Qty', enableSorting: true },
    ]
    render(<DataTable columns={sortable} data={rows} initialSorting={[{ id: 'qty', desc: false }]} />)
    const qtys = () =>
      body()
        .getAllByRole('row')
        .map((r) => r.querySelectorAll('td')[1]?.textContent)
    expect(qtys()).toEqual(['3', '5', '7'])
  })

  it('filters rows through the global search box', () => {
    render(<DataTable columns={columns} data={rows} searchPlaceholder="Search widgets" />)
    fireEvent.change(screen.getByPlaceholderText('Search widgets'), { target: { value: 'Bolt' } })
    expect(body().getAllByRole('row')).toHaveLength(1)
    expect(body().getByText('Bolt')).toBeTruthy()
  })

  it('tracks row selection through the selection column', () => {
    render(
      <DataTable
        columns={[selectColumn<Widget>(), ...columns]}
        data={rows}
        enableRowSelection
        getRowId={(r) => r.id}
      />,
    )
    const boxes = screen.getAllByLabelText('Select row')
    expect(boxes).toHaveLength(3)
    fireEvent.click(boxes[0])
    expect((boxes[0] as HTMLElement).getAttribute('data-state')).toBe('checked')
  })

  // The all-matching promise is scoped to the CURRENT filters, so changing them has to drop it -
  // adjusted during render, like the clamp above
  it('drops the select-all-matching promise when the filter changes', () => {
    const many = Array.from({ length: 12 }, (_, i) => ({ id: String(i), name: `Row ${i}`, qty: i }))
    render(
      <DataTable
        columns={[selectColumn<Widget>(), ...columns]}
        data={many}
        pageSize={5}
        enableRowSelection
        getRowId={(r) => r.id}
        searchPlaceholder="Search widgets"
        bulkActions={[{ id: 'noop', label: 'Do nothing', perform: async () => {} }]}
      />,
    )
    fireEvent.click(screen.getByLabelText('Select all'))
    fireEvent.click(screen.getByText('Select all 12 matching'))
    expect(screen.getByText('All 12 matching selected')).toBeTruthy()

    // Deliberately a filter that still matches all 12 and leaves the page fully selected - a
    // narrowing filter would be dropped by the pageFullySelected check instead, testing nothing
    fireEvent.change(screen.getByPlaceholderText('Search widgets'), { target: { value: 'Row' } })
    expect(screen.queryByText('All 12 matching selected')).toBeNull()
    expect(screen.getByText('5 selected')).toBeTruthy()
  })

  it('honours initial column visibility', () => {
    render(<DataTable columns={columns} data={rows} initialColumnVisibility={{ qty: false }} />)
    const heads = Array.from(table().querySelectorAll('thead th')).map((h) => h.textContent?.trim())
    expect(heads).toContain('Name')
    expect(heads).not.toContain('Qty')
  })
})
