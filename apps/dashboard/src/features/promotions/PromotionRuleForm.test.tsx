import { fireEvent, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { PromotionRuleCreatePage, PromotionRuleEditPage } from './PromotionRuleForm'
import { CreatePromotionRuleDocument, PromotionRuleCodesDocument, PromotionRuleDocument, UpdatePromotionRuleDocument } from './Promotions.gql'

// What these pin: a condition row with no type is refused before the rule is created, and an edit now
// goes through the review dialog (it used to write straight away) with the dates round-tripping intact

// What every rule write answers with - the detail query adds contents, and the codes table asks for its own page
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
        codesMock(),
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

  // A rule can hold thousands of generated codes - the table used to load the first 1000 and search those in the
  // browser, so the rest could never be found. Search, the Active filter and the page now reach the server
  it('asks the server for the searched, filtered page of codes', async () => {
    const asked = vi.fn()
    const filter = encodeURIComponent(JSON.stringify([{ id: 'isActive', value: ['true'] }]))
    renderPage({
      path: `/promotions/r1?codes_q=spring&codes_f=${filter}&codes_size=10`,
      adminId: 'admin-1',
      routes: { '/promotions/:id': <PromotionRuleEditPage ruleId="r1" /> },
      mocks: [
        meMock(),
        { request: { query: PromotionRuleDocument, variables: { id: 'r1' } }, result: { data: { promotionRule: rule } }, maxUsageCount: 3 },
        codesMock(asked, 45),
      ],
    })
    expect(await screen.findByText('SPRING-1', {}, SLOW)).toBeDefined()
    expect(asked).toHaveBeenCalledWith({ id: 'r1', search: 'spring', isActive: true, limit: 10, offset: 0 })
    expect(screen.getByText('45 rows')).toBeDefined()
  })
})

function codesMock(asked = vi.fn(), totalCount = 0) {
  const code = {
    __typename: 'PromotionCode', id: 'c1', ruleId: 'r1', code: 'SPRING-1', maxUses: null, perCustomerLimit: null, validFrom: null,
    validTo: null, restrictedToEmail: '', usedCount: 0, isActive: true, createdAt: '2026-03-01T00:00:00Z', updatedAt: '2026-03-01T00:00:00Z',
  }
  return {
    request: { query: PromotionRuleCodesDocument, variables: captureInput(asked) },
    result: { data: { promotionRule: { __typename: 'PromotionRule', id: 'r1', codes: { __typename: 'PromotionCodeList', items: totalCount ? [code] : [], totalCount } } } },
    maxUsageCount: 3,
  }
}
