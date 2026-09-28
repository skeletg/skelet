import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, renderDialog } from '@/test/page-harness'
import { CategoryPicker } from './CategoryPicker'
import { SchemaFormRootCategoriesDocument } from './SchemaFormPickers.gql'

// What this pins: the picker labels a category by its path through the tree, falling back to the slug
// where a category has no name in the current locale. It used to ask for an `effectiveName` field
// Category never had, so the server refused the query and the picker listed nothing

const node = (id: string, slug: string, name: string | null) => ({ __typename: 'Category' as const, id, slug, name })

const tree = [
  {
    ...node('c1', 'apparel', 'Apparel'),
    children: [
      {
        ...node('c2', 'shirts', 'Shirts'),
        children: [node('c3', 't-shirts', null)],
      },
    ],
  },
]

describe('CategoryPicker', () => {
  it('labels a nested category by its path, falling back to the slug', async () => {
    renderDialog(<CategoryPicker schema={{ type: 'string' }} value="c3" onChange={() => {}} />, [
      { request: { query: SchemaFormRootCategoriesDocument }, result: { data: { rootCategories: tree } } },
    ])
    expect(await screen.findByText('Apparel › Shirts › t-shirts', {}, SLOW)).toBeDefined()
  })
})
