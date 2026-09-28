import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderSection } from '@/test/page-harness'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { CmsTagsDocument, SetCmsPageChannelListingDocument, UpdateCmsPageDocument } from '../Cms.gql'
import { CmsPageTagsSection } from './TagsSection'
import { CmsPageChannelsSection } from './ChannelsSection'

// What these pin: tag picks and channel listings stage into the page's Save, and are written only
// for what changed

const tag = (slug: string, name: string) => ({ __typename: 'CMSTag' as const, slug, name, pageCount: 0, createdAt: '2026-01-01T00:00:00Z' })

const channel = (id: string, name: string) => ({
  __typename: 'Channel' as const,
  id,
  slug: name.toLowerCase(),
  name,
  defaultLocale: 'en',
  locales: [],
  isActive: true,
  isDefault: false,
  currencyCode: 'EUR',
  publicUrl: null,
  createdAt: '2026-01-01T00:00:00Z',
})

async function apply(changes: RegExp) {
  fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
  fireEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: changes }))
}

describe('CMS page tags and channels', () => {
  it('writes the picked tags', async () => {
    const onUpdate = vi.fn()
    renderSection(<CmsPageTagsSection pageId="pg1" tags={['news']} canManage refetchVariables={{ id: 'pg1' }} />, [
      { request: { query: CmsTagsDocument }, result: { data: { cmsTags: [tag('news', 'News'), tag('guides', 'Guides')] } }, maxUsageCount: 3 },
      { request: { query: UpdateCmsPageDocument, variables: captureInput(onUpdate) }, result: { data: { updateCMSPage: null } } },
    ])
    fireEvent.click(await screen.findByRole('button', { name: 'Guides' }))
    await apply(/Apply 1 change/)
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalledWith({ id: 'pg1', tags: ['news', 'guides'] }), SLOW)
  })

  it('writes only the channel whose listing changed', async () => {
    const onListing = vi.fn()
    renderSection(
      <CmsPageChannelsSection
        pageId="pg1"
        listings={[{ __typename: 'CMSPageChannelListing', channelId: 'c1', isPublished: true, isHomepage: false, seoNoindex: false } as never]}
        canManage
        refetchVariables={{ id: 'pg1' }}
      />,
      [
        { request: { query: ChannelsDocument }, result: { data: { channels: [channel('c1', 'Wheels'), channel('c2', 'Tyres')] } }, maxUsageCount: 3 },
        { request: { query: SetCmsPageChannelListingDocument, variables: captureInput(onListing) }, result: { data: { setCMSPageChannelListing: null } } },
      ],
    )
    // Published channels sort first, so Tyres (unlisted) is the second row
    await screen.findByText('Tyres')
    fireEvent.click(screen.getAllByRole('switch', { name: 'Published' })[1])
    await apply(/Apply 1 change/)
    await vi.waitFor(() => expect(onListing).toHaveBeenCalledTimes(1), SLOW)
    expect(onListing).toHaveBeenCalledWith({ pageId: 'pg1', channelId: 'c2', isPublished: true, seoNoindex: false, isHomepage: false })
  })
})
