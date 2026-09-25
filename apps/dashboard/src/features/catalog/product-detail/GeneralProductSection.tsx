import { useMemo, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { TranslationLocales } from '@/components/TranslationLocales'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import {
  DeleteProductContentDocument,
  ProductDocument,
  SetProductTaxClassDocument,
  UpdateProductDocument,
  UpsertProductContentDocument,
} from '../Catalog.gql'
import {
  PRODUCT_GENERAL_LABELS,
  productGeneralChanges,
  productGeneralDiff,
  productGeneralSchema,
  toProductGeneralValues,
} from '../schemas/product'
import { Section, type ProductCategoryItem, type ProductContentEntry } from './shared'
import { ProductCategoriesPicker } from './ProductCategoriesPicker'

type GeneralProduct = {
  id: string
  name?: string | null
  sku?: string | null
  taxClass?: string | null
  defaultPrice: number
  defaultCurrencyCode: string
  content: ProductContentEntry[]
}

// The locale's name, the SKU, tax class and default price - staged into the product page's save
// registry, with its own stored draft
export function GeneralProductSection({
  product,
  locale,
  scopeLabel,
  canManage,
  productVariables,
  categories,
  primaryCategory,
  pricingSlot,
}: {
  product: GeneralProduct
  locale: string
  scopeLabel: string
  canManage: boolean
  productVariables: { id: string }
  categories: ProductCategoryItem[]
  primaryCategory: ProductCategoryItem | null
  // Per-store price + publish, rendered right under Default price so all pricing lives in one place
  pricingSlot?: ReactNode
}) {
  const refetchQueries = ['ProductsPaged', { query: ProductDocument, variables: productVariables }]
  const [updateProduct] = useMutation(UpdateProductDocument, { refetchQueries })
  const [setTaxClassMut] = useMutation(SetProductTaxClassDocument, { refetchQueries })
  const [upsertContent] = useMutation(UpsertProductContentDocument, { refetchQueries })
  const [deleteContent] = useMutation(DeleteProductContentDocument, { refetchQueries })

  const baseline = useMemo(() => toProductGeneralValues(product, locale), [product, locale])
  const { form, draft } = useSectionForm({
    id: 'general',
    title: 'General',
    baseline,
    schema: productGeneralSchema,
    diff: productGeneralDiff(product.defaultCurrencyCode),
    routeKey: `product-general:${product.id}:${locale}`,
    submit: async (v) => {
      const changes = productGeneralChanges(baseline, v)
      if (changes.update) await updateProduct({ variables: { input: { id: product.id, ...changes.update } } })
      if (changes.taxClass) await setTaxClassMut({ variables: { productId: product.id, taxClass: changes.taxClass } })
      if (changes.name) await upsertContent({ variables: { productId: product.id, locale, input: { name: changes.name } } })
    },
  })

  return (
    <Section
      title="General"
      description={`Product name, SKU, and pricing for ${scopeLabel}. Cost, sale pricing, shipping, and volume tiers are under Advanced inventory & pricing; the URL is under SEO.`}
      trailing={
        <Badge variant="outline" className="font-mono font-normal">
          {locale}
        </Badge>
      }
    >
      {draft.restoredAt !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_GENERAL_LABELS} />}
      <fieldset disabled={!canManage} className="grid gap-4">
        <form.AppField name="name">{(f) => <f.TextField id="pd-name" label="Name" />}</form.AppField>
        <form.AppField name="sku">
          {(f) => <f.TextField id="pd-sku" label="SKU" placeholder="PROD-001" hint="Leave blank for configurable wrapper products." />}
        </form.AppField>
        <form.AppField name="taxClass">
          {(f) => (
            <f.TextField
              id="pd-tax-class"
              label="Tax class"
              placeholder="standard"
              hint="Tax rules match lines on this class (e.g. standard, reduced, zero, exempt). Defaults to “standard”."
            />
          )}
        </form.AppField>

        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-[1fr_120px]">
          <form.AppField name="priceDollars">
            {(f) => (
              <f.TextField
                id="pd-price"
                label="Default price"
                type="number"
                step="0.01"
                min="0"
                inputMode="decimal"
                hint="Amount only. Scoped store pricing is edited below."
              />
            )}
          </form.AppField>
          <div className="grid gap-2">
            <Label>Currency</Label>
            <div className="flex h-9 items-center rounded-md border bg-muted/40 px-3">
              <code className="font-mono text-sm">{product.defaultCurrencyCode}</code>
            </div>
            <p className="text-xs text-muted-foreground">From store settings.</p>
          </div>
        </div>
      </fieldset>

      {pricingSlot}

      <Separator className="my-1" />

      <ProductCategoriesPicker
        productId={product.id}
        categories={categories}
        primaryCategory={primaryCategory}
        canManage={canManage}
        productVariables={productVariables}
      />

      <Separator className="my-1" />

      {/* Name and the SEO/description copy elsewhere on this page are per-locale - this is where a
          locale's copy gets dropped again */}
      <TranslationLocales
        locale={locale}
        locales={product.content.map((entry) => entry.locale)}
        entityLabel="product"
        canManage={canManage}
        defaultLocaleLocked
        onRemove={async () => {
          await deleteContent({ variables: { productId: product.id, locale } })
        }}
      />
    </Section>
  )
}
