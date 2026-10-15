import { expect, type Page } from '@playwright/test'
import { adminApi, saveThroughDialog, test } from './helpers'
import { RUN, adminCan, cleanup, expectOnlyChanged, firstChannel, lexical, open, openScoped, pageQuery, registerCleanups } from './fixtures'

// Every record page on the staged-save forms, round-tripped against the real API: seed a record with
// every optional field set to something other than its default, change ONE field through the page,
// and read the whole record back through the query the page itself renders from. Anything else that
// moved is a lossy mapping somewhere between the stored record, the form's baseline and the mutation
// input - a null saved back as "", cents drifting, a date shifted by the browser's offset, a list the
// update dropped. The unit tests' mocked Apollo can't see any of those
//
// Needs the dev stack up with Signals (see README.md)

registerCleanups()

// A zone away from UTC, so a date that shifts by the offset on each save shows up
test.use({ timezoneId: 'America/New_York' })

const del = (mutation: string, id: string, arg = 'id', type = 'ID!') => () => adminApi(`mutation($v:${type}){ ${mutation}(${arg}:$v) }`, { v: id })

/** The page's own query for one record, unwrapped to the record */
async function readRecord(op: string, vars: Record<string, unknown>): Promise<unknown> {
  const data = await adminApi(pageQuery(op), vars)
  return Object.values(data)[0]
}

/** The page's own list query, narrowed to the one row the spec seeded */
async function readRow(op: string, id: string): Promise<unknown> {
  const rows = (await readRecord(op, {})) as { id: string }[]
  const row = rows.find((r) => r.id === id)
  if (!row) throw new Error(`${op} has no row ${id}`)
  return row
}

/** Waits for the seeded value to be on screen before typing - editing a still-loading form proves nothing */
async function replace(page: Page, selector: string, from: string, to: string) {
  const input = page.locator(selector)
  await expect(input).toHaveValue(from)
  await input.fill(to)
}

type Case = {
  name: string
  /** Creates the record, registers its cleanup, returns the route key (an id or a code) */
  seed: () => Promise<string>
  path: (key: string) => string
  read: (key: string) => Promise<unknown>
  edit: (page: Page) => Promise<void>
  /** Rows the review dialog must list */
  rows: (string | RegExp)[]
  /** Paths in the read-back that the edit is allowed to move */
  changed: string[]
  /** Translation pages need a channel + locale picked in the header */
  scoped?: boolean
  /** A permission the e2e admin may not hold - the case skips, saying so, rather than failing on a 403 */
  needs?: string
}

const cases: Case[] = [
  {
    name: 'admin',
    async seed() {
      const { createAdmin } = await adminApi<{ createAdmin: { id: string } }>('mutation($i:CreateAdminInput!){ createAdmin(input:$i){ id } }', {
        i: { email: `${RUN}-admin@e2e.test`, password: 'E2e-password-1234!', firstName: 'Seeded', lastName: 'Admin', roleSlugs: ['support_agent'] },
      })
      cleanup(del('deleteAdmin', createAdmin.id))
      return createAdmin.id
    },
    path: (id) => `/admins/${id}`,
    read: (id) => readRecord('Admin', { id }),
    edit: (page) => replace(page, '#ad-first', 'Seeded', 'Renamed'),
    rows: ['First name'],
    changed: ['firstName'],
  },
  {
    name: 'role keeps every scope, channel and locale',
    async seed() {
      const channel = await firstChannel()
      const { createRole } = await adminApi<{ createRole: { id: string } }>('mutation($i:CreateRoleInput!){ createRole(input:$i){ id } }', {
        i: {
          name: 'Seeded role',
          slug: `${RUN}-role`,
          description: 'Seeded description',
          scopes: [
            { permissionSlug: 'customers.view' },
            { permissionSlug: 'blog.view', channelId: channel.id },
            { permissionSlug: 'cms.view', channelId: channel.id, locale: 'en' },
          ],
        },
      })
      cleanup(del('deleteRole', createRole.id))
      return createRole.id
    },
    path: (id) => `/admins/roles/${id}/edit`,
    read: (id) => readRecord('Role', { id }),
    edit: (page) => replace(page, '#role-name', 'Seeded role', 'Renamed role'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'customer keeps the fields other mutations own',
    async seed() {
      const channel = await firstChannel()
      const { adminCreateCustomer } = await adminApi<{ adminCreateCustomer: { id: string } }>(
        'mutation($i:AdminCreateCustomerInput!){ adminCreateCustomer(input:$i){ id } }',
        {
          i: {
            channelId: channel.id,
            email: `${RUN}-customer@e2e.test`,
            password: 'E2e-password-1234!',
            firstName: 'Seeded',
            lastName: 'Customer',
            phone: '+381601234567',
            isActive: true,
            isEmailVerified: true,
          },
        },
      )
      const id = adminCreateCustomer.id
      cleanup(del('deleteCustomer', id))
      await adminApi(
        `mutation($id:ID!,$a:AddressInput!,$u:AdminUpdateCustomerInput!){
          adminAddAddress(customerId:$id, input:$a){ id }
          adminUpdateCustomer(id:$id, input:$u){ id }
          setCustomerTaxStatus(customerId:$id, taxExempt:true, note:"Seeded exemption"){ id }
        }`,
        {
          id,
          // In the dev store's own country - an address in one it doesn't sell to is refused
          a: { firstName: 'Seeded', lastName: 'Customer', company: 'Seeded Co', street1: 'Seeded street 1', street2: 'Floor 2', city: 'New York', state: 'NY', postalCode: '10118', countryCode: 'US', phone: '+381601234567' },
          u: { note: 'Seeded note', metadata: [{ key: 'seeded', value: 'yes' }] },
        },
      )
      return id
    },
    path: (id) => `/customers/${id}`,
    read: (id) => readRecord('Customer', { id }),
    edit: (page) => replace(page, '#cd-first', 'Seeded', 'Renamed'),
    rows: ['First name'],
    changed: ['firstName'],
  },
  {
    name: 'integration keeps its permissions and expiry',
    async seed() {
      const { createIntegration } = await adminApi<{ createIntegration: { integration: { id: string } } }>(
        'mutation($i:CreateIntegrationInput!){ createIntegration(input:$i){ integration { id } } }',
        { i: { name: 'Seeded integration', description: 'Seeded description', permissionSlugs: ['customers.view', 'orders.view'], expiresAt: '2031-03-01T12:00:00Z' } },
      )
      const id = createIntegration.integration.id
      cleanup(del('deleteIntegration', id))
      return id
    },
    path: (id) => `/integrations/${id}`,
    read: (id) => readRow('Integrations', id),
    edit: (page) => replace(page, '#int-desc', 'Seeded description', 'New description'),
    rows: ['Description'],
    changed: ['description'],
  },
  {
    name: 'customer segment',
    async seed() {
      const { createCustomerSegment } = await adminApi<{ createCustomerSegment: { id: string } }>(
        'mutation($i:CreateCustomerSegmentInput!){ createCustomerSegment(input:$i){ id } }',
        { i: { slug: `${RUN}-segment`, name: 'Seeded segment', description: 'Seeded description', isActive: false } },
      )
      cleanup(del('deleteCustomerSegment', createCustomerSegment.id))
      return createCustomerSegment.id
    },
    path: (id) => `/segments/${id}`,
    read: (id) => readRecord('SegmentDetail', { id }),
    edit: (page) => replace(page, '#seg-name', 'Seeded segment', 'Renamed segment'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'review question keeps the other locale',
    async seed() {
      // A run stopped part way leaves its question and the locale behind - its cleanup never ran. The question goes
      // first: while its xz translation exists, the locale refuses to be deleted
      const { reviewQuestions } = await adminApi<{ reviewQuestions: { id: string; slug: string }[] }>('{ reviewQuestions { id slug } }')
      for (const q of reviewQuestions.filter((q) => q.slug.startsWith('e2e-forms-') && q.slug.endsWith('-question'))) {
        await adminApi('mutation($id:ID!){ deleteReviewQuestion(id:$id) }', { id: q.id }).catch(() => undefined)
      }
      await adminApi('mutation($c:String!){ deleteLocale(code:$c) }', { c: 'xz' }).catch(() => undefined)
      await adminApi('mutation($i:CreateLocaleInput!){ createLocale(input:$i){ code } }', { i: { code: 'xz', name: 'E2E review locale', isActive: true, sortOrder: 99 } })
      cleanup(del('deleteLocale', 'xz', 'code', 'String!'))
      const { createReviewQuestion } = await adminApi<{ createReviewQuestion: { id: string } }>(
        'mutation($i:CreateReviewQuestionInput!){ createReviewQuestion(input:$i){ id } }',
        { i: { slug: `${RUN}-question`, inputType: 'YES_NO', isRequired: true, allowComment: true, sortOrder: 7, locale: 'en', label: 'Seeded label', helpText: 'Seeded help' } },
      )
      const id = createReviewQuestion.id
      cleanup(del('deleteReviewQuestion', id))
      await adminApi('mutation($i:UpsertReviewQuestionContentInput!){ upsertReviewQuestionContent(input:$i){ id } }', {
        i: { questionId: id, locale: 'xz', label: 'Other label', helpText: 'Other help' },
      })
      return id
    },
    path: (id) => `/reviews/questions/${id}`,
    read: (id) => readRecord('ReviewQuestion', { id }),
    async edit(page) {
      const label = page.getByLabel('Label (en)')
      await expect(label).toHaveValue('Seeded label')
      await label.fill('Renamed label')
    },
    rows: [/Renamed label/],
    changed: ['allContent', 'label', 'content'],
  },
  {
    name: 'attribute keeps its flags and metadata',
    async seed() {
      const { createAttribute } = await adminApi<{ createAttribute: { id: string } }>('mutation($i:CreateAttributeInput!){ createAttribute(input:$i){ id } }', {
        i: {
          slug: `${RUN}-attr`,
          name: 'Seeded attribute',
          inputType: 'PLAIN_TEXT',
          unit: 'cm',
          valueRequired: true,
          filterable: true,
          searchable: true,
          isPublic: true,
          facetCollapsed: true,
          isIndexable: true,
          h1Position: 'AFTER',
          sortOrder: 9,
          defaultValue: 'Seeded default',
          metadata: [{ key: 'seeded', value: 'yes' }],
        },
      })
      cleanup(del('deleteAttribute', createAttribute.id))
      return createAttribute.id
    },
    path: (id) => `/catalog/attributes/${id}`,
    read: (id) => readRecord('Attribute', { id }),
    edit: (page) => replace(page, '#s-name', 'Seeded attribute', 'Renamed attribute'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'product type keeps its flags and URL template',
    async seed() {
      const { createProductType } = await adminApi<{ createProductType: { id: string } }>('mutation($i:CreateProductTypeInput!){ createProductType(input:$i){ id } }', {
        i: { slug: `${RUN}-type`, name: 'Seeded type', isDigital: true, isShippingRequired: false, urlTemplate: 'e2e/{name}', tracksInventory: false, metadata: [{ key: 'seeded', value: 'yes' }] },
      })
      cleanup(del('deleteProductType', createProductType.id))
      return createProductType.id
    },
    path: (id) => `/catalog/product-types/${id}`,
    read: (id) => readRecord('ProductType', { id }),
    edit: (page) => replace(page, '#pt-name', 'Seeded type', 'Renamed type'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'CMS attribute keeps its validation',
    async seed() {
      const { createCMSAttribute } = await adminApi<{ createCMSAttribute: { id: string } }>('mutation($i:CreateCMSAttributeInput!){ createCMSAttribute(input:$i){ id } }', {
        i: {
          slug: `${RUN}-cms-attr`,
          inputType: 'TEXT',
          isTranslatable: true,
          isRequiredDefault: true,
          unit: 'px',
          validation: { minLength: 2, maxLength: 40 },
          defaultValue: 'Seeded default',
          metadata: [{ key: 'seeded', value: 'yes' }],
        },
      })
      cleanup(del('deleteCMSAttribute', createCMSAttribute.id))
      return createCMSAttribute.id
    },
    path: (id) => `/cms/attributes/${id}`,
    read: (id) => readRow('CmsAttributes', id),
    edit: (page) => replace(page, '#attr-default', 'Seeded default', 'New default'),
    rows: ['Default value'],
    changed: ['defaultValue'],
  },
  {
    name: 'CMS page type',
    async seed() {
      const { createCMSPageType } = await adminApi<{ createCMSPageType: { id: string } }>('mutation($i:CreateCMSPageTypeInput!){ createCMSPageType(input:$i){ id } }', {
        i: { slug: `${RUN}-page-type`, icon: 'file', position: 4, locale: 'en', name: 'Seeded page type', description: 'Seeded description' },
      })
      cleanup(del('deleteCMSPageType', createCMSPageType.id))
      return createCMSPageType.id
    },
    path: (id) => `/cms/page-types/${id}`,
    read: (id) => readRow('CmsPageTypes', id),
    edit: (page) => replace(page, '#pt-position', '4', '6'),
    rows: ['Position'],
    changed: ['position'],
    scoped: true,
  },
  {
    name: 'CMS block type keeps its template',
    async seed() {
      const { createCMSBlockType } = await adminApi<{ createCMSBlockType: { id: string } }>('mutation($i:CreateCMSBlockTypeInput!){ createCMSBlockType(input:$i){ id } }', {
        i: { key: `${RUN}-block`, name: 'Seeded block', icon: 'box', fields: [], htmlTemplate: '<p>Seeded template</p>' },
      })
      cleanup(del('deleteCMSBlockType', createCMSBlockType.id))
      return createCMSBlockType.id
    },
    path: (id) => `/cms/block-types/${id}`,
    read: (id) => readRecord('CmsBlockType', { id }),
    edit: (page) => replace(page, '#bt-name', 'Seeded block', 'Renamed block'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'blog author keeps the bio',
    async seed() {
      const { createBlogAuthor } = await adminApi<{ createBlogAuthor: { id: string } }>('mutation($i:CreateBlogAuthorInput!){ createBlogAuthor(input:$i){ id } }', {
        i: { slug: `${RUN}-author`, displayName: 'Seeded author', email: 'seeded-author@e2e.test', locale: 'en', name: 'Seeded author', bio: lexical('Seeded bio'), metadata: [{ key: 'seeded', value: 'yes' }] },
      })
      cleanup(del('deleteBlogAuthor', createBlogAuthor.id))
      return createBlogAuthor.id
    },
    path: (id) => `/blog/authors/${id}`,
    read: (id) => readRecord('BlogAuthor', { id }),
    edit: (page) => replace(page, '#display-name', 'Seeded author', 'Renamed author'),
    rows: ['Display name'],
    // The translation's name follows the display name by design - the bio must not move
    changed: ['displayName', 'content.name', 'allContent[0].name'],
    scoped: true,
  },
  {
    name: 'blog category keeps its meta copy',
    async seed() {
      const { createBlogCategory } = await adminApi<{ createBlogCategory: { id: string } }>('mutation($i:CreateBlogCategoryInput!){ createBlogCategory(input:$i){ id } }', {
        i: {
          slug: `${RUN}-category`,
          position: 3,
          isPublished: true,
          seoNoindex: true,
          locale: 'en',
          name: 'Seeded category',
          description: lexical('Seeded description'),
          metaTitle: 'Seeded meta',
          metaDescription: 'Seeded meta description',
          metaKeywords: 'seeded, keywords',
        },
      })
      cleanup(del('deleteBlogCategory', createBlogCategory.id))
      return createBlogCategory.id
    },
    path: (id) => `/blog/categories/${id}`,
    read: (id) => readRecord('BlogCategory', { id }),
    edit: (page) => replace(page, '#name', 'Seeded category', 'Renamed category'),
    rows: ['Name'],
    changed: ['name', 'content', 'allContent'],
    scoped: true,
  },
  {
    name: 'blog tag keeps its meta copy',
    async seed() {
      const { createBlogTag } = await adminApi<{ createBlogTag: { id: string } }>('mutation($i:CreateBlogTagInput!){ createBlogTag(input:$i){ id } }', {
        i: { slug: `${RUN}-tag`, isPublished: true, locale: 'en', name: 'Seeded tag', description: lexical('Seeded description'), metaTitle: 'Seeded meta', metaDescription: 'Seeded meta description', metaKeywords: 'seeded' },
      })
      cleanup(del('deleteBlogTag', createBlogTag.id))
      return createBlogTag.id
    },
    path: (id) => `/blog/tags/${id}`,
    read: (id) => readRecord('BlogTag', { id }),
    edit: (page) => replace(page, '#name', 'Seeded tag', 'Renamed tag'),
    rows: ['Name'],
    changed: ['name', 'content', 'allContent'],
    scoped: true,
  },
  {
    name: 'country',
    async seed() {
      await adminApi('mutation($c:String!){ deleteCountry(code:$c) }', { c: 'XZ' }).catch(() => undefined)
      await adminApi('mutation($i:CreateCountryInput!){ createCountry(input:$i){ code } }', {
        i: { code: 'XZ', name: 'E2E Land', nativeName: 'E2E Native', emojiFlag: '🏳', phoneCode: '+999', defaultCurrencyCode: 'EUR', isActive: false, sortOrder: 99, metadata: [{ key: 'seeded', value: 'yes' }] },
      })
      cleanup(del('deleteCountry', 'XZ', 'code', 'String!'))
      return 'XZ'
    },
    path: (code) => `/countries/${code}`,
    read: (code) => readRecord('Country', { code }),
    edit: (page) => replace(page, '#country-name', 'E2E Land', 'E2E Renamed'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'currency',
    async seed() {
      await adminApi('mutation($c:String!){ deleteCurrency(code:$c) }', { c: 'XZZ' }).catch(() => undefined)
      await adminApi('mutation($i:CreateCurrencyInput!){ createCurrency(input:$i){ code } }', {
        i: { code: 'XZZ', name: 'E2E coin', symbol: 'Ƶ', decimals: 3, symbolPosition: 'suffix', isActive: false, sortOrder: 99, metadata: [{ key: 'seeded', value: 'yes' }] },
      })
      cleanup(del('deleteCurrency', 'XZZ', 'code', 'String!'))
      return 'XZZ'
    },
    path: (code) => `/currencies/${code}`,
    read: (code) => readRecord('Currency', { code }),
    edit: (page) => replace(page, '#currency-name', 'E2E coin', 'E2E renamed coin'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'locale keeps its formats',
    async seed() {
      await adminApi('mutation($c:String!){ deleteLocale(code:$c) }', { c: 'xy' }).catch(() => undefined)
      await adminApi('mutation($i:CreateLocaleInput!){ createLocale(input:$i){ code } }', {
        i: {
          code: 'xy',
          name: 'E2E locale',
          nativeName: 'E2E native',
          isActive: false,
          isRTL: true,
          sortOrder: 98,
          dateFormat: 'dd.MM.yyyy',
          timeFormat: 'HH:mm',
          decimalSeparator: ',',
          thousandsSeparator: '.',
          firstDayOfWeek: 1,
          flagEmoji: '🏳',
          hreflangValue: 'xy',
          description: 'Seeded description',
          metadata: [{ key: 'seeded', value: 'yes' }],
        },
      })
      cleanup(del('deleteLocale', 'xy', 'code', 'String!'))
      return 'xy'
    },
    path: (code) => `/locales/${code}`,
    read: (code) => readRecord('Locale', { code }),
    edit: (page) => replace(page, '#locale-name', 'E2E locale', 'E2E renamed'),
    rows: ['Name'],
    changed: ['name'],
  },
  {
    name: 'tax rule',
    async seed() {
      const channel = await firstChannel()
      const { createTaxRule } = await adminApi<{ createTaxRule: { id: string } }>('mutation($i:CreateTaxRuleInput!){ createTaxRule(input:$i){ id } }', {
        i: { channelId: channel.id, name: 'Seeded tax rule', priority: 5, exclusivityGroup: 'e2e', sortOrder: 3, isActive: false },
      })
      cleanup(del('deleteTaxRule', createTaxRule.id))
      return createTaxRule.id
    },
    path: (id) => `/tax/${id}`,
    read: (id) => readRecord('TaxRule', { id }),
    edit: (page) => replace(page, '#name', 'Seeded tax rule', 'Renamed tax rule'),
    rows: ['Display name'],
    changed: ['name'],
    needs: 'settings.manage',
  },
  {
    name: 'shipping zone keeps its countries and methods',
    async seed() {
      const channel = await firstChannel()
      const { createShippingZone } = await adminApi<{ createShippingZone: { id: string } }>('mutation($i:CreateShippingZoneInput!){ createShippingZone(input:$i){ id } }', {
        i: { channelId: channel.id, code: `${RUN}-zone`, name: 'Seeded zone', sortOrder: 4, isActive: false, countries: ['DE', 'AT'] },
      })
      const id = createShippingZone.id
      cleanup(del('deleteShippingZone', id))
      await adminApi('mutation($i:CreateShippingMethodInput!){ createShippingMethod(input:$i){ id } }', {
        i: { zoneId: id, code: `${RUN}-method`, name: 'Seeded method', calcType: 'FREE_ABOVE', flatCents: 499, freeAboveCents: 5000, sortOrder: 2, isActive: true },
      })
      return id
    },
    path: (id) => `/shipping/${id}`,
    read: (id) => readRecord('ShippingZone', { id }),
    edit: (page) => replace(page, '#name', 'Seeded zone', 'Renamed zone'),
    rows: ['Display name'],
    changed: ['name'],
    needs: 'settings.manage',
  },
  {
    name: 'inventory location keeps its address and coordinates',
    async seed() {
      const { createInventoryLocation } = await adminApi<{ createInventoryLocation: { id: string } }>(
        'mutation($i:CreateInventoryLocationInput!){ createInventoryLocation(input:$i){ id } }',
        {
          i: {
            slug: `${RUN}-location`,
            name: 'Seeded location',
            isActive: false,
            type: 'warehouse',
            line1: 'Seeded street 1',
            line2: 'Dock 4',
            city: 'Novi Sad',
            region: 'Vojvodina',
            postalCode: '21000',
            countryCode: 'DE',
            latitude: 45.2671,
            longitude: 19.8335,
            timezone: 'Europe/Belgrade',
            email: 'seeded-location@e2e.test',
            phone: '+381601234567', // a valid number - phones are checked and stored as E.164
            contactName: 'Seeded contact',
            externalReference: 'EXT-1',
            metadata: [{ key: 'seeded', value: 'yes' }],
          },
        },
      )
      cleanup(del('deleteInventoryLocation', createInventoryLocation.id))
      return createInventoryLocation.id
    },
    path: (id) => `/inventory/locations/${id}`,
    read: (id) => readRecord('InventoryLocation', { id }),
    edit: (page) => replace(page, '#loc-name', 'Seeded location', 'Renamed location'),
    rows: ['Name'],
    changed: ['name'],
  },
]

for (const c of cases) {
  test(c.name, async ({ page }) => {
    if (c.needs) test.skip(!(await adminCan(c.needs, (await firstChannel()).id)), `the e2e admin lacks ${c.needs}`)
    const key = await c.seed()
    const before = await c.read(key)
    await (c.scoped ? openScoped : open)(page, c.path(key))
    await c.edit(page)
    await saveThroughDialog(page, c.rows)
    expectOnlyChanged(before, await c.read(key), c.changed)
  })
}
