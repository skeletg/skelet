import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CmsAttributeCreate } from './CmsAttributeCreate'
import { CmsAttributesDocument, CreateCmsAttributeDocument, UpsertCmsAttributeContentDocument } from './Cms.gql'
import { emptyNewCmsAttribute, newCmsAttributeSchema, toNewCmsAttributeInput } from './schemas/cmsAttribute'

describe('CMS attribute schema', () => {
  it('needs a target for a relationship, and keeps a unit to numbers', () => {
    const base = { ...emptyNewCmsAttribute, slug: 'hero', name: 'Hero' }
    expect(newCmsAttributeSchema.safeParse({ ...base, inputType: 'RELATIONSHIP' }).error?.issues[0].path).toEqual(['target'])
    expect(toNewCmsAttributeInput({ ...base, unit: 'px' }).unit).toBeNull()
    expect(toNewCmsAttributeInput({ ...base, inputType: 'NUMBER', unit: 'px' }).unit).toBe('px')
  })
})

describe('CmsAttributeCreate', () => {
  it('creates the definition, then names it for the editing locale', async () => {
    const onCreate = vi.fn()
    const onName = vi.fn()
    renderPage({
      path: '/cms/attributes/new',
      routes: { '/cms/attributes/new': <CmsAttributeCreate />, '/cms/attributes/:id': <p>attribute page</p> },
      mocks: [
        meMock(),
        {
          request: { query: CreateCmsAttributeDocument, variables: captureInput(onCreate) },
          result: { data: { createCMSAttribute: { __typename: 'CMSAttribute', id: 'ca1' } } },
        },
        {
          request: { query: UpsertCmsAttributeContentDocument, variables: captureInput(onName) },
          result: { data: { upsertCMSAttributeContent: { __typename: 'CMSAttribute', id: 'ca1' } } },
        },
        { request: { query: CmsAttributesDocument, variables: () => true }, result: { data: { cmsAttributes: [] } }, maxUsageCount: 5 },
      ],
    })
    fireEvent.change(await screen.findByLabelText('Slug'), { target: { value: 'hero_headline' } })
    fireEvent.change(screen.getByLabelText('Display name (en)'), { target: { value: 'Hero headline' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Create attribute' }))
    expect(await screen.findByText('attribute page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({ slug: 'hero_headline', inputType: 'TEXT', relationshipTarget: null }))
    expect(onName).toHaveBeenCalledWith({ attributeId: 'ca1', locale: 'en', name: 'Hero headline' })
  })
})
