import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MockedProvider } from '@apollo/client/testing/react'
import type { MockLink } from '@apollo/client/testing'
import { describe, expect, it } from 'vitest'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { SLOW } from '@/test/page-harness'
import { MediaAssetSharesDocument, RevokeMediaShareDocument, ShareMediaAssetDocument } from '../MediaLibrary.gql'
import { Shares } from './Shares'


const share = (id: string, email: string, over: object = {}) => ({
  __typename: 'MediaShare', id, customerId: `c-${id}`, email, downloadsUsed: 0, maxDownloads: null, expiresAt: null, createdAt: '2026-11-01T00:00:00Z', ...over,
})
const ana = {
  __typename: 'Customer', id: 'c-ana', email: 'ana@example.com', firstName: 'Ana', lastName: 'Ilic', phone: '', isActive: true, isEmailVerified: true, createdAt: '2026-01-01T00:00:00Z', group: null,
}

type Vars = Record<string, unknown>

function rig({ shared = [] as object[], canShare = true, canSeeCustomers = true, failing = false, onlyIfAny = false } = {}) {
  const sent: { op: string; vars: Vars }[] = []
  const now = [...shared]
  const mocks: MockLink.MockedResponse[] = [
    failing
      ? { request: { query: MediaAssetSharesDocument, variables: { assetId: 'a1' } }, error: new Error('the database is away'), maxUsageCount: 5 }
      : { request: { query: MediaAssetSharesDocument, variables: { assetId: 'a1' } }, result: () => ({ data: { mediaAssetShares: [...now] } }), maxUsageCount: 10 },
    { request: { query: CustomersPagedDocument, variables: () => true }, result: { data: { customersPaged: { __typename: 'CustomerPage', totalCount: 1, items: [ana] } } }, maxUsageCount: 5 },
    {
      request: {
        query: ShareMediaAssetDocument,
        variables: (v: Vars) => {
          sent.push({ op: 'share', vars: v })
          return true
        },
      },
      result: () => {
        const made = share('s-new', 'ana@example.com', { customerId: 'c-ana' })
        now.push(made)
        return { data: { shareMediaAsset: made } }
      },
    },
    {
      request: {
        query: RevokeMediaShareDocument,
        variables: (v: Vars) => {
          sent.push({ op: 'revoke', vars: v })
          return true
        },
      },
      result: (v: Vars) => {
        now.splice(0, now.length, ...now.filter((s) => (s as { id: string }).id !== v.id))
        return { data: { revokeMediaShare: true } }
      },
    },
  ]
  render(
    <MockedProvider mocks={mocks}>
      <Shares assetId="a1" canShare={canShare} canSeeCustomers={canSeeCustomers} onlyIfAny={onlyIfAny} />
    </MockedProvider>,
  )
  return { sent, panel: () => screen.getByRole('region', { name: 'Shared with' }) }
}

describe('Shares', () => {
  it('says no one has the file yet - once it knows, not while it asks', async () => {
    rig()
    expect(screen.getByText('Loading…')).toBeDefined()
    expect(screen.queryByText(/No one yet/)).toBeNull()
    expect(await screen.findByText('No one yet. Only admins can open this file.', {}, SLOW)).toBeDefined()
  })

  it('lists who has it, how much of the share is used and when it ends', async () => {
    rig({
      shared: [
        share('s1', 'bo@example.com', { downloadsUsed: 2, maxDownloads: 5, expiresAt: '2099-01-10T23:59:59Z' }),
        share('s2', 'cy@example.com', { downloadsUsed: 1 }),
        share('s3', 'di@example.com', { expiresAt: '2020-01-01T00:00:00Z' }),
      ],
    })
    const rows = within(await screen.findByRole('list', {}, SLOW)).getAllByRole('listitem')
    expect(rows.map((r) => r.textContent)).toEqual([
      `bo@example.com2 of 5 downloads, until ${new Date('2099-01-10T23:59:59Z').toLocaleDateString()}`,
      'cy@example.com1 download, no end date',
      'di@example.com0 downloads, ended',
    ])
  })

  it('shares the file with a customer found by name, under a limit and a last day', async () => {
    const { sent } = rig()
    fireEvent.click(await screen.findByRole('button', { name: 'Share with a customer' }, SLOW))
    expect(screen.getByRole('button', { name: 'Share' })).toHaveProperty('disabled', true)

    fireEvent.change(screen.getByPlaceholderText(/Search customers/), { target: { value: 'ana' } })
    fireEvent.click(await screen.findByRole('button', { name: /Ana Ilic/ }, SLOW))
    fireEvent.change(screen.getByLabelText('Download limit'), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText('Last day'), { target: { value: '2027-02-01' } })
    fireEvent.click(screen.getByRole('button', { name: 'Share' }))

    expect(await screen.findByText('ana@example.com', {}, SLOW)).toBeDefined()
    expect(sent).toEqual([{ op: 'share', vars: { input: { assetId: 'a1', customerId: 'c-ana', maxDownloads: 3, expiresAt: new Date('2027-02-01T23:59:59').toISOString() } } }])
    expect(screen.queryByLabelText('Download limit')).toBeNull()
    expect(screen.getByRole('button', { name: 'Share with a customer' })).toBeDefined()
  })

  it('shares with no limit and no end when neither is given, and lets the customer be changed first', async () => {
    const { sent } = rig()
    fireEvent.click(await screen.findByRole('button', { name: 'Share with a customer' }, SLOW))
    fireEvent.change(screen.getByPlaceholderText(/Search customers/), { target: { value: 'ana' } })
    fireEvent.click(await screen.findByRole('button', { name: /Ana Ilic/ }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Change the customer' }))
    expect(screen.getByRole('button', { name: 'Share' })).toHaveProperty('disabled', true)

    fireEvent.change(screen.getByPlaceholderText(/Search customers/), { target: { value: 'ilic' } })
    fireEvent.click(await screen.findByRole('button', { name: /Ana Ilic/ }, SLOW))
    fireEvent.click(screen.getByRole('button', { name: 'Share' }))
    await waitFor(() => expect(sent).toEqual([{ op: 'share', vars: { input: { assetId: 'a1', customerId: 'c-ana', maxDownloads: null, expiresAt: null } } }]), SLOW)
  })

  it('takes a share back', async () => {
    const { sent } = rig({ shared: [share('s1', 'bo@example.com'), share('s2', 'cy@example.com')] })
    fireEvent.click(await screen.findByRole('button', { name: 'Stop sharing with bo@example.com' }, SLOW))
    await waitFor(() => expect(screen.queryByText('bo@example.com')).toBeNull(), SLOW)
    expect(screen.getByText('cy@example.com')).toBeDefined()
    expect(sent).toEqual([{ op: 'revoke', vars: { id: 's1' } }])
  })

  it('only lists for someone without the grant to share', async () => {
    rig({ shared: [share('s1', 'bo@example.com')], canShare: false })
    expect(await screen.findByText('bo@example.com', {}, SLOW)).toBeDefined()
    expect(screen.queryByRole('button', { name: /Stop sharing/ })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Share with a customer' })).toBeNull()
  })

  it('keeps a customer it was not given unnamed, and can still take that share back', async () => {
    const { sent } = rig({ shared: [share('s1', '', { customerId: null }), share('s2', 'cy@example.com')] })
    expect(await screen.findByText("A customer you can't see", {}, SLOW)).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: 'Stop sharing with this customer' }))
    await waitFor(() => expect(sent).toEqual([{ op: 'revoke', vars: { id: 's1' } }]), SLOW)
  })

  it('says what else sharing takes when customers cannot be seen, and still takes a share back', async () => {
    const { panel } = rig({ shared: [share('s1', '', { customerId: null })], canSeeCustomers: false })
    expect(await within(panel()).findByText(/also takes the permission to view customers/, {}, SLOW)).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Share with a customer' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Stop sharing with this customer' })).toBeDefined()
  })

  it('shows on a file made public only while it has shares to take back', async () => {
    rig({ shared: [share('s1', 'bo@example.com')], onlyIfAny: true })
    fireEvent.click(await screen.findByRole('button', { name: 'Stop sharing with bo@example.com' }, SLOW))
    await waitFor(() => expect(screen.queryByRole('region', { name: 'Shared with' })).toBeNull(), SLOW)
  })

  it('says so when the shares could not be loaded, and offers no sharing', async () => {
    const { panel } = rig({ failing: true })
    expect(await within(panel()).findByText("Who this file is shared with couldn't be loaded.", {}, SLOW)).toBeDefined()
    expect(screen.queryByRole('button', { name: 'Share with a customer' })).toBeNull()
  })
})
