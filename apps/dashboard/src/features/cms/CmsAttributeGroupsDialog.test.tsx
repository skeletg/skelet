import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { CmsAttributeGroupsDialog } from './CmsAttributeGroupsDialog'
import { CmsAttributeGroupsDocument, DeleteCmsAttributeGroupDocument } from './Cms.gql'

// What this pins: deleting a group ungroups every page-type field under it, so it asks first and sends nothing on cancel

const group = {
  __typename: 'CMSAttributeGroup' as const,
  id: 'g1',
  slug: 'seo',
  position: 0,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

describe('CmsAttributeGroupsDialog delete', () => {
  it('confirms before deleting a group', async () => {
    const removed = vi.fn()
    renderDialog(<CmsAttributeGroupsDialog trigger={<button type="button">Groups</button>} />, [
      { request: { query: CmsAttributeGroupsDocument }, result: { data: { cmsAttributeGroups: [group] } }, maxUsageCount: 5 },
      { request: { query: DeleteCmsAttributeGroupDocument, variables: captureInput(removed) }, result: { data: { deleteCMSAttributeGroup: true } } },
    ])
    fireEvent.click(screen.getByRole('button', { name: 'Groups' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Delete group seo' }, SLOW))
    let confirm = await screen.findByRole('alertdialog')
    fireEvent.click(within(confirm).getByRole('button', { name: 'Cancel' }))
    await vi.waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), SLOW)
    expect(removed).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Delete group seo' }))
    confirm = await screen.findByRole('alertdialog')
    fireEvent.click(within(confirm).getByRole('button', { name: 'Delete group' }))
    await vi.waitFor(() => expect(removed).toHaveBeenCalledWith({ id: 'g1' }), SLOW)
  })
})
