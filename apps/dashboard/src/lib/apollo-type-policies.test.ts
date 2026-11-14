import { InMemoryCache, gql } from '@apollo/client'
import { describe, expect, it } from 'vitest'
import { typePolicies } from './apollo-type-policies'

// What these pin: a query that selects fewer translation fields no longer blanks the ones another
// query fetched, and a translation the server dropped still goes

const DETAIL = gql`
  query Detail {
    category(id: "k1") {
      id
      content {
        locale
        name
        metaTitle
      }
    }
  }
`
const TREE = gql`
  query Tree {
    category(id: "k1") {
      id
      content {
        locale
        name
      }
    }
  }
`

const row = (locale: string, name: string, metaTitle?: string) => ({ __typename: 'CategoryContent', locale, name, ...(metaTitle ? { metaTitle } : {}) })
const category = (content: object[]) => ({ category: { __typename: 'Category', id: 'k1', content } })

describe('translation rows in the cache', () => {
  it('keeps the fields a narrower query did not select', () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: DETAIL, data: category([row('en', 'Shirts', 'Buy shirts')]) })
    cache.writeQuery({ query: TREE, data: category([row('en', 'Shirts')]) })
    expect(cache.readQuery<{ category: { content: object[] } }>({ query: DETAIL })?.category.content).toEqual([row('en', 'Shirts', 'Buy shirts')])
  })

  it('drops a locale the incoming list no longer has', () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: DETAIL, data: category([row('en', 'Shirts', 'Buy shirts'), row('de', 'Hemden', 'Hemden kaufen')]) })
    cache.writeQuery({ query: DETAIL, data: category([row('en', 'Shirts', 'Buy shirts')]) })
    expect(cache.readQuery<{ category: { content: { locale: string }[] } }>({ query: DETAIL })?.category.content.map((c) => c.locale)).toEqual(['en'])
  })
})

const PAGE = gql`
  query Page {
    productType(id: "t1") {
      id
      attributes {
        sortOrder
        groupId
        attribute {
          id
          slug
        }
      }
    }
  }
`
const FILTER = gql`
  query Filter {
    productType(id: "t1") {
      id
      attributes {
        sortOrder
        attribute {
          id
          slug
        }
      }
    }
  }
`

const attr = (id: string, groupId?: string) => ({ __typename: 'ProductTypeAttribute', sortOrder: 0, ...(groupId !== undefined ? { groupId } : {}), attribute: { __typename: 'Attribute', id, slug: id } })
const productType = (attributes: object[]) => ({ productType: { __typename: 'ProductType', id: 't1', attributes } })

describe('product type attribute rows in the cache', () => {
  it('keeps the group a narrower query did not select', () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: PAGE, data: productType([attr('a1', 'g1'), attr('a2', 'g2')]) })
    cache.writeQuery({ query: FILTER, data: productType([attr('a1'), attr('a2')]) })
    expect(cache.readQuery({ query: PAGE })).toEqual(productType([attr('a1', 'g1'), attr('a2', 'g2')]))
  })

  it('drops an attribute the incoming list no longer has', () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: PAGE, data: productType([attr('a1', 'g1'), attr('a2', 'g2')]) })
    cache.writeQuery({ query: FILTER, data: productType([attr('a2')]) })
    expect(cache.readQuery({ query: PAGE })).toEqual(productType([attr('a2', 'g2')]))
  })
})

describe('an order address in the cache', () => {
  const FULL = gql`
    query Full {
      order(id: "o1") {
        id
        shippingAddress {
          firstName
          lastName
          city
        }
      }
    }
  `
  const NARROW = gql`
    query Narrow {
      order(id: "o1") {
        id
        shippingAddress {
          firstName
          lastName
        }
      }
    }
  `
  const order = (shippingAddress: object) => ({ order: { __typename: 'Order', id: 'o1', shippingAddress: { __typename: 'OrderAddress', ...shippingAddress } } })

  it('keeps the fields a narrower query did not select', () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: FULL, data: order({ firstName: 'Ada', lastName: 'Astro', city: 'New York' }) })
    cache.writeQuery({ query: NARROW, data: order({ firstName: 'Ada', lastName: 'Astro' }) })
    expect(cache.readQuery<{ order: { shippingAddress: object } }>({ query: FULL })?.order.shippingAddress).toEqual({
      __typename: 'OrderAddress',
      firstName: 'Ada',
      lastName: 'Astro',
      city: 'New York',
    })
  })
})

describe("a CMS page's content in the cache", () => {
  const EDITOR = gql`
    query Editor {
      cmsPage(id: "p1") {
        id
        content {
          locale
          title
          body
        }
      }
    }
  `
  const LIST = gql`
    query List {
      cmsPage(id: "p1") {
        id
        content {
          locale
          title
        }
      }
    }
  `
  const page = (content: object | null) => ({ cmsPage: { __typename: 'CMSPage', id: 'p1', content: content && { __typename: 'CMSPageContent', ...content } } })
  const read = (cache: InMemoryCache) => cache.readQuery<{ cmsPage: { content: object | null } }>({ query: EDITOR })?.cmsPage.content

  it('keeps the body a list did not select', () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: EDITOR, data: page({ locale: 'en', title: 'About', body: { blocks: [] } }) })
    cache.writeQuery({ query: LIST, data: page({ locale: 'en', title: 'About us' }) })
    expect(read(cache)).toEqual({ __typename: 'CMSPageContent', locale: 'en', title: 'About us', body: { blocks: [] } })
  })

  it("doesn't lend one locale's body to another", () => {
    const cache = new InMemoryCache({ typePolicies })
    cache.writeQuery({ query: EDITOR, data: page({ locale: 'en', title: 'About', body: { blocks: [] } }) })
    cache.writeQuery({ query: LIST, data: page({ locale: 'sr', title: 'O nama' }) })
    expect(read(cache)).toBeUndefined()
    cache.writeQuery({ query: LIST, data: page(null) })
    expect(cache.readQuery<{ cmsPage: { content: object | null } }>({ query: LIST })?.cmsPage.content).toBeNull()
  })
})
