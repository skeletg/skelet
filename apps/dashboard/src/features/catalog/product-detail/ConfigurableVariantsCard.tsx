import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useStore } from '@tanstack/react-form'
import { AlertTriangleIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { AttributeInputType, VariantMode } from '@/graphql/schema.types'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import { AttributeInputTypeBadge } from '../AttributeInputTypeBadge'
import {
  PRODUCT_VARIANT_LABELS,
  productVariantChanges,
  productVariantsDiff,
  productVariantsSchema,
  toProductVariantValues,
} from '../schemas/product'

// ---------- Configurable options card ----------

type OptionAttr = { id: string; slug: string; name: string; inputType: AttributeInputType }

const ALLOWED_OPTION_TYPES: AttributeInputType[] = ['DROPDOWN', 'SWATCH']

// Variant mode + configurable options, staged as ONE section so the two mutations fire in a safe
// order: a mode change is applied before any option change (the backend rejects
// setConfigurableOptions while a product is still in LIST mode) - ATTRIBUTE mode differentiates
// children by unique option-attribute values; LIST mode is an attribute-less curated set of children
// (a "group" of configurables)
export function ConfigurableVariantsCard({
  product,
  canManage,
  onSaveOptions,
  onSaveMode,
}: {
  product: {
    id: string
    variantMode?: VariantMode | null
    children: { id: string }[]
    configurableOptions: OptionAttr[]
    productType: { id: string; attributes: Array<{ attribute: OptionAttr }> }
  }
  canManage: boolean
  onSaveOptions: (attributeIds: string[]) => Promise<unknown>
  onSaveMode: (mode: VariantMode) => Promise<unknown>
}) {
  const hasChildren = product.children.length > 0
  const hasOptions = product.configurableOptions.length > 0
  const eligibleAttrs = useMemo(
    () => product.productType.attributes.map((a) => a.attribute).filter((a) => ALLOWED_OPTION_TYPES.includes(a.inputType)),
    [product.productType.attributes],
  )

  const baseline = useMemo(() => toProductVariantValues(product), [product])
  const diff = useMemo(() => {
    const names = new Map([...eligibleAttrs, ...product.configurableOptions].map((a) => [a.id, a.name]))
    return productVariantsDiff((id) => names.get(id) ?? id)
  }, [eligibleAttrs, product.configurableOptions])
  const { form, draft } = useSectionForm({
    id: 'variants-mode-options',
    title: 'Variants',
    baseline,
    schema: productVariantsSchema,
    diff,
    routeKey: `product-variants:${product.id}`,
    submit: async (v) => {
      const changes = productVariantChanges(baseline, v)
      if (changes.mode) await onSaveMode(changes.mode)
      if (changes.options) await onSaveOptions(changes.options)
    },
  })
  const { mode, options } = useStore(form.store, (s) => s.values)

  // LIST mode forbids configurable options - mirror the backend rule by only allowing the LIST
  // choice once the persisted options are gone
  const listBlocked = hasOptions
  // The reverse flip is blocked too: attribute mode with linked children and no option axis would
  // let the storefront vacuously match the first child
  const attributeBlocked = baseline.mode === 'LIST' && hasChildren && !hasOptions

  return (
    <div className="grid gap-3">
      {draft.restoredAt !== null && <RestoredDraftBanner draft={draft} labels={PRODUCT_VARIANT_LABELS} />}
      <div className="grid gap-1">
        <h3 className="text-sm font-medium">Variant mode</h3>
        <p className="text-sm text-muted-foreground">How shoppers pick a variant. Changes are queued and applied when you click Save.</p>
      </div>

      <form.Field name="mode">
        {(f) => (
          <div className="flex flex-col gap-2 sm:flex-row">
            <ModeOption
              value="ATTRIBUTE"
              title="Attribute-based"
              desc="Children differ by one or more option attributes (e.g. Color, Size); each combination must be unique."
              disabled={attributeBlocked}
              mode={f.state.value}
              canManage={canManage}
              onSelect={f.handleChange}
            />
            <ModeOption
              value="LIST"
              title="List (no attributes)"
              desc="No option attribute - children are a curated list the storefront lets shoppers pick directly."
              disabled={listBlocked}
              mode={f.state.value}
              canManage={canManage}
              onSelect={f.handleChange}
            />
          </div>
        )}
      </form.Field>

      {listBlocked && mode !== 'LIST' && <p className="text-xs text-muted-foreground">Remove all configurable options below before switching to List mode.</p>}
      {attributeBlocked && (
        <p className="text-xs text-muted-foreground">
          Unlink the children before switching to Attribute mode - options can't be added while children are linked, so the product would be
          stuck unsellable.
        </p>
      )}
      {hasChildren && mode !== baseline.mode && (
        <Alert>
          <AlertTriangleIcon className="size-4" />
          <AlertTitle>Existing children won't be re-validated</AlertTitle>
          <AlertDescription>Changing variant mode does not re-check child products already linked to this configurable.</AlertDescription>
        </Alert>
      )}

      {mode === 'ATTRIBUTE' && (
        <div className="grid gap-3 border-t pt-3">
          <div className="grid gap-1">
            <h3 className="text-sm font-medium">Configurable options</h3>
            <p className="text-sm text-muted-foreground">
              Selectable attributes that differentiate child products, like Color or Size. Children must have a value set for every option.
            </p>
          </div>

          {hasChildren && (
            <Alert>
              <AlertTriangleIcon className="size-4" />
              <AlertTitle>Options locked</AlertTitle>
              <AlertDescription>Remove all children before changing configurable options.</AlertDescription>
            </Alert>
          )}

          {eligibleAttrs.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No dropdown or swatch attributes assigned to this product type.{' '}
              <Link to={`/catalog/product-types/${product.productType.id}`} className="hover:underline">
                Assign some on the product type page.
              </Link>
            </p>
          ) : (
            <div className="grid gap-1.5">
              {eligibleAttrs.map((a) => {
                const isChecked = options.includes(a.id)
                const dirty = isChecked !== baseline.options.includes(a.id)
                const locked = hasChildren || !canManage
                return (
                  <label
                    key={a.id}
                    className={`flex items-center gap-2.5 rounded border bg-card px-3 py-2 transition-colors hover:bg-muted/50 has-checked:border-foreground/40 has-checked:bg-muted ${
                      locked ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
                    } ${dirty ? 'ring-1 ring-amber-500/50' : ''}`}
                  >
                    <input
                      type="checkbox"
                      className="size-4 rounded"
                      checked={isChecked}
                      disabled={locked}
                      onChange={() => form.setFieldValue('options', (prev) => (prev.includes(a.id) ? prev.filter((id) => id !== a.id) : [...prev, a.id]))}
                    />
                    <span className="flex-1 text-sm font-medium">{a.name}</span>
                    {dirty ? (
                      <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
                        pending
                      </Badge>
                    ) : null}
                    <AttributeInputTypeBadge type={a.inputType} />
                  </label>
                )
              })}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function ModeOption({
  value,
  title,
  desc,
  disabled,
  mode,
  canManage,
  onSelect,
}: {
  value: VariantMode
  title: string
  desc: string
  disabled?: boolean
  mode: VariantMode
  canManage: boolean
  onSelect: (mode: VariantMode) => void
}) {
  const active = mode === value
  const blocked = disabled || !canManage
  return (
    <button
      type="button"
      role="radio"
      aria-checked={active}
      disabled={blocked}
      onClick={() => onSelect(value)}
      className={`flex-1 rounded border px-3 py-2.5 text-left transition-colors ${
        active ? 'border-foreground/40 bg-muted ring-1 ring-foreground/20' : 'bg-card hover:bg-muted/50'
      } ${blocked ? 'cursor-not-allowed opacity-60' : ''}`}
    >
      <div className="flex items-center gap-2">
        <span className={`grid size-4 place-items-center rounded-full border ${active ? 'border-foreground' : 'border-muted-foreground/40'}`}>
          {active ? <span className="size-2 rounded-full bg-foreground" /> : null}
        </span>
        <span className="text-sm font-medium">{title}</span>
      </div>
      <p className="mt-1 pl-6 text-xs text-muted-foreground">{desc}</p>
    </button>
  )
}
