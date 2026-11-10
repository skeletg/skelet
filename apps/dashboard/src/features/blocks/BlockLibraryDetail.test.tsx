import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { DeleteEmailBlockDocument, EmailBlocksDocument } from '@/features/email/Email.gql'
import { BlockLibraryDetail } from './BlockLibraryDetail'

// What this pins: the block delete confirm runs through the shared ConfirmDialog without waiting on the server -
// it closes as soon as it is confirmed, then goes back to the block list once the delete lands

const channelScope = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'ch1', locale: '' }, isGlobal: false, isChannel: true, effectiveChannelId: 'ch1' }
const hero = { __typename: 'EmailBlockSnippet', channelId: 'ch1', slug: 'hero', name: 'Hero', icon: null, fields: [], blocks: [], htmlTemplate: null, textTemplate: null, css: null, scripted: false }

describe('BlockLibraryDetail delete', () => {
  it('closes at once, then goes back to the list', async () => {
    renderPage({
      path: '/emails/blocks/hero',
      adminId: 'admin-1',
      scope: channelScope,
      routes: { '/emails/blocks/:slug': <BlockLibraryDetail />, '/emails/blocks': <p>Blocks list</p> },
      mocks: [
        meMock(),
        { request: { query: EmailBlocksDocument, variables: { channelId: 'ch1' } }, result: { data: { emailBlocks: [hero] } }, maxUsageCount: 10 },
        { request: { query: DeleteEmailBlockDocument, variables: { channelId: 'ch1', slug: 'hero' } }, result: { data: { deleteEmailBlock: true } }, delay: 500 },
      ],
    })
    // Delete sits behind emails.manage - wait for the grants before clicking it
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
    // Well inside the server's 500ms - an awaited delete would still be open
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), { timeout: 200 })
    expect(await screen.findByText('Blocks list', {}, SLOW)).toBeDefined()
  })
})

// What this pins: a new block's name and slug sit under Settings, which a selected canvas block covers - so a save
// without them brings Settings back with the cursor in the missing field, and the slug follows the name until it's
// typed by hand
describe('BlockLibraryDetail new block', () => {
  it('points a nameless save at the name, and slugs the name', async () => {
    renderPage({
      path: '/emails/blocks/new',
      adminId: 'admin-1',
      scope: channelScope,
      routes: { '/emails/blocks/new': <BlockLibraryDetail /> },
      mocks: [meMock(), { request: { query: EmailBlocksDocument, variables: { channelId: 'ch1' } }, result: { data: { emailBlocks: [hero] } }, maxUsageCount: 10 }],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Add Heading' }, SLOW))
    expect(screen.getByRole('tab', { name: 'Block', selected: true })).toBeDefined()
    fireEvent.change(screen.getAllByRole('textbox', { name: 'Heading text' })[0], { target: { value: 'Big sale' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Give the block a name.')).toBeDefined()
    expect(screen.getByRole('tab', { name: 'Settings', selected: true })).toBeDefined()
    const name = screen.getByLabelText('Block name')
    await waitFor(() => expect(document.activeElement).toBe(name))
    fireEvent.change(name, { target: { value: 'Summer Sale Banner' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('summer-sale-banner')
    expect(screen.queryByText('Give the block a name.')).toBeNull()
    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'hero-2' } })
    fireEvent.change(name, { target: { value: 'Summer Sale Banner 2' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('hero-2')
  })
})
