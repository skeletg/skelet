import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { CategoryOptionsDocument } from '@/features/catalog/category-picker/CategoryOptions.gql'
import { CategoryPicker } from './CategoryPicker'

// What this pins: the picker labels a category by its path through the tree, falling back to the slug
// where a category has no name in the current locale, at any depth - the old tree query stopped at
// three levels, so a deeper category could never be chosen

const option = (id: string, slug: string, name: string | null, parentId: string | null = null, isPublished = true) => ({
  __typename: 'CategoryOption' as const,
  id,
  parentId,
  slug,
  name,
  position: 0,
  isPublished,
  productCount: 0,
})

const options = [
  option('c1', 'apparel', 'Apparel'),
  option('c2', 'shirts', 'Shirts', 'c1'),
  option('c3', 't-shirts', null, 'c2'),
  // A draft stays pickable - a promotion matches on membership, published or not
  option('c4', 'v-necks', 'V-necks', 'c3', false),
]

const mocks = [{ request: { query: CategoryOptionsDocument }, result: { data: { categoryOptions: options } }, maxUsageCount: 3 }]

describe('CategoryPicker', () => {
  it('labels a nested category by its path, falling back to the slug', async () => {
    renderDialog(<CategoryPicker schema={{ type: 'string' }} value="c3" onChange={() => {}} />, mocks)
    expect(await screen.findByText('Apparel › Shirts › t-shirts', {}, SLOW)).toBeDefined()
  })

  it('finds a draft category four levels down, marks it, and ticks it into the list', async () => {
    const onChange = vi.fn()
    renderDialog(<CategoryPicker schema={{ type: 'array' }} value={['c2']} onChange={onChange} />, mocks)
    expect(await screen.findByText('Apparel › Shirts', {}, SLOW)).toBeDefined()
    fireEvent.click(screen.getByRole('combobox', { name: 'Categories' }))
    fireEvent.change(await screen.findByRole('combobox', { name: 'Search categories' }), { target: { value: 'v-neck' } })
    const hits = await screen.findByRole('listbox', { name: 'Categories' })
    const row = (await within(hits).findByRole('checkbox', { name: 'Select V-necks' }, SLOW)).closest('[role="option"]') as HTMLElement
    expect(within(row).getByText('Draft').getAttribute('title')).toBe('Draft - hidden on every store')
    fireEvent.click(within(row).getByRole('checkbox'))
    expect(onChange).toHaveBeenCalledWith(['c2', 'c4'])
  })
})
