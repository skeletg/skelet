import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { CmsPageTypeCreate } from './CmsPageTypeCreate'
import { CmsPageTypesDocument, CreateCmsPageTypeDocument } from './Cms.gql'

describe('CmsPageTypeCreate', () => {
  it('derives the slug, seeds the name for the editing locale and opens the new type', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/cms/page-types/new',
      routes: { '/cms/page-types/new': <CmsPageTypeCreate />, '/cms/page-types/:id': <p>page type page</p> },
      mocks: [
        {
          request: { query: CreateCmsPageTypeDocument, variables: captureInput(onCreate) },
          result: { data: { createCMSPageType: { __typename: 'CMSPageType', id: 'pt1' } } },
        },
        { request: { query: CmsPageTypesDocument, variables: () => true }, result: { data: { cmsPageTypes: [] } }, maxUsageCount: 3 },
      ],
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create page type' }))
    expect(await screen.findByText('Name is required')).toBeDefined()

    fireEvent.change(screen.getByLabelText('Name (en)'), { target: { value: 'Landing page' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('landing-page')
    fireEvent.click(screen.getByRole('button', { name: 'Create page type' }))
    expect(await screen.findByText('page type page', {}, SLOW)).toBeDefined()
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ slug: 'landing-page', name: 'Landing page', locale: 'en', description: null, icon: null }),
    )
  })
})
