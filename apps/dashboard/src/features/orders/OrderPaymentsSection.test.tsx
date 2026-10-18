import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderDialog } from '@/test/page-harness'
import { dayStartISO } from '@/lib/format'
import { OrderPaymentsSection } from './OrderPaymentsSection'
import {
  OfflinePaymentMethodsDocument,
  OrderDocument,
  OrdersPagedDocument,
  RecordOfflinePaymentDocument,
  RefundPaymentDocument,
} from './Orders.gql'

// What these pin: the refund amount is typed in major units - "12.50" refunds 1250 cents, not 12 - and a
// blank field's placeholder shows what a blank refund would actually move

const txn = (id: string, kind: string, amount: number) => ({
  __typename: 'PaymentTransaction',
  id,
  kind,
  amount,
  status: 'succeeded',
  providerSlug: 'stripe',
  providerReference: 'ref',
  reason: null,
  createdAt: '2026-01-01T00:00:00Z',
})

const order = {
  id: 'o1',
  channelId: 'c1',
  currencyCode: 'USD',
  paymentStatus: 'PARTIALLY_REFUNDED',
  paymentProvider: 'stripe',
  paymentReference: 'pi_1',
  total: 5000,
  giftCardAmount: 0,
  paymentTransactions: [txn('t1', 'authorize', 5000), txn('t2', 'capture', 5000), txn('t3', 'refund', 1250)],
} as never

async function renderRefund(onInput: (v: Record<string, unknown>) => void) {
  renderDialog(<OrderPaymentsSection order={order} />, [
    meMock(),
    {
      request: { query: OfflinePaymentMethodsDocument, variables: { channelId: 'c1' } },
      result: { data: { paymentMethods: [] } },
      maxUsageCount: 3,
    },
    {
      request: { query: RefundPaymentDocument, variables: captureInput(onInput) },
      result: { data: { refundPayment: { __typename: 'Order', id: 'o1', paymentStatus: 'PARTIALLY_REFUNDED' } } },
    },
    { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
    { request: { query: OrdersPagedDocument, variables: () => true }, result: { data: { ordersPaged: null } }, maxUsageCount: 3 },
  ])
  // the button waits on usePermissions' me query
  fireEvent.click(await screen.findByRole('button', { name: 'Refund' }, SLOW))
  return screen.getByLabelText('Amount') as HTMLInputElement
}

describe('OrderPaymentsSection refund', () => {
  it('sends a decimal amount as cents', async () => {
    const calls: Record<string, unknown>[] = []
    const input = await renderRefund((v) => calls.push(v))
    fireEvent.change(input, { target: { value: '12.50' } })
    fireEvent.click(screen.getAllByRole('button', { name: 'Refund' }).at(-1)!)
    await vi.waitFor(() => expect(calls).toEqual([{ orderId: 'o1', amount: 1250, reason: null }]), SLOW)
  })

  it('shows the refundable remaining as the placeholder', async () => {
    expect((await renderRefund(() => {})).placeholder).toBe('37.50')
  })

  it('refuses more decimals than the currency has', async () => {
    const input = await renderRefund(() => {})
    fireEvent.change(input, { target: { value: '1.234' } })
    expect((screen.getAllByRole('button', { name: 'Refund' }).at(-1) as HTMLButtonElement).disabled).toBe(true)
  })
})

// Offline payments: the amount uses the order currency's decimals - "1500" yen is 1500, not 150000
const unpaidJpy = { ...(order as object), currencyCode: 'JPY', paymentStatus: 'UNPAID', paymentProvider: '', paymentReference: '', total: 3000, paymentTransactions: [] } as never

async function renderRecord(onInput: (v: Record<string, unknown>) => void) {
  renderDialog(<OrderPaymentsSection order={unpaidJpy} />, [
    meMock(),
    {
      request: { query: OfflinePaymentMethodsDocument, variables: { channelId: 'c1' } },
      result: { data: { paymentMethods: [] } },
      maxUsageCount: 3,
    },
    {
      request: { query: RecordOfflinePaymentDocument, variables: captureInput(onInput) },
      result: { data: { recordOfflinePayment: { __typename: 'Order', id: 'o1', paymentStatus: 'PARTIALLY_PAID' } } },
    },
    { request: { query: OrderDocument, variables: () => true }, result: { data: { order: null } }, maxUsageCount: 3 },
    { request: { query: OrdersPagedDocument, variables: () => true }, result: { data: { ordersPaged: null } }, maxUsageCount: 3 },
  ])
  fireEvent.click(await screen.findByRole('button', { name: 'Record payment' }, SLOW))
  return screen.getByLabelText('Amount') as HTMLInputElement
}

const submitRecord = () => fireEvent.click(screen.getAllByRole('button', { name: 'Record payment' }).at(-1)!)

describe('OrderPaymentsSection offline payment', () => {
  it('converts by the order currency decimals', async () => {
    const calls: Record<string, unknown>[] = []
    const input = await renderRecord((v) => calls.push(v))
    expect(input.placeholder).toBe('3000')
    fireEvent.change(input, { target: { value: '1500' } })
    submitRecord()
    await vi.waitFor(() => expect(calls[0]).toMatchObject({ orderId: 'o1', amount: 1500 }), SLOW)
  })

  it('stores the received day as the start of that local day', async () => {
    const calls: Record<string, unknown>[] = []
    await renderRecord((v) => calls.push(v))
    fireEvent.change(screen.getByLabelText('Received on'), { target: { value: '2026-03-04' } })
    submitRecord()
    await vi.waitFor(() => expect(calls[0]).toMatchObject({ receivedAt: dayStartISO('2026-03-04') }), SLOW)
    expect(new Date(calls[0].receivedAt as string).getDate()).toBe(4)
  })

  it('refuses text that is not an amount', async () => {
    const calls: Record<string, unknown>[] = []
    const input = await renderRecord((v) => calls.push(v))
    fireEvent.change(input, { target: { value: '12abc' } })
    submitRecord()
    expect(await screen.findByText(/Enter a positive amount/)).toBeTruthy()
    expect(calls).toHaveLength(0)
  })
})
