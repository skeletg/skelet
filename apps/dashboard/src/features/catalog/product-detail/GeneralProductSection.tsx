import { useCallback, useMemo, useState, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { formatCents } from '@/lib/format'
import { TranslationLocales } from '@/components/TranslationLocales'
import {
  DeleteProductContentDocument,
  ProductDocument,
  SetProductTaxClassDocument,
  UpdateProductDocument,
  UpsertProductContentDocument,
} from '../Catalog.gql'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'
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

function dollarsToCents(input: string): number {
  const n = Number(input)
  if (!Number.isFinite(n)) return 0
  return Math.round(n * 100)
}

function centsToDollars(cents: number): string {
  return (cents / 100).toFixed(2)
}

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
  // Per-store price + publish, rendered right under Default price so all
  // pricing lives in one place
  pricingSlot?: ReactNode
}) {
  const content = product.content.find((entry) => entry.locale === locale)
  const originalName = content?.name ?? product.name ?? ''
  const originalSku = product.sku ?? ''
  const originalTaxClass = product.taxClass ?? 'standard'
  const originalPriceDollars = centsToDollars(product.defaultPrice)

  const [name, setName] = useState(originalName)
  const [sku, setSku] = useState(originalSku)
  const [taxClass, setTaxClass] = useState(originalTaxClass)
  const [priceDollars, setPriceDollars] = useState(originalPriceDollars)

  const refetchQueries = [
    'ProductsPaged',
    { query: ProductDocument, variables: productVariables },
  ]
  const [updateProduct] = useMutation(UpdateProductDocument, { refetchQueries })
  const [setTaxClassMut] = useMutation(SetProductTaxClassDocument, { refetchQueries })
  const [upsertContent] = useMutation(UpsertProductContentDocument, { refetchQueries })
  const [deleteContent] = useMutation(DeleteProductContentDocument, { refetchQueries })

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    if (name.trim() !== originalName) {
      entries.push({ label: 'Name', before: originalName, after: name.trim() })
    }
    if (sku !== originalSku) {
      entries.push({ label: 'SKU', before: originalSku, after: sku })
    }
    if (taxClass !== originalTaxClass) {
      entries.push({ label: 'Tax class', before: originalTaxClass, after: taxClass })
    }
    // Money inputs hold raw strings ("12.5") while originals arrive toFixed(2) ("12.50") - normalize BOTH sides before
    // comparing or a saved value keeps the section dirty forever
    const money = (s: string) => (s.trim() === '' ? '' : (Number(s) || 0).toFixed(2))
    if (money(priceDollars) !== money(originalPriceDollars)) {
      entries.push({
        label: 'Default price',
        before: formatCents(dollarsToCents(originalPriceDollars), product.defaultCurrencyCode),
        after: formatCents(dollarsToCents(priceDollars), product.defaultCurrencyCode),
      })
    }
    return entries
  }, [name, sku, taxClass, priceDollars, originalName, originalSku, originalTaxClass, originalPriceDollars, product.defaultCurrencyCode])

  const save = useCallback(async () => {
    if (sku !== originalSku || priceDollars !== originalPriceDollars) {
      await updateProduct({
        variables: {
          input: {
            id: product.id,
            sku: sku || undefined,
            defaultPrice: dollarsToCents(priceDollars),
          },
        },
      })
    }
    if (taxClass !== originalTaxClass) {
      await setTaxClassMut({
        variables: { productId: product.id, taxClass: taxClass.trim() || 'standard' },
      })
    }
    if (name.trim() !== originalName) {
      await upsertContent({
        variables: {
          productId: product.id,
          locale,
          input: { name: name.trim() },
        },
      })
    }
  }, [name, sku, taxClass, priceDollars, originalName, originalSku, originalTaxClass, originalPriceDollars, product.id, locale, updateProduct, setTaxClassMut, upsertContent])

  useProductSaveSection({ id: 'general', title: 'General', diff, save })

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
      <div className="grid gap-4">
        <div className="grid gap-2">
          <Label htmlFor="pd-name">Name</Label>
          <Input
            id="pd-name"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={!canManage}
          />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="pd-sku">SKU</Label>
          <Input
            id="pd-sku"
            value={sku}
            onChange={(e) => setSku(e.target.value)}
            disabled={!canManage}
            placeholder="PROD-001"
          />
          <p className="text-xs text-muted-foreground">Leave blank for configurable wrapper products.</p>
        </div>

        <div className="grid gap-2">
          <Label htmlFor="pd-tax-class">Tax class</Label>
          <Input
            id="pd-tax-class"
            value={taxClass}
            onChange={(e) => setTaxClass(e.target.value)}
            disabled={!canManage}
            placeholder="standard"
          />
          <p className="text-xs text-muted-foreground">
            Tax rules match lines on this class (e.g. standard, reduced, zero, exempt). Defaults to “standard”.
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_120px]">
          <div className="grid gap-2">
            <Label htmlFor="pd-price">Default price</Label>
            <Input
              id="pd-price"
              type="number"
              step="0.01"
              min="0"
              inputMode="decimal"
              value={priceDollars}
              onChange={(e) => setPriceDollars(e.target.value)}
              disabled={!canManage}
            />
            <p className="text-xs text-muted-foreground">
              Amount only. Scoped store pricing is edited below.
            </p>
          </div>
          <div className="grid gap-2">
            <Label>Currency</Label>
            <div className="flex h-9 items-center rounded-md border bg-muted/40 px-3">
              <code className="font-mono text-sm">{product.defaultCurrencyCode}</code>
            </div>
            <p className="text-xs text-muted-foreground">From store settings.</p>
          </div>
        </div>

        {pricingSlot}
      </div>

      <Separator className="my-1" />

      <ProductCategoriesPicker
        productId={product.id}
        categories={categories}
        primaryCategory={primaryCategory}
        canManage={canManage}
        productVariables={productVariables}
      />

      <Separator className="my-1" />

      {/* Name + the SEO/description copy elsewhere on this page are all
          per-locale - this is where a locale's copy gets dropped again */}
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
