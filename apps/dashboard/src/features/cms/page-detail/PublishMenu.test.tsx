import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import '@testing-library/jest-dom/vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { PublishCmsPageDocument, RejectCmsPageDocument } from '../Cms.gql'
import { PublishMenu } from './PublishMenu'
import { publishStatus, type PublishPage } from './shared'

// What these pin: the status reads the page's own flag and its channel listings together, and the menu's
// transitions apply at once - a reject carries its reason

const listing = (channelId: string, isPublished: boolean) => ({ __typename: 'CMSPageChannelListing', channelId, isPublished, isHomepage: false, seoNoindex: false })
const draft = { id: 'pg1', isPublished: false, scheduledAt: null, workflowState: 'DRAFT', assignedTo: null, channelListings: [] } as unknown as PublishPage
const page = (over: Record<string, unknown>) => ({ ...draft, ...over }) as PublishPage

const openMenu = () =>
  fireEvent.pointerDown(screen.getByRole('button', { name: /^Publishing:/ }), { button: 0, ctrlKey: false, pointerType: 'mouse' })

describe('publishStatus', () => {
  it('calls a page live only where it is published and listed', () => {
    expect(publishStatus(page({ isPublished: true, channelListings: [listing('c1', true), listing('c2', false)] })).label).toBe('Live on 1 channel')
    expect(publishStatus(page({ isPublished: true, channelListings: [listing('c1', false)] })).label).toBe('Published, on no channel')
    expect(publishStatus(page({ channelListings: [listing('c1', true)] })).label).toBe('Unpublished')
    expect(publishStatus(page({ scheduledAt: '2030-01-15T10:30:00Z' })).label).toMatch(/^Scheduled for /)
  })
})

describe('PublishMenu', () => {
  it('publishes straight from the menu', async () => {
    const onPublish = vi.fn()
    renderDialog(<PublishMenu page={draft} canApprove refetchVariables={{ id: 'pg1' }} />, [
      { request: { query: PublishCmsPageDocument, variables: captureInput(onPublish) }, result: { data: { publishCMSPage: null } } },
    ])
    openMenu()
    expect(screen.queryByRole('menuitem', { name: /Unpublish/ })).toBeNull()
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Publish now' }))
    await vi.waitFor(() => expect(onPublish).toHaveBeenCalledWith({ id: 'pg1' }), SLOW)
  })

  it('sends a rejection with its reason', async () => {
    const onReject = vi.fn()
    renderDialog(<PublishMenu page={page({ workflowState: 'IN_REVIEW' })} canApprove refetchVariables={{ id: 'pg1' }} />, [
      { request: { query: RejectCmsPageDocument, variables: captureInput(onReject) }, result: { data: { rejectCMSPage: null } } },
    ])
    openMenu()
    fireEvent.click(await screen.findByRole('menuitem', { name: 'Reject…' }))
    const dialog = await screen.findByRole('dialog', { name: 'Reject and return to draft' })
    fireEvent.change(within(dialog).getByLabelText('Reason'), { target: { value: ' Hero copy is off-brand ' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'Reject' }))
    await vi.waitFor(() => expect(onReject).toHaveBeenCalledWith({ id: 'pg1', note: 'Hero copy is off-brand' }), SLOW)
  })

  it('only lets an approver approve', async () => {
    renderDialog(<PublishMenu page={page({ workflowState: 'IN_REVIEW' })} canApprove={false} refetchVariables={{ id: 'pg1' }} />)
    openMenu()
    expect(await screen.findByRole('menuitem', { name: 'Awaiting an approver' })).toHaveAttribute('aria-disabled', 'true')
    expect(screen.queryByRole('menuitem', { name: 'Approve' })).toBeNull()
  })
})
