import { describe, expect, it } from 'vitest'
import { buildSchema, parse, type GraphQLObjectType } from 'graphql'
import { buildOperation, matchesTerm, type OperationEntry } from './buildOperation'

// A schema with the shapes that actually break a naive generator: a self-referential edge, a
// union, required and optional args, and a list-of-non-null wrapper
const schema = buildSchema(/* GraphQL */ `
  scalar UUID

  type Product {
    id: UUID!
    name: String!
    price: Int
    kind: Kind!
    # Self-referential - the reason there's a depth cap at all
    related: [Product!]!
    owner: Owner
  }

  enum Kind {
    SIMPLE
    BUNDLE
  }

  type Person {
    name: String!
  }
  type Company {
    vat: String!
  }
  union Owner = Person | Company

  type Query {
    product(id: UUID!, locale: String): Product!
    products(first: Int = 20): [Product!]!
    ping: String!
  }

  type Mutation {
    deleteProduct(id: UUID!): Boolean!
  }
`)

function entry(kind: 'query' | 'mutation', name: string): OperationEntry {
  const root = (
    kind === 'query' ? schema.getQueryType() : schema.getMutationType()
  ) as GraphQLObjectType
  const field = root.getFields()[name]
  if (!field) throw new Error(`no field ${name}`)
  return { kind, name, description: field.description ?? '', field }
}

// Every generated operation has to parse - an IDE that opens a syntax error is worse than one that
// opens nothing
function expectParses(query: string) {
  expect(() => parse(query)).not.toThrow()
}

describe('buildOperation', () => {
  it('promotes required args to variables and leaves optional ones out', () => {
    const { query, variables } = buildOperation(entry('query', 'product'))
    expect(query).toContain('query Product($id: UUID!)')
    expect(query).toContain('product(id: $id)')
    expect(query).not.toContain('locale')
    expect(JSON.parse(variables)).toEqual({ id: null })
    expectParses(query)
  })

  it('treats a defaulted non-null arg as optional', () => {
    const { query, variables } = buildOperation(entry('query', 'products'))
    expect(query).toContain('query Products {')
    expect(variables).toBe('')
    expectParses(query)
  })

  it('emits no selection set for a scalar root field', () => {
    const { query } = buildOperation(entry('query', 'ping'))
    expect(query).toBe('query Ping {\n  ping\n}\n')
    expectParses(query)
  })

  it('keeps a mutation a mutation', () => {
    const { query } = buildOperation(entry('mutation', 'deleteProduct'))
    expect(query.startsWith('mutation DeleteProduct($id: UUID!)')).toBe(true)
    expectParses(query)
  })

  it('stops recursing on a self-referential edge', () => {
    const { query } = buildOperation(entry('query', 'product'))
    expectParses(query)
    // related -> related is allowed by the depth cap; a third level is not
    const depth = query.split('\n').filter((l) => l.trim().startsWith('related')).length
    expect(depth).toBeGreaterThan(0)
    expect(depth).toBeLessThanOrEqual(2)
    expect(query.length).toBeLessThan(4000)
  })

  it('reduces a union to __typename rather than guessing a member', () => {
    const { query } = buildOperation(entry('query', 'product'))
    expect(query).toMatch(/owner \{\s*\n\s*__typename/)
    expectParses(query)
  })

  it('selects enum and scalar leaves directly', () => {
    const { query } = buildOperation(entry('query', 'product'))
    expect(query).toMatch(/^\s+kind$/m)
    expect(query).toMatch(/^\s+name$/m)
  })
})

describe('matchesTerm', () => {
  const e = { ...entry('query', 'product'), description: 'Fetch one catalog product' }

  it('is empty-term permissive', () => {
    expect(matchesTerm(e, '   ')).toBe(true)
  })

  it('matches name case-insensitively', () => {
    expect(matchesTerm(e, 'PROD')).toBe(true)
  })

  it('matches on description too, so a search finds things named differently', () => {
    expect(matchesTerm(e, 'catalog')).toBe(true)
  })

  it('rejects a miss', () => {
    expect(matchesTerm(e, 'zzz')).toBe(false)
  })
})
