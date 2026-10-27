import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SLOW, globalScope, meMock, renderPage } from '@/test/page-harness'
import { WebFormDetail } from './WebFormDetail'

// What this pins: the form builder's labels name their inputs - they sat beside them unlinked, so a screen reader
// announced every box as an unnamed edit field and clicking a label focused nothing

// Forms are per channel - the builder only opens with one picked
const channelScope = { ...globalScope, scope: { mode: 'CHANNEL' as const, channelId: 'c1', locale: '' }, effectiveChannelId: 'c1', isChannel: true, isGlobal: false }

describe('WebFormDetail labels', () => {
  it('names the status and settings inputs by their labels', async () => {
    renderPage({ path: '/cms/forms/new', adminId: 'admin-1', routes: { '/cms/forms/:id': <WebFormDetail /> }, mocks: [meMock()], scope: channelScope })
    expect((await screen.findByLabelText('Status', {}, SLOW)).getAttribute('role')).toBe('combobox')
    for (const label of ['Submit button label', 'Success message', 'Redirect URL (optional)', 'Notify these emails (comma-separated)', 'Record consent under (optional)']) {
      expect(screen.getByLabelText(label).tagName).toMatch(/INPUT|TEXTAREA/)
    }
  })
})
