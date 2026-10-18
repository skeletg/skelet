import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderDialog } from '@/test/page-harness'
import { StoreCreditSection } from './StoreCreditSection'
import { AdjustStoreCreditDocument, StoreCreditDocument } from './Customers.gql'

// What these pin: the adjustment is typed in the balance currency's major units - "500" yen grants 500, not 50000

function renderAdjust(currencyCode: string, onInput: (v: Record<string, unknown>) => void) {
  renderDialog(<StoreCreditSection customerId="cu1" />, [
    meMock(),
    {
      request: { query: StoreCreditDocument, variables: { customerId: 'cu1' } },
      result: { data: { storeCredit: { __typename: 'StoreCredit', balanceCents: 0, currencyCode, entries: [] } } },
      maxUsageCount: 3,
    },
    {
      request: { query: AdjustStoreCreditDocument, variables: captureInput(onInput) },
      result: { data: { adjustStoreCredit: { __typename: 'StoreCredit', balanceCents: 500, currencyCode } } },
    },
  ])
}

async function adjust(amount: string) {
  fireEvent.click(await screen.findByRole('button', { name: 'Adjust balance' }, SLOW))
  fireEvent.change(screen.getByLabelText('Amount (+/−)'), { target: { value: amount } })
  fireEvent.change(screen.getByLabelText('Reason'), { target: { value: 'goodwill' } })
  fireEvent.click(screen.getByRole('button', { name: 'Apply adjustment' }))
}

describe('StoreCreditSection adjust', () => {
  it('converts by the balance currency decimals', async () => {
    const calls: Record<string, unknown>[] = []
    renderAdjust('JPY', (v) => calls.push(v))
    await adjust('500')
    await vi.waitFor(() => expect(calls).toEqual([{ customerId: 'cu1', amountCents: 500, reason: 'goodwill' }]), SLOW)
  })

  it('sends two-decimal currencies as cents', async () => {
    const calls: Record<string, unknown>[] = []
    renderAdjust('EUR', (v) => calls.push(v))
    await adjust('-12.50')
    await vi.waitFor(() => expect(calls[0]).toMatchObject({ amountCents: -1250 }), SLOW)
  })

  it('lets three-decimal currencies through the input step', async () => {
    const calls: Record<string, unknown>[] = []
    renderAdjust('BHD', (v) => calls.push(v))
    await adjust('1.234')
    expect((screen.getByLabelText('Amount (+/−)') as HTMLInputElement).step).toBe('0.001')
    await vi.waitFor(() => expect(calls[0]).toMatchObject({ amountCents: 1234 }), SLOW)
  })
})
