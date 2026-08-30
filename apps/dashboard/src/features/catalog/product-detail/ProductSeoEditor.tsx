import { useCallback, useMemo, useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { ChevronDownIcon } from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  ProductDocument,
  UpdateProductDocument,
  UpsertProductContentDocument,
} from '../Catalog.gql'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'
import { UrlRewritesSection } from '../UrlRewritesSection'
import type { ContentProduct } from './shared'

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

  const originalMetaTitle = content?.metaTitle ?? product.metaTitle ?? ''
  const originalMetaDescription = content?.metaDescription ?? product.metaDescription ?? ''
  const originalMetaKeywords = content?.metaKeywords ?? ''
  const originalUrl = product.url ?? ''
  const originalUrlAuto = product.urlAuto
  const originalUrlTemplateOverride = product.urlTemplateOverride ?? ''
  const originalCanonicalUrl = product.canonicalUrl ?? ''
  const originalSeoNoindex = product.seoNoindex

  const [metaTitle, setMetaTitle] = useState(originalMetaTitle)
  const [metaDescription, setMetaDescription] = useState(originalMetaDescription)
  const [metaKeywords, setMetaKeywords] = useState(originalMetaKeywords)
  const [url, setUrl] = useState(originalUrl)
  const [urlAuto, setUrlAuto] = useState(originalUrlAuto)
  const [urlTemplateOverride, setUrlTemplateOverride] = useState(originalUrlTemplateOverride)
  const [canonicalUrl, setCanonicalUrl] = useState(originalCanonicalUrl)
  const [seoNoindex, setSeoNoindex] = useState(originalSeoNoindex)
  const [urlRewritesOpen, setUrlRewritesOpen] = useState(false)
  const ptTemplate = product.productType?.urlTemplate ?? ''
  const effectiveTemplate = urlTemplateOverride.trim() !== '' ? urlTemplateOverride : ptTemplate

  const urlDirty = !urlAuto && url !== originalUrl

  const refetchQueries = [
    'ProductsPaged',
    { query: ProductDocument, variables: productVariables },
  ]
  const [updateProduct] = useMutation(UpdateProductDocument, { refetchQueries })
  const [upsertContent] = useMutation(UpsertProductContentDocument, { refetchQueries })

  // Diff (only includes URL + SEO changes - content meta is diffed separately below)
  const productDirty =
    (!urlAuto && url !== originalUrl) ||
    urlAuto !== originalUrlAuto ||
    urlTemplateOverride !== originalUrlTemplateOverride ||
    canonicalUrl !== originalCanonicalUrl ||
    seoNoindex !== originalSeoNoindex
  const contentDirty =
    metaTitle.trim() !== originalMetaTitle.trim() ||
    metaDescription.trim() !== originalMetaDescription.trim() ||
    metaKeywords.trim() !== originalMetaKeywords.trim()

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    if (urlAuto !== originalUrlAuto) {
      entries.push({
        label: 'Auto-generate URL',
        before: originalUrlAuto ? 'on' : 'off',
        after: urlAuto ? 'on' : 'off',
      })
    }
    if (!urlAuto && url !== originalUrl) {
      entries.push({ label: 'URL', before: originalUrl || '-', after: url || '-' })
    }
    if (urlTemplateOverride !== originalUrlTemplateOverride) {
      entries.push({
        label: 'URL template override',
        before: originalUrlTemplateOverride || '-',
        after: urlTemplateOverride || '-',
      })
    }
    if (canonicalUrl.trim() !== originalCanonicalUrl.trim()) {
      entries.push({
        label: 'Canonical URL',
        before: originalCanonicalUrl || '-',
        after: canonicalUrl.trim() || '-',
      })
    }
    if (seoNoindex !== originalSeoNoindex) {
      entries.push({
        label: 'Noindex',
        before: originalSeoNoindex ? 'yes' : 'no',
        after: seoNoindex ? 'yes' : 'no',
      })
    }
    if (metaTitle.trim() !== originalMetaTitle.trim()) {
      entries.push({ label: 'Meta title', before: originalMetaTitle || '-', after: metaTitle.trim() || '-' })
    }
    if (metaDescription.trim() !== originalMetaDescription.trim()) {
      entries.push({
        label: 'Meta description',
        before: originalMetaDescription || '-',
        after: metaDescription.trim() || '-',
      })
    }
    if (metaKeywords.trim() !== originalMetaKeywords.trim()) {
      entries.push({
        label: 'Meta keywords',
        before: originalMetaKeywords || '-',
        after: metaKeywords.trim() || '-',
      })
    }
    return entries
  }, [
    url, originalUrl, urlAuto, originalUrlAuto,
    urlTemplateOverride, originalUrlTemplateOverride, canonicalUrl, originalCanonicalUrl,
    seoNoindex, originalSeoNoindex, metaTitle, originalMetaTitle,
    metaDescription, originalMetaDescription, metaKeywords, originalMetaKeywords,
  ])

  const save = useCallback(async () => {
    if (productDirty) {
      await updateProduct({
        variables: {
          input: {
            id: product.id,
            // The backend rejects a manual url while auto is on (ErrManualPathWhileAuto), so only forward url when auto
            // is off An empty manual url tells the backend to auto-generate from the product name
            ...(urlAuto ? {} : { url: url.trim() }),
            urlAuto,
            urlTemplateOverride,
            canonicalUrl: canonicalUrl.trim() || undefined,
            seoNoindex,
          },
        },
      })
    }
    if (contentDirty) {
      await upsertContent({
        variables: {
          productId: product.id,
          locale,
          input: {
            name: existingName,
            metaTitle: metaTitle.trim() || undefined,
            metaDescription: metaDescription.trim() || undefined,
            metaKeywords: metaKeywords.trim() || undefined,
          },
        },
      })
    }
  }, [
    productDirty, contentDirty, urlAuto, url,
    urlTemplateOverride, canonicalUrl, seoNoindex, metaTitle, metaDescription,
    metaKeywords, product.id, locale, existingName, updateProduct, upsertContent,
  ])

  useProductSaveSection({ id: 'seo', title: 'SEO', diff, save })

  return (
    <div className="grid gap-6">
      <div className="grid gap-4">

      <div className="grid gap-3 rounded-md border p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-0.5">
            <Label htmlFor="pd-url-auto" className="text-sm">Auto-generate URL</Label>
            <p className="text-xs text-muted-foreground">
              When on, the URL is computed from the product type's template (or
              the per-product override below), falling back to the product name.
              Turn off to set the full URL by hand.
            </p>
          </div>
          <Switch
            id="pd-url-auto"
            checked={urlAuto}
            onCheckedChange={setUrlAuto}
            disabled={!canManage}
          />
        </div>

        {urlAuto ? (
          <div className="grid gap-2">
            <Label htmlFor="pd-url-override">Template override</Label>
            <Input
              id="pd-url-override"
              value={urlTemplateOverride}
              onChange={(e) => setUrlTemplateOverride(e.target.value)}
              disabled={!canManage}
              placeholder={ptTemplate || '(no product type template; falls back to the product name)'}
            />
            <p className="text-xs text-muted-foreground">
              Empty inherits from the product type. Tokens: <code>{'{name}'}</code>,
              {' '}<code>{'{category}'}</code>, <code>{'{categorySlug}'}</code>,
              {' '}<code>{'{attr:<slug>}'}</code> (e.g. <code>{'{attr:model}'}</code>).
            </p>
            {effectiveTemplate ? (
              <p className="text-xs text-muted-foreground">
                Effective template: <span className="font-mono">{effectiveTemplate}</span>
              </p>
            ) : (
              <p className="text-xs text-muted-foreground">
                No template set - the URL falls back to the product name.
              </p>
            )}
            {product.url && (
              <p className="text-xs text-muted-foreground">
                Current URL: <span className="font-mono">{product.url}</span>
              </p>
            )}
          </div>
        ) : (
          <div className="grid gap-2">
            <Label htmlFor="pd-url">URL</Label>
            <Input
              id="pd-url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              disabled={!canManage}
              placeholder="shoes/nike/my-product"
            />
            <p className="text-xs text-muted-foreground">
              Full URL path for this product. Leave empty to auto-generate from
              the product name. Include a category prefix to nest the URL, e.g.
              {' '}<span className="font-mono">shoes/nike/my-product</span>.
            </p>
            {urlDirty && (
              <p className="text-xs text-amber-600 dark:text-amber-400">
                A 301 redirect will be created from the old URL automatically.
              </p>
            )}
          </div>
        )}
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <Label htmlFor="pd-meta-title">Meta title</Label>
          <span className="text-xs text-muted-foreground">{metaTitle.length}/60</span>
        </div>
        <Input
          id="pd-meta-title"
          value={metaTitle}
          onChange={(e) => setMetaTitle(e.target.value)}
          disabled={!canManage}
          placeholder={existingName}
        />
      </div>

      <div className="grid gap-2">
        <div className="flex items-center justify-between gap-3">
          <Label htmlFor="pd-meta-description">Meta description</Label>
          <span className="text-xs text-muted-foreground">{metaDescription.length}/160</span>
        </div>
        <Textarea
          id="pd-meta-description"
          value={metaDescription}
          onChange={(e) => setMetaDescription(e.target.value)}
          disabled={!canManage}
          rows={4}
          placeholder="Search result description for this product."
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="pd-meta-keywords">Meta keywords</Label>
        <Input
          id="pd-meta-keywords"
          value={metaKeywords}
          onChange={(e) => setMetaKeywords(e.target.value)}
          disabled={!canManage}
          placeholder="keyword1, keyword2, keyword3"
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="pd-canonical-url">Canonical URL</Label>
        <Input
          id="pd-canonical-url"
          value={canonicalUrl}
          onChange={(e) => setCanonicalUrl(e.target.value)}
          disabled={!canManage}
          placeholder="https://example.com/products/my-product"
        />
        <p className="text-xs text-muted-foreground">
          Overrides the default canonical. Leave blank to use the product's own URL.
        </p>
      </div>

      <div className="flex items-center justify-between gap-4 rounded-md border p-3">
        <div className="grid gap-1">
          <Label htmlFor="pd-seo-noindex">Noindex</Label>
          <p className="text-sm text-muted-foreground">
            Ask search engines not to index this product.
          </p>
        </div>
        <Switch
          id="pd-seo-noindex"
          checked={seoNoindex}
          onCheckedChange={setSeoNoindex}
          disabled={!canManage}
        />
      </div>

      </div>

      <Separator />

      <Collapsible open={urlRewritesOpen} onOpenChange={setUrlRewritesOpen}>
        <CollapsibleTrigger asChild>
          <button className="flex w-full items-start justify-between gap-4 rounded-md border bg-muted/40 px-4 py-3 text-left transition-colors hover:bg-muted/70">
            <div className="grid gap-1">
              <span className="text-base font-medium">URL rewrites</span>
              <span className="text-sm text-muted-foreground">
                Review canonical paths and manage old product URLs that redirect here.
              </span>
            </div>
            <ChevronDownIcon className={`mt-1 size-4 shrink-0 text-muted-foreground transition-transform ${urlRewritesOpen ? 'rotate-180' : ''}`} />
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-4">
          <div className="grid gap-3">
            <p className="text-sm text-muted-foreground">
              Keep permanent redirects for renamed products so existing links and search results
              keep working.
            </p>
            <UrlRewritesSection entityId={product.id} entityLabel="product" canManage={canManage} />
          </div>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
