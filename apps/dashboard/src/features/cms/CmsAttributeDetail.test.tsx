import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CmsAttributeDetail } from './CmsAttributeDetail'
import { CmsAttributesDocument, UpdateCmsAttributeDocument } from './Cms.gql'

// What these pin: a number attribute's rules are checked before they're written (a max below the
// min used to reach the server), and the rules the input type supports go as one update

const attribute = {
  __typename: 'CMSAttribute' as const,
  id: 'a1',
  slug: 'reading_time',
  inputType: 'NUMBER' as const,
  isTranslatable: false,
  isRequiredDefault: false,
  unit: 'min',
  validation: { min: 1 },
  defaultValue: null,
  relationshipTarget: null,
  values: [],
  content: { __typename: 'CMSAttributeContent' as const, locale: 'en', name: 'Reading time', helperText: '', placeholder: '' },
  allContent: [],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

describe('CmsAttributeDetail', () => {
  it('refuses a max below the min, then writes the rules and default', async () => {
    const onUpdate = vi.fn()
    renderPage({
      path: '/cms/attributes/a1',
      routes: { '/cms/attributes/:id': <CmsAttributeDetail /> },
      mocks: [
        meMock(),
        { request: { query: CmsAttributesDocument }, result: { data: { cmsAttributes: [attribute] } }, maxUsageCount: 3 },
        {
          request: { query: UpdateCmsAttributeDocument, variables: captureInput(onUpdate) },
          result: { data: { updateCMSAttribute: attribute } },
        },
      ],
    })

    const save = await screen.findByRole('button', { name: 'Save changes' })
    fireEvent.change(screen.getByLabelText('Max'), { target: { value: '0' } })
    fireEvent.change(screen.getByLabelText('Default value'), { target: { value: '3' } })
    fireEvent.click(save)
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/Max must be at least min/)).toBeDefined()
    expect(onUpdate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Max'), { target: { value: '30' } })
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(
      () => expect(onUpdate).toHaveBeenCalledWith({ id: 'a1', validation: { min: 1, max: 30 }, defaultValue: 3 }),
      SLOW,
    )
  })
})
