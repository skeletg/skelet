import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { MockLink } from '@apollo/client/testing'
import { SLOW, meMock, renderPage } from '@/test/page-harness'
import { FormBuilder } from './FormBuilder'
import { DeleteSignupFormDocument, SignupFormDocument } from './Forms.gql'

// What this pins: the delete confirm runs through the shared ConfirmDialog without waiting on the server - it closes
// as soon as it is confirmed, a refused delete reports on the page, and a successful one goes back to the list

const form = {
  __typename: 'SignupForm', id: 'f1', channelId: 'ch1', slug: 'newsletter', name: 'Newsletter', status: 'draft', type: 'popup',
  heading: 'Join', body: '', buttonLabel: 'Subscribe', successMessage: 'Thanks', collectName: false, consentKind: 'marketing_email',
  targetSegmentId: null, triggerFlow: false, submissionCount: 0,
  display: { __typename: 'SignupFormDisplay', trigger: 'immediate', delaySeconds: 5, scrollPercent: 50, frequencyDays: 7 },
}

async function confirmDelete(mock: MockLink.MockedResponse) {
  renderPage({
    path: '/forms/f1',
    routes: { '/forms/:id': <FormBuilder />, '/forms': <p>Forms list</p> },
    mocks: [meMock(), { request: { query: SignupFormDocument, variables: { id: 'f1' } }, result: { data: { signupForm: form } }, maxUsageCount: 5 }, mock],
  })
  // Delete is behind forms.manage - wait for the grants before clicking it
  fireEvent.click(await screen.findByRole('button', { name: 'Delete' }, SLOW))
  const dialog = await screen.findByRole('alertdialog')
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }))
  await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull())
}

describe('FormBuilder delete', () => {
  it('closes at once and reports a refused delete on the page', async () => {
    await confirmDelete({ request: { query: DeleteSignupFormDocument, variables: { id: 'f1' } }, result: { errors: [{ message: 'form is live' }] }, delay: 500 })
    expect(screen.queryByText('form is live')).toBeNull() // closed before the server answered
    expect(await screen.findByText('form is live', {}, SLOW)).toBeDefined()
  })

  it('goes back to the list once the delete goes through', async () => {
    await confirmDelete({ request: { query: DeleteSignupFormDocument, variables: { id: 'f1' } }, result: { data: { deleteSignupForm: true } } })
    expect(await screen.findByText('Forms list', {}, SLOW)).toBeDefined()
  })
})
