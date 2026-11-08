import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, captureInput, globalScope, renderPage } from '@/test/page-harness'
import type { ConfigScopeContextValue } from '@/lib/config-scope'
import { CampaignMergeTagsDocument } from '@/features/campaigns/Campaigns.gql'
import { FlowMergeTagsDocument } from '@/features/flows/Flows.gql'
import { EmailBlocksDocument } from '@/features/email/Email.gql'
import {
  DeleteEmailTranslationDocument,
  EmailGroupsDocument,
  EmailPreflightDocument,
  EmailTranslationsDocument,
  EmailsAiStatusDocument,
  LibraryEmailDocument,
  SaveEmailTranslationDocument,
  SaveLibraryEmailDocument,
} from './Emails.gql'
import { EmailLibraryDetail } from './EmailLibraryDetail'

// What this pins: a language with no version yet opens as a copy of the email, and saving it writes that language's
// version - never the email every other shopper gets, even when the channel isn't in the scope list. Switching language
// with unsaved edits asks first, and an existing version opens as itself and can be deleted

const email = {
  __typename: 'LibraryEmail' as const,
  id: 'e1',
  channelId: 'ch1',
  name: 'Order confirmed',
  subject: 'Thanks for your order',
  preheader: '',
  blocks: [{ type: 'text', content: 'Hello' }],
  styles: {},
  groupId: null,
  slot: 'order_confirmation',
  createdAt: '2026-11-01T00:00:00Z',
  updatedAt: '2026-11-01T00:00:00Z',
}

const scope: ConfigScopeContextValue = {
  ...globalScope,
  channels: [{ id: 'ch1', name: 'Shop', defaultLocale: 'en', locales: [{ code: 'en' }, { code: 'de' }], isActive: true }],
}

const germanVersion = {
  __typename: 'EmailTranslation' as const,
  emailId: 'e1',
  locale: 'de',
  subject: 'Danke',
  preheader: '',
  blocks: [{ type: 'text', content: 'Hallo' }],
  styles: {},
  updatedAt: '2026-10-01T00:00:00Z', // older than the email, which changed on 11-01
}

function mocks(
  onTranslation: (input: Record<string, unknown>) => void,
  onEmail: () => void,
  translations: unknown[] = [],
  onDelete: (vars: Record<string, unknown>) => void = () => {},
): MockLink.MockedResponse[] {
  return [
    { request: { query: LibraryEmailDocument, variables: { id: 'e1' } }, result: { data: { libraryEmail: email } }, maxUsageCount: 5 },
    { request: { query: EmailTranslationsDocument, variables: { emailId: 'e1' } }, result: { data: { emailTranslations: translations } }, maxUsageCount: 5 },
    {
      request: { query: DeleteEmailTranslationDocument, variables: (vars: Record<string, unknown>) => (onDelete(vars), true) },
      result: { data: { deleteEmailTranslation: true } },
    },
    { request: { query: EmailGroupsDocument }, result: { data: { emailGroups: [] } }, maxUsageCount: 5 },
    { request: { query: EmailBlocksDocument, variables: { channelId: 'ch1' } }, result: { data: { emailBlocks: [] } }, maxUsageCount: 5 },
    { request: { query: EmailsAiStatusDocument }, result: { data: { aiStatus: { __typename: 'AiStatus', enabled: false } } }, maxUsageCount: 5 },
    { request: { query: EmailPreflightDocument, variables: { id: 'e1' } }, result: { data: { emailPreflight: [] } }, maxUsageCount: 5 },
    { request: { query: CampaignMergeTagsDocument }, result: { data: { campaignMergeTags: [] } }, maxUsageCount: 5 },
    { request: { query: FlowMergeTagsDocument }, result: { data: { flowMergeTags: [] } }, maxUsageCount: 5 },
    {
      request: { query: SaveEmailTranslationDocument, variables: captureInput(onTranslation) },
      result: {
        data: {
          saveEmailTranslation: {
            __typename: 'EmailTranslation',
            emailId: 'e1',
            locale: 'de',
            subject: 'Danke für Ihre Bestellung',
            preheader: '',
            blocks: email.blocks,
            styles: {},
            updatedAt: '2026-11-08T00:00:00Z',
          },
        },
      },
    },
    {
      request: { query: SaveLibraryEmailDocument, variables: () => true },
      result: () => {
        onEmail()
        return { data: { saveLibraryEmail: email } }
      },
    },
  ]
}

describe('EmailLibraryDetail languages', () => {
  it('writes a language version without touching the email itself', async () => {
    const onTranslation = vi.fn()
    const onEmail = vi.fn()
    renderPage({ path: '/emails/e1?lang=de', routes: { '/emails/:id': <EmailLibraryDetail /> }, mocks: mocks(onTranslation, onEmail), scope })

    const subject = (await screen.findByLabelText('Subject', {}, SLOW)) as HTMLInputElement
    expect(subject.value).toBe('Thanks for your order')
    expect(screen.getByText(/Starts as a copy of the default/)).toBeDefined()
    expect((screen.getByLabelText('Email name') as HTMLInputElement).disabled).toBe(true)

    fireEvent.change(subject, { target: { value: 'Danke für Ihre Bestellung' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(
      () =>
        expect(onTranslation).toHaveBeenCalledWith(
          expect.objectContaining({ emailId: 'e1', locale: 'de', subject: 'Danke für Ihre Bestellung', blocks: email.blocks }),
        ),
      SLOW,
    )
    expect(onEmail).not.toHaveBeenCalled()
  })

  it("saves a language's version even when its channel isn't in the scope list", async () => {
    const onTranslation = vi.fn()
    const onEmail = vi.fn()
    renderPage({
      path: '/emails/e1?lang=de',
      routes: { '/emails/:id': <EmailLibraryDetail /> },
      mocks: mocks(onTranslation, onEmail),
      scope: { ...globalScope, channels: [] },
    })
    const subject = (await screen.findByLabelText('Subject', {}, SLOW)) as HTMLInputElement
    fireEvent.change(subject, { target: { value: 'Danke' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await vi.waitFor(() => expect(onTranslation).toHaveBeenCalledWith(expect.objectContaining({ locale: 'de', subject: 'Danke' })), SLOW)
    expect(onEmail).not.toHaveBeenCalled()
  })

  it('asks before switching language drops unsaved edits', async () => {
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, writable: true, value: vi.fn() })
    renderPage({ path: '/emails/e1', routes: { '/emails/:id': <EmailLibraryDetail /> }, mocks: mocks(vi.fn(), vi.fn()), scope })
    const subject = (await screen.findByLabelText('Subject', {}, SLOW)) as HTMLInputElement
    fireEvent.change(subject, { target: { value: 'Thanks, really' } })
    fireEvent.click(screen.getByRole('combobox', { name: 'Language' }))
    fireEvent.click(await screen.findByRole('option', { name: /German \(de\)/ }))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Discard your changes?')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect((screen.getByLabelText('Subject') as HTMLInputElement).value).toBe('Thanks, really')
  })

  it('opens an existing version as itself, flags it as older, and deletes it', async () => {
    const onDelete = vi.fn()
    renderPage({
      path: '/emails/e1?lang=de',
      routes: { '/emails/:id': <EmailLibraryDetail /> },
      mocks: mocks(vi.fn(), vi.fn(), [germanVersion], onDelete),
      scope,
    })
    const subject = (await screen.findByLabelText('Subject', {}, SLOW)) as HTMLInputElement
    expect(subject.value).toBe('Danke')
    expect(screen.getByText(/The default changed after it was saved/)).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Delete the de version' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete version' }))
    await vi.waitFor(() => expect(onDelete).toHaveBeenCalledWith({ emailId: 'e1', locale: 'de' }), SLOW)
  })
})
