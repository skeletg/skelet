import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { ProductTypeFormDialog } from './ProductTypeFormDialog'
import {
  CreateProductTypeDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  SetProductTypeUrlPathTemplateDocument,
  UpdateProductTypeDocument,
} from './Catalog.gql'

const lists = [
  { request: { query: ProductTypesDocument }, result: { data: { productTypes: [] } }, maxUsageCount: 5 },
  { request: { query: ProductTypeDocument, variables: () => true }, result: { data: { productType: null } }, maxUsageCount: 5 },
]

describe('ProductTypeFormDialog', () => {
  it('turns shipping off for digital goods and hands the new type back', async () => {
    const onCreate = vi.fn()
    const onCreated = vi.fn()
    renderDialog(<ProductTypeFormDialog open onOpenChange={() => {}} onCreated={onCreated} />, [
      ...lists,
      {
        request: { query: CreateProductTypeDocument, variables: captureInput(onCreate) },
        result: { data: { createProductType: { __typename: 'ProductType', id: 'pt1', name: 'E-book' } } },
      },
    ])
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'E-book' } })
    fireEvent.click(screen.getByLabelText('Digital'))
    expect(screen.getByLabelText('Requires shipping').getAttribute('aria-checked')).toBe('false')
    fireEvent.click(screen.getByRole('button', { name: 'Create product type' }))

    await vi.waitFor(() => expect(onCreated).toHaveBeenCalledWith({ __typename: 'ProductType', id: 'pt1', name: 'E-book' }), SLOW)
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ slug: 'e-book', name: 'E-book', isDigital: true, isShippingRequired: false }),
    )
  })

  it('routes a changed URL template through the mutation that recomputes product URLs', async () => {
    const onUpdate = vi.fn()
    const onTemplate = vi.fn()
    const productType = {
      id: 'pt1',
      slug: 'tee',
      name: 'T-shirt',
      isDigital: false,
      isShippingRequired: true,
      urlTemplate: '{name}',
      tracksInventory: false,
    }
    const onOpenChange = vi.fn()
    renderDialog(<ProductTypeFormDialog open onOpenChange={onOpenChange} productType={productType} />, [
      ...lists,
      {
        request: { query: UpdateProductTypeDocument, variables: captureInput(onUpdate) },
        result: { data: { updateProductType: { __typename: 'ProductType', id: 'pt1' } } },
      },
      {
        request: {
          query: SetProductTypeUrlPathTemplateDocument,
          variables: (vars: Record<string, unknown>) => {
            onTemplate(vars)
            return true
          },
        },
        result: { data: { setProductTypeUrlPathTemplate: { __typename: 'ProductType', id: 'pt1' } } },
      },
    ])
    fireEvent.change(screen.getByLabelText('URL path template'), { target: { value: '{category}/{name}' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onUpdate.mock.calls[0][0]).not.toHaveProperty('urlTemplate')
    expect(onTemplate).toHaveBeenCalledWith({ productTypeId: 'pt1', template: '{category}/{name}' })
  })
})
