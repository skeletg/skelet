import { useCallback, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangleIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import type { AttributeInputType, VariantMode } from '@/graphql/schema.types'
import { AttributeInputTypeBadge } from '../AttributeInputTypeBadge'
import {
  useProductSaveSection,
  type SectionDiffEntry,
} from '../productSaveRegistry'

// ---------- Configurable options card ----------

type OptionAttr = { id: string; slug: string; name: string; inputType: AttributeInputType }

const ALLOWED_OPTION_TYPES: AttributeInputType[] = ['DROPDOWN', 'SWATCH']

const VARIANT_MODE_LABELS: Record<VariantMode, string> = {
  ATTRIBUTE: 'Attribute-based',
  LIST: 'List (no attributes)',
}

// Variant mode + configurable options, staged into ONE save section so
// the two mutations fire in a safe order: a mode change is applied
// before any option change (the backend rejects setConfigurableOptions
// while a product is still in LIST mode) - ATTRIBUTE mode differentiates
// children by unique option-attribute values; LIST mode is an
// attribute-less curated set of children (a "group" of configurables)
export function ConfigurableVariantsCard({
  product,
  canManage,
  onSaveOptions,
  onSaveMode,
}: {
  product: {
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
  const eligibleAttrs = product.productType.attributes
    .map((a) => a.attribute)
    .filter((a) => ALLOWED_OPTION_TYPES.includes(a.inputType))

  const persistedMode: VariantMode = product.variantMode ?? 'ATTRIBUTE'
  const [mode, setMode] = useState<VariantMode>(persistedMode)
  const [prevPersistedMode, setPrevPersistedMode] = useState(persistedMode)
  if (prevPersistedMode !== persistedMode) {
    setPrevPersistedMode(persistedMode)
    setMode(persistedMode)
  }

  const originalIds = useMemo(
    () => new Set(product.configurableOptions.map((a) => a.id)),
    [product.configurableOptions],
  )
  const [selected, setSelected] = useState<Set<string>>(originalIds)

  // Re-sync the draft when the loaded product refetches (e.g. after a save)
  const originalKey = useMemo(() => [...originalIds].sort().join('|'), [originalIds])
  const [prevOriginalKey, setPrevOriginalKey] = useState(originalKey)
  if (prevOriginalKey !== originalKey) {
    setPrevOriginalKey(originalKey)
    setSelected(new Set(originalIds))
  }

  const labelById = useMemo(() => {
    const m = new Map<string, string>()
    for (const a of eligibleAttrs) m.set(a.id, a.name)
    for (const a of product.configurableOptions) m.set(a.id, a.name)
    return m
  }, [eligibleAttrs, product.configurableOptions])

  // LIST mode forbids configurable options - mirror the backend rule by
  // only allowing the LIST choice once the persisted options are gone
  const listBlocked = hasOptions
  // The reverse flip is blocked too: attribute mode with linked children and no
  // option axis would let the storefront vacuously match the first child
  const attributeBlocked = persistedMode === 'LIST' && hasChildren && !hasOptions

  const optionsChanged = useMemo(() => {
    if (mode !== 'ATTRIBUTE') return false
    if (selected.size !== originalIds.size) return true
    for (const id of selected) if (!originalIds.has(id)) return true
    return false
  }, [mode, selected, originalIds])

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    if (mode !== persistedMode) {
      entries.push({
        label: 'Variant mode',
        before: VARIANT_MODE_LABELS[persistedMode],
        after: VARIANT_MODE_LABELS[mode],
      })
    }
    if (mode === 'ATTRIBUTE') {
      for (const id of selected) {
        if (!originalIds.has(id)) entries.push({ label: 'Add option', before: '-', after: labelById.get(id) ?? id })
      }
      for (const id of originalIds) {
        if (!selected.has(id)) entries.push({ label: 'Remove option', before: labelById.get(id) ?? id, after: '-' })
      }
    }
    return entries
  }, [mode, persistedMode, selected, originalIds, labelById])

  const save = useCallback(async () => {
    // Mode first: setConfigurableOptions is rejected while still LIST
    if (mode !== persistedMode) {
      await onSaveMode(mode)
    }
    if (mode === 'ATTRIBUTE' && optionsChanged) {
      await onSaveOptions([...selected])
    }
  }, [mode, persistedMode, optionsChanged, selected, onSaveMode, onSaveOptions])

  useProductSaveSection({ id: 'variants-mode-options', title: 'Variants', diff, save })

  function toggle(id: string) {
    if (hasChildren) return
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  return (
    <div className="grid gap-3">
      <div className="grid gap-1">
        <h3 className="text-sm font-medium">Variant mode</h3>
        <p className="text-sm text-muted-foreground">
          How shoppers pick a variant. Changes are queued and applied when you click Save.
        </p>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row">
        <ModeOption
          value="ATTRIBUTE"
          title="Attribute-based"
          desc="Children differ by one or more option attributes (e.g. Color, Size); each combination must be unique."
          disabled={attributeBlocked}
          mode={mode}
          canManage={canManage}
          onSelect={setMode}
        />
        <ModeOption
          value="LIST"
          title="List (no attributes)"
          desc="No option attribute - children are a curated list the storefront lets shoppers pick directly."
          disabled={listBlocked}
          mode={mode}
          canManage={canManage}
          onSelect={setMode}
        />
      </div>

      {listBlocked && mode !== 'LIST' && (
        <p className="text-xs text-muted-foreground">
          Remove all configurable options below before switching to List mode.
        </p>
      )}
      {attributeBlocked && (
        <p className="text-xs text-muted-foreground">
          Unlink the children before switching to Attribute mode - options can't be
          added while children are linked, so the product would be stuck unsellable.
        </p>
      )}
      {hasChildren && mode !== persistedMode && (
        <Alert>
          <AlertTriangleIcon className="size-4" />
          <AlertTitle>Existing children won't be re-validated</AlertTitle>
          <AlertDescription>
            Changing variant mode does not re-check child products already linked to this configurable.
          </AlertDescription>
        </Alert>
      )}

      {mode === 'ATTRIBUTE' && (
        <div className="grid gap-3 border-t pt-3">
          <div className="grid gap-1">
            <h3 className="text-sm font-medium">Configurable options</h3>
            <p className="text-sm text-muted-foreground">
              Selectable attributes that differentiate child products, like Color or Size.
              Children must have a value set for every option.
            </p>
          </div>

          {hasChildren && (
            <Alert>
              <AlertTriangleIcon className="size-4" />
              <AlertTitle>Options locked</AlertTitle>
              <AlertDescription>
                Remove all children before changing configurable options.
              </AlertDescription>
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
                const isChecked = selected.has(a.id)
                const wasOriginallyChecked = originalIds.has(a.id)
                const dirty = isChecked !== wasOriginallyChecked
                return (
                  <label
                    key={a.id}
                    className={`flex cursor-pointer items-center gap-2.5 rounded border bg-card px-3 py-2 transition-colors hover:bg-accent/50 has-[:checked]:border-primary has-[:checked]:bg-accent ${
                      hasChildren || !canManage ? 'cursor-not-allowed opacity-60' : ''
                    } ${dirty ? 'ring-1 ring-primary/40' : ''}`}
                  >
                    <input
                      type="checkbox"
                      className="size-4 rounded"
                      checked={isChecked}
                      disabled={hasChildren || !canManage}
                      onChange={() => toggle(a.id)}
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
      disabled={blocked}
      onClick={() => onSelect(value)}
      className={`flex-1 rounded border px-3 py-2.5 text-left transition-colors ${
        active ? 'border-primary bg-accent ring-1 ring-primary/40' : 'bg-card hover:bg-accent/50'
      } ${blocked ? 'cursor-not-allowed opacity-60' : ''}`}
    >
      <div className="flex items-center gap-2">
        <span
          className={`grid size-4 place-items-center rounded-full border ${active ? 'border-primary' : 'border-muted-foreground/40'}`}
        >
          {active ? <span className="size-2 rounded-full bg-primary" /> : null}
        </span>
        <span className="text-sm font-medium">{title}</span>
      </div>
      <p className="mt-1 pl-6 text-xs text-muted-foreground">{desc}</p>
    </button>
  )
}
