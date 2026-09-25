import { useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { ChevronDownIcon } from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Separator } from '@/components/ui/separator'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { ProductDocument, UpdateProductDocument } from '../Catalog.gql'
import { PRODUCT_SEO_LABELS, productSeoChanges, productSeoDiff, productSeoSchema, toProductSeoValues } from '../schemas/product'
import { UrlRewritesSection } from '../UrlRewritesSection'
import type { ContentProduct } from './shared'
import { useProductContentWrite } from './productContent'

// The product's URL, canonical and noindex, plus the locale's meta copy - one staged section that
// writes the product and the content row only for the parts that changed

export function ProductSeoEditor({
  product,
  locale,
  canManage,
  productVariables,
}: {
  product: ContentProduct
  locale: string
  canManage: boolean
  productVariables: { id: string }
}) {
  const content = product.content.find((entry) => entry.locale === locale)
  const existingName = content?.name ?? product.name ?? product.content[0]?.name ?? 'Untitled product'
  const [urlRewritesOpen, setUrlRewritesOpen] = useState(false)

  const [updateProduct] = useMutation(UpdateProductDocument, {
    refetchQueries: ['ProductsPaged', { query: ProductDocument, variables: productVariables }],
  })
  const writeContent = useProductContentWrite(product.id, locale)

  const baseline = useMemo(() => toProductSeoValues(product, locale), [product, locale])
  const { form, draft } = useSectionForm({
    id: 'seo',
    title: 'SEO',
    baseline,
    schema: productSeoSchema,
    diff: productSeoDiff,
    routeKey: `product-seo:${product.id}:${locale}`,
    submit: async (v) => {
      const changes = productSeoChanges(baseline, v)
      if (changes.update) await updateProduct({ variables: { input: { id: product.id, ...changes.update } } })
      if (changes.content) await writeContent(changes.content)
    },
  })
  const values = useStore(form.store, (s) => s.values)
  const ptTemplate = product.productType?.urlTemplate ?? ''
  const effectiveTemplate = values.urlTemplateOverride.trim() !== '' ? values.urlTemplateOverride : ptTemplate
  const urlDirty = !values.urlAuto && values.url !== baseline.url

  return (
    <div className="grid gap-6">
      {draft.restoredAt !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_SEO_LABELS} />}
      <fieldset disabled={!canManage} className="grid gap-4">
        <div className="grid gap-3 rounded-md border p-4">
          <form.AppField name="urlAuto">
            {(f) => (
              <f.SwitchField
                id="pd-url-auto"
                label="Auto-generate URL"
                description="When on, the URL is computed from the product type's template (or the per-product override below), falling back to the product name. Turn off to set the full URL by hand."
                disabled={!canManage}
                bordered={false}
              />
            )}
          </form.AppField>

          {values.urlAuto ? (
            <div className="grid gap-2">
              <form.AppField name="urlTemplateOverride">
                {(f) => (
                  <f.TextField
                    id="pd-url-override"
                    label="Template override"
                    placeholder={ptTemplate || '(no product type template; falls back to the product name)'}
                  />
                )}
              </form.AppField>
              <p className="text-xs text-muted-foreground">
                Empty inherits from the product type. Tokens: <code>{'{name}'}</code>, <code>{'{category}'}</code>,{' '}
                <code>{'{categorySlug}'}</code>, <code>{'{attr:<slug>}'}</code> (e.g. <code>{'{attr:model}'}</code>).
              </p>
              {effectiveTemplate ? (
                <p className="text-xs text-muted-foreground">
                  Effective template: <span className="font-mono">{effectiveTemplate}</span>
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">No template set - the URL falls back to the product name.</p>
              )}
              {product.url && (
                <p className="text-xs text-muted-foreground">
                  Current URL: <span className="font-mono">{product.url}</span>
                </p>
              )}
            </div>
          ) : (
            <div className="grid gap-2">
              <form.AppField name="url">
                {(f) => (
                  <f.TextField
                    id="pd-url"
                    label="URL"
                    placeholder="shoes/nike/my-product"
                    hint={
                      <>
                        Full URL path for this product. Leave empty to auto-generate from the product name. Include a category prefix to
                        nest the URL, e.g. <span className="font-mono">shoes/nike/my-product</span>.
                      </>
                    }
                  />
                )}
              </form.AppField>
              {urlDirty && (
                <p className="text-xs text-warning">A 301 redirect will be created from the old URL automatically.</p>
              )}
            </div>
          )}
        </div>

        <form.AppField name="metaTitle">
          {(f) => <f.TextField id="pd-meta-title" label="Meta title" placeholder={existingName} hint={`${values.metaTitle.length}/60`} />}
        </form.AppField>
        <form.AppField name="metaDescription">
          {(f) => (
            <f.TextareaField
              id="pd-meta-description"
              label="Meta description"
              rows={4}
              placeholder="Search result description for this product."
              hint={`${values.metaDescription.length}/160`}
            />
          )}
        </form.AppField>
        <form.AppField name="metaKeywords">
          {(f) => <f.TextField id="pd-meta-keywords" label="Meta keywords" placeholder="keyword1, keyword2, keyword3" />}
        </form.AppField>
        <form.AppField name="canonicalUrl">
          {(f) => (
            <f.TextField
              id="pd-canonical-url"
              label="Canonical URL"
              placeholder="https://example.com/products/my-product"
              hint="Overrides the default canonical. Leave blank to use the product's own URL."
            />
          )}
        </form.AppField>
        <form.AppField name="seoNoindex">
          {(f) => <f.SwitchField id="pd-seo-noindex" label="Noindex" description="Ask search engines not to index this product." disabled={!canManage} />}
        </form.AppField>
      </fieldset>

      <Separator />

      <Collapsible open={urlRewritesOpen} onOpenChange={setUrlRewritesOpen}>
        <CollapsibleTrigger asChild>
          <button className="flex w-full items-start justify-between gap-4 rounded-md border bg-muted/40 px-4 py-3 text-left transition-colors hover:bg-muted/70">
            <div className="grid gap-1">
              <span className="text-base font-medium">URL rewrites</span>
              <span className="text-sm text-muted-foreground">Review canonical paths and manage old product URLs that redirect here.</span>
            </div>
            <ChevronDownIcon className={`mt-1 size-4 shrink-0 text-muted-foreground transition-transform ${urlRewritesOpen ? 'rotate-180' : ''}`} />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-4">
          <div className="grid gap-3">
            <p className="text-sm text-muted-foreground">
              Keep permanent redirects for renamed products so existing links and search results keep working.
            </p>
            <UrlRewritesSection entityId={product.id} entityLabel="product" canManage={canManage} />
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
