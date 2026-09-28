import { fireEvent, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, renderSection } from '@/test/page-harness'
import { UpdateProductDocument, UpsertProductContentDocument } from '../Catalog.gql'
import { ProductSeoEditor } from './ProductSeoEditor'

// What these pin: the product update carries only what changed, and an emptied canonical URL is
// sent as '' - the backend reads a missing one as "keep it"

const product = {
  id: 'p1',
  name: 'Tee',
  url: 'tee',
  urlAuto: true,
  urlTemplateOverride: '',
  canonicalUrl: 'https://example.com/tee',
  seoNoindex: false,
  content: [{ locale: 'en', name: 'Tee', metaTitle: 'Buy tee', metaDescription: '', metaKeywords: '' }],
  media: [],
  productType: null,
} as never

const recorder = (fn: (v: Record<string, unknown>) => void) => (vars: Record<string, unknown>) => {
  fn(vars)
  return true
}

function renderSeo() {
  const writes = { update: vi.fn(), content: vi.fn() }
  renderSection(<ProductSeoEditor product={product} locale="en" canManage productVariables={{ id: 'p1' }} />, [
    { request: { query: UpdateProductDocument, variables: recorder(writes.update) }, result: { data: { updateProduct: null } } },
    { request: { query: UpsertProductContentDocument, variables: recorder(writes.content) }, result: { data: { upsertProductContent: null } } },
  ])
  return writes
}

describe('ProductSeoEditor', () => {
  it('clears the canonical URL and leaves the meta copy alone', async () => {
    const writes = renderSeo()
    fireEvent.change(screen.getByLabelText('Canonical URL'), { target: { value: '' } })
    fireEvent.click(screen.getByRole('switch', { name: 'Noindex' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    const dialog = await screen.findByRole('alertdialog')
    fireEvent.click(within(dialog).getByRole('button', { name: /Apply 2 changes/ }))
    await vi.waitFor(() => expect(writes.update).toHaveBeenCalled(), SLOW)
    expect(writes.update).toHaveBeenCalledWith({ input: { id: 'p1', canonicalUrl: '', seoNoindex: true } })
    expect(writes.content).not.toHaveBeenCalled()
  })
})
