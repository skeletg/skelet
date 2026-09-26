import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { toast } from 'sonner'
import {
  BoxIcon,
  GiftIcon,
  LayersIcon,
  PackageIcon,
  PlusIcon,
  RepeatIcon,
  ShoppingBagIcon,
  SlidersHorizontalIcon,
  TruckIcon,
} from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { FieldError } from '@/components/form/fields'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import type { ProductKind } from '@/graphql/schema.types'
import { usePermissions } from '@/shared/auth/usePermissions'
import { filterChannelsByAllowedScopes, localesForAllowedChannel, useConfigScope } from '@/lib/config-scope'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { dollarsToCents } from '@/shared/schemas/fields'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { AttributeControl, emptyAttrState, type AttrState, type AttributeSummary } from './AttributesEditor'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import { CreateProductDocument, ProductsPagedDocument, ProductTypeDocument, ProductTypesDocument, SetProductAttributesDocument } from './Catalog.gql'
import { FormRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'
import { ProductTypeFormDialog } from './ProductTypeFormDialog'
import {
  emptyNewProduct,
  newProductSchema,
  toAttributeAssignments,
  toNewProductInput,
} from './schemas/newProduct'

const KIND_OPTIONS: { value: ProductKind; label: string; description: string; Icon: typeof BoxIcon }[] = [
  { value: 'SIMPLE', label: 'Simple', description: 'A standalone product with its own SKU, price, and inventory.', Icon: BoxIcon },
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

export function ProductCreate() {
  const navigate = useNavigate()
  const permissions = usePermissions()
  const { globalDefaultLocale, isGlobal } = useConfigScope()
  const allowedScopes = useMemo(() => permissions.allowedChannelScopes('catalog.manage'), [permissions])
  // Product types are global - same gate as the product types list
  const canCreateProductType = permissions.has('catalog.manage')
  const [typeDialogOpen, setTypeDialogOpen] = useState(false)

  const productTypesQuery = useQuery(ProductTypesDocument)
  const productTypes = productTypesQuery.data?.productTypes ?? []
  const channelsQuery = useQuery(ChannelsDocument)
  const currencies = useQuery(CurrenciesPickerDocument).data?.currencies ?? []
  const channels = useMemo(
    () => filterChannelsByAllowedScopes(channelsQuery.data?.channels ?? [], allowedScopes),
    [allowedScopes, channelsQuery.data],
  )
  const locales = useMemo(
    () => [...new Set(channels.flatMap((c) => localesForAllowedChannel(c, allowedScopes)).filter(Boolean))],
    [allowedScopes, channels],
  )
  const fallbackLocale =
    globalDefaultLocale && (locales.length === 0 || locales.includes(globalDefaultLocale)) ? globalDefaultLocale : (locales[0] ?? '')

  const [createProduct] = useMutation(CreateProductDocument, { refetchQueries: [ProductsPagedDocument] })
  const [setAttrs] = useMutation(SetProductAttributesDocument)
  const createdId = useRef('')

  // The chosen type decides which attributes are asked for - and which are required. It lives in the
  // form (so a restored draft brings it back), so the schema reads the loaded set when it validates
  const loadedRef = useRef<{ typeId: string; attributes: AttributeSummary[] } | null>(null)
  // The getter runs when the form validates, never while rendering - the schema has to exist before the
  // form, and the attributes only exist once the form has a type, so a ref is the one way to join them
  // eslint-disable-next-line react-hooks/refs
  const schema = useMemo(() => newProductSchema({ fallbackLocale, attributes: (id) => (loadedRef.current?.typeId === id ? loadedRef.current.attributes : null) }), [fallbackLocale])

  const record = useRecordForm({
    id: 'product',
    title: 'Product',
    label: 'New product',
    isEdit: false,
    baseline: emptyNewProduct,
    schema,
    diff: () => [],
    persist: { routeKey: 'product:new' },
    submit: async (v) => {
      const locale = v.locale && (locales.length === 0 || locales.includes(v.locale)) ? v.locale : fallbackLocale
      const result = await createProduct({ variables: { input: toNewProductInput(v, locale) } })
      const id = result.data?.createProduct.id
      if (!id) throw new Error('Create succeeded but no id returned. Please reload and check the products list.')
      createdId.current = id
      const assignments = toAttributeAssignments(v, loadedRef.current?.attributes ?? [])
      if (assignments.length === 0) return
      // The product exists either way - an attribute failure is reported on its page, not as a failed create
      try {
        await setAttrs({ variables: { productId: id, assignments } })
      } catch (err) {
        toast.error(`${err instanceof Error ? err.message : 'Attribute write failed.'} The product was created - finish attribute setup on its page.`)
      }
    },
    onSaved: () => navigate(`/products/${createdId.current}`),
  })
  const { form } = record
  const values = useStore(form.store, (s) => s.values)
  const productTypeQuery = useQuery(ProductTypeDocument, { variables: { id: values.productTypeId }, skip: !values.productTypeId })
  const productType = productTypeQuery.data?.productType
  const attributes: AttributeSummary[] = useMemo(() => productType?.attributes.map((a) => a.attribute) ?? [], [productType])
  useEffect(() => {
    loadedRef.current = productType ? { typeId: productType.id, attributes } : null
    // The schema reads these, but no field changed - re-check, or a result from before they loaded sticks
    if (productType) void form.validate('change')
  }, [productType, attributes, form])

  // type="button" - these sit inside the page and must never submit anything
  function newTypeButton(label: string, className: string) {
    if (!canCreateProductType) return null
    return (
      <Button
        type="button"
        variant="outline"
        className={className}
        disabled={!isGlobal}
        title={isGlobal ? undefined : 'Switch the header to GLOBAL to add a product type.'}
        onClick={() => setTypeDialogOpen(true)}
      >
        <PlusIcon className="size-4" />
        {label}
      </Button>
    )
  }

  return (
    <RecordFormPage
      record={record}
      backTo="/products"
      backLabel="Back to products"
      title="New product"
      meta={
        <span>
          Basics and first translation. Store listings, per-store prices, publishing, media
          {values.kind === 'CONFIGURABLE' ? ', and variants' : ''} are set on the detail page after creating.
        </span>
      }
      createLabel="Create product"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/products">Cancel</Link>
        </Button>
      }
    >
      {productTypesQuery.error && (
        <div className="px-6 pt-4">
          <Alert variant="destructive">
            <AlertTitle>Couldn't load product types</AlertTitle>
            <AlertDescription>{productTypesQuery.error.message}</AlertDescription>
          </Alert>
        </div>
      )}

      <DetailPageSection title="Kind" description="The product's structural shape. Immutable after creation.">
        <form.Field name="kind">
          {(f) => (
            <div className="grid gap-3 sm:grid-cols-2">
              {KIND_OPTIONS.map(({ value, label, description, Icon }) => {
                const selected = f.state.value === value
                return (
                  <button
                    key={value}
                    type="button"
                    onClick={() => f.handleChange(value)}
                    aria-pressed={selected}
                    className={[
                      'group relative flex flex-col items-start gap-1 rounded-md border px-3 py-3 text-left transition',
                      selected ? 'border-foreground/40 bg-muted shadow-sm' : 'hover:border-foreground/30 hover:bg-muted/40',
                    ].join(' ')}
                  >
                    <div className="flex w-full items-center justify-between">
                      <div className="flex items-center gap-2">
                        <Icon className={`size-4 ${selected ? 'text-foreground' : 'text-muted-foreground'}`} />
                        <span className="text-sm font-medium">{label}</span>
                      </div>
                      {selected && (
                        <Badge variant="secondary" className="text-[10px] uppercase tracking-wide">
                          Selected
                        </Badge>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground">{description}</p>
                  </button>
                )
              })}
            </div>
          )}
        </form.Field>
        {values.kind === 'CONFIGURABLE' && (
          <Alert>
            <AlertTitle>Heads up</AlertTitle>
            <AlertDescription>
              After creation, open the detail page to define the option attributes (size, color, …) and link the simple-product
              variants that fill them.
            </AlertDescription>
          </Alert>
        )}
      </DetailPageSection>

      <DetailPageSection title="General" description="The product identity, type, and first translation.">
        <div className="grid gap-2">
          {/* Label sits above the row so the select and the button share one line */}
          <Label htmlFor="p-type">Product type</Label>
          <div className="flex items-start gap-2 sm:max-w-md">
            <form.AppField
              name="productTypeId"
              listeners={{
                // Attributes are per type - a different type starts them over
                onChange: () => form.setFieldValue('attrState', {}),
              }}
            >
              {(f) => (
                <f.SelectField
                  id="p-type"
                  disabled={productTypesQuery.loading || productTypes.length === 0}
                  placeholder={
                    productTypesQuery.loading
                      ? 'Loading…'
                      : productTypes.length === 0
                        ? 'No product types available'
                        : 'Select a product type…'
                  }
                  options={productTypes.map((pt) => ({ value: pt.id, label: pt.name }))}
                  className="flex-1"
                />
              )}
            </form.AppField>
            {productTypes.length > 0 && newTypeButton('New type', 'h-10 shrink-0')}
          </div>

          {productTypes.length === 0 && !productTypesQuery.loading && !productTypesQuery.error && (
            <Alert>
              <AlertTitle>No product types yet</AlertTitle>
              <AlertDescription>
                <p>Create a product type before adding products so attributes can be assigned.</p>
                {newTypeButton('Create product type', 'w-fit')}
              </AlertDescription>
            </Alert>
          )}
          <ProductTypeFormDialog
            open={typeDialogOpen}
            onOpenChange={setTypeDialogOpen}
            onCreated={(pt) => form.setFieldValue('productTypeId', pt.id)}
          />

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
          <p className="text-xs text-muted-foreground">Determines the attribute set and cannot be changed later.</p>
        </div>

        <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
          <form.AppField name="name">{(f) => <f.TextField id="p-name" label="Name" placeholder="Red T-Shirt" />}</form.AppField>
          <form.AppField name="sku">
            {(f) => <f.TextField id="p-sku" label="SKU" placeholder="PROD-001" hint="Leave blank to auto-generate one from the name." />}
          </form.AppField>
        </div>

        <form.AppField name="locale">
          {(f) => (
            <f.SelectField
              id="p-locale"
              label="Initial translation locale"
              // Nothing picked yet reads as the fallback - that's what gets sent
              placeholder={channelsQuery.loading ? 'Loading…' : fallbackLocale || 'Select a locale…'}
              disabled={channelsQuery.loading}
              options={(locales.length > 0 ? locales : [fallbackLocale]).filter(Boolean).map((code) => ({ value: code, label: code }))}
              hint="This creates the first locale entry. Additional translations can be added later."
              className="sm:max-w-xs"
            />
          )}
        </form.AppField>
      </DetailPageSection>

      <DetailPageSection title="Pricing" description="Default price applied when a store listing inherits.">
        <div className="grid grid-cols-1 items-start gap-4 sm:max-w-md sm:grid-cols-[1fr_140px]">
          <form.AppField name="priceDollars">
            {(f) => (
              <f.TextField
                id="p-price"
                label="Default price"
                type="number"
                step="0.01"
                min="0"
                inputMode="decimal"
                placeholder="19.99"
                hint={`Entered in major units; stored as integer cents (${dollarsToCents(values.priceDollars)}).`}
              />
            )}
          </form.AppField>
          <form.AppField name="currencyCode">
            {(f) => (
              <f.SelectField
                id="p-currency"
                label="Default currency"
                placeholder="Pick a currency"
                options={currencies.map((c) => ({
                  value: c.code,
                  label: (
                    <>
                      <span className="font-mono text-xs">{c.code}</span> · {c.name}
                    </>
                  ),
                }))}
                hint="Add currencies under Settings - Currencies."
              />
            )}
          </form.AppField>
        </div>
      </DetailPageSection>

      <DetailPageSection title="Content" description="First-translation copy. More locales can be added on the detail page.">
        <div className="grid gap-2">
          <Label>Short description</Label>
          <FormRichTextEditor
            value={values.shortDescription as RichTextValue}
            fallbackText=""
            placeholder="One-line teaser shown in listings"
            contentClassName="min-h-24"
            onChange={(next, plain) => {
              form.setFieldValue('shortDescription', next)
              form.setFieldValue('shortDescriptionText', plain)
            }}
          />
        </div>
        <div className="grid gap-2">
          <Label>Description</Label>
          <FormRichTextEditor
            value={values.description as RichTextValue}
            fallbackText=""
            onChange={(next, plain) => {
              form.setFieldValue('description', next)
              form.setFieldValue('descriptionText', plain)
            }}
          />
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Search & URL"
        description="The product's path and search-engine metadata for the initial locale. All optional - sensible defaults apply."
      >
        <form.AppField name="urlAuto">
          {(f) => (
            <f.SwitchField
              id="p-url-auto"
              label="Auto-generate URL"
              description="Derive the path from the product type's URL template, falling back to the name."
            />
          )}
        </form.AppField>
        {values.urlAuto ? (
          <form.AppField name="urlTemplateOverride">
            {(f) => (
              <f.TextField
                id="p-url-template"
                label="URL template override"
                placeholder="{category}/{name}"
                hint="Optional. Overrides the product type's template for this product only."
              />
            )}
          </form.AppField>
        ) : (
          <form.AppField name="url">
            {(f) => <f.TextField id="p-url" label="URL" placeholder="shoes/nike-air-max" hint="Full URL path, without a leading slash." />}
          </form.AppField>
        )}
        <form.AppField name="metaTitle">
          {(f) => (
            <f.TextField
              id="p-meta-title"
              label={`Meta title (${values.metaTitle.length}/60)`}
              maxLength={70}
              placeholder="Defaults to the product name"
            />
          )}
        </form.AppField>
        <form.AppField name="metaDescription">
          {(f) => (
            <f.TextareaField
              id="p-meta-description"
              label={`Meta description (${values.metaDescription.length}/160)`}
              rows={3}
              maxLength={320}
              placeholder="Short summary shown in search results"
            />
          )}
        </form.AppField>
        <form.AppField name="metaKeywords">
          {(f) => <f.TextField id="p-meta-keywords" label="Meta keywords" placeholder="comma, separated, keywords" />}
        </form.AppField>
        <form.AppField name="canonicalUrl">
          {(f) => (
            <f.TextField
              id="p-canonical"
              label="Canonical URL"
              placeholder="https://example.com/products/…"
              hint="Absolute URL. Leave blank to use the product's own URL."
            />
          )}
        </form.AppField>
        <form.AppField name="seoNoindex">
          {(f) => (
            <f.SwitchField
              id="p-noindex"
              label="Discourage search indexing"
              description={
                <>
                  Adds a <code className="font-mono">noindex</code> hint so search engines skip this product.
                </>
              }
            />
          )}
        </form.AppField>
      </DetailPageSection>

      {values.productTypeId && (
        <DetailPageSection title="Attributes" description="Initial attribute values. Required attributes must be filled.">
          {productTypeQuery.loading ? (
            <p className="text-sm text-muted-foreground">Loading attributes…</p>
          ) : productTypeQuery.error ? (
            <Alert variant="destructive">
              <AlertTitle>Couldn't load attributes</AlertTitle>
              <AlertDescription>{productTypeQuery.error.message}</AlertDescription>
            </Alert>
          ) : attributes.length === 0 ? (
            <p className="text-sm text-muted-foreground">The selected product type has no attributes assigned.</p>
          ) : (
            <div className="grid gap-5">
              {attributes.map((attr) => (
                <form.Field key={attr.id} name={`attrState.${attr.id}`}>
                  {(f) => {
                    const state = (f.state.value as AttrState | undefined) ?? emptyAttrState()
                    return (
                      <div className="grid gap-2 border-b pb-5 last:border-b-0 last:pb-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <Label className="text-sm font-medium">{attr.name}</Label>
                          <AttributeInputTypeBadge type={attr.inputType} />
                          {attr.valueRequired && <span className="text-[10px] uppercase tracking-wider text-destructive">required</span>}
                          {attr.unit && <span className="text-xs text-muted-foreground">({attr.unit})</span>}
                        </div>
                        <AttributeControl attribute={attr} state={state} onUpdate={(patch) => f.handleChange({ ...state, ...patch })} />
                        <FieldError field={f} />
                      </div>
                    )
                  }}
                </form.Field>
              ))}
            </div>
          )}
        </DetailPageSection>
      )}
    </RecordFormPage>
  )
}
