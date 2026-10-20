import type { ReactNode } from 'react'
import { screen } from '@testing-library/react'
import type { DocumentNode } from 'graphql'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { EmailCampaignDocument } from '@/features/campaigns/Campaigns.gql'
import { CampaignComposer } from '@/features/campaigns/CampaignComposer'
import { LibraryEmailDocument } from '@/features/emails/Emails.gql'
import { EmailLibraryDetail } from '@/features/emails/EmailLibraryDetail'
import { FlowDocument } from '@/features/flows/Flows.gql'
import { FlowBuilder } from '@/features/flows/FlowBuilder'
import { SignupFormDocument } from '@/features/forms/Forms.gql'
import { FormBuilder } from '@/features/forms/FormBuilder'
import { ExperimentDocument } from '@/features/prism/Prism.gql'
import { PrismEditor } from '@/features/prism/PrismEditor'

// What these pin: opening a Pulse editor on a failed load or a deleted record says which - it used to leave the
// editor's header over an empty body

const channelScope = {
  ...globalScope,
  scope: { mode: 'CHANNEL', channelId: 'c1', locale: '' },
  isGlobal: false,
  isChannel: true,
  effectiveChannelId: 'c1',
  selectedChannel: { id: 'c1', name: 'Main', slug: 'main' },
} as unknown as ConfigScopeContextValue

const editors: Array<[string, ReactNode, DocumentNode, string]> = [
  ['FlowBuilder', <FlowBuilder />, FlowDocument, 'flow'],
  ['CampaignComposer', <CampaignComposer />, EmailCampaignDocument, 'emailCampaign'],
  ['PrismEditor', <PrismEditor />, ExperimentDocument, 'experiment'],
  ['EmailLibraryDetail', <EmailLibraryDetail />, LibraryEmailDocument, 'libraryEmail'],
  ['FormBuilder', <FormBuilder />, SignupFormDocument, 'signupForm'],
]

function open(editor: ReactNode, mock: Record<string, unknown>) {
  renderPage({
    path: '/e/x1',
    routes: { '/e/:id': editor },
    adminId: 'admin-1',
    scope: channelScope,
    mocks: [meMock(), mock as never],
  })
}

describe('Pulse editors', () => {
  it.each(editors)('%s shows the load error with a retry', async (_name, editor, query) => {
    open(editor, { request: { query, variables: () => true }, error: new Error('pulse is down'), maxUsageCount: 3 })
    expect(await screen.findByText('pulse is down', {}, SLOW)).toBeTruthy()
    expect(screen.getByRole('button', { name: /Retry/ })).toBeTruthy()
  })

  it.each(editors)('%s says not found for a missing record', async (_name, editor, query, field) => {
    open(editor, { request: { query, variables: () => true }, result: { data: { [field]: null } }, maxUsageCount: 3 })
    expect(await screen.findByText('Page not found', {}, SLOW)).toBeTruthy()
  })
})
