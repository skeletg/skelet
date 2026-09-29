import { buildSchema, parse } from 'graphql'
import { describe, expect, it } from 'vitest'
import {
  buildListQuery,
  findValueSlots,
  listerIndex,
  pickOperation,
  readListResult,
  readVariable,
  writeVariable,
} from './valueTargets'

const schema = buildSchema(`
  type Channel { id: ID! name: String! slug: String! }
  type Product { id: ID! name: String! sku: String! }
  type Category { id: ID! name: String! slug: String! }
  type CMSPage { id: ID! title: String! }
  type Cart { id: ID! }
  type CartList { items: [Cart!]! total: Int! }
  type Report { revenue: [Int!]! }
  input AddToCartInput { cartId: ID! productId: ID! quantity: Int! options: OptionInput }
  input OptionInput { categoryIds: [ID!] }
  input CategoryInput { name: String! slug: String! parentId: ID }
  type Query {
    channels: [Channel!]!
    channel(id: ID!): Channel
    products(channelId: ID, search: String, limit: Int): [Product!]!
    featuredProducts(channelId: ID!, tag: String!): [Product!]!
    categories(channelId: ID!): [Category!]!
    cmsPages(limit: Int): [CMSPage!]!
    carts(limit: Int): CartList!
    report: Report
  }
  type Mutation {
    addToCart(input: AddToCartInput!): Cart
    deleteProduct(id: ID!): Boolean!
    createCategory(input: CategoryInput!): Category
    linkPage(pageId: ID!): Boolean!
    addLine(cartId: ID!, productSku: String!, note: String): Cart!
  }
`)

function slots(doc: string) {
  return findValueSlots(schema, pickOperation(parse(doc)))
}

describe('findValueSlots', () => {
  it('maps a field id argument to the type the field returns', () => {
    expect(slots('query Q($id: ID!) { channel(id: $id) { id } }')).toEqual([
      { path: ['id'], targetType: 'Channel', valueField: 'id', isList: false, required: true },
    ])
  })

  it('opens up an input-object variable, nested inputs included', () => {
    const got = slots('mutation M($input: AddToCartInput!) { addToCart(input: $input) { id } }')
    expect(got.map((s) => [s.path.join('.'), s.targetType, s.isList])).toEqual([
      ['input.cartId', 'Cart', false],
      ['input.productId', 'Product', false],
      ['input.options.categoryIds', 'Category', true],
    ])
  })

  it('follows a variable written inside an object literal', () => {
    const got = slots('mutation M($p: ID!) { addToCart(input: { cartId: "c", productId: $p, quantity: 1 }) { id } }')
    expect(got).toEqual([{ path: ['p'], targetType: 'Product', valueField: 'id', isList: false, required: true }])
  })

  it('names the entity from the mutation when it returns no object, and resolves suffix and parent ids', () => {
    expect(slots('mutation M($id: ID!) { deleteProduct(id: $id) }')[0].targetType).toBe('Product')
    expect(slots('mutation M($p: ID!) { linkPage(pageId: $p) }')[0].targetType).toBe('CMSPage')
    expect(slots('mutation M($c: CategoryInput!) { createCategory(input: $c) { id } }')).toEqual([
      { path: ['c', 'parentId'], targetType: 'Category', valueField: 'id', isList: false, required: false },
    ])
  })

  it('picks a product by sku for a string sku argument, and leaves other strings alone', () => {
    expect(slots('mutation M($c: ID!, $s: String!, $n: String) { addLine(cartId: $c, productSku: $s, note: $n) { id } }')).toEqual([
      { path: ['c'], targetType: 'Cart', valueField: 'id', isList: false, required: true },
      { path: ['s'], targetType: 'Product', valueField: 'sku', isList: false, required: true },
    ])
  })
})

describe('listerIndex', () => {
  const index = listerIndex(schema)

  it('finds plain lists and items pages, and skips queries with other required arguments', () => {
    expect(index.get('Product')).toMatchObject({ field: 'products', viaItems: false, searchArg: 'search', limitArg: true, labelField: 'name', detailField: 'sku' })
    expect(index.get('Cart')).toMatchObject({ field: 'carts', viaItems: true })
    expect(index.get('Category')?.channelArg).toEqual({ type: 'ID!', required: true })
    expect([...index.values()].some((l) => l.field === 'featuredProducts' || l.field === 'report')).toBe(false)
  })

  it('builds a query with only the arguments the field takes', () => {
    const products = index.get('Product')!
    expect(buildListQuery(products, { search: 'shoe', channelId: 'ch1' })).toEqual({
      query: 'query PlaygroundPick($channelId: ID, $search: String) { products(channelId: $channelId, search: $search, limit: 50) { id name sku } }',
      variables: { channelId: 'ch1', search: 'shoe' },
    })
    expect(buildListQuery(index.get('Channel')!, { search: 'x' }).query).toBe('query PlaygroundPick { channels { id name slug } }')
    expect(readListResult(index.get('Cart')!, { carts: { items: [{ id: 'c1' }] } })).toEqual([{ id: 'c1', value: 'c1', label: 'c1', detail: null }])
  })
})

describe('writeVariable', () => {
  it('sets a nested path and keeps the rest', () => {
    const out = writeVariable('{"input":{"quantity":2}}', { path: ['input', 'productId'], isList: false }, 'p1')
    expect(JSON.parse(out)).toEqual({ input: { quantity: 2, productId: 'p1' } })
    expect(readVariable(out, ['input', 'productId'])).toBe('p1')
  })

  it('toggles an id in a list slot and starts from empty text', () => {
    const once = writeVariable('', { path: ['ids'], isList: true }, 'a')
    const twice = writeVariable(writeVariable(once, { path: ['ids'], isList: true }, 'b'), { path: ['ids'], isList: true }, 'a')
    expect(JSON.parse(twice)).toEqual({ ids: ['b'] })
  })

  it('refuses to guess over variables that are not valid JSON', () => {
    expect(() => writeVariable('{ broken', { path: ['id'], isList: false }, 'x')).toThrow()
  })
})
