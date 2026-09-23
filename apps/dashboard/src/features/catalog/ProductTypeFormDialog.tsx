import { useRef, useState, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { slugify } from '@/lib/slug'
import {
  CreateProductTypeDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  SetProductTypeUrlPathTemplateDocument,
  UpdateProductTypeDocument,
} from './Catalog.gql'
import {
  productTypeSchema,
  toProductTypeCreateInput,
  toProductTypeUpdateInput,
  toProductTypeValues,
  type ExistingProductType,
} from './schemas/productType'

export type { ExistingProductType }

type Created = { id: string; name: string }

export function ProductTypeFormDialog({
  productType,
  trigger,
  open,
  onOpenChange,
  onCreated,
}: {
  productType?: ExistingProductType
  trigger?: ReactNode
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onCreated?: (productType: Created) => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} trigger={trigger}>
      {(close) => <ProductTypeForm productType={productType} onCreated={onCreated} onClose={close} />}
    </FormDialog>
  )
}

function ProductTypeForm({
  productType,
  onCreated,
  onClose,
}: {
  productType?: ExistingProductType
  onCreated?: (productType: Created) => void
  onClose: () => void
}) {
  const [slugTouched, setSlugTouched] = useState(false)
  const refetchQueries = productType
    ? [{ query: ProductTypesDocument }, { query: ProductTypeDocument, variables: { id: productType.id } }]
    : [{ query: ProductTypesDocument }]
  const [createProductType] = useMutation(CreateProductTypeDocument, { refetchQueries })
  const [updateProductType] = useMutation(UpdateProductTypeDocument, { refetchQueries })
  const [setUrlTemplate] = useMutation(SetProductTypeUrlPathTemplateDocument, { refetchQueries })
  const created = useRef<Created | null>(null)

  const dialog = useDialogForm({
    baseline: toProductTypeValues(productType),
    schema: productTypeSchema,
    submit: async (v) => {
      if (productType) {
        const { input, newTemplate } = toProductTypeUpdateInput(v, productType)
        await updateProductType({ variables: { input } })
        if (newTemplate !== null) await setUrlTemplate({ variables: { productTypeId: productType.id, template: newTemplate } })
        return
      }
      const result = await createProductType({ variables: { input: toProductTypeCreateInput(v) } })
      created.current = result.data?.createProductType ?? null
    },
    onSaved: () => {
      onClose()
      if (created.current) onCreated?.(created.current)
    },
  })
  const { form } = dialog
  const isDigital = useStore(form.store, (s) => s.values.isDigital)

  return (
    <FormDialogBody
      dialog={dialog}
      title={productType ? `Edit ${productType.name}` : 'New product type'}
      description={
        productType
          ? 'Slug is immutable after creation.'
          : 'Product types are the blueprint products inherit - they fix which attributes are shared.'
      }
      submitLabel={productType ? 'Save changes' : 'Create product type'}
      onCancel={onClose}
    >
      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
        <form.AppField
          name="name"
          listeners={{
            onChange: ({ value }) => {
              if (!productType && !slugTouched) form.setFieldValue('slug', slugify(value))
            },
          }}
        >
          {(f) => <f.TextField id="pt-name" label="Name" placeholder="T-shirt" />}
        </form.AppField>
        <form.AppField name="slug">
          {(f) => (
            <f.TextField
              id="pt-slug"
              label="Slug"
              placeholder="t-shirt"
              disabled={!!productType}
              onValueChange={() => setSlugTouched(true)}
              hint={productType ? 'Slug is immutable.' : 'Auto-filled from name - edit to override.'}
            />
          )}
        </form.AppField>
      </div>

      <div className="grid gap-3 rounded-md border p-4">
        <form.AppField
          name="isDigital"
          listeners={{
            // Digital goods never ship
            onChange: ({ value }) => {
              if (value) form.setFieldValue('isShippingRequired', false)
            },
          }}
        >
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="pt-digital"
              label="Digital"
              description="Digital goods have no physical shipment (e.g. downloads, gift cards)."
            />
          )}
        </form.AppField>
        <form.AppField name="isShippingRequired">
          {(f) => (
            <f.SwitchField
              bordered={false}
              id="pt-shipping"
              label="Requires shipping"
              description="Physical goods that need a fulfillment step."
              disabled={isDigital}
            />
          )}
        </form.AppField>
      </div>

      <form.AppField name="tracksInventory">
        {(f) => (
          <f.SwitchField
            id="pt-tracks-inv"
            label="Track inventory"
            description="New products of this type opt into inventory tracking by default. Per-product override is available on the product page."
          />
        )}
      </form.AppField>

      <form.AppField name="urlTemplate">
        {(f) => (
          <f.TextField
            id="pt-url-template"
            label="URL path template"
            placeholder="{category}/{attr:brand}/{name}"
            hint={
              <>
                Empty disables auto-generation. Tokens: <code>{'{name}'}</code>, <code>{'{category}'}</code>,{' '}
                <code>{'{categorySlug}'}</code>, <code>{'{attr:<slug>}'}</code> (e.g. <code>{'{attr:model}'}</code>). Multiselect
                attributes aren't allowed.
              </>
            }
          />
        )}
      </form.AppField>
    </FormDialogBody>
  )
}
