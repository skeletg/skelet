import { buildSchema } from 'graphql'
import { describe, expect, it } from 'vitest'
import { buildCreateOperation, creatorIndex, readCreated, skeletonVariables } from './createTargets'

const schema = buildSchema(`
  type Cart { id: ID! }
  type Product { id: ID! name: String! }
  type Category { id: ID! name: String! slug: String! }
  enum Status { ACTIVE DRAFT }
  input CategoryInput { name: String! slug: String! parentId: ID status: Status! sort: Int = 0 meta: MetaInput! tags: [String!]! }
  input MetaInput { key: String! }
  type CreateProductPayload { product: Product warnings: [String!]! }
  type Query { categories: [Category!]! }
  type Mutation {
    createGuestCart: Cart!
    addToCart(cartId: ID!, sku: String!, quantity: Float!): Cart!
    addProductToCategory(productId: ID!, categoryId: ID!): Product
    createProduct(name: String!, categoryId: ID): CreateProductPayload!
    createCategory(input: CategoryInput!): Category
    startCart(channelId: ID!): Cart!
  }
`)

describe('creatorIndex', () => {
  const index = creatorIndex(schema)

  it('keeps mutations that make a record and drops ones that need an existing one', () => {
    expect(index.get('Cart')?.map((c) => c.field)).toEqual(['createGuestCart', 'startCart'])
    expect(index.get('Product')?.map((c) => c.field)).toEqual(['createProduct'])
  })

  it("doesn't mistake the new record's own slug for a lookup of an existing one", () => {
    expect(index.get('Category')?.map((c) => c.field)).toEqual(['createCategory'])
  })

  it('reaches into a payload for the new record', () => {
    expect(index.get('Product')?.[0]).toMatchObject({ viaField: 'product', labelField: 'name', requiredArgs: [{ name: 'name', type: 'String!' }] })
  })
})

describe('buildCreateOperation', () => {
  it('declares only the required arguments', () => {
    const [guest] = creatorIndex(schema).get('Cart')!
    expect(buildCreateOperation(guest)).toBe('mutation PlaygroundCreate { createGuestCart { id } }')
    expect(buildCreateOperation(creatorIndex(schema).get('Product')![0])).toBe(
      'mutation PlaygroundCreate($name: String!) { createProduct(name: $name) { product { id name } } }',
    )
  })
})

describe('skeletonVariables', () => {
  it('stubs required fields by type and skips optional and defaulted ones', () => {
    expect(skeletonVariables(schema, creatorIndex(schema).get('Category')![0])).toEqual({
      input: { name: '', slug: '', status: 'ACTIVE', meta: { key: '' }, tags: [] },
    })
  })
})

describe('readCreated', () => {
  it('pulls the id and label from a direct or payload result', () => {
    const [guest] = creatorIndex(schema).get('Cart')!
    expect(readCreated(guest, { createGuestCart: { id: 'c1' } })).toEqual({ id: 'c1', label: null })
    expect(readCreated(creatorIndex(schema).get('Product')![0], { createProduct: { product: { id: 'p1', name: 'Tee' } } })).toEqual({ id: 'p1', label: 'Tee' })
    expect(readCreated(guest, { createGuestCart: null })).toBeNull()
  })
})
