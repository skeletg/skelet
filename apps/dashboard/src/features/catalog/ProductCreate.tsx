import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  BoxIcon,
  GiftIcon,
  LayersIcon,
  PackageIcon,
  RepeatIcon,
  ShoppingBagIcon,
  SlidersHorizontalIcon,
  TruckIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import type { ProductAttributeAssignmentInput, ProductKind } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  filterChannelsByAllowedScopes,
  localesForAllowedChannel,
  useConfigScope,
} from '@/lib/config-scope'
import {
  AttributeControl,
  buildAssignment,
  emptyAttrState,
  type AttrState,
  type AttributeSummary,
} from './AttributesEditor'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import {
  CreateProductDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  SetProductAttributesDocument,
} from './Catalog.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { LexicalRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'

function dollarsToCents(input: string): number {
  const n = Number(input)
  if (!Number.isFinite(n)) return 0
  return Math.round(n * 100)
}

function isValidPrice(input: string): boolean {
  const n = Number(input)
  return Number.isFinite(n) && n >= 0
}

function isValidCurrencyCode(input: string): boolean {
  return /^[A-Z]{3}$/.test(input)
}

function uniqueLocales(input: string[]): string[] {
  return Array.from(new Set(input.filter(Boolean)))
}

// nullifyEmpty returns the trimmed string, or null when it's blank - the shape
// the nullable String inputs on CreateProductInput expect
function nullifyEmpty(input: string): string | null {
  const trimmed = input.trim()
  return trimmed === '' ? null : trimmed
}

type KindOption = {
  value: ProductKind
  label: string
  description: string
  Icon: typeof BoxIcon
}

const KIND_OPTIONS: KindOption[] = [
  {
    value: 'SIMPLE',
    label: 'Simple',
    description: 'A standalone product with its own SKU, price, and inventory.',
    Icon: BoxIcon,
  },
  {
    value: 'CONFIGURABLE',
    label: 'Configurable',
    description: 'A wrapper that groups simple-product variants by option attributes (size, color, …).',
    Icon: SlidersHorizontalIcon,
  },
  {
    value: 'BUNDLE',
    label: 'Bundle',
    description: 'A pick-your-mix product: customer chooses components from option menus you define after creation.',
    Icon: LayersIcon,
  },
  {
    value: 'GROUPED',
    label: 'Grouped',
    description: 'A list of simple products shown side-by-side. Each is added to cart individually with its own price.',
    Icon: ShoppingBagIcon,
  },
  {
    value: 'KIT',
    label: 'Kit',
    description: 'A fixed assembly sold as one SKU at its own price. Fulfillment decrements the component products you pick after creation.',
    Icon: PackageIcon,
  },
  {
    value: 'GIFTCARD',
    label: 'Gift card',
    description: 'Sells stored value at its face price. Buying one issues a redeemable card; it carries no inventory.',
    Icon: GiftIcon,
  },
  {
    value: 'SUBSCRIPTION',
    label: 'Subscription',
    description: 'A recurring plan billed per cycle. Set the interval, frequency and trial on the product after creation.',
    Icon: RepeatIcon,
  },
]

// Single source of truth - the picker and both submit gates read the same list, so an offered kind can never be
// rejected on submit - All seven backend kinds are creatable; there's no declared-but-unsupported tier
const CREATABLE_KINDS: string[] = KIND_OPTIONS.map((o) => o.value)

export function ProductCreate() {
  const navigate = useNavigate()
  const permissions = usePermissions()
  const { globalDefaultLocale } = useConfigScope()
  const allowedCatalogScopes = useMemo(
    () => permissions.allowedChannelScopes('catalog.manage'),
    [permissions],
  )

  const productTypesQuery = useQuery(ProductTypesDocument)
  const channelsQuery = useQuery(ChannelsDocument)
  const currenciesQuery = useQuery(CurrenciesPickerDocument)
  const currencyOptions = currenciesQuery.data?.currencies ?? []

  const draft = useDraft({
    kind: 'SIMPLE' as ProductKind,
    productTypeId: '',
    name: '',
    shortDescriptionText: '',
    shortDescription: null as RichTextValue,
    descriptionText: '',
    descriptionRichText: null as RichTextValue,
    sku: '',
    priceDollars: '0.00',
    currencyCode: 'USD',
    locale: '',
    urlAuto: true,
    url: '',
    urlTemplateOverride: '',
    metaTitle: '',
    metaDescription: '',
    metaKeywords: '',
    canonicalUrl: '',
    seoNoindex: false,
    attrState: {} as Record<string, AttrState>,
  }, 'New product')
  const { kind, productTypeId, name, shortDescriptionText, shortDescription, descriptionText, descriptionRichText, sku, priceDollars, currencyCode, locale, urlAuto, url, urlTemplateOverride, metaTitle, metaDescription, metaKeywords, canonicalUrl, seoNoindex, attrState } = draft.values
  const { setKind, setProductTypeId, setName, setShortDescriptionText, setShortDescription, setDescriptionText, setDescriptionRichText, setSku, setPriceDollars, setCurrencyCode, setLocale, setUrlAuto, setUrl, setUrlTemplateOverride, setMetaTitle, setMetaDescription, setMetaKeywords, setCanonicalUrl, setSeoNoindex, setAttrState } = draft.setters

  // Search & URL. urlAuto derives the path from the product-type template (or name); the backend rejects urlAuto=true
  // together with an explicit url, so we only send one of the two based on this toggle


  const [submitError, setSubmitError] = useState<string | null>(null)

  const productTypeQuery = useQuery(ProductTypeDocument, {
    variables: { id: productTypeId },
    skip: !productTypeId,
  })
  const productType = productTypeQuery.data?.productType
  const channels = useMemo(
    () => filterChannelsByAllowedScopes(channelsQuery.data?.channels ?? [], allowedCatalogScopes),
    [allowedCatalogScopes, channelsQuery.data],
  )

  const availableLocales = useMemo(() => {
    const all = channels.flatMap((sv) => localesForAllowedChannel(sv, allowedCatalogScopes))
    return uniqueLocales(all)
  }, [allowedCatalogScopes, channels])
  const fallbackLocale =
    globalDefaultLocale && (availableLocales.length === 0 || availableLocales.includes(globalDefaultLocale))
      ? globalDefaultLocale
      : availableLocales[0] ?? ''
  const selectedLocale =
    locale && (availableLocales.length === 0 || availableLocales.includes(locale))
      ? locale
      : fallbackLocale

  const nonVariantAttrs: AttributeSummary[] = useMemo(() => {
    if (!productType) return []
    return productType.attributes.map((a) => a.attribute)
  }, [productType])

  const [createProduct, { loading: creating }] = useMutation(CreateProductDocument, {
    refetchQueries: ['ProductsPaged'],
  })
  const [setAttrs, { loading: savingAttrs }] = useMutation(SetProductAttributesDocument)
  const submitting = creating || savingAttrs

  function patchAttr(attrId: string, patch: Partial<AttrState>) {
    setAttrState((prev) => ({ ...prev, [attrId]: { ...prev[attrId], ...patch } }))
  }

  function validateRequired(): string | null {
    for (const a of nonVariantAttrs) {
      if (!a.valueRequired) continue
      const s = attrState[a.id]
      if (!s) return `${a.name} is required.`
      const isSelectable = ['DROPDOWN', 'MULTISELECT', 'SWATCH'].includes(a.inputType)
      if (isSelectable) {
        if (s.valueIds.length === 0) return `${a.name} is required.`
      } else if (a.inputType === 'BOOLEAN') {
        // any explicit value satisfies it
      } else {
        if (!s.scalarString.trim()) return `${a.name} is required.`
      }
    }
    return null
  }

  function validateForm(): string | null {
    if (!CREATABLE_KINDS.includes(kind)) return 'This product kind is not yet supported.'
    if (!productTypeId) return 'Pick a product type first.'
    if (!name.trim()) return 'Name is required.'
    if (!selectedLocale.trim()) return 'Pick a translation locale.'
    if (!isValidPrice(priceDollars)) return 'Default price must be zero or greater.'
    if (!isValidCurrencyCode(currencyCode)) return 'Default currency must be a 3-letter ISO code such as USD.'
    return validateRequired()
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    const validationError = validateForm()
    if (validationError) {
      setSubmitError(validationError)
      return
    }

    try {
      const createResult = await createProduct({
        variables: {
          input: {
            productTypeId,
            kind,
            name: name.trim(),
            locale: selectedLocale,
            shortDescription: shortDescriptionText.trim() ? shortDescription : undefined,
            descriptionRichText: descriptionText.trim() ? descriptionRichText : undefined,
            // SKU is required at the schema level; the service auto-generates one from the name when blank, so we
            // forward an empty string here when the admin leaves it empty
            sku: sku.trim(),
            defaultPrice: dollarsToCents(priceDollars),
            defaultCurrencyCode: currencyCode,
            // URL: auto-generate (optionally with a template override) OR an
            // explicit path - never both (the backend rejects that pairing)
            urlAuto,
            url: urlAuto ? null : nullifyEmpty(url),
            urlTemplateOverride: urlAuto ? nullifyEmpty(urlTemplateOverride) : null,
            // Search metadata for the initial locale
            metaTitle: nullifyEmpty(metaTitle),
            metaDescription: nullifyEmpty(metaDescription),
            metaKeywords: nullifyEmpty(metaKeywords),
            canonicalUrl: nullifyEmpty(canonicalUrl),
            seoNoindex,
          },
        },
      })
      const newId = createResult.data?.createProduct.id
      if (!newId) {
        setSubmitError('Create succeeded but no id returned. Please reload and check the products list.')
        return
      }

      const assignments: ProductAttributeAssignmentInput[] = nonVariantAttrs
        .map((a) => buildAssignment(a, attrState[a.id] ?? emptyAttrState()))
        .filter((a) => {
          if (a.valueIds != null) return a.valueIds.length > 0
          return a.scalarValue !== null && a.scalarValue !== undefined
        })

      if (assignments.length > 0) {
        try {
          await setAttrs({ variables: { productId: newId, assignments } })
        } catch (attrErr) {
          setSubmitError(
            (attrErr instanceof Error ? attrErr.message : 'Attribute write failed.') +
              ' The product was created - finish attribute setup on its detail page.',
          )
          await duringCommit(() => navigate(`/products/${newId}`))
          return
        }
      }

      await duringCommit(() => navigate(`/products/${newId}`))
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  const productTypes = productTypesQuery.data?.productTypes ?? []
  const formReady =
    CREATABLE_KINDS.includes(kind) &&
    Boolean(productTypeId) &&
    Boolean(name.trim()) &&
    Boolean(selectedLocale.trim()) &&
    isValidPrice(priceDollars) &&
    isValidCurrencyCode(currencyCode)

  return (
    <form id="product-create-form" onSubmit={onSubmit}>
      <DetailPageLayout>
        <DetailPageHeader
          backTo="/products"
          backLabel="Back to products"
          title="New product"
          meta={
            <span>
              Basics and first translation. Store listings, per-store prices, publishing, media
              {kind === 'CONFIGURABLE' ? ', and variants' : ''} are set on the detail page after creating.
            </span>
          }
          actions={
            <>
              <Button type="button" variant="outline" size="sm" asChild>
                <Link to="/products">Cancel</Link>
              </Button>
              <Button type="submit" size="sm" disabled={submitting || !formReady}>
                {submitting ? 'Creating…' : 'Create product'}
              </Button>
            </>
          }
        />

        <DetailPageBody>
          {productTypesQuery.error && (
            <div className="px-6 pt-4">
              <Alert variant="destructive">
                <AlertTitle>Couldn't load product types</AlertTitle>
                <AlertDescription>{productTypesQuery.error.message}</AlertDescription>
              </Alert>
            </div>
          )}

          <DetailPageSection
            title="Kind"
            description="The product's structural shape. Immutable after creation."
          >
            <div className="grid gap-3 sm:grid-cols-2">
              {KIND_OPTIONS.map((opt) => {
                const Icon = opt.Icon
                const selected = kind === opt.value
                return (
                  <button
                    key={opt.value}
                    type="button"
                    onClick={() => setKind(opt.value)}
                    aria-pressed={selected}
                    className={[
                      'group relative flex flex-col items-start gap-1 rounded-md border px-3 py-3 text-left transition',
                      selected
                        ? 'border-primary bg-primary/5 shadow-sm'
                        : 'hover:border-foreground/30 hover:bg-accent/30',
                    ].join(' ')}
                  >
                    <div className="flex w-full items-center justify-between">
                      <div className="flex items-center gap-2">
                        <Icon className={`size-4 ${selected ? 'text-primary' : 'text-muted-foreground'}`} />
                        <span className="text-sm font-medium">{opt.label}</span>
                      </div>
                      {selected ? (
                        <Badge className="text-[10px] uppercase tracking-wide">Selected</Badge>
                      ) : null}
                    </div>
                    <p className="text-xs text-muted-foreground">{opt.description}</p>
                  </button>
                )
              })}
            </div>
            {kind === 'CONFIGURABLE' ? (
              <Alert>
                <AlertTitle>Heads up</AlertTitle>
                <AlertDescription>
                  After creation, open the detail page to define the option attributes (size, color, …)
                  and link the simple-product variants that fill them.
                </AlertDescription>
              </Alert>
            ) : null}
          </DetailPageSection>

          <DetailPageSection
            title="General"
            description="The product identity, type, and first translation."
          >
            <div className="grid gap-2">
              <Label htmlFor="p-type">Product type</Label>
              <Select
                value={productTypeId}
                onValueChange={setProductTypeId}
                disabled={productTypesQuery.loading || productTypes.length === 0}
              >
                <SelectTrigger id="p-type" className="sm:max-w-md">
                  <SelectValue
                    placeholder={
                      productTypesQuery.loading
                        ? 'Loading…'
                        : productTypes.length === 0
                          ? 'No product types available'
                          : 'Select a product type…'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {productTypes.map((pt) => (
                    <SelectItem key={pt.id} value={pt.id}>
                      {pt.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              {productTypes.length === 0 && !productTypesQuery.loading && !productTypesQuery.error && (
                <Alert>
                  <AlertTitle>No product types yet</AlertTitle>
                  <AlertDescription>
                    Create a product type before adding products so attributes can be assigned.
                  </AlertDescription>
                </Alert>
              )}

              {productType && (
                <div className="flex flex-wrap gap-1.5 pt-1 text-xs">
                  <code className="font-mono text-muted-foreground">{productType.slug}</code>
                  <Badge variant="outline">
                    {productType.attributes.length} attribute{productType.attributes.length === 1 ? '' : 's'}
                  </Badge>
                  {productType.isDigital && <Badge variant="secondary">Digital</Badge>}
                  {productType.isShippingRequired && (
                    <Badge variant="secondary" className="gap-1">
                      <TruckIcon className="size-3" />
                      Ships
                    </Badge>
                  )}
                </div>
              )}
              <p className="text-xs text-muted-foreground">
                Determines the attribute set and cannot be changed later.
              </p>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="p-name">Name</Label>
                <Input
                  id="p-name"
                  required
                  placeholder="Red T-Shirt"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="p-sku">SKU</Label>
                <Input
                  id="p-sku"
                  placeholder="PROD-001"
                  value={sku}
                  onChange={(e) => setSku(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Leave blank to auto-generate one from the name.
                </p>
              </div>
            </div>

            <div className="grid gap-2 sm:max-w-xs">
              <Label htmlFor="p-locale">Initial translation locale</Label>
              <Select value={selectedLocale} onValueChange={setLocale} disabled={channelsQuery.loading}>
                <SelectTrigger id="p-locale">
                  <SelectValue placeholder={channelsQuery.loading ? 'Loading…' : 'Select a locale…'} />
                </SelectTrigger>
                <SelectContent>
                  {(availableLocales.length > 0 ? availableLocales : [selectedLocale])
                    .filter(Boolean)
                    .map((localeCode) => (
                      <SelectItem key={localeCode} value={localeCode}>
                        {localeCode}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                This creates the first locale entry. Additional translations can be added later.
              </p>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Pricing"
            description="Default price applied when a store listing inherits."
          >
            <div className="grid grid-cols-1 gap-4 sm:max-w-md sm:grid-cols-[1fr_140px]">
              <div className="grid gap-2">
                <Label htmlFor="p-price">Default price</Label>
                <Input
                  id="p-price"
                  type="number"
                  step="0.01"
                  min="0"
                  inputMode="decimal"
                  placeholder="19.99"
                  value={priceDollars}
                  aria-invalid={priceDollars.length > 0 && !isValidPrice(priceDollars)}
                  onChange={(e) => setPriceDollars(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Entered in major units; stored as integer cents ({dollarsToCents(priceDollars)}).
                </p>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="p-currency">Default currency</Label>
<Select value={currencyCode} onValueChange={setCurrencyCode}>
                  <SelectTrigger id="p-currency">
                    <SelectValue placeholder="Pick a currency" />
                  </SelectTrigger>
                  <SelectContent>
                    {currencyOptions.map((c) => (
                      <SelectItem key={c.code} value={c.code}>
                        <span className="font-mono text-xs">{c.code}</span> · {c.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">Add currencies under Settings - Currencies.</p>
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Content"
            description="First-translation copy. More locales can be added on the detail page."
          >
            <div className="grid gap-2">
              <Label htmlFor="p-short-description">Short description</Label>
              <LexicalRichTextEditor
                value={shortDescription}
                fallbackText=""
                placeholder="One-line teaser shown in listings"
                contentClassName="min-h-24"
                onChange={(nextRichText, nextPlainText) => {
                  setShortDescription(nextRichText)
                  setShortDescriptionText(nextPlainText)
                }}
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="p-description">Description</Label>
              <LexicalRichTextEditor
                value={descriptionRichText}
                fallbackText=""
                onChange={(nextRichText, nextPlainText) => {
                  setDescriptionRichText(nextRichText)
                  setDescriptionText(nextPlainText)
                }}
              />
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="Search & URL"
            description="The product's path and search-engine metadata for the initial locale. All optional - sensible defaults apply."
          >
            <div className="flex items-start justify-between gap-4 rounded-md border p-3">
              <div className="grid gap-0.5">
                <Label htmlFor="p-url-auto">Auto-generate URL</Label>
                <p className="text-xs text-muted-foreground">
                  Derive the path from the product type's URL template, falling back to the name.
                </p>
              </div>
              <Switch id="p-url-auto" checked={urlAuto} onCheckedChange={setUrlAuto} />
            </div>

            {urlAuto ? (
              <div className="grid gap-2">
                <Label htmlFor="p-url-template">URL template override</Label>
                <Input
                  id="p-url-template"
                  placeholder="{category}/{name}"
                  value={urlTemplateOverride}
                  onChange={(e) => setUrlTemplateOverride(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Optional. Overrides the product type's template for this product only.
                </p>
              </div>
            ) : (
              <div className="grid gap-2">
                <Label htmlFor="p-url">URL</Label>
                <Input
                  id="p-url"
                  placeholder="shoes/nike-air-max"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                />
                <p className="text-xs text-muted-foreground">
                  Full URL path, without a leading slash.
                </p>
              </div>
            )}

            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="p-meta-title">Meta title</Label>
                <span className="text-xs text-muted-foreground">{metaTitle.length}/60</span>
              </div>
              <Input
                id="p-meta-title"
                maxLength={70}
                placeholder="Defaults to the product name"
                value={metaTitle}
                onChange={(e) => setMetaTitle(e.target.value)}
              />
            </div>

            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="p-meta-description">Meta description</Label>
                <span className="text-xs text-muted-foreground">{metaDescription.length}/160</span>
              </div>
              <Textarea
                id="p-meta-description"
                rows={3}
                maxLength={320}
                placeholder="Short summary shown in search results"
                value={metaDescription}
                onChange={(e) => setMetaDescription(e.target.value)}
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="p-meta-keywords">Meta keywords</Label>
              <Input
                id="p-meta-keywords"
                placeholder="comma, separated, keywords"
                value={metaKeywords}
                onChange={(e) => setMetaKeywords(e.target.value)}
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="p-canonical">Canonical URL</Label>
              <Input
                id="p-canonical"
                placeholder="https://example.com/products/…"
                value={canonicalUrl}
                onChange={(e) => setCanonicalUrl(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                Absolute URL. Leave blank to use the product's own URL.
              </p>
            </div>

            <div className="flex items-start justify-between gap-4 rounded-md border p-3">
              <div className="grid gap-0.5">
                <Label htmlFor="p-noindex">Discourage search indexing</Label>
                <p className="text-xs text-muted-foreground">
                  Adds a <code className="font-mono">noindex</code> hint so search engines skip this product.
                </p>
              </div>
              <Switch id="p-noindex" checked={seoNoindex} onCheckedChange={setSeoNoindex} />
            </div>
          </DetailPageSection>

          {productTypeId ? (
            <DetailPageSection
              title="Attributes"
              description="Initial attribute values. Required attributes must be filled."
            >
              {productTypeQuery.loading ? (
                <p className="text-sm text-muted-foreground">Loading attributes…</p>
              ) : productTypeQuery.error ? (
                <Alert variant="destructive">
                  <AlertTitle>Couldn't load attributes</AlertTitle>
                  <AlertDescription>{productTypeQuery.error.message}</AlertDescription>
                </Alert>
              ) : nonVariantAttrs.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  The selected product type has no attributes assigned.
                </p>
              ) : (
                <div className="grid gap-5">
                  {nonVariantAttrs.map((attr) => (
                    <div key={attr.id} className="grid gap-2 border-b pb-5 last:border-b-0 last:pb-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <Label className="text-sm font-medium">{attr.name}</Label>
                        <AttributeInputTypeBadge type={attr.inputType} />
                        {attr.valueRequired && (
                          <span className="text-[10px] uppercase tracking-wider text-destructive">
                            required
                          </span>
                        )}
                        {attr.unit && (
                          <span className="text-xs text-muted-foreground">({attr.unit})</span>
                        )}
                      </div>
                      <AttributeControl
                        attribute={attr}
                        state={attrState[attr.id] ?? emptyAttrState()}
                        onUpdate={(patch) => patchAttr(attr.id, patch)}
                      />
                    </div>
                  ))}
                </div>
              )}
            </DetailPageSection>
          ) : null}

          {submitError && (
            <div className="px-6 py-4">
              <Alert variant="destructive">
                <AlertTitle>Create failed</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          )}
        </DetailPageBody>
      </DetailPageLayout>
    </form>
  )
}
