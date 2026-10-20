import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { usePageTitle } from '@/lib/page-title'
import {
  CheckCircle2Icon,
  CircleOffIcon,
  Trash2Icon,
} from 'lucide-react'
import { LoadingState } from '@/components/ui/loading-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
} from '@/components/layout/detail-page'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  ClearProductAttributeDocument,
  DeleteProductDocument,
  ProductDocument,
  SetConfigurableOptionsDocument,
  SetProductVariantModeDocument,
  SetProductAttributesDocument,
  UpdateProductDocument,
  ProductsPagedDocument,
} from './Catalog.gql'
import { AttributesEditor } from './AttributesEditor'
import { ProductReferencesCard } from './ProductReferencesCard'
import { BundleStructureSection } from './BundleStructureSection'
import { GroupedStructureSection } from './GroupedStructureSection'
import { KitStructureSection } from './KitStructureSection'
import { ProductOptionsSection } from './ProductOptionsSection'
import { ProductDownloadsSection } from './ProductDownloadsSection'
import { SubscriptionTermsSection } from './SubscriptionTermsSection'
import { ChildrenSection } from './ChildrenSection'
import { ProductListingsEditor } from './ProductListingsEditor'
import { ProductSaveContext } from './productSaveRegistry'
import { useSaveRegistry } from '@/shared/saving/saveRegistry'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { ProductInventorySection } from '@/features/inventory/ProductInventorySection'
import { LoadError } from '@/shared/errors/LoadError'
import {
  CollapsibleSection,
  KindBadge,
  ScopeIndicator,
} from './product-detail/shared'
import { GeneralProductSection } from './product-detail/GeneralProductSection'
import { ProductSidebar } from './product-detail/ProductSidebar'
import { AdvancedInventoryPricingSection } from './product-detail/AdvancedInventoryPricingSection'
import { TierPricingSection } from './product-detail/TierPricingSection'
import { RecommendationsSection } from './product-detail/RecommendationsSection'
import { ContentAndMediaSection } from './product-detail/ContentAndMediaSection'
import { ProductSeoEditor } from './product-detail/ProductSeoEditor'
import { ChannelUrlOverridesEditor } from './product-detail/ChannelUrlOverridesEditor'
import { ConfigurableVariantsCard } from './product-detail/ConfigurableVariantsCard'
import { ProductMetadataEditor } from './product-detail/MetadataEditor'
import { PluginSections } from '@/plugins/detail-sections'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

export function ProductDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const { scope, selectedChannel, effectiveLocale, globalDefaultLocale } = useConfigScope()
  const canManage = has('catalog.manage')
  // In Global mode the header doesn't pick a locale; we fall back to the server-resolved default (product.contentLocale) so the form edits
  // the canonical translation row - To edit a different locale, switch the header to a channel + locale
  const scopedLocale = effectiveLocale || undefined
  const translationLocale = effectiveLocale
  // Locale is attached as the `Locale` header by Apollo's `localeLink`
  const productVariables = { id }

  const productQuery = useQuery(ProductDocument, {
    variables: productVariables,
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const product = productQuery.data?.product
  usePageTitle(product?.name ?? product?.sku)

  const productRefetch = [{ query: ProductDocument, variables: productVariables }]
  const [deleteProduct, { loading: deleting }] = useMutation(DeleteProductDocument, {
    refetchQueries: [ProductsPagedDocument],
  })
  const [setAttrs, { loading: savingAttrs }] = useMutation(SetProductAttributesDocument, {
    refetchQueries: productRefetch,
  })
  const [clearAttr, { loading: clearingAttr }] = useMutation(ClearProductAttributeDocument, {
    refetchQueries: productRefetch,
  })
  const [setOptions] = useMutation(SetConfigurableOptionsDocument, {
    refetchQueries: productRefetch,
  })
  const [setVariantMode] = useMutation(SetProductVariantModeDocument, {
    refetchQueries: productRefetch,
  })
  const [updateProductMetadata] = useMutation(UpdateProductDocument, {
    refetchQueries: productRefetch,
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [attributesOpen, setAttributesOpen] = useState(false)
  const [referencesOpen, setReferencesOpen] = useState(false)
  // STABLE array references: the attribute editors reset their draft state
  // whenever the attributeValues prop changes identity, so a fresh
  // product.attributeValues.filter(...) inline in JSX would wipe edits on every
  // render - Memoize the reference/non-reference split so identity is stable
  // until the product data actually changes
  const nonReferenceAttrs = useMemo(
    () => (product?.attributeValues ?? []).filter((av) => av.attribute.inputType !== 'REFERENCE'),
    [product],
  )
  const referenceAttrs = useMemo(
    () => (product?.attributeValues ?? []).filter((av) => av.attribute.inputType === 'REFERENCE'),
    [product],
  )
  const [variantsOpen, setVariantsOpen] = useState(false)
  const [downloadsOpen, setDownloadsOpen] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [subscriptionOpen, setSubscriptionOpen] = useState(false)
  const [seoOpen, setSeoOpen] = useState(false)
  const [metadataOpen, setMetadataOpen] = useState(false)

  // Section registry: each editable section reports its dirty diff + save fn - this page had its own
  // copy of the hook's internals, which is how it missed out on the unsaved-changes guard
  const {
    contextValue: saveContextValue,
    dirtySections,
    isDirty,
    saveAll,
  } = useSaveRegistry(product?.name ? `Product '${product.name}'` : undefined)

  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  async function onDelete() {
    await deleteProduct({ variables: { id } })
    navigate('/products', { replace: true })
  }

  if (productQuery.loading) return <LoadingState label="Loading product…" />
  if (productQuery.error) {
    return (
      <LoadError
        title="Couldn't load product"
        message={productQuery.error.message}
        onRetry={() => void productQuery.refetch()}
        retrying={productQuery.loading}
      />
    )
  }
  if (!product) return <p className="text-sm text-muted-foreground">Product not found.</p>

  const displayName = product.name ?? product.content[0]?.name ?? product.sku ?? ''
  const publishedCount = product.listings.filter((l) => l.isPublished).length

  const scopeLabel =
    scope.mode === 'CHANNEL'
      ? `${selectedChannel?.name ?? 'Selected store'} · ${scopedLocale ?? selectedChannel?.defaultLocale ?? 'default'}`
      : `Global · ${product.contentLocale ?? 'default'}`

  return (
    <ProductSaveContext.Provider value={saveContextValue}>
    <DetailPageLayout rightSidebar={<ProductSidebar product={product} />}>
      <DetailPageHeader
        backTo="/products"
        backLabel="Back to products"
        title={displayName}
        badges={
          <>
            <KindBadge kind={product.kind} />
            {publishedCount > 0 ? (
              <Badge variant="secondary" className="gap-1">
                <CheckCircle2Icon className="size-3" /> Published on {publishedCount} store{publishedCount === 1 ? '' : 's'}
              </Badge>
            ) : (
              <Badge variant="outline" className="gap-1 text-muted-foreground">
                <CircleOffIcon className="size-3" /> Unpublished
              </Badge>
            )}
          </>
        }
        meta={
          <>
            <code className="font-mono">{product.url}</code>
            <span>·</span>
            <Link to={`/catalog/product-types/${product.productType.id}`} className="hover:underline">
              {product.productType.name}
            </Link>
            <span>·</span>
            <span>
              Default <span className="font-mono text-foreground">{formatCents(product.defaultPrice, product.defaultCurrencyCode)}</span>
            </span>
            {product.sku ? (
              <>
                <span>·</span>
                <span>
                  SKU <code className="font-mono text-foreground">{product.sku}</code>
                </span>
              </>
            ) : null}
          </>
        }
        actions={
          <>
            <ScopeIndicator
              mode={scope.mode}
              storeName={selectedChannel?.name ?? null}
              locale={
                scope.mode === 'CHANNEL'
                  ? scopedLocale ?? selectedChannel?.defaultLocale ?? null
                  : product.contentLocale ?? null
              }
            />
            {canManage ? (
              <>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => setConfirmingDelete(true)}
                  disabled={deleting}
                >
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
                <Button
                  type="button"
                  size="sm"
                  disabled={!isDirty || savingAll}
                  onClick={() => { setSaveError(null); setConfirmingSave(true) }}
                >
                  {isDirty
                    ? `Save changes${dirtySections.length > 0 ? ` (${dirtySections.reduce((n, s) => n + s.diff.length, 0)})` : ''}`
                    : 'No changes'}
                </Button>
              </>
            ) : null}
          </>
        }
      />

      <DetailPageBody>
          <GeneralProductSection
            key={`${product.id}:${translationLocale || 'no-locale'}:general`}
            product={product}
            locale={translationLocale || product.contentLocale || product.content[0]?.locale || globalDefaultLocale}
            scopeLabel={
              scope.mode === 'CHANNEL'
                ? `${selectedChannel?.name ?? 'Selected store'} / ${scopedLocale ?? selectedChannel?.defaultLocale ?? 'default locale'}`
                : `Global / ${product.contentLocale ?? 'default locale'}`
            }
            canManage={canManage}
            productVariables={productVariables}
            categories={product.categories ?? []}
            primaryCategory={product.primaryCategory ?? null}
            pricingSlot={
              <div className="grid gap-2">
                <div>
                  <div className="text-sm font-medium">Channel price &amp; availability</div>
                  <p className="text-xs text-muted-foreground">
                    {scope.mode === 'CHANNEL'
                      ? 'Override the default amount, or publish/unpublish, on the active store. Switch stores in the header.'
                      : 'Read-only summary per store. Switch the header to a store to edit its price and publish state.'}
                  </p>
                </div>
                {scope.mode === 'CHANNEL' ? (
                  <ProductListingsEditor
                    product={product}
                    canManage={canManage}
                    channelId={scope.channelId}
                  />
                ) : (
                  <div className="rounded-md border bg-muted/30 p-4 text-sm">
                    {product.listings.length === 0 ? (
                      <p className="text-muted-foreground">Not listed on any store yet. Pick a store in the header to add a listing.</p>
                    ) : (
                      <ul className="space-y-1">
                        {product.listings.map((l) => (
                          <li key={l.channel.id} className="flex items-center justify-between gap-2">
                            <span className="font-medium">{l.channel.name}</span>
                            <span className="text-muted-foreground">
                              {l.isPublished ? 'published' : 'unpublished'}
                              {' · '}
                              {l.price != null ? `${(l.price / 100).toFixed(2)} ${l.effectiveCurrency}` : `inherits ${l.effectiveCurrency}`}
                            </span>
                          </li>
                        ))}
                      </ul>
                    )}
                  </div>
                )}
              </div>
            }
          />

          <CollapsibleSection
            title="Advanced inventory & pricing"
            description="Unit cost, sale pricing, volume tiers, shipping dimensions, and order-quantity limits."
            open={advancedOpen}
            onOpenChange={setAdvancedOpen}
          >
            <div className="grid gap-4">
              <AdvancedInventoryPricingSection
                key={`${product.id}:advanced`}
                product={product}
                canManage={canManage}
                productVariables={productVariables}
              />
              <TierPricingSection
                key={`${product.id}:tier-pricing`}
                productId={product.id}
                groupPrices={product.groupPrices ?? []}
                currencyCode={product.defaultCurrencyCode}
                canManage={canManage}
                productVariables={productVariables}
              />
            </div>
          </CollapsibleSection>

          <RecommendationsSection
            key={`${product.id}:links`}
            productId={product.id}
            canManage={canManage}
          />

          <ContentAndMediaSection
            key={`${product.id}:${translationLocale || 'no-locale'}:content-media`}
            product={product}
            locale={translationLocale || product.contentLocale || product.content[0]?.locale || globalDefaultLocale}
            scopeLabel={
              scope.mode === 'CHANNEL'
                ? `${selectedChannel?.name ?? 'Selected store'} / ${scopedLocale ?? selectedChannel?.defaultLocale ?? 'default locale'}`
                : `Global / ${product.contentLocale ?? 'default locale'}`
            }
            canManage={canManage}
            productVariables={productVariables}
          />

          <ProductInventorySection productId={product.id} />

          <CollapsibleSection
            title="Attributes"
            description={
              <>
                Inherited from{' '}
                <Link to={`/catalog/product-types/${product.productType.id}`} className="hover:underline">
                  {product.productType.name}
                </Link>
                .
              </>
            }
            open={attributesOpen}
            onOpenChange={setAttributesOpen}
          >
            <AttributesEditor
              productId={product.id}
              attributeValues={nonReferenceAttrs}
              attributeGroups={product.productType.groups}
              canManage={canManage}
              saving={savingAttrs}
              clearing={clearingAttr}
              onSave={async (assignments) => {
                await setAttrs({ variables: { productId: product.id, assignments } })
              }}
              onClear={async (attributeId) => {
                await clearAttr({ variables: { productId: product.id, attributeId } })
              }}
              emptyHint="This product type has no attributes assigned."
            />
          </CollapsibleSection>

          {referenceAttrs.length > 0 && (
            <CollapsibleSection
              title="References"
              description="Link this product to other products, categories, or collections (reference attributes)."
              open={referencesOpen}
              onOpenChange={setReferencesOpen}
            >
              <ProductReferencesCard
                productId={product.id}
                canManage={canManage}
                referenceAttrs={referenceAttrs}
              />
            </CollapsibleSection>
          )}

          {product.kind === 'CONFIGURABLE' ? (
            <CollapsibleSection
              title="Variants"
              description="Configure which attributes define variants, then link child products that provide those option values."
              open={variantsOpen}
              onOpenChange={setVariantsOpen}
            >
              <div className="grid gap-6">
                <ConfigurableVariantsCard
                  product={product}
                  canManage={canManage}
                  onSaveOptions={async (attributeIds) => {
                    await setOptions({ variables: { productId: product.id, attributeIds } })
                  }}
                  onSaveMode={async (mode) => {
                    await setVariantMode({ variables: { productId: product.id, mode } })
                  }}
                />
                <Separator />
                <ChildrenSection
                  product={product}
                  canManage={canManage}
                  embedded
                />
              </div>
            </CollapsibleSection>
          ) : null}

          {product.kind === 'BUNDLE' ? (
            <CollapsibleSection
              title="Bundle structure"
              description="Define the bundle's option groups and the simple products that fill them."
              open={variantsOpen}
              onOpenChange={setVariantsOpen}
            >
              <BundleStructureSection product={product} canManage={canManage} />
            </CollapsibleSection>
          ) : null}

          {product.kind === 'GROUPED' ? (
            <CollapsibleSection
              title="Grouped members"
              description="Pick the simple products this group displays together. Each member is added to the cart individually."
              open={variantsOpen}
              onOpenChange={setVariantsOpen}
            >
              <GroupedStructureSection product={product} canManage={canManage} />
            </CollapsibleSection>
          ) : null}

          {product.kind === 'KIT' ? (
            <CollapsibleSection
              title="Kit components"
              description="The fixed set of simple products this kit assembles. Sold as one line at the kit's own price; fulfillment decrements each component."
              open={variantsOpen}
              onOpenChange={setVariantsOpen}
            >
              <KitStructureSection product={product} canManage={canManage} />
            </CollapsibleSection>
          ) : null}

          {product.kind === 'SIMPLE' || product.kind === 'CONFIGURABLE' ? (
            <CollapsibleSection
              title="Product options"
              description="Custom inputs the customer fills in at add-to-cart (text, select, date, file). Each option can adjust the line price."
              open={variantsOpen}
              onOpenChange={setVariantsOpen}
            >
              <ProductOptionsSection product={product} canManage={canManage} />
            </CollapsibleSection>
          ) : null}

          {product.kind !== 'GROUPED' ? (
            <CollapsibleSection
              title="Downloads"
              description="Digital files this product delivers. Purchasing grants the customer token-gated download links."
              open={downloadsOpen}
              onOpenChange={setDownloadsOpen}
            >
              <ProductDownloadsSection product={product} canManage={canManage} />
            </CollapsibleSection>
          ) : null}

          {product.kind === 'SUBSCRIPTION' ? (
            <CollapsibleSection
              title="Subscription terms"
              description="Billing interval, frequency, and trial for this recurring product."
              open={subscriptionOpen}
              onOpenChange={setSubscriptionOpen}
            >
              <SubscriptionTermsSection product={product} canManage={canManage} />
            </CollapsibleSection>
          ) : null}

          <CollapsibleSection
            title="SEO"
            description="Set the product URL (auto-generated or by hand), per-store URL overrides, search metadata, and indexing."
            open={seoOpen}
            onOpenChange={setSeoOpen}
          >
            <div className="grid gap-6">
              <ProductSeoEditor
                key={`${product.id}:${scopedLocale ?? 'default'}:seo`}
                product={product}
                locale={translationLocale || product.contentLocale || product.content[0]?.locale || globalDefaultLocale}
                canManage={canManage}
                productVariables={productVariables}
              />
              <ChannelUrlOverridesEditor
                key={`${product.id}:channel-urls`}
                productId={product.id}
                listings={product.listings}
                canManage={canManage}
              />
            </div>
          </CollapsibleSection>

          <CollapsibleSection
            title="Metadata"
            description="Edit product metadata and private metadata key-value pairs."
            open={metadataOpen}
            onOpenChange={setMetadataOpen}
          >
            <ProductMetadataEditor
              key={`${product.id}:metadata`}
              productId={product.id}
              metadata={product.metadata}
              privateMetadata={product.privateMetadata}
              canManage={canManage}
              onSave={async ({ metadata, privateMetadata }) => {
                await updateProductMetadata({
                  variables: {
                    input: {
                      id: product.id,
                      metadata,
                      privateMetadata,
                    },
                  },
                })
              }}
            />
          </CollapsibleSection>

          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-6 py-4 text-xs text-muted-foreground">
            <span>Created {new Date(product.createdAt).toLocaleDateString()}</span>
            <span>·</span>
            <span>Updated {new Date(product.updatedAt).toLocaleDateString()}</span>
          </div>
        <PluginSections target="product" entity={product} mode="edit" />
      </DetailPageBody>
    </DetailPageLayout>

      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={<>Delete {displayName}?</>}
        description="Permanently removes the product and all its attribute assignments."
        confirmLabel="Delete product"
        busyLabel="Deleting…"
        destructive
        onConfirm={onDelete}
      />

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={scopeLabel}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { setConfirmingSave(false); setSaveError(null) }}
        onConfirm={onConfirmSave}
      />
    </ProductSaveContext.Provider>
  )
}
