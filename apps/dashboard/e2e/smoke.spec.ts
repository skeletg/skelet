import { readFileSync } from 'node:fs'
import { expect, type Page } from '@playwright/test'
import { adminApi, startClean, test } from './helpers'
import { RUN, cleanup, registerCleanups } from './fixtures'

// Every page with a fixed path, opened once against the real API. The unit suite mocks Apollo, so a
// query the schema no longer accepts, a resolver that refuses an admin it should serve, or a page that
// throws on real data all pass there - here each shows up as a named failing step
//
// The paths come straight from the nav registry's source, so a new page is covered the day it lands.
// Pages with an :id get the second test, below

const SKIP: Record<string, string> = {
  '/playground': 'opens over the current page (actionId) - there is no route behind it',
}

const source = readFileSync(new URL('../src/nav/routes.tsx', import.meta.url), 'utf8')
const PATHS = [...new Set([...source.matchAll(/path: '(\/[^']*)'/g)].map((m) => m[1]))].filter((p) => !p.includes(':') && !(p in SKIP))

// Collects what went wrong on the page - the walk drains it after each route
function watch(page: Page) {
  const problems: string[] = []
  let inflight = 0
  let lastActivity = Date.now()
  const isApi = (url: string) => url.includes('/graphql')
  const touch = () => (lastActivity = Date.now())

  page.on('request', (r) => {
    if (!isApi(r.url())) return
    inflight++
    touch()
  })
  page.on('requestfinished', (r) => {
    if (!isApi(r.url())) return
    inflight--
    touch()
  })
  page.on('requestfailed', (r) => {
    if (!isApi(r.url())) return
    inflight--
    touch()
    // Apollo cancels a page's queries when it unmounts - that is the walk moving on, not a failure
    const why = r.failure()?.errorText ?? ''
    if (!why.includes('ERR_ABORTED')) problems.push(`request failed: ${why}`)
  })
  page.on('response', async (res) => {
    if (!isApi(res.url())) return
    let op = '?'
    try {
      op = (JSON.parse(res.request().postData() ?? '{}') as { operationName?: string }).operationName ?? '?'
    } catch {
      // a GET or a non-JSON body - the operation name is only for the message
    }
    try {
      const body = (await res.json()) as { errors?: { message: string; extensions?: { code?: string } }[] }
      for (const e of body.errors ?? []) problems.push(`${op}: [${e.extensions?.code ?? 'no code'}] ${e.message}`)
    } catch {
      if (res.status() >= 400) problems.push(`${op}: HTTP ${res.status()}`)
    }
  })
  page.on('pageerror', (e) => problems.push(`uncaught: ${e.message}`))
  page.on('console', (m) => {
    if (m.type() === 'error') problems.push(`console.error: ${m.text().slice(0, 300)}`)
  })

  return {
    drain: () => problems.splice(0),
    // Quiet once no API call has been open for a beat - a page that keeps polling still gets a cap
    async settle() {
      const start = Date.now()
      await page.waitForTimeout(300)
      while (Date.now() - start < 15_000) {
        if (inflight <= 0 && Date.now() - lastActivity > 700) return
        await page.waitForTimeout(100)
      }
    },
  }
}

/** Opens one route in-app and fails the step softly on anything the page got wrong */
async function visit(page: Page, seen: ReturnType<typeof watch>, path: string) {
  // In-app navigation - a full load per page would triple the run, and the router is what the
  // admin drives anyway
  await page.evaluate((p) => {
    history.pushState({}, '', p)
    dispatchEvent(new PopStateEvent('popstate'))
  }, path)
  await seen.settle()
  const text = await page.locator('main').last().innerText()
  expect.soft(text, `${path} hit the route error boundary`).not.toContain('This page hit an error')
  expect.soft(text, `${path} rendered the 404 page`).not.toContain('Page not found')
  expect.soft(seen.drain(), `${path} logged errors`).toEqual([])
}

test('every fixed-path page renders without an error', async ({ page }) => {
  test.setTimeout(PATHS.length * 10_000)
  expect(PATHS.length, 'the route parse found nothing - has routes.tsx changed shape?').toBeGreaterThan(50)

  await startClean(page)
  const seen = watch(page)
  await seen.settle()
  expect(seen.drain(), 'the app shell itself').toEqual([])

  for (const path of PATHS) await test.step(path, () => visit(page, seen, path))
})

// ---------- Detail pages ----------
//
// Every route with a parameter, opened on a real record: one the spec creates when the API can make
// it cheaply, else the first the dev data has. A route with neither is skipped by name, so a new
// parameterised page shows up here as "no resolver" until someone says how to reach it

registerCleanups()

type Resolved = string | { skip: string }
type Resolver = () => Promise<Resolved>

const del = (mutation: string, id: string) => () => adminApi(`mutation($id:ID!){ ${mutation}(id:$id) }`, { id })
const skip = (why: string): Resolved => ({ skip: why })

/** The first row a list query returns, or null */
async function first<T>(query: string, variables: Record<string, unknown> = {}): Promise<T | null> {
  const data = await adminApi<Record<string, unknown>>(query, variables).catch(() => null)
  if (!data) return null
  const list = Object.values(data)[0] as T[] | { items?: T[] } | null
  const rows = Array.isArray(list) ? list : (list?.items ?? [])
  return rows[0] ?? null
}

/** Creates a record through the API, registers its delete, and returns its id */
async function made(mutation: string, input: unknown, remove: string): Promise<string> {
  const name = mutation.match(/^\s*mutation[^{]*\{\s*(\w+)/)?.[1] ?? ''
  const data = await adminApi<Record<string, { id?: string; webhook?: { id: string }; integration?: { id: string } }>>(mutation, { i: input })
  const row = data[name]
  const id = row.id ?? row.webhook?.id ?? row.integration?.id
  if (!id) throw new Error(`${name} returned no id`)
  cleanup(del(remove, id))
  return id
}

function detailResolvers(channelId: string, currencyCode: string): Record<string, Resolver> {
  let customer: Promise<string> | null = null
  const seededCustomer = () =>
    (customer ??= made(
      'mutation($i:AdminCreateCustomerInput!){ adminCreateCustomer(input:$i){ id } }',
      { channelId, email: `${RUN}-detail-customer@e2e.test`, password: 'E2e-password-1234!', firstName: 'Detail', lastName: 'Customer' },
      'deleteCustomer',
    ))
  let segment: Promise<string> | null = null
  const seededSegment = () =>
    (segment ??= made(
      'mutation($i:CreateCustomerSegmentInput!){ createCustomerSegment(input:$i){ id } }',
      { slug: `${RUN}-detail-segment`, name: 'Detail segment' },
      'deleteCustomerSegment',
    ))
  let role: Promise<string> | null = null
  const anyRole = () => (role ??= first<{ id: string }>('{ roles { id } }').then((r) => r?.id ?? ''))
  let webForm: Promise<string> | null = null
  const seededWebForm = async () =>
    (webForm ??= adminApi<{ createWebForm: { id: string } }>('mutation($c:ID!,$i:SaveWebFormInput!){ createWebForm(channelId:$c, input:$i){ id } }', {
      c: channelId,
      i: { slug: `${RUN}-detail-form`, name: 'Detail form', status: 'draft', fields: [], settings: {} },
    }).then(({ createWebForm }) => {
      cleanup(del('deleteWebForm', createWebForm.id))
      return createWebForm.id
    }))
  const or = async (path: (id: string) => string, id: Promise<string | null | undefined>, why: string): Promise<Resolved> => {
    const value = await id.catch(() => null)
    return value ? path(value) : skip(why)
  }

  return {
    '/admins/:id': async () => `/admins/${(await first<{ id: string }>('{ admins { id } }'))?.id}`,
    '/admins/roles/:id': async () => `/admins/roles/${await anyRole()}`,
    '/admins/roles/:id/edit': async () => `/admins/roles/${await anyRole()}/edit`,
    '/blog/authors/:id': async () =>
      `/blog/authors/${await made('mutation($i:CreateBlogAuthorInput!){ createBlogAuthor(input:$i){ id } }', { slug: `${RUN}-detail-author`, displayName: 'Detail author', locale: 'en', name: 'Detail author' }, 'deleteBlogAuthor')}`,
    '/blog/categories/:id': async () =>
      `/blog/categories/${await made('mutation($i:CreateBlogCategoryInput!){ createBlogCategory(input:$i){ id } }', { slug: `${RUN}-detail-blog-category`, locale: 'en', name: 'Detail category' }, 'deleteBlogCategory')}`,
    '/blog/posts/:id': async () =>
      `/blog/posts/${await made('mutation($i:CreateBlogPostInput!){ createBlogPost(input:$i){ id } }', { slug: `${RUN}-detail-post`, locale: 'en', title: 'Detail post' }, 'deleteBlogPost')}`,
    '/blog/tags/:id': async () =>
      `/blog/tags/${await made('mutation($i:CreateBlogTagInput!){ createBlogTag(input:$i){ id } }', { slug: `${RUN}-detail-tag`, locale: 'en', name: 'Detail tag' }, 'deleteBlogTag')}`,
    '/campaigns/:id': () =>
      or((id) => `/campaigns/${id}`, first<{ id: string }>('query($c:ID!){ emailCampaigns(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no email campaign'),
    '/carts/:id': async () =>
      `/carts/${(await adminApi<{ adminCreateCart: { id: string } }>('mutation($c:ID!){ adminCreateCart(channelId:$c){ id } }', { c: channelId })).adminCreateCart.id}`,
    '/catalog/attributes/:id': async () =>
      `/catalog/attributes/${await made('mutation($i:CreateAttributeInput!){ createAttribute(input:$i){ id } }', { slug: `${RUN}-detail-attr`, name: 'Detail attribute', inputType: 'PLAIN_TEXT' }, 'deleteAttribute')}`,
    '/catalog/product-types/:id': async () =>
      `/catalog/product-types/${await made('mutation($i:CreateProductTypeInput!){ createProductType(input:$i){ id } }', { slug: `${RUN}-detail-type`, name: 'Detail type' }, 'deleteProductType')}`,
    '/categories/:id': async () =>
      `/categories/${await made('mutation($i:CreateCategoryInput!){ createCategory(input:$i){ id } }', { slug: `${RUN}-detail-category`, locale: 'en', name: 'Detail category' }, 'deleteCategory')}`,
    '/channels/:id': async () => `/channels/${channelId}`,
    '/cms/attributes/:id': async () =>
      `/cms/attributes/${await made('mutation($i:CreateCMSAttributeInput!){ createCMSAttribute(input:$i){ id } }', { slug: `${RUN}-detail-cms-attr`, inputType: 'TEXT' }, 'deleteCMSAttribute')}`,
    '/cms/block-types/:id': async () =>
      `/cms/block-types/${await made('mutation($i:CreateCMSBlockTypeInput!){ createCMSBlockType(input:$i){ id } }', { key: `${RUN}-detail-block`, name: 'Detail block', fields: [] }, 'deleteCMSBlockType')}`,
    '/cms/forms/:id': () => or((id) => `/cms/forms/${id}`, seededWebForm(), 'a web form could not be created'),
    '/cms/forms/:id/submissions': () => or((id) => `/cms/forms/${id}/submissions`, seededWebForm(), 'a web form could not be created'),
    '/cms/page-types/:id': async () =>
      `/cms/page-types/${await made('mutation($i:CreateCMSPageTypeInput!){ createCMSPageType(input:$i){ id } }', { slug: `${RUN}-detail-page-type`, locale: 'en', name: 'Detail page type' }, 'deleteCMSPageType')}`,
    '/cms/pages/:id': async () => {
      const typeId = await made('mutation($i:CreateCMSPageTypeInput!){ createCMSPageType(input:$i){ id } }', { slug: `${RUN}-detail-page-type-2`, locale: 'en', name: 'Detail page type 2' }, 'deleteCMSPageType')
      return `/cms/pages/${await made('mutation($i:CreateCMSPageInput!){ createCMSPage(input:$i){ id } }', { pageTypeId: typeId, slug: `${RUN}-detail-page`, locale: 'en', title: 'Detail page' }, 'deleteCMSPage')}`
    },
    '/collections/:id': async () =>
      `/collections/${await made('mutation($i:CreateCollectionInput!){ createCollection(input:$i){ id } }', { slug: `${RUN}-detail-collection`, locale: 'en', name: 'Detail collection' }, 'deleteCollection')}`,
    '/countries/:code': async () => '/countries/DE',
    '/currencies/:code': async () => `/currencies/${(await first<{ code: string }>('{ currencies { code } }'))?.code}`,
    '/customers/:id': async () => `/customers/${await seededCustomer()}`,
    '/customers/groups/:id': async () =>
      `/customers/groups/${await made('mutation($i:CreateCustomerGroupInput!){ createCustomerGroup(input:$i){ id } }', { slug: `${RUN}-detail-group`, name: 'Detail group' }, 'deleteCustomerGroup')}`,
    '/emails/:id': () =>
      or((id) => `/emails/${id}`, first<{ id: string }>('query($c:ID!){ libraryEmails(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no library email'),
    '/emails/blocks/:slug': () =>
      or((slug) => `/emails/blocks/${slug}`, first<{ slug: string }>('query($c:ID!){ emailBlocks(channelId:$c){ slug } }', { c: channelId }).then((r) => r?.slug), 'the dev data has no email block'),
    '/flows/:id': () =>
      or((id) => `/flows/${id}`, first<{ id: string }>('query($c:ID!){ flows(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no flow'),
    '/forms/:id': () =>
      or((id) => `/forms/${id}`, first<{ id: string }>('query($c:ID!){ signupForms(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no signup form'),
    '/integrations/:id': async () =>
      `/integrations/${await made('mutation($i:CreateIntegrationInput!){ createIntegration(input:$i){ integration { id } } }', { name: `${RUN} detail integration`, permissionSlugs: ['customers.view'] }, 'deleteIntegration')}`,
    '/inventory/locations/:id': async () =>
      `/inventory/locations/${await made('mutation($i:CreateInventoryLocationInput!){ createInventoryLocation(input:$i){ id } }', { slug: `${RUN}-detail-location`, name: 'Detail location' }, 'deleteInventoryLocation')}`,
    '/lens/sessions/:id': () =>
      or((id) => `/lens/sessions/${id}`, first<{ id: string }>('query($c:ID!){ lensSessions(channelId:$c, limit:1){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no recorded session'),
    '/locales/:code': async () => '/locales/en',
    '/menus/:id': async () => `/menus/${await made('mutation($i:CreateMenuInput!){ createMenu(input:$i){ id } }', { slug: `${RUN}-detail-menu`, name: 'Detail menu' }, 'deleteMenu')}`,
    '/orders/:id': () =>
      or((id) => `/orders/${id}`, first<{ id: string }>('query($c:ID){ orders(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no order'),
    '/prism/:id': () =>
      or((id) => `/prism/${id}`, first<{ id: string }>('query($c:ID!){ experiments(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no experiment'),
    '/products/:id': async () => {
      const { productTypes } = await adminApi<{ productTypes: { id: string }[] }>('{ productTypes { id } }')
      return `/products/${await made('mutation($i:CreateProductInput!){ createProduct(input:$i){ id } }', { productTypeId: productTypes[0]?.id, kind: 'SIMPLE', sku: `${RUN}-detail-product`, locale: 'en', name: 'Detail product', defaultPrice: 1500, defaultCurrencyCode: currencyCode }, 'deleteProduct')}`
    },
    '/promotions/:id': async () =>
      `/promotions/${await made('mutation($i:CreatePromotionRuleInput!){ createPromotionRule(input:$i){ id } }', { slug: `${RUN}-detail-rule`, name: 'Detail rule', isActive: false }, 'deletePromotionRule')}`,
    '/reviews/:id': () =>
      or((id) => `/reviews/${id}`, first<{ id: string }>('{ reviews(limit:1){ items { id } } }').then((r) => r?.id), 'the dev data has no review'),
    '/reviews/questions/:id': async () =>
      `/reviews/questions/${await made('mutation($i:CreateReviewQuestionInput!){ createReviewQuestion(input:$i){ id } }', { slug: `${RUN}-detail-question`, inputType: 'YES_NO', isRequired: false, allowComment: false, sortOrder: 0, locale: 'en', label: 'Detail question' }, 'deleteReviewQuestion')}`,
    '/segments/:id': async () => `/segments/${await seededSegment()}`,
    '/settings/:sectionSlug/:groupSlug?': async () => {
      const section = await first<{ slug: string; groups: { slug: string }[] }>('{ configSections { slug groups { slug } } }')
      return section ? `/settings/${section.slug}/${section.groups[0]?.slug ?? ''}` : skip('no config sections')
    },
    '/settings/email/:key': async () => skip('a redirect to /emails - the fixed-path walk opens the target'),
    '/shipping/:id': () =>
      or((id) => `/shipping/${id}`, first<{ id: string }>('query($c:ID!){ shippingZones(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'no shipping zone the e2e admin can read'),
    '/synapse/customers/:id': async () => `/synapse/customers/${await seededCustomer()}`,
    // Behavioural segments only - a plain customer segment is refused there, by design
    '/synapse/segments/:id': () =>
      or((id) => `/synapse/segments/${id}`, first<{ id: string }>('query($c:ID!){ synapseSegments(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'the dev data has no behavioural segment'),
    '/synapse/visitors/:anonId': () =>
      or((id) => `/synapse/visitors/${id}`, first<{ anonId: string }>('query($c:ID!){ lensSessions(channelId:$c, limit:1){ anonId } }', { c: channelId }).then((r) => r?.anonId), 'the dev data has no recorded visitor'),
    '/tax/:id': () =>
      or((id) => `/tax/${id}`, first<{ id: string }>('query($c:ID!){ taxRules(channelId:$c){ id } }', { c: channelId }).then((r) => r?.id), 'no tax rule the e2e admin can read'),
    '/webhooks/:id': async () =>
      `/webhooks/${await made('mutation($i:CreateWebhookInput!){ createWebhook(input:$i){ webhook { id } } }', { name: `${RUN} detail webhook`, targetUrl: 'https://example.invalid/hook', kind: 'ASYNC', eventTypes: ['order.placed'] }, 'deleteWebhook')}`,
  }
}

const DETAIL_ROUTES = [...new Set([...source.matchAll(/path: '(\/[^']*)'/g)].map((m) => m[1]))].filter((p) => p.includes(':'))

test('every detail page renders without an error', async ({ page }) => {
  test.setTimeout(DETAIL_ROUTES.length * 15_000)
  const { channels } = await adminApi<{ channels: { id: string; currencyCode: string }[] }>('{ channels { id currencyCode } }')
  const resolvers = detailResolvers(channels[0].id, channels[0].currencyCode)

  await startClean(page)
  const seen = watch(page)
  await seen.settle()
  seen.drain()

  const skipped: string[] = []
  for (const route of DETAIL_ROUTES) {
    await test.step(route, async () => {
      const resolve = resolvers[route]
      expect.soft(resolve, `${route} has no resolver - add one to detailResolvers so the page gets opened`).toBeDefined()
      if (!resolve) return
      // A resolver that can't make its record fails that route, not the walk
      const target = await resolve().catch((e: Error) => {
        expect.soft(e.message, `${route} could not get a record to open`).toBe('')
        return null
      })
      if (target === null) return
      if (typeof target !== 'string') {
        skipped.push(`${route}: ${target.skip}`)
        return
      }
      expect.soft(target, `${route} resolved to a path with an empty segment`).not.toMatch(/\/(undefined|null)?(\/|$)/)
      await visit(page, seen, target)
    })
  }
  test.info().annotations.push(...skipped.map((description) => ({ type: 'skipped route', description })))
})
