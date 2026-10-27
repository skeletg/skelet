import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { CmsBlockTypesPage } from './CmsBlockTypesPage'
import { CmsTagsPage } from './CmsTagsPage'
import { CmsBlockTypesDocument, CmsTagsDocument, DeleteCmsBlockTypeDocument, DeleteCmsTagDocument } from './Cms.gql'

const { toastError } = vi.hoisted(() => ({ toastError: vi.fn() }))
vi.mock('sonner', () => ({ toast: { error: toastError, success: vi.fn() } }))

// What this pins: the block type and tag deletes run through the shared ConfirmDialog without waiting on the
// server - the dialog closes as soon as it is confirmed, and a block type's failure still lands in a toast

async function confirm(label: string) {
  fireEvent.click(await screen.findByRole('button', { name: label }, SLOW))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
}

describe('CMS library deletes', () => {
  it('closes a block type delete at once and toasts its failure', async () => {
    const hero = { __typename: 'CMSBlockType', id: 'bt1', key: 'hero', name: 'Hero', icon: null, fields: [], htmlTemplate: '', updatedAt: '2026-10-01T00:00:00Z' }
    renderPage({
      path: '/cms/block-types',
      adminId: 'admin-1',
      routes: { '/cms/block-types': <CmsBlockTypesPage /> },
      mocks: [
        meMock(),
        { request: { query: CmsBlockTypesDocument }, result: { data: { cmsBlockTypes: [hero] } }, maxUsageCount: 5 },
        { request: { query: DeleteCmsBlockTypeDocument, variables: { id: 'bt1' } }, result: { errors: [{ message: 'type in use' }] }, delay: 500 },
      ],
    })
    await confirm('Delete Hero')
    expect(toastError).not.toHaveBeenCalled() // closed before the server answered
    await waitFor(() => expect(toastError).toHaveBeenCalledWith('type in use'))
  })

  it('closes a tag delete at once and sends it', async () => {
    const sale = { __typename: 'CMSTag', slug: 'sale', name: 'Sale', pageCount: 2, createdAt: '2026-10-01T00:00:00Z' }
    const deleted = vi.fn(() => ({ data: { deleteCMSTag: true } }))
    renderPage({
      path: '/cms/tags',
      routes: { '/cms/tags': <CmsTagsPage /> },
      mocks: [
        meMock(),
        { request: { query: CmsTagsDocument }, result: { data: { cmsTags: [sale] } }, maxUsageCount: 5 },
        { request: { query: DeleteCmsTagDocument, variables: { slug: 'sale' } }, result: deleted, delay: 50 },
      ],
    })
    await confirm('Delete Sale')
    await waitFor(() => expect(deleted).toHaveBeenCalled())
  })
})
