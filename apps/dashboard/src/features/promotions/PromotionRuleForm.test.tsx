import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { PromotionRuleCreatePage, PromotionRuleEditPage } from './PromotionRuleForm'
import { CreatePromotionRuleDocument, PromotionRuleDocument, UpdatePromotionRuleDocument } from './Promotions.gql'

// What these pin: a condition row with no type is refused before the rule is created, and an edit now
// goes through the review dialog (it used to write straight away) with the dates round-tripping intact

// What every rule write answers with - the detail query adds contents and codes
const ruleFields = {
  __typename: 'PromotionRule',
  id: 'r1',
  slug: 'ten-off',
  name: '10% off',
  description: '',
  publicLabel: '',
  channelId: null,
  requiresCode: true,
  priority: 10,
  stopAfterApply: 'NONE',
  exclusivityGroup: '',
  validFrom: '2026-03-01T09:30:00Z',
  validTo: null,
  maxUses: null,
  perCustomerLimit: null,
  usedCount: 0,
  isActive: true,
  labelMediaId: null,
  bannerMediaId: null,
  pillText: '',
  pillTextColor: '',
  pillBgColor: '',
  scheduleJson: null,
  conditions: [],
  actions: [{ __typename: 'PromotionAction', id: 'a1', type: 'percent_off', params: { percent: 10 }, paramsVersion: 1, sortOrder: 0 }],
  createdAt: '2026-02-01T00:00:00Z',
  updatedAt: '2026-02-01T00:00:00Z',
}

const rule = {
  ...ruleFields,
  contents: [],
  codes: { __typename: 'PromotionCodePage', items: [], totalCount: 0 },
}

afterEach(() => localStorage.clear())

describe('PromotionRuleForm', () => {
  it('refuses an untyped condition before creating the rule', async () => {
    const onCreate = vi.fn()
    renderPage({
      path: '/promotions/new',
      adminId: 'admin-1',
      routes: { '/promotions/new': <PromotionRuleCreatePage />, '/promotions/:id': <p>rule page</p> },
      mocks: [
        meMock(),
        {
          request: { query: CreatePromotionRuleDocument, variables: captureInput(onCreate) },
          result: { data: { createPromotionRule: { ...ruleFields, id: 'r9' } } },
        },
      ],
    })

    fireEvent.change(await screen.findByLabelText('Name', {}, SLOW), { target: { value: 'Summer sale' } })
    expect((screen.getByLabelText('Slug') as HTMLInputElement).value).toBe('summer-sale')
    fireEvent.click(screen.getAllByRole('button', { name: /Add condition/ })[0])
    fireEvent.click(await screen.findByRole('button', { name: 'Create' }))
    expect(await screen.findByText(/Row 1: Pick a type/)).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()
  })

  it('reviews an edit before writing it, keeping the start time', async () => {
    const onUpdate = vi.fn()
    renderPage({
      path: '/promotions/r1',
      adminId: 'admin-1',
      routes: { '/promotions/:id': <PromotionRuleEditPage ruleId="r1" /> },
      mocks: [
        meMock(),
        { request: { query: PromotionRuleDocument, variables: { id: 'r1' } }, result: { data: { promotionRule: rule } }, maxUsageCount: 3 },
        { request: { query: UpdatePromotionRuleDocument, variables: captureInput(onUpdate) }, result: { data: { updatePromotionRule: ruleFields } } },
      ],
    })

    fireEvent.change(await screen.findByLabelText('Exclusivity group', {}, SLOW), { target: { value: 'summer' } })
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))
    await vi.waitFor(() => expect(onUpdate).toHaveBeenCalled(), SLOW)
    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'r1', exclusivityGroup: 'summer', validFrom: '2026-03-01T09:30:00.000Z', actions: [{ type: 'percent_off', params: { percent: 10 }, sortOrder: 0 }] }),
    )
  })
})
