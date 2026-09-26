import { useMemo } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { ProductDocument, ProductsPagedDocument, UpdateProductDocument } from '../Catalog.gql'
import {
  PRODUCT_ADVANCED_LABELS,
  productAdvancedDiff,
  productAdvancedSchema,
  toProductAdvancedInput,
  toProductAdvancedValues,
} from '../schemas/product'

// Advanced pricing + inventory fields, split out of the General section so the primary form stays
// focused on name / SKU / price - stages its own section; the parent groups it (and Tier pricing)
// under one collapsible

type AdvancedProduct = Parameters<typeof toProductAdvancedValues>[0] & { id: string; defaultCurrencyCode: string }

export function AdvancedInventoryPricingSection({
  product,
  canManage,
  productVariables,
}: {
  product: AdvancedProduct
  canManage: boolean
  productVariables: { id: string }
}) {
  const [updateProduct] = useMutation(UpdateProductDocument, {
    refetchQueries: [ProductsPagedDocument, { query: ProductDocument, variables: productVariables }],
  })
  const baseline = useMemo(() => toProductAdvancedValues(product), [product])
  const { form, draft } = useSectionForm({
    id: 'advanced',
    title: 'Advanced inventory & pricing',
    baseline,
    schema: productAdvancedSchema,
    diff: productAdvancedDiff,
    routeKey: `product-advanced:${product.id}`,
    submit: async (v) => {
      await updateProduct({ variables: { input: { id: product.id, ...toProductAdvancedInput(baseline, v) } } })
    },
  })
  const special = useStore(form.store, (s) => s.values.special)
  const currency = product.defaultCurrencyCode
  const money = { type: 'number', step: '0.01', min: '0', inputMode: 'decimal' as const, placeholder: '-' }
  const count = { type: 'number', min: '0', inputMode: 'numeric' as const, placeholder: '-' }

  return (
    <fieldset disabled={!canManage} className="grid gap-4">
      {draft.offer !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_ADVANCED_LABELS} />}
      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Cost &amp; sale pricing</div>
          <p className="text-xs text-muted-foreground">
            Unit cost powers margin reporting. Compare-at renders a strikethrough "was" price; a special price is what customers pay while
            its window is active (leave dates empty for an open-ended sale).
          </p>
        </div>
        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="cost">
            {(f) => <f.TextField id="adv-cost" label="Unit cost (COGS)" {...money} hint={`In ${currency}. Snapshotted onto order lines at placement.`} />}
          </form.AppField>
          <form.AppField name="compareAt">{(f) => <f.TextField id="adv-compare-at" label="Compare-at price" {...money} />}</form.AppField>
          <form.AppField name="special">{(f) => <f.TextField id="adv-special" label="Special price" {...money} />}</form.AppField>
          <div className="grid grid-cols-2 items-start gap-4 sm:col-span-1">
            <form.AppField name="specialFrom">
              {(f) => <f.TextField id="adv-special-from" label="Sale starts" type="datetime-local" disabled={!special.trim()} />}
            </form.AppField>
            <form.AppField name="specialTo">
              {(f) => <f.TextField id="adv-special-to" label="Sale ends" type="datetime-local" disabled={!special.trim()} />}
            </form.AppField>
          </div>
        </div>
      </div>

      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Shipping &amp; dimensions</div>
          <p className="text-xs text-muted-foreground">
            Weight and box dimensions feed carrier rate quotes and labels. Leave empty when not applicable (digital goods).
          </p>
        </div>
        <div className="grid grid-cols-2 items-start gap-4 sm:grid-cols-4">
          <form.AppField name="weight">{(f) => <f.TextField id="adv-weight" label="Weight (g)" {...count} />}</form.AppField>
          <form.AppField name="length">{(f) => <f.TextField id="adv-length" label="Length (mm)" {...count} />}</form.AppField>
          <form.AppField name="width">{(f) => <f.TextField id="adv-width" label="Width (mm)" {...count} />}</form.AppField>
          <form.AppField name="height">{(f) => <f.TextField id="adv-height" label="Height (mm)" {...count} />}</form.AppField>
        </div>
      </div>

      <div className="grid gap-3 rounded-lg border border-dashed p-4">
        <div>
          <div className="text-sm font-medium">Order quantity limits</div>
          <p className="text-xs text-muted-foreground">Minimum and maximum units per order line. Leave empty for no limit.</p>
        </div>
        <div className="grid grid-cols-2 items-start gap-4 sm:max-w-sm">
          <form.AppField name="minQty">{(f) => <f.TextField id="adv-min-qty" label="Min order qty" {...count} step="any" inputMode="decimal" />}</form.AppField>
          <form.AppField name="maxQty">{(f) => <f.TextField id="adv-max-qty" label="Max order qty" {...count} step="any" inputMode="decimal" />}</form.AppField>
        </div>
      </div>
    </fieldset>
  )
}
