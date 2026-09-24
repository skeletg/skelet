import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CmsSnippetsPage } from './CmsSnippetsPage'
import { CmsSnippetsDocument, CreateCmsSnippetDocument, UpdateCmsSnippetDocument } from './Cms.gql'

// What these pin: a slug the backend would refuse is caught before anything is sent, and an edit
// writes the section's name and its blocks back as one document

const banner = {
  __typename: 'CMSSnippet' as const,
  id: 's1',
  slug: 'promo_banner',
  name: 'Promo banner',
  blocks: { version: 1, blocks: [] },
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

function renderSnippets(writes = { create: vi.fn(), update: vi.fn() }) {
  renderPage({
    path: '/cms/sections',
    routes: { '/cms/sections': <CmsSnippetsPage /> },
    mocks: [
      meMock(),
      { request: { query: CmsSnippetsDocument }, result: { data: { cmsSnippets: [banner] } }, maxUsageCount: 5 },
      {
        request: { query: CreateCmsSnippetDocument, variables: captureInput(writes.create) },
        result: { data: { createCMSSnippet: { ...banner, id: 's2' } } },
      },
      {
        request: { query: UpdateCmsSnippetDocument, variables: captureInput(writes.update) },
        result: { data: { updateCMSSnippet: banner } },
      },
    ],
  })
  return writes
}

describe('CmsSnippetsPage', () => {
  it('refuses a slug with spaces, then creates the section', async () => {
    const writes = renderSnippets()
    fireEvent.click(await screen.findByRole('button', { name: /New section/ }))
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Hero' } })
    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'Hero banner' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save section' }))
    expect(await screen.findByText(/Lowercase letters, digits/)).toBeDefined()
    expect(writes.create).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'hero-banner' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save section' }))
    await vi.waitFor(
      () => expect(writes.create).toHaveBeenCalledWith({ slug: 'hero-banner', name: 'Hero', blocks: { version: 1, blocks: [] } }),
      SLOW,
    )
  })

  it('writes an existing section back with its name and blocks', async () => {
    const writes = renderSnippets()
    fireEvent.click(await screen.findByText('Promo banner'))
    const save = await screen.findByRole('button', { name: 'Save section' })
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Promo strip' } })
    fireEvent.click(save)
    await vi.waitFor(
      () => expect(writes.update).toHaveBeenCalledWith({ id: 's1', name: 'Promo strip', blocks: { version: 1, blocks: [] } }),
      SLOW,
    )
  })
})
