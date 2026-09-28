/* eslint-disable react-refresh/only-export-components */
import { useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { LanguagesIcon, SaveIcon, SearchIcon, XCircleIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type {
  AttributeInputType,
  ProductAttributeAssignmentInput,
} from '@/graphql/schema.types'
import { slugify } from '@/lib/slug'
import { cn } from '@/lib/utils'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import { useConfigScope } from '@/lib/config-scope'
import {
  DeleteProductAttributeSlugDocument,
  ProductAttributeLocaleValuesDocument,
  ProductAttributeSlugsDocument,
  SetProductAttributeLocaleValueDocument,
  SetProductAttributeSlugDocument,
} from './Catalog.gql'
import {
  ProductSaveContext,
  useProductSaveSection,
  type SectionDiffEntry,
} from './productSaveRegistry'
import { useSaveSection } from '@/shared/saving/saveRegistry'

export type AttrValue = { id: string; slug: string; name: string; value: string }

export type AttributeSummary = {
  id: string
  slug: string
  name: string
  inputType: AttributeInputType
  unit: string
  valueRequired: boolean
  values: AttrValue[]
}

export type AttributeValueRow = {
  attribute: AttributeSummary
  selectedValues: AttrValue[]
  scalarValue?: unknown
}

export type AttributeGroupSummary = {
  id: string
  name: string
  sortOrder: number
  attributes: Array<{ sortOrder: number; attribute: { id: string } }>
}

export type AttrState = {
  valueIds: string[]
  scalarString: string
  scalarBool: boolean
  hasScalarValue: boolean
}

const SELECTABLE_INPUT_TYPES: AttributeInputType[] = ['DROPDOWN', 'MULTISELECT', 'SWATCH']

export function isSelectableInputType(t: AttributeInputType): boolean {
  return SELECTABLE_INPUT_TYPES.includes(t)
}

export function emptyAttrState(): AttrState {
  return { valueIds: [], scalarString: '', scalarBool: false, hasScalarValue: true }
}

function clearedAttrState(): AttrState {
  return { valueIds: [], scalarString: '', scalarBool: false, hasScalarValue: false }
}

export function initAttrStateFromRow(row: AttributeValueRow): AttrState {
  const selVal = row.scalarValue
  return {
    valueIds: row.selectedValues.map((v) => v.id),
    scalarString:
      selVal == null
        ? ''
        : typeof selVal === 'string' || typeof selVal === 'number'
          ? String(selVal)
          : '',
    scalarBool: selVal === true,
    hasScalarValue: selVal != null,
  }
}

// Convert a local edit state + attribute definition into the wire shape the backend expects - Shared by the
// AttributesEditor (edit existing) and any form that builds initial attributes inline (e.g. variant create)
export function buildAssignment(
  attribute: AttributeSummary,
  state: AttrState,
): ProductAttributeAssignmentInput {
  if (isSelectableInputType(attribute.inputType)) {
    return { attributeId: attribute.id, valueIds: state.valueIds }
  }
  let scalarValue: unknown = null
  switch (attribute.inputType) {
    case 'PLAIN_TEXT':
      scalarValue = state.scalarString ? state.scalarString : null
      break
    case 'NUMERIC': {
      if (state.scalarString === '') {
        scalarValue = null
      } else {
        const n = Number(state.scalarString)
        scalarValue = Number.isFinite(n) ? n : null
      }
      break
    }
    case 'BOOLEAN':
      scalarValue = state.hasScalarValue ? state.scalarBool : null
      break
    case 'DATE':
    case 'DATE_TIME':
      scalarValue = state.scalarString || null
      break
  }
  return { attributeId: attribute.id, scalarValue }
}

function isEmptyAssignment(assignment: ProductAttributeAssignmentInput): boolean {
  if (assignment.valueIds != null) return assignment.valueIds.length === 0
  return assignment.scalarValue === null || assignment.scalarValue === undefined
}

function attributeMatchesSearch(row: AttributeValueRow, query: string): boolean {
  const haystack = [
    row.attribute.name,
    row.attribute.slug,
    row.attribute.inputType,
    row.attribute.unit,
    ...row.attribute.values.flatMap((value) => [value.name, value.slug, value.value]),
    ...row.selectedValues.flatMap((value) => [value.name, value.slug, value.value]),
    row.scalarValue == null ? '' : String(row.scalarValue),
  ]

  return haystack.some((value) => value.toLowerCase().includes(query))
}

// Reusable editor: one row per attribute with a type-appropriate control, a per-attribute Clear button, and a single
// Save that pushes all current values - Parent wires onSave/onClear to the right product/variant mutation
export function AttributesEditor({
  productId,
  attributeValues,
  attributeGroups,
  canManage,
  onSave,
  onClear,
  saving,
  clearing,
  emptyHint,
}: {
  // When provided, scalar rows render an inline slug + indexable editor round-tripping through
  // the per-product slug-overrides table - omit on create forms where no product id exists yet
  productId?: string
  attributeValues: AttributeValueRow[]
  attributeGroups?: AttributeGroupSummary[]
  canManage: boolean
  onSave: (assignments: ProductAttributeAssignmentInput[]) => Promise<void>
  onClear: (attributeId: string) => Promise<void>
  saving: boolean
  clearing: boolean
  emptyHint?: string
}) {
  const initial = useMemo<Record<string, AttrState>>(() => {
    const out: Record<string, AttrState> = {}
    for (const row of attributeValues) {
      out[row.attribute.id] = initAttrStateFromRow(row)
    }
    return out
  }, [attributeValues])

  const [state, setState] = useState<Record<string, AttrState>>(initial)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  // Loaded only when productId is provided, so the create form skips it - each scalar row reads
  // its override here - Facet indexability is attribute-level, so there's no per-product toggle
  const slugOverridesQuery = useQuery(ProductAttributeSlugsDocument, {
    variables: { productId: productId ?? '' },
    skip: !productId,
  })
  const slugOverridesByAttr = useMemo(() => {
    // The inline editor manages the GLOBAL override only - channel- narrowed rows (channelId set) are skipped so they
    // don't shadow the global slug in this form
    const m = new Map<string, { id: string; slug: string }>()
    for (const o of slugOverridesQuery.data?.productAttributeSlugs ?? []) {
      if (o.channelId) continue
      m.set(o.attributeId, { id: o.id, slug: o.slug })
    }
    return m
  }, [slugOverridesQuery.data])

  // plain_text per-locale value translations - loaded once for the
  // whole product; the per-row editor filters by attribute
  const localeValuesQuery = useQuery(ProductAttributeLocaleValuesDocument, {
    variables: { productId: productId ?? '' },
    skip: !productId,
  })
  const localeValuesByAttr = useMemo(() => {
    const m = new Map<string, Array<{ locale: string; value: string }>>()
    for (const row of localeValuesQuery.data?.productAttributeLocaleValues ?? []) {
      const list = m.get(row.attributeId) ?? []
      list.push({ locale: row.locale, value: row.value })
      m.set(row.attributeId, list)
    }
    return m
  }, [localeValuesQuery.data])
  const normalizedSearch = search.trim().toLowerCase()
  const filteredAttributeValues = useMemo(() => {
    if (!normalizedSearch) return attributeValues
    return attributeValues.filter((row) => attributeMatchesSearch(row, normalizedSearch))
  }, [attributeValues, normalizedSearch])
  const groupedAttributeValues = useMemo(() => {
    if (!attributeGroups?.length) return null

    const rowsByAttributeId = new Map(filteredAttributeValues.map((row) => [row.attribute.id, row]))
    const groupedAttributeIds = new Set<string>()
    const groups = [...attributeGroups]
      .sort((a, b) => a.sortOrder - b.sortOrder)
      .map((group) => {
        const rows = [...group.attributes]
          .sort((a, b) => a.sortOrder - b.sortOrder)
          .map((assignment) => {
            groupedAttributeIds.add(assignment.attribute.id)
            return rowsByAttributeId.get(assignment.attribute.id)
          })
          .filter((row): row is AttributeValueRow => Boolean(row))

        return { id: group.id, name: group.name, sortOrder: group.sortOrder, rows }
      })

    return {
      groups,
      ungroupedRows: filteredAttributeValues.filter((row) => !groupedAttributeIds.has(row.attribute.id)),
    }
  }, [attributeGroups, filteredAttributeValues])

  useEffect(() => {
    // Attribute rows are refetched after saves/clears; mirror that incoming state into the editable draft
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setState(initial)
    setError(null)
  }, [initial])

  function updateAttr(attrId: string, patch: Partial<AttrState>) {
    setState((prev) => ({ ...prev, [attrId]: { ...prev[attrId], ...patch } }))
  }

  // When a parent ProductSaveContext is present, the editor reports its diff upward and the global Save button drives the actual mutation
  // flow - The inline "Save attributes" button + per-row immediate clear remain as a fallback when used standalone (no context)
  const saveCtx = useContext(ProductSaveContext)
  const inGlobalSaveFlow = !!saveCtx

  function describeAttrState(attribute: AttributeSummary, s: AttrState | undefined): string {
    if (!s) return '-'
    if (isSelectableInputType(attribute.inputType)) {
      if (s.valueIds.length === 0) return '-'
      return s.valueIds
        .map((id) => attribute.values.find((v) => v.id === id)?.name ?? id)
        .join(', ')
    }
    switch (attribute.inputType) {
      case 'BOOLEAN':
        return s.hasScalarValue ? (s.scalarBool ? 'true' : 'false') : '-'
      default:
        return s.scalarString || '-'
    }
  }

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const entries: SectionDiffEntry[] = []
    for (const row of attributeValues) {
      const cur = state[row.attribute.id]
      const orig = initial[row.attribute.id]
      if (!cur || !orig) continue
      const curAsn = JSON.stringify(buildAssignment(row.attribute, cur))
      const origAsn = JSON.stringify(buildAssignment(row.attribute, orig))
      if (curAsn === origAsn) continue
      entries.push({
        label: row.attribute.name,
        before: describeAttrState(row.attribute, orig),
        after: describeAttrState(row.attribute, cur),
      })
    }
    return entries
  }, [state, initial, attributeValues])

  const flushChanges = useCallback(async () => {
    const changes = attributeValues.map((row) => {
      const current = buildAssignment(row.attribute, state[row.attribute.id])
      const previous = buildAssignment(row.attribute, initial[row.attribute.id] ?? clearedAttrState())
      return { current, previous }
    })
    const assignmentsToClear = changes
      .filter(({ current, previous }) => isEmptyAssignment(current) && !isEmptyAssignment(previous))
      .map(({ current }) => current)
    const assignmentsToSave = changes
      .map(({ current }) => current)
      .filter((assignment) => !isEmptyAssignment(assignment))

    await Promise.all(assignmentsToClear.map((assignment) => onClear(assignment.attributeId)))
    if (assignmentsToSave.length > 0) {
      await onSave(assignmentsToSave)
    }
  }, [attributeValues, state, initial, onSave, onClear])

  useProductSaveSection({ id: 'attributes', title: 'Attributes', diff, save: flushChanges })

  async function handleSave() {
    setError(null)
    try {
      await flushChanges()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function handleClear(attributeId: string) {
    if (inGlobalSaveFlow) {
      // Defer: just reset the local draft; the global Save flushes onClear
      setState((prev) => ({ ...prev, [attributeId]: clearedAttrState() }))
      return
    }
    setError(null)
    try {
      await onClear(attributeId)
      setState((prev) => ({ ...prev, [attributeId]: clearedAttrState() }))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Clear failed.')
    }
  }

  if (attributeValues.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {emptyHint ?? 'No attributes to edit.'}
      </p>
    )
  }

  function renderAttributeRow(row: AttributeValueRow) {
    const showSlugInline =
      !!productId && !isSelectableInputType(row.attribute.inputType)
    return (
      <div
        key={row.attribute.id}
        className={cn(
          'grid gap-3 rounded-md border bg-card/50 p-3 md:items-center',
          // Selectable rows keep the original 3-col layout; scalar rows with the inline slug editor add a fourth slot
          // that sits to the right of the value input
          showSlugInline
            ? 'md:grid-cols-[minmax(150px,220px)_minmax(180px,1fr)_minmax(220px,auto)_auto]'
            : 'md:grid-cols-[minmax(150px,220px)_minmax(180px,1fr)_auto]',
        )}
      >
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Label className="text-sm font-medium leading-tight">{row.attribute.name}</Label>
            {row.attribute.valueRequired && (
              <span className="text-[10px] font-medium uppercase tracking-wider text-destructive">
                required
              </span>
            )}
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <AttributeInputTypeBadge type={row.attribute.inputType} />
            <code
              className="font-mono text-[10px] text-muted-foreground"
              title="Attribute slug - used in public URL params"
            >
              {row.attribute.slug}
            </code>
            {row.attribute.unit && (
              <span className="text-xs text-muted-foreground">{row.attribute.unit}</span>
            )}
          </div>
        </div>
        <div className="min-w-0 md:max-w-md">
          {/* items-start + h-8 trigger: the control is input + slug badge
              stacked, so centering would sink the button below the input
              line - top-align and match the compact input height instead */}
          <div className="flex min-w-0 items-start gap-4">
            <div className="min-w-0 flex-1">
              <AttributeControl
                attribute={row.attribute}
                state={state[row.attribute.id]}
                disabled={!canManage}
                compact
                onUpdate={(patch) => updateAttr(row.attribute.id, patch)}
              />
            </div>
            {/* Translations sit beside the canonical value they override */}
            {!!productId &&
              row.attribute.inputType === 'PLAIN_TEXT' &&
              !!state[row.attribute.id]?.scalarString && (
              <PlainTextTranslationEditor
                productId={productId}
                attributeId={row.attribute.id}
                attributeName={row.attribute.name}
                canonicalValue={state[row.attribute.id]?.scalarString ?? ''}
                translations={localeValuesByAttr.get(row.attribute.id) ?? []}
                canManage={canManage}
              />
            )}
          </div>
        </div>
        {/* Per-product slug + indexable controls for scalar attrs,
            rendered as the third column (right of the input)
            Selectable attrs manage their slugs globally on the
            Attribute detail page - this column collapses for them */}
        {showSlugInline && (
          <div className="grid min-w-0 gap-1.5">
            <ScalarSlugInlineEditor
              productId={productId!}
              attributeId={row.attribute.id}
              attributeSlug={row.attribute.slug}
              // Slug derivation source: bool maps to the literal "true"/"false" the indexer writes; everything else
              // uses the typed scalarString - Empty string surfaces a helpful empty preview ("no value set yet")
              scalarDisplay={
                row.attribute.inputType === 'BOOLEAN'
                  ? state[row.attribute.id]?.scalarBool ? 'true' : 'false'
                  : state[row.attribute.id]?.scalarString ?? ''
              }
              existing={slugOverridesByAttr.get(row.attribute.id)}
              canManage={canManage}
            />
          </div>
        )}
        {canManage && (
          <Button
            variant="ghost"
            size="sm"
            className="h-8 justify-self-start gap-1 text-xs md:justify-self-end"
            onClick={() => handleClear(row.attribute.id)}
            disabled={saving || clearing}
          >
            <XCircleIcon className="size-3" />
            Clear
          </Button>
        )}
      </div>
    )
  }

  return (
    <div className="grid gap-3">
      <div className="relative">
        <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search attributes..."
          className="pl-8"
        />
      </div>

      {groupedAttributeValues ? (
        <div className="grid gap-6">
          {groupedAttributeValues.groups.map((group) => (
            <section key={group.id} className="grid gap-3">
              <div className="flex items-center justify-between border-b pb-2">
                <h3 className="text-sm font-medium">{group.name}</h3>
                <span className="text-xs text-muted-foreground">
                  {group.rows.length} attribute{group.rows.length === 1 ? '' : 's'}
                </span>
              </div>
              {group.rows.length === 0 ? (
                <p className="rounded-md border border-dashed px-3 py-2 text-sm text-muted-foreground">
                  No attributes in this group.
                </p>
              ) : (
                <div className="grid gap-2">{group.rows.map(renderAttributeRow)}</div>
              )}
            </section>
          ))}

          {groupedAttributeValues.ungroupedRows.length > 0 && (
            <section className="grid gap-3">
              <div className="flex items-center justify-between border-b pb-2">
                <h3 className="text-sm font-medium text-muted-foreground">Ungrouped</h3>
                <span className="text-xs text-muted-foreground">
                  {groupedAttributeValues.ungroupedRows.length} attribute
                  {groupedAttributeValues.ungroupedRows.length === 1 ? '' : 's'}
                </span>
              </div>
              <div className="grid gap-2">
                {groupedAttributeValues.ungroupedRows.map(renderAttributeRow)}
              </div>
            </section>
          )}

          {filteredAttributeValues.length === 0 && (
            <p className="rounded-md border border-dashed px-3 py-3 text-sm text-muted-foreground">
              No attributes match your search.
            </p>
          )}
        </div>
      ) : (
        filteredAttributeValues.length > 0 ? (
          filteredAttributeValues.map(renderAttributeRow)
        ) : (
          <p className="rounded-md border border-dashed px-3 py-3 text-sm text-muted-foreground">
            No attributes match your search.
          </p>
        )
      )}

      {error && !inGlobalSaveFlow && (
        <Alert variant="destructive">
          <AlertTitle>Couldn't update attributes</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {canManage && !inGlobalSaveFlow && (
        <div className="flex justify-end">
          <Button onClick={handleSave} disabled={saving || clearing}>
            <SaveIcon className="size-4" />
            {saving || clearing ? 'Saving…' : 'Save attributes'}
          </Button>
        </div>
      )}
    </div>
  )
}

// valueSlug mirrors the backend's slug normalization (the keyword
// `slug_normalizer` in internal/search/products.go + the Go-side
// `slugify` in internal/search/browse.go): lowercase, keep [a-z0-9],
// collapse anything else to a single dash, trim leading/trailing
// dashes - Used to preview the slug that scalar-attribute values would
// land on in public URLs (?attr_brand=skelet-atelier) - drift here
// would mislead operators about the URL form they're committing to
function valueSlug(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// SlugBadge renders the standard URL-slug pill used next to attribute values throughout the editor - Hidden when slug
// is empty so blank scalar inputs don't show a stray "-"
function SlugBadge({ slug, className }: { slug: string; className?: string }) {
  if (!slug) return null
  return (
    <code
      className={cn(
        'font-mono text-[10px] text-muted-foreground',
        className,
      )}
      title="Public URL slug for this value"
    >
      {slug}
    </code>
  )
}

// Standalone attribute control - a single typed input for one attribute Used by the editor above and by create-time
// forms that need to collect initial attribute values before a row exists to edit
export function AttributeControl({
  attribute,
  state,
  disabled,
  compact,
  onUpdate,
}: {
  attribute: AttributeSummary
  state: AttrState | undefined
  disabled?: boolean
  compact?: boolean
  onUpdate: (patch: Partial<AttrState>) => void
}) {
  if (!state) return null
  const inputClassName = compact ? 'h-8 text-sm' : undefined

  switch (attribute.inputType) {
    case 'PLAIN_TEXT':
      return (
        <div className="flex min-w-0 items-center gap-2">
          <Input
            className={cn(inputClassName, 'min-w-0 flex-1')}
            value={state.scalarString}
            onChange={(e) => onUpdate({ scalarString: e.target.value })}
            placeholder={attribute.unit ? `e.g. 500 ${attribute.unit}` : 'Enter a value'}
            disabled={disabled}
          />
          <SlugBadge slug={valueSlug(state.scalarString)} className="max-w-[40%] shrink-0 truncate" />
        </div>
      )
    case 'NUMERIC':
      return (
        <div className="flex min-w-0 items-center gap-2">
          <Input
            className={cn(inputClassName, 'min-w-0 flex-1')}
            type="number"
            inputMode="decimal"
            step="any"
            value={state.scalarString}
            onChange={(e) => onUpdate({ scalarString: e.target.value })}
            placeholder="0"
            disabled={disabled}
          />
          <SlugBadge slug={valueSlug(state.scalarString)} className="max-w-[40%] shrink-0 truncate" />
        </div>
      )
    case 'BOOLEAN':
      return (
        <div className={cn('flex items-center gap-2', compact && 'h-8')}>
          <Switch
            checked={state.scalarBool}
            onCheckedChange={(v) => onUpdate({ scalarBool: v, hasScalarValue: true })}
            disabled={disabled}
          />
          <span className="text-xs text-muted-foreground">
            {state.scalarBool ? 'Yes' : 'No'}
          </span>
          <SlugBadge slug={state.scalarBool ? 'true' : 'false'} />
        </div>
      )
    case 'DATE':
      return (
        <div className="flex min-w-0 items-center gap-2">
          <Input
            className={cn(inputClassName, 'min-w-0 flex-1')}
            type="date"
            value={state.scalarString}
            onChange={(e) => onUpdate({ scalarString: e.target.value })}
            disabled={disabled}
          />
          <SlugBadge slug={valueSlug(state.scalarString)} className="max-w-[40%] shrink-0 truncate" />
        </div>
      )
    case 'DATE_TIME':
      return (
        <div className="grid gap-1">
          <Input
            className={inputClassName}
            value={state.scalarString}
            onChange={(e) => onUpdate({ scalarString: e.target.value })}
            placeholder="2026-04-23T10:30:00Z"
            disabled={disabled}
          />
          <div className="flex items-center justify-between gap-2">
            <p className="text-[11px] text-muted-foreground">
              RFC 3339 / ISO 8601 (e.g. <code className="font-mono">2026-04-23T10:30:00Z</code>).
            </p>
            <SlugBadge slug={valueSlug(state.scalarString)} />
          </div>
        </div>
      )
    case 'DROPDOWN':
    case 'SWATCH': {
      const selectedSlug =
        attribute.values.find((v) => v.id === state.valueIds[0])?.slug ?? ''
      return (
        <div className="flex min-w-0 items-center gap-2">
          <Select
            value={state.valueIds[0] ?? ''}
            onValueChange={(v) => onUpdate({ valueIds: v ? [v] : [] })}
            disabled={disabled}
          >
            <SelectTrigger className={cn(compact && 'h-8', 'min-w-0 flex-1')}>
              <SelectValue placeholder="Select a value…" />
            </SelectTrigger>
            <SelectContent>
              {attribute.values.map((v) => (
                <SelectItem key={v.id} value={v.id}>
                  <span className="flex w-full items-center gap-2">
                    {attribute.inputType === 'SWATCH' && v.value ? (
                      <span
                        className="inline-block size-3 rounded border"
                        style={{ backgroundColor: v.value }}
                      />
                    ) : null}
                    <span>{v.name}</span>
                    <SlugBadge slug={v.slug} className="ml-auto" />
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <SlugBadge slug={selectedSlug} className="max-w-[40%] shrink-0 truncate" />
        </div>
      )
    }
    case 'MULTISELECT':
      return (
        <div className={cn('grid gap-2 rounded-md border p-3', compact && 'max-h-32 overflow-y-auto py-2')}>
          {attribute.values.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              This attribute has no values yet - add some on its detail page.
            </p>
          ) : (
            attribute.values.map((v) => {
              const checked = state.valueIds.includes(v.id)
              return (
                <div key={v.id} className="flex items-center gap-2">
                  <Checkbox
                    id={`attr-${attribute.id}-val-${v.id}`}
                    checked={checked}
                    onCheckedChange={(next) => {
                      if (next) {
                        onUpdate({ valueIds: [...state.valueIds, v.id] })
                      } else {
                        onUpdate({ valueIds: state.valueIds.filter((id) => id !== v.id) })
                      }
                    }}
                    disabled={disabled}
                  />
                  <Label
                    htmlFor={`attr-${attribute.id}-val-${v.id}`}
                    className="flex-1 text-sm font-normal"
                  >
                    {v.name}
                  </Label>
                  <SlugBadge slug={v.slug} />
                </div>
              )
            })
          )}
        </div>
      )
  }
}

// Per-locale translation editor for one plain_text scalar value on one
// product: a "Translations n/m" trigger opening a popover with one row
// per locale, all editable at once - The canonical value lives in the
// product's attributes JSONB; rows here override it per locale - An
// untranslated locale's input shows the canonical value as placeholder
// - the exact fallback it renders - Empty clears the translation, and a
// value equal to the canonical is normalized to empty for the same
// reason - an identical override row would silently shadow future
// canonical edits on that locale
//
// STAGED: edits register with the page save registry (one section per
// attribute) and apply through the header Save + confirm modal - no
// inline save button, per the dashboard convention
function PlainTextTranslationEditor({
  productId,
  attributeId,
  attributeName,
  canonicalValue,
  translations,
  canManage,
}: {
  productId: string
  attributeId: string
  attributeName: string
  canonicalValue: string
  translations: Array<{ locale: string; value: string }>
  canManage: boolean
}) {
  const { availableLocales, channels } = useConfigScope()
  const localeOptions = useMemo(() => {
    const seen = new Set<string>()
    const out: string[] = []
    // Channel scope → that channel's locales - Global scope has no channel selection (availableLocales is empty), but
    // translations are global rows - offer the union across visible channels
    const base = availableLocales.length
      ? availableLocales
      : channels.flatMap((c) =>
          c.locales.length ? c.locales.map((l) => l.code) : [c.defaultLocale],
        )
    for (const l of base) {
      if (l && !seen.has(l)) { seen.add(l); out.push(l) }
    }
    // Stray locales that already carry a translation but are no longer
    // in any channel's list still need to be visible (and clearable)
    for (const t of translations) {
      if (t.locale && !seen.has(t.locale)) { seen.add(t.locale); out.push(t.locale) }
    }
    return out
  }, [availableLocales, channels, translations])

  const byLocale = useMemo(() => {
    const m = new Map<string, string>()
    for (const t of translations) m.set(t.locale, t.value)
    return m
  }, [translations])

  // Value fingerprint of the server rows - The `translations` prop is rebuilt on every parent render (`... ?? []`), so resetting on its
  // identity would wipe staged drafts mid-typing - any keystroke that updates the save registry re-renders the parent
  const translationsKey = useMemo(
    () => translations.map((t) => `${t.locale}\u0000${t.value}`).join('\u0001'),
    [translations],
  )

  // locale → staged value - Absent key = untouched (server value)
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  useEffect(() => {
    // Refetch landed - staged edits were applied or are stale
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDrafts({})
  }, [translationsKey])

  // The staged canonical value: drafts matching it collapse to '' so
  // they ride the delete/fallback path instead of storing a shadow row
  const canonical = canonicalValue.trim()

  const dirtyLocales = useMemo(
    () =>
      Object.entries(drafts)
        .filter(([l, v]) => {
          const next = v.trim() === canonical ? '' : v.trim()
          return next !== (byLocale.get(l) ?? '')
        })
        .map(([l]) => l)
        .sort(),
    [drafts, byLocale, canonical],
  )

  const [save] = useMutation(SetProductAttributeLocaleValueDocument, {
    refetchQueries: [
      { query: ProductAttributeLocaleValuesDocument, variables: { productId } },
    ],
  })

  const flushRef = useRef({ drafts, dirtyLocales, byLocale, canonical })
  useEffect(() => {
    // Keep the save closure's snapshot fresh on every commit
    flushRef.current = { drafts, dirtyLocales, byLocale, canonical }
  })
  useSaveSection({
    id: `attrtrans:${attributeId}`,
    title: 'Attribute value translations',
    diff: dirtyLocales.map((l) => {
      const t = drafts[l]?.trim() ?? ''
      return {
        label: `${attributeName} · ${l}`,
        before: byLocale.get(l) ?? '-',
        after: (t === canonical ? '' : t) || '- (canonical)',
      }
    }),
    save: async () => {
      const cur = flushRef.current
      for (const l of cur.dirtyLocales) {
        const t = cur.drafts[l].trim()
        await save({
          variables: {
            productId,
            attributeId,
            locale: l,
            value: t === cur.canonical ? '' : t,
          },
        })
      }
    },
  })

  // Effective (staged) per-locale value after the same normalization the
  // save path applies: equal-to-canonical counts as "not translated"
  const effectiveValue = (l: string) => {
    const v = (drafts[l] ?? byLocale.get(l) ?? '').trim()
    return v === canonical ? '' : v
  }
  const translatedCount = localeOptions.filter((l) => effectiveValue(l) !== '').length

  if (localeOptions.length === 0) return null

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 w-fit gap-1.5 px-2 text-xs font-normal text-muted-foreground hover:text-foreground"
        >
          <LanguagesIcon className="size-3.5" />
          Translations
          <span className="tabular-nums">
            {translatedCount}/{localeOptions.length}
          </span>
          {dirtyLocales.length > 0 && (
            <span className="size-1.5 rounded-full bg-warning" aria-label="unsaved translation edits" />
          )}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 p-3">
        <div className="grid gap-2.5">
          <div className="grid gap-1">
            <div className="text-xs font-medium">{attributeName} translations</div>
            <p className="text-[11px] leading-4 text-muted-foreground">
              A locale left empty shows the canonical value. Applied on Save.
            </p>
          </div>
          <div className="grid gap-1.5">
            {localeOptions.map((l) => {
              const dirty = dirtyLocales.includes(l)
              const translated = effectiveValue(l) !== ''
              return (
                <div key={l} className="flex items-center gap-2">
                  <span className="w-14 shrink-0 truncate font-mono text-[11px] text-muted-foreground">
                    {l}
                  </span>
                  <Input
                    className="h-7 flex-1 text-xs"
                    value={drafts[l] ?? byLocale.get(l) ?? ''}
                    placeholder={canonical || 'canonical'}
                    disabled={!canManage}
                    aria-label={`${attributeName} translation for ${l}`}
                    onChange={(e) => setDrafts((prev) => ({ ...prev, [l]: e.target.value }))}
                  />
                  <span
                    className={cn(
                      'size-1.5 shrink-0 rounded-full',
                      dirty ? 'bg-warning' : translated ? 'bg-success' : 'bg-transparent',
                    )}
                    aria-hidden
                  />
                </div>
              )
            })}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}

// Inline editor for the per-(product, scalar attribute) URL slug
// Renders directly under the attribute's value input in the Attributes
// section
//
// STAGED: the input stages an override; empty = no override (the
// indexer derives slugify(value)) - Registers with the page save
// registry and applies through the header Save + confirm modal - no
// inline save button, per the dashboard convention
function ScalarSlugInlineEditor({
  productId,
  attributeId,
  attributeSlug,
  scalarDisplay,
  existing,
  canManage,
}: {
  productId: string
  attributeId: string
  attributeSlug: string
  // The current value typed by the operator - used to derive the
  // default slug ("slugify(value)") shown when no override exists
  scalarDisplay: string
  existing: { id: string; slug: string } | undefined
  canManage: boolean
}) {
  const derivedSlug = slugify(scalarDisplay)
  const serverSlug = existing?.slug ?? ''
  const [slug, setSlug] = useState(serverSlug)

  // Re-seed local state when the underlying override changes (refetch after save or another tab) - Keeps the inline form in sync with the
  // source of truth without manual refresh - Adjusted during render (React's "adjust state on prop change" pattern)
  const [prevExisting, setPrevExisting] = useState({ id: existing?.id, slug: existing?.slug })
  if (prevExisting.id !== existing?.id || prevExisting.slug !== existing?.slug) {
    setPrevExisting({ id: existing?.id, slug: existing?.slug })
    setSlug(existing?.slug ?? '')
  }

  const refetch = [
    { query: ProductAttributeSlugsDocument, variables: { productId } },
  ]
  const [setMut] = useMutation(SetProductAttributeSlugDocument, { refetchQueries: refetch })
  const [delMut] = useMutation(DeleteProductAttributeSlugDocument, { refetchQueries: refetch })

  const staged = slugify(slug)
  const dirty = staged !== serverSlug

  const flushRef = useRef({ staged, hadOverride: !!existing })
  useEffect(() => {
    // Keep the save closure's snapshot fresh on every commit
    flushRef.current = { staged, hadOverride: !!existing }
  })
  useSaveSection({
    id: `attrslug:${attributeId}`,
    title: 'Attribute URL slugs',
    diff: dirty
      ? [{
          label: attributeSlug,
          before: serverSlug || `(derived: ${derivedSlug || '-'})`,
          after: staged || `(derived: ${derivedSlug || '-'})`,
        }]
      : [],
    save: async () => {
      const cur = flushRef.current
      if (cur.staged === '') {
        if (cur.hadOverride) await delMut({ variables: { productId, attributeId } })
        return
      }
      await setMut({ variables: { input: { productId, attributeId, slug: cur.staged } } })
    },
  })

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs">
      <Input
        className="h-7 w-36 font-mono text-xs"
        value={slug}
        placeholder={derivedSlug ? `${derivedSlug} (derived)` : 'slug'}
        disabled={!canManage}
        onChange={(e) => setSlug(e.target.value)}
        onBlur={() => setSlug((s) => (s.trim() === '' ? '' : slugify(s)))}
        title={
          existing || dirty
            ? `URL: /products/f/${attributeSlug}/v/${staged || derivedSlug}${dirty ? ' - applied on Save' : ''}`
            : `No override - defaults to /products/f/${attributeSlug}/v/${derivedSlug || 'slug'}`
        }
      />
      {dirty && <span className="text-[11px] text-muted-foreground">*</span>}
    </div>
  )
}
