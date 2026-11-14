import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { beforeAll, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { LibraryEmailDocument, LibraryEmailNamesDocument, RenderLibraryPreviewDocument } from '@/features/emails/Emails.gql'
import { CampaignComposer } from './CampaignComposer'
import { CampaignAudienceEstimateDocument, CampaignGroupsDocument, CampaignLimitsDocument, CampaignSegmentsDocument, CancelEmailCampaignDocument, CreateEmailCampaignDocument, EmailCampaignDocument, RenderCampaignPreviewDocument, ScheduleEmailCampaignDocument, UpdateEmailCampaignDocument } from './Campaigns.gql'

const campaign = {
  __typename: 'EmailCampaign', id: 'cmp1', channelId: 'ch1', name: 'Autumn sale', emailId: null, subject: 'Sale', blocks: [], styles: {},
  audienceKind: 'all', audienceId: null, status: 'scheduled', scheduledFor: '2026-11-01T09:00:00Z', sentAt: null,
  totalRecipients: 0, sentCount: 0, failedCount: 0, skippedCount: 0, openedCount: 0, clickedCount: 0,
  abEnabled: false, abSubjectB: '', abTestPercent: 20, abWinnerMetric: 'open', abTestMinutes: 240, abWinner: '', abTestStartedAt: null,
  createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z', deletedAt: null,
}

// What this pins: the dialog closes without waiting on the server, and a campaign past editing skips the editor's lists
describe('CampaignComposer cancel send', () => {
  it('closes at once, then sends the cancel', async () => {
    const cancelled = vi.fn(() => ({ data: { cancelEmailCampaign: { ...campaign, status: 'cancelled' } } }))
    const library = vi.fn(() => ({ data: { libraryEmails: [] } }))
    renderPage({
      path: '/campaigns/cmp1',
      routes: { '/campaigns/:id': <CampaignComposer /> },
      mocks: [
        meMock(),
        { request: { query: EmailCampaignDocument, variables: { id: 'cmp1' } }, result: { data: { emailCampaign: campaign } }, maxUsageCount: 10 },
        { request: { query: CancelEmailCampaignDocument, variables: { id: 'cmp1' } }, result: cancelled, delay: 500 },
        { request: { query: LibraryEmailNamesDocument, variables: () => true }, result: library, maxUsageCount: 5 },
        { request: { query: RenderCampaignPreviewDocument, variables: () => true }, result: { data: { renderCampaignPreview: { __typename: 'EmailPreview', subject: 'Sale', html: '<p>Sale</p>' } } }, maxUsageCount: 5 },
      ],
    })
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel send' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByRole('button', { name: 'Keep' })).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel send' }))
    // Well inside the server's 500ms - an awaited cancel would still be open
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull(), { timeout: 200 })
    await waitFor(() => expect(cancelled).toHaveBeenCalled(), SLOW)
    expect(await screen.findByText(/was cancelled\./, {}, SLOW)).toBeDefined()
    expect(library).not.toHaveBeenCalled()
  })
})

const store = { id: 'ch1', name: 'Demo Store', defaultLocale: 'en', locales: [{ code: 'en' }], isActive: true }
const inStore: ConfigScopeContextValue = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'ch1', locale: '' },
  channels: [store],
  selectedChannel: store,
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'ch1',
}

const email = {
  __typename: 'LibraryEmail', id: 'e1', channelId: 'ch1', name: 'Autumn deals', subject: 'Deals for autumn', preheader: 'Ends Sunday',
  blocks: [{ type: 'heading', text: 'Hi' }], styles: {}, groupId: null, slot: '', createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z',
}

const draft = { ...campaign, id: 'cmp9', status: 'draft', scheduledFor: null }

// The A/B bounds are not the server's real ones, so a slider that shows them can only have read them
const reads = () => [
  meMock(),
  { request: { query: LibraryEmailNamesDocument, variables: { channelId: 'ch1' } }, result: { data: { libraryEmails: [{ __typename: 'LibraryEmail', id: 'e1', name: email.name, subject: email.subject, preheader: email.preheader }] } }, maxUsageCount: 10 },
  { request: { query: LibraryEmailDocument, variables: { id: 'e1' } }, result: { data: { libraryEmail: email } }, maxUsageCount: 10 },
  { request: { query: CampaignLimitsDocument }, result: { data: { campaignLimits: { __typename: 'CampaignLimits', abTestPercentMin: 5, abTestPercentMax: 50 } } }, maxUsageCount: 10 },
  { request: { query: CampaignGroupsDocument }, result: { data: { customerGroups: [] } }, maxUsageCount: 10 },
  { request: { query: CampaignSegmentsDocument }, result: { data: { customerSegments: { __typename: 'CustomerSegmentList', items: [] } } }, maxUsageCount: 10 },
  {
    request: { query: CampaignAudienceEstimateDocument, variables: { channelId: 'ch1', audienceKind: 'all', audienceId: null } },
    result: { data: { campaignAudienceEstimate: { __typename: 'CampaignAudienceEstimate', total: 10, eligible: 4 } } },
    maxUsageCount: 10,
  },
  {
    request: { query: RenderLibraryPreviewDocument, variables: () => true },
    result: { data: { renderLibraryPreview: { __typename: 'EmailPreview', subject: 'Deals for autumn', html: '<p>Hi</p>', text: 'Hi', preheader: 'Ends Sunday', fromName: 'Demo Store', fromAddress: 'hello@demo.test' } } },
    maxUsageCount: 10,
  },
]

async function nameAndPickTheEmail() {
  fireEvent.change(await screen.findByLabelText('Campaign name', {}, SLOW), { target: { value: 'Autumn sale' } })
  fireEvent.click(screen.getByRole('button', { name: 'Email: an email' }))
  fireEvent.click(await screen.findByRole('option', { name: /Autumn deals/ }, SLOW))
  expect(await screen.findByRole('button', { name: 'Email: Autumn deals' })).toBeDefined()
}

// What this pins: the send button waits for every blank, and confirming saves the sentence as it stands, then schedules
describe('CampaignComposer send sentence', () => {
  beforeAll(() => {
    Element.prototype.scrollIntoView ??= () => {}
  })

  it('fills the blanks, then saves and sends what the sentence says', async () => {
    let saved: Record<string, unknown> | undefined
    const scheduled = vi.fn(() => ({ data: { scheduleEmailCampaign: { ...campaign, id: 'cmp9' } } }))
    renderPage({
      path: '/campaigns/new',
      scope: inStore,
      routes: { '/campaigns/new': <CampaignComposer />, '/campaigns/:id': <p>Scheduled page</p> },
      mocks: [
        ...reads(),
        { request: { query: CreateEmailCampaignDocument, variables: captureInput((input) => (saved = input)) }, result: { data: { createEmailCampaign: draft } } },
        { request: { query: ScheduleEmailCampaignDocument, variables: { id: 'cmp9', scheduledFor: null } }, result: scheduled },
      ],
    })

    const send = await screen.findByRole('button', { name: 'Send now' }, SLOW)
    expect(send).toHaveProperty('disabled', true)
    expect(screen.getByText('Still needs a name and an email.')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Audience: everyone on Demo Store' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'When: now' })).toBeDefined()
    expect(await screen.findByText(/of 10 customers in this audience are opted in/, {}, SLOW)).toBeDefined()

    await nameAndPickTheEmail()
    expect(await screen.findByText('From Demo Store <hello@demo.test>', {}, SLOW)).toBeDefined()

    await waitFor(() => expect(send).toHaveProperty('disabled', false))
    fireEvent.click(send)
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Autumn deals goes to everyone on Demo Store now\./)).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Send now' }))

    expect(await screen.findByText('Scheduled page', {}, SLOW)).toBeDefined()
    expect(saved).toMatchObject({ channelId: 'ch1', name: 'Autumn sale', emailId: 'e1', audienceKind: 'all', audienceId: null, scheduledFor: null, abEnabled: false })
    expect(scheduled).toHaveBeenCalledTimes(1)
  })

  it('adds a subject test as a second sentence, bounded by what the server allows', async () => {
    let saved: Record<string, unknown> | undefined
    renderPage({
      path: '/campaigns/new',
      scope: inStore,
      routes: { '/campaigns/new': <CampaignComposer />, '/campaigns/:id': <p>Saved page</p> },
      mocks: [...reads(), { request: { query: CreateEmailCampaignDocument, variables: captureInput((input) => (saved = input)) }, result: { data: { createEmailCampaign: draft } } }],
    })
    await nameAndPickTheEmail()

    fireEvent.click(screen.getByRole('switch', { name: 'Test two subject lines first' }))
    expect(screen.getByText('Still needs a second subject.')).toBeDefined()
    expect(screen.getByRole('button', { name: 'Send now' })).toHaveProperty('disabled', true)

    fireEvent.click(screen.getByRole('button', { name: 'Subject B: a second subject' }))
    fireEvent.change(await screen.findByLabelText('Subject B'), { target: { value: 'Last days of the sale' } })
    fireEvent.keyDown(screen.getByLabelText('Subject B'), { key: 'Enter' })
    expect(await screen.findByRole('button', { name: 'Subject B: "Last days of the sale"' })).toBeDefined()
    expect(screen.getAllByText('Last days of the sale').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: 'Share of the audience in the test: 20%' }))
    const slider = await screen.findByLabelText('How much of the audience is in the test')
    await waitFor(() => expect(slider).toHaveProperty('disabled', false), SLOW)
    expect([slider.getAttribute('min'), slider.getAttribute('max')]).toEqual(['5', '50'])
    fireEvent.change(slider, { target: { value: '40' } })
    expect(screen.getByRole('button', { name: 'Share of the audience in the test: 40%' })).toBeDefined()
    fireEvent.keyDown(slider, { key: 'Escape' })

    fireEvent.click(screen.getByRole('button', { name: 'What decides the winner: opens' }))
    fireEvent.click(await screen.findByRole('radio', { name: 'Clicks' }))
    expect(await screen.findByRole('button', { name: 'What decides the winner: clicks' })).toBeDefined()
    // 40% of 10 is a test of four, two a side, and two in five of the audience are opted in
    expect(screen.getByText(/Subject A goes to about 1 and subject B to about 1\./)).toBeDefined()
    expect(screen.getByText(/The subject with more clicks goes to the other 2\. A tie goes to subject A\./)).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }))
    expect(await screen.findByText('Saved page', {}, SLOW)).toBeDefined()
    expect(saved).toMatchObject({ abEnabled: true, abSubjectB: 'Last days of the sale', abTestPercent: 40, abWinnerMetric: 'click', abTestMinutes: 240 })
  })

  // A guard that took the move after a first save for walking away would leave the test on the composer
  it('leaves for the saved campaign without asking about unsaved changes', async () => {
    const asked = vi.spyOn(window, 'confirm').mockReturnValue(false)
    renderPage({
      path: '/campaigns/new',
      scope: inStore,
      guarded: true,
      routes: { '/campaigns/new': <CampaignComposer />, '/campaigns/:id': <p>Saved page</p> },
      mocks: [...reads(), { request: { query: CreateEmailCampaignDocument, variables: () => true }, result: { data: { createEmailCampaign: draft } } }],
    })
    await nameAndPickTheEmail()
    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }))
    expect(await screen.findByText('Saved page', {}, SLOW)).toBeDefined()
    expect(asked).not.toHaveBeenCalled()
    asked.mockRestore()
  })

  it('narrows the audience to a segment, and counts and saves that', async () => {
    let saved: Record<string, unknown> | undefined
    const vips = { __typename: 'CustomerSegment', id: 's1', name: 'VIPs', memberCount: 12 }
    renderPage({
      path: '/campaigns/new',
      scope: inStore,
      routes: { '/campaigns/new': <CampaignComposer />, '/campaigns/:id': <p>Saved page</p> },
      mocks: [
        { request: { query: CampaignSegmentsDocument }, result: { data: { customerSegments: { __typename: 'CustomerSegmentList', items: [vips] } } }, maxUsageCount: 10 },
        {
          request: { query: CampaignAudienceEstimateDocument, variables: { channelId: 'ch1', audienceKind: 'segment', audienceId: 's1' } },
          result: { data: { campaignAudienceEstimate: { __typename: 'CampaignAudienceEstimate', total: 12, eligible: 9 } } },
          maxUsageCount: 10,
        },
        ...reads(),
        { request: { query: CreateEmailCampaignDocument, variables: captureInput((input) => (saved = input)) }, result: { data: { createEmailCampaign: draft } } },
      ],
    })
    await nameAndPickTheEmail()

    fireEvent.click(screen.getByRole('button', { name: 'Audience: everyone on Demo Store' }))
    fireEvent.click(await screen.findByRole('radio', { name: 'A segment' }))
    expect(screen.getByText('Still needs a segment.')).toBeDefined()
    fireEvent.click(await screen.findByRole('button', { name: /VIPs/ }, SLOW))
    expect(await screen.findByRole('button', { name: 'Audience: the VIPs segment' })).toBeDefined()
    expect(await screen.findByText(/of 12 customers in this audience are opted in/, {}, SLOW)).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }))
    expect(await screen.findByText('Saved page', {}, SLOW)).toBeDefined()
    expect(saved).toMatchObject({ audienceKind: 'segment', audienceId: 's1' })
  })

  it('keeps the time a draft plans to go out', async () => {
    const planned = '2031-05-06T09:00'
    let saved: Record<string, unknown> | undefined
    renderPage({
      path: '/campaigns/cmp9',
      scope: inStore,
      routes: { '/campaigns/:id': <CampaignComposer /> },
      mocks: [
        ...reads(),
        { request: { query: EmailCampaignDocument, variables: { id: 'cmp9' } }, result: { data: { emailCampaign: { ...draft, emailId: 'e1', scheduledFor: '2030-03-04T10:00:00Z' } } }, maxUsageCount: 10 },
        {
          request: { query: UpdateEmailCampaignDocument, variables: captureInput((input) => (saved = input)) },
          result: { data: { updateEmailCampaign: { ...draft, emailId: 'e1', scheduledFor: new Date(planned).toISOString() } } },
        },
      ],
    })

    const when = await screen.findByRole('button', { name: /^When: on .*2030/ }, SLOW)
    expect(screen.getByRole('button', { name: 'Schedule' })).toBeDefined()
    expect(screen.getByRole('button', { name: 'Saved' })).toBeDefined()

    fireEvent.click(when)
    fireEvent.change(await screen.findByLabelText('Date and time'), { target: { value: planned } })
    expect(await screen.findByRole('button', { name: /^When: on .*2031/ })).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Save draft' }))

    await waitFor(() => expect(saved).toMatchObject({ scheduledFor: new Date(planned).toISOString() }), SLOW)
    expect(await screen.findByRole('button', { name: 'Saved' }, SLOW)).toBeDefined()
  })
})
