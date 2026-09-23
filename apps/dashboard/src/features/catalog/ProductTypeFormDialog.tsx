import { useState, type FormEvent, type ReactNode } from 'react'
import { useMutation } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { slugify } from '@/lib/slug'
import {
  CreateProductTypeDocument,
  ProductTypeDocument,
  ProductTypesDocument,
  SetProductTypeUrlPathTemplateDocument,
  UpdateProductTypeDocument,
} from './Catalog.gql'

export type ExistingProductType = {
  id: string
  slug: string
  name: string
  isDigital: boolean
  isShippingRequired: boolean
  urlTemplate: string
  tracksInventory: boolean
}

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
  onCreated?: (productType: { id: string; name: string }) => void
}) {
  const isEdit = !!productType
  const [internalOpen, setInternalOpen] = useState(false)
  const actualOpen = open ?? internalOpen
  const setOpen = onOpenChange ?? setInternalOpen

  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [name, setName] = useState('')
  const [isDigital, setIsDigital] = useState(false)
  const [isShippingRequired, setIsShippingRequired] = useState(true)
  const [urlTemplate, setUrlPathTemplate] = useState('')
  const [tracksInventory, setTracksInventory] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Reset the form whenever the dialog opens, or the target product type
  // changes while it is open (state adjusted during render)
  const [prevOpen, setPrevOpen] = useState(false)
  const [prevProductType, setPrevProductType] = useState<ExistingProductType | undefined>(undefined)
  if (prevOpen !== actualOpen || prevProductType !== productType) {
    setPrevOpen(actualOpen)
    setPrevProductType(productType)
    if (actualOpen) {
      setSlug(productType?.slug ?? '')
      setSlugTouched(false)
      setName(productType?.name ?? '')
      setIsDigital(productType?.isDigital ?? false)
      setIsShippingRequired(productType?.isShippingRequired ?? true)
      setUrlPathTemplate(productType?.urlTemplate ?? '')
      setTracksInventory(productType?.tracksInventory ?? false)
      setSubmitError(null)
    }
  }

  function onNameChange(next: string) {
    setName(next)
    if (!isEdit && !slugTouched) setSlug(slugify(next))
  }

  const refetchQueries = isEdit && productType
    ? [
        { query: ProductTypesDocument },
        { query: ProductTypeDocument, variables: { id: productType.id } },
      ]
    : [{ query: ProductTypesDocument }]

  const [createProductType, { loading: creating }] = useMutation(CreateProductTypeDocument, { refetchQueries })
  const [updateProductType, { loading: updating }] = useMutation(UpdateProductTypeDocument, { refetchQueries })
  const [setUrlTemplate] = useMutation(SetProductTypeUrlPathTemplateDocument, { refetchQueries })
  const loading = creating || updating

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    // The dialog portals out of the DOM but not out of the React tree - without this a submit here also fires the
    // surrounding form's onSubmit when the dialog is opened from inside one (ProductCreate)
    e.stopPropagation()
    setSubmitError(null)
    try {
      if (isEdit && productType) {
        const templateChanged = urlTemplate !== productType.urlTemplate
        // updateProductType validates a new template but does NOT recompute the affected products' url_path - only the
        // focused mutation does Route a template change through it so existing URLs don't go stale
        await updateProductType({
          variables: {
            input: {
              id: productType.id,
              name,
              isDigital,
              isShippingRequired,
              tracksInventory,
              ...(templateChanged ? {} : { urlTemplate }),
            },
          },
        })
        if (templateChanged) {
          await setUrlTemplate({
            variables: { productTypeId: productType.id, template: urlTemplate },
          })
        }
      } else {
        const result = await createProductType({
          variables: { input: { slug, name, isDigital, isShippingRequired, urlTemplate, tracksInventory } },
        })
        const created = result.data?.createProductType
        if (created) onCreated?.(created)
      }
      setOpen(false)
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <Dialog open={actualOpen} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? `Edit ${productType?.name}` : 'New product type'}</DialogTitle>
          <DialogDescription>
            {isEdit
              ? 'Slug is immutable after creation.'
              : 'Product types are the blueprint products inherit - they fix which attributes are shared.'}
          </DialogDescription>
        </DialogHeader>

        <form id="product-type-form" onSubmit={onSubmit} className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 items-start">
            <div className="grid gap-2">
              <Label htmlFor="pt-name">Name</Label>
              <Input
                id="pt-name"
                required
                placeholder="T-shirt"
                value={name}
                onChange={(e) => onNameChange(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="pt-slug">Slug</Label>
              <Input
                id="pt-slug"
                required
                disabled={isEdit}
                placeholder="t-shirt"
                value={slug}
                onChange={(e) => { setSlug(e.target.value); setSlugTouched(true) }}
              />
              <p className="text-xs text-muted-foreground">
                {isEdit ? 'Slug is immutable.' : 'Auto-filled from name - edit to override.'}
              </p>
            </div>
          </div>

          <div className="grid gap-3 rounded-md border p-4">
            <SwitchRow
              id="pt-digital"
              label="Digital"
              description="Digital goods have no physical shipment (e.g. downloads, gift cards)."
              checked={isDigital}
              onCheckedChange={(v) => { setIsDigital(v); if (v) setIsShippingRequired(false) }}
            />
            <SwitchRow
              id="pt-shipping"
              label="Requires shipping"
              description="Physical goods that need a fulfillment step."
              checked={isShippingRequired}
              onCheckedChange={setIsShippingRequired}
              disabled={isDigital}
            />
          </div>

          <div className="grid gap-3 rounded-md border p-4">
            <SwitchRow
              id="pt-tracks-inv"
              label="Track inventory"
              description="New products of this type opt into inventory tracking by default. Per-product override is available on the product page."
              checked={tracksInventory}
              onCheckedChange={setTracksInventory}
            />
          </div>

          <div className="grid gap-2">
            <Label htmlFor="pt-url-template">URL path template</Label>
            <Input
              id="pt-url-template"
              placeholder="{category}/{attr:brand}/{name}"
              value={urlTemplate}
              onChange={(e) => setUrlPathTemplate(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              Empty disables auto-generation. Tokens: <code>{'{name}'}</code>,
              {' '}<code>{'{category}'}</code>, <code>{'{categorySlug}'}</code>,
              {' '}<code>{'{attr:<slug>}'}</code> (e.g. <code>{'{attr:model}'}</code>).
              {' '}Multiselect attributes aren't allowed.
            </p>
          </div>

          {submitError && (
            <Alert variant="destructive">
              <AlertTitle>Save failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          )}
        </form>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
          <Button type="submit" form="product-type-form" disabled={loading}>
            {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create product type'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SwitchRow({
  id, label, description, checked, onCheckedChange, disabled,
}: {
  id: string; label: string; description: string
  checked: boolean; onCheckedChange: (v: boolean) => void; disabled?: boolean
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="grid gap-0.5">
        <Label htmlFor={id} className="text-sm">{label}</Label>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
    </div>
  )
}
