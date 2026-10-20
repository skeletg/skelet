import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { AdjustGiftCardDialog } from './GiftCardsList'
import { AdjustGiftCardDocument, GiftCardsAdminListDocument } from './GiftCards.gql'

// What these pin: an adjustment is typed in the card currency's major units - "500" yen is 500, not 50000

function renderAdjust(currencyCode: string, onInput: (v: Record<string, unknown>) => void) {
  const card = { id: 'g1', code: 'GC-1', currencyCode, balance: 1000 } as never
  renderDialog(<AdjustGiftCardDialog card={card} onClose={() => {}} />, [
    {
      request: { query: AdjustGiftCardDocument, variables: captureInput(onInput) },
      result: { data: { adjustGiftCard: { __typename: 'GiftCard', id: 'g1', balance: 1500, status: 'active' } } },
    },
    { request: { query: GiftCardsAdminListDocument, variables: () => true }, result: { data: { giftCards: [] } } },
  ])
}

function apply(amount: string) {
  fireEvent.change(screen.getByLabelText('Amount (+/−)'), { target: { value: amount } })
  fireEvent.click(screen.getByRole('button', { name: 'Apply adjustment' }))
}

describe('AdjustGiftCardDialog', () => {
  it('converts by the card currency decimals', async () => {
    const calls: Record<string, unknown>[] = []
    renderAdjust('JPY', (v) => calls.push(v))
    apply('500')
    await vi.waitFor(() => expect(calls).toEqual([{ id: 'g1', amount: 500, note: null }]), SLOW)
  })

  it('sends two-decimal currencies as cents', async () => {
    const calls: Record<string, unknown>[] = []
    renderAdjust('USD', (v) => calls.push(v))
    apply('-2.25')
    await vi.waitFor(() => expect(calls[0]).toMatchObject({ amount: -225 }), SLOW)
  })

  it('refuses more decimals than the currency has', async () => {
    const calls: Record<string, unknown>[] = []
    renderAdjust('JPY', (v) => calls.push(v))
    apply('5.5')
    // the input's step stops the browser submitting, and toMinor refuses it if a submit gets through anyway
    expect((screen.getByLabelText('Amount (+/−)') as HTMLInputElement).step).toBe('1')
    await new Promise((r) => setTimeout(r, 50))
    expect(calls).toHaveLength(0)
  })
})
