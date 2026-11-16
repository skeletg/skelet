import { fireEvent, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { renderPage } from '@/test/page-harness'
import { AdminSsoLinksDocument, AdminSsoUnlinkDocument } from './Login.gql'
import { SsoLinksCard } from './SsoLinksCard'

const link = { __typename: 'AdminSsoLink' as const, id: 'l1', issuer: 'https://idp.example', email: 'ops@acme.example', createdAt: '2026-11-16T10:00:00Z', lastUsedAt: '2026-11-16T10:00:00Z' }
const option = { __typename: 'SsoOption' as const, label: 'Acme' }

function at(data: object, more: Parameters<typeof renderPage>[0]['mocks'] = []) {
  renderPage({ path: '/p', routes: { '/p': <SsoLinksCard /> }, mocks: [{ request: { query: AdminSsoLinksDocument }, result: { data }, maxUsageCount: 3 }, ...(more ?? [])] })
}

describe('SsoLinksCard', () => {
  it('lists the admin’s connected accounts and disconnects one', async () => {
    const unlinked = vi.fn(() => ({ data: { adminSsoUnlink: true } }))
    at({ adminSsoOption: option, adminSsoLinks: [link] }, [{ request: { query: AdminSsoUnlinkDocument, variables: { id: 'l1' } }, result: unlinked }])
    expect(await screen.findByText('ops@acme.example')).toBeDefined()
    expect(screen.getByRole('button', { name: /Connect another account/ })).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }))
    await waitFor(() => expect(unlinked).toHaveBeenCalled())
  })

  it('shows nothing where no provider is set up and nothing is connected', async () => {
    const { container } = renderPage({ path: '/p', routes: { '/p': <SsoLinksCard /> }, mocks: [{ request: { query: AdminSsoLinksDocument }, result: { data: { adminSsoOption: null, adminSsoLinks: [] } } }] })
    await waitFor(() => expect(container.textContent).not.toContain('Sign-in provider'))
  })
})
