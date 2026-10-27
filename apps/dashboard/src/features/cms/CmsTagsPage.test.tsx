import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, meMock, renderPage } from '@/test/page-harness'
import { CmsTagsPage } from './CmsTagsPage'
import { CmsTagsDocument, RenameCmsTagDocument } from './Cms.gql'

// What this pins: a tag rename waits for the header Save and its review - it used to write on blur, unreviewed

const tag = { __typename: 'CMSTag' as const, slug: 'summer', name: 'Summer', pageCount: 3, createdAt: '2026-01-01T00:00:00Z' }

describe('CmsTagsPage rename', () => {
  it('stages a rename until Save, then writes it', async () => {
    const renamed = vi.fn()
    renderPage({
      path: '/cms/tags',
      routes: { '/cms/tags': <CmsTagsPage /> },
      mocks: [
        meMock(),
        { request: { query: CmsTagsDocument }, result: { data: { cmsTags: [tag] } }, maxUsageCount: 5 },
        { request: { query: RenameCmsTagDocument, variables: captureInput(renamed) }, result: { data: { renameCMSTag: { ...tag, name: 'Summer sale' } } } },
      ],
    })
    const input = await screen.findByLabelText('Name of tag summer', {}, SLOW)
    fireEvent.change(input, { target: { value: 'Summer sale' } })
    fireEvent.blur(input)
    expect(renamed).not.toHaveBeenCalled()

    fireEvent.click(await screen.findByRole('button', { name: 'Save' }, SLOW))
    const dialog = await screen.findByRole('alertdialog')
    expect(within(dialog).getByText('Tag · summer · name')).toBeTruthy()
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply/ }))
    await vi.waitFor(() => expect(renamed).toHaveBeenCalledWith({ slug: 'summer', name: 'Summer sale' }), SLOW)
  })
})
