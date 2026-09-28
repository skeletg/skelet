import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderPage } from '@/test/page-harness'
import { CmsBlockTypeDetail } from './CmsBlockTypeDetail'
import { CmsBlockTypeDocument, UpdateCmsBlockTypeDocument } from './Cms.gql'
import { blockTypeDiff, blockTypeSchema, toBlockTypeValues } from './schemas/blockType'

// Through the real page: relabelling a field lights the header Save (it never did - the section
// registered into a context that wasn't there yet, and the diff only compared the key list), the
// dialog names the field, and two fields on one key are stopped at the field

const blockType = {
  __typename: 'CMSBlockType' as const,
  id: 'bt1',
  key: 'promo',
  name: 'Promo',
  icon: '',
  htmlTemplate: '<strong>{{heading}}</strong>',
  fields: [{ key: 'heading', label: 'Heading', kind: 'text', required: true }],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

describe('block type schema', () => {
  it('diffs a relabelled field by key', () => {
    const before = toBlockTypeValues(blockType)
    const after = { ...before, fields: [{ ...before.fields[0], label: 'Title' }] }
    expect(blockTypeDiff(before, after)).toEqual([
      { kind: 'change', label: 'Field · heading', before: 'Heading (text, required)', after: 'Title (text, required)' },
    ])
  })

  it('rejects two fields on one key', () => {
    const values = toBlockTypeValues(blockType)
    const dup = { ...values, fields: [...values.fields, { ...values.fields[0] }] }
    expect(blockTypeSchema.safeParse(dup).error?.issues[0]).toMatchObject({ path: ['fields', 1, 'key'] })
  })
})

describe('CmsBlockTypeDetail', () => {
  function renderEdit(onUpdate = vi.fn()) {
    renderPage({
      path: '/cms/block-types/bt1',
      routes: { '/cms/block-types/:id': <CmsBlockTypeDetail /> },
      mocks: [
        { request: { query: CmsBlockTypeDocument, variables: { id: 'bt1' } }, result: { data: { cmsBlockType: blockType } }, maxUsageCount: 3 },
        {
          request: { query: UpdateCmsBlockTypeDocument, variables: captureInput(onUpdate) },
          result: { data: { updateCMSBlockType: { __typename: 'CMSBlockType', id: 'bt1' } } },
        },
      ],
    })
    return onUpdate
  }

  it('reviews and saves a relabelled field', async () => {
    const onUpdate = renderEdit()
    fireEvent.change(await screen.findByLabelText('Field 1 label'), { target: { value: 'Title' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Field · heading')).toBeDefined()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 1 change/ }))

    await vi.waitFor(
      () =>
        expect(onUpdate).toHaveBeenCalledWith(
          expect.objectContaining({ id: 'bt1', fields: [{ key: 'heading', label: 'Title', kind: 'text', required: true }] }),
        ),
      SLOW,
    )
  })

  it('stops a duplicate field key at the field', async () => {
    const onUpdate = renderEdit()
    await screen.findByLabelText('Field 1 key')
    fireEvent.click(screen.getByRole('button', { name: 'Add field' }))
    fireEvent.change(screen.getByLabelText('Field 2 key'), { target: { value: 'heading' } })
    expect(await screen.findByText('Field key "heading" is used twice')).toBeDefined()

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    expect(await within(dialog).findByText(/used twice/)).toBeDefined()
    expect(onUpdate).not.toHaveBeenCalled()
  })
})
