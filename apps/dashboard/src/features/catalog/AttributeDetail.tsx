import { useMemo, useRef, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  CheckIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
  XIcon,
  Undo2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { LoadingState } from '@/components/ui/loading-state'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { slugify } from '@/lib/slug'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import {
  AddAttributeValueDocument,
  AttributeDocument,
  AttributesDocument,
  DeleteAttributeDocument,
  DeleteAttributeValueDocument,
  SetAttributeIndexableDocument,
  UpdateAttributeDocument,
  UpdateAttributeValueDocument,
} from './Catalog.gql'
import {
  AttributeDescriptionEditor,
  AttributeValueDescriptionEditor,
  type EmbeddedPending,
} from './AttributeDescriptionEditor'
import { ValueChannelSlugEditor } from './ValueChannelSlugEditor'
import { AttributeSearchTuningCard } from './AttributeSearchTuningCard'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { display } from '@/shared/saving/diff'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import { INPUT_TYPES_WITH_VALUES } from './attributeInputTypes'
import { PluginSections } from '@/plugins/detail-sections'

export function AttributeDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('catalog.manage')

  const attributeQuery = useQuery(AttributeDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const attribute = attributeQuery.data?.attribute

  const refetch = [
    { query: AttributeDocument, variables: { id } },
    { query: AttributesDocument },
  ]

  const [deleteAttribute, { loading: deleting }] = useMutation(DeleteAttributeDocument, {
    refetchQueries: [{ query: AttributesDocument }],
  })
  const [updateAttribute] = useMutation(UpdateAttributeDocument, {
    refetchQueries: refetch,
  })
  const [addValue] = useMutation(AddAttributeValueDocument, {
    refetchQueries: refetch,
  })
  const [updateValue] = useMutation(UpdateAttributeValueDocument, { refetchQueries: refetch })
  const [deleteValue] = useMutation(DeleteAttributeValueDocument, {
    refetchQueries: refetch,
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Page save registry: every editable section (settings, translations, search tuning) stages its edits and the header Save applies them
  // through the SaveConfirmDialog diff modal - the dashboard-wide convention (see docs/conventions.md)
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
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

  async function onDeleteAttribute() {
    setDeleteError(null)
    try {
      await deleteAttribute({ variables: { id } })
      navigate('/catalog/attributes', { replace: true })
    } catch (e) {
      setDeleteError(e instanceof Error ? e.message : 'Delete failed.')
    }
  }

  if (attributeQuery.loading) return <LoadingState label="Loading attribute…" />
  if (attributeQuery.error) {
    return (
      <LoadError
        title="Couldn't load attribute"
        message={attributeQuery.error.message}
        onRetry={() => void attributeQuery.refetch()}
        retrying={attributeQuery.loading}
      />
    )
  }
  if (!attribute) return <p className="text-sm text-muted-foreground">Attribute not found.</p>

  const supportsValues = INPUT_TYPES_WITH_VALUES.has(attribute.inputType)
  const isSwatch = attribute.inputType === 'SWATCH'

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/catalog/attributes"
        backLabel="Back to attributes"
        title={attribute.name}
        badges={<AttributeInputTypeBadge type={attribute.inputType} />}
        meta={
          <>
            <code className="font-mono">{attribute.slug}</code>
            <span>·</span>
            <span>slug and input type are immutable</span>
          </>
        }
        actions={
          canManage ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => { setDeleteError(null); setConfirmingDelete(true) }}
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
                {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
              </Button>
            </>
          ) : null
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Settings"
          description="Name, unit, and behaviour flags."
        >
          <SettingsCard
            attribute={attribute}
            defaultContext={{
              inputType: attribute.inputType,
              values: attribute.values,
              currentDefault: attribute.defaultValue,
            }}
            canManage={canManage}
            onSave={async (input, defaultChange) => {
              // `input` already contains `id` (AttributeSettings carries it for the cascade mutation), so just spread
              // it - no separate `id` override needed
              await updateAttribute({
                variables: {
                  input: {
                    ...input,
                    ...(defaultChange ? { defaultValue: defaultChange } : {}),
                  },
                },
              })
            }}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Public description"
          description="Short locale-aware copy rendered next to the facet panel header. Sparse - translate only the locales you care about; the rest fall back to nothing."
        >
          <AttributeDescriptionEditor
            attributeId={id}
            descriptions={attribute.descriptions ?? []}
            canManage={canManage}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Search tuning"
          description="How this attribute participates in the channel product search. Stored centrally - saving here updates this attribute's row in /search-engine's Product Search table."
        >
          <AttributeSearchTuningCard
            attributeSlug={attribute.slug}
            searchable={attribute.searchable}
          />
        </DetailPageSection>

        {supportsValues && (
          <DetailPageSection
            title="Values"
            description={
              isSwatch
                ? 'Swatch options - name shown to customers, value holds the hex colour.'
                : 'Pre-defined options for this attribute.'
            }
          >
            <ValuesCard
              attributeId={id}
              isSwatch={isSwatch}
              values={attribute.values}
              canManage={canManage}
              onAdd={async (input) => {
                await addValue({ variables: { attributeId: id, input } })
              }}
              onUpdate={async (valueId, input) => {
                await updateValue({ variables: { id: valueId, input } })
              }}
              onDelete={async (valueId) => {
                await deleteValue({ variables: { id: valueId } })
              }}
            />
          </DetailPageSection>
        )}

        <PluginSections target="attribute" entity={attribute} mode="edit" />
      </DetailPageBody>

      {/* Delete attribute confirm */}
      <AlertDialog
        open={confirmingDelete}
        onOpenChange={(v) => { if (!v) { setConfirmingDelete(false); setDeleteError(null) } }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {attribute.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              Permanently removes this attribute, all its values, and its assignments to any product types.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={deleting}
              onClick={(e) => { e.preventDefault(); onDeleteAttribute() }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete attribute'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Review-before-save modal: lists every staged change across the
          page's sections (settings, translations, search tuning) */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Attribute · ${attribute.name}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// ---------- Settings card ----------

type AttributeSettings = {
  // SettingsCard needs the id for the master-switch cascade mutation (separate from the regular UpdateAttribute save) - Included on the
  // settings shape rather than passed as a separate prop so the draft reset still works against a single `attribute` object
  id: string
  name: string
  unit: string
  valueRequired: boolean
  filterable: boolean
  searchable: boolean
  isPublic: boolean
  facetCollapsed: boolean
  isIndexable: boolean
  h1Position: 'BEFORE' | 'AFTER'
  synapseTrack: boolean
  autoSegment: boolean
  sortOrder: number
}

// Used by SettingsCard to render the type-aware default-value control - The
// attribute's input type plus (for selectables) the value list are read-only
// from the parent and don't round-trip through onSave - only AttributeSettings
// + an optional defaultValue change pass through
type DefaultValueContext = {
  inputType: import('@/graphql/schema.types').AttributeInputType
  values: Array<{ id: string; name: string; value: string }>
  currentDefault: unknown
}

// Three-state outcome from the default control:
//   undefined → no change (caller should omit defaultValue from the mutation)
//   { clear: true } → clear the default
//   { value: ... } → set the default to this JSON-encodable value
type DefaultValueChange = { clear?: boolean; value?: unknown } | undefined

function SettingsCard({
  attribute,
  defaultContext,
  canManage,
  onSave,
}: {
  attribute: AttributeSettings
  defaultContext: DefaultValueContext
  canManage: boolean
  onSave: (input: AttributeSettings, defaultChange: DefaultValueChange) => Promise<unknown>
}) {
  const [name, setName] = useState(attribute.name)
  const [unit, setUnit] = useState(attribute.unit ?? '')
  const [valueRequired, setValueRequired] = useState(attribute.valueRequired)
  const [filterable, setFilterable] = useState(attribute.filterable)
  const [searchable, setSearchable] = useState(attribute.searchable)
  const [isPublic, setIsPublic] = useState(attribute.isPublic)
  const [facetCollapsed, setFacetCollapsed] = useState(attribute.facetCollapsed)
  const [isIndexable, setIsIndexable] = useState(attribute.isIndexable)
  const [h1Position, setH1Position] = useState<'BEFORE' | 'AFTER'>(attribute.h1Position)
  const [synapseTrack, setSynapseTrack] = useState(attribute.synapseTrack)
  const [autoSegment, setAutoSegment] = useState(attribute.autoSegment)
  // Master-switch cascade state - The user can't just toggle the switch - the change implies bulk-updating every
  // value/override under this attribute, so we surface a confirm first
  const [cascadePending, setCascadePending] = useState<{ next: boolean } | null>(null)
  const [cascadeError, setCascadeError] = useState<string | null>(null)
  const [cascadeResult, setCascadeResult] = useState<{ values: number; products: number } | null>(null)
  const [cascadeIndexable, { loading: cascading }] = useMutation(SetAttributeIndexableDocument, {
    refetchQueries: [
      { query: AttributeDocument, variables: { id: attribute.id } },
      { query: AttributesDocument },
    ],
  })
  const [sortOrder, setSortOrder] = useState(attribute.sortOrder)

  const initialDefault = serializeDefault(defaultContext.inputType, defaultContext.currentDefault)
  const [defaultEnabled, setDefaultEnabled] = useState(defaultContext.currentDefault != null)
  const [defaultDraft, setDefaultDraft] = useState(initialDefault)

  // Re-seed the drafts when the server data changes (save refetch landed) - Adjusted during render (React's "adjust
  // state on prop change" pattern), keyed on the server data itself
  const [prevSource, setPrevSource] = useState({
    attribute,
    currentDefault: defaultContext.currentDefault,
    inputType: defaultContext.inputType,
  })
  if (
    prevSource.attribute !== attribute ||
    prevSource.currentDefault !== defaultContext.currentDefault ||
    prevSource.inputType !== defaultContext.inputType
  ) {
    setPrevSource({
      attribute,
      currentDefault: defaultContext.currentDefault,
      inputType: defaultContext.inputType,
    })
    setName(attribute.name)
    setUnit(attribute.unit ?? '')
    setValueRequired(attribute.valueRequired)
    setFilterable(attribute.filterable)
    setSearchable(attribute.searchable)
    setIsPublic(attribute.isPublic)
    setFacetCollapsed(attribute.facetCollapsed)
    setIsIndexable(attribute.isIndexable)
    setH1Position(attribute.h1Position)
    setSynapseTrack(attribute.synapseTrack)
    setAutoSegment(attribute.autoSegment)
    setSortOrder(attribute.sortOrder)
    setDefaultEnabled(defaultContext.currentDefault != null)
    setDefaultDraft(serializeDefault(defaultContext.inputType, defaultContext.currentDefault))
  }

  const defaultDirty = (() => {
    const wasEnabled = defaultContext.currentDefault != null
    if (defaultEnabled !== wasEnabled) return true
    if (!defaultEnabled) return false
    return !sameSerializedDefault(defaultDraft, initialDefault)
  })()

  function buildDefaultChange(): DefaultValueChange {
    if (!defaultDirty) return undefined
    if (!defaultEnabled) return { clear: true }
    const value = decodeDefault(defaultContext.inputType, defaultDraft)
    if (value === SERIALIZE_INVALID) {
      throw new Error('Default value is invalid for this attribute.')
    }
    return { value }
  }

  // Required ⇒ default invariant - Predicts whether the form would land in a state the server rejects so we can surface
  // a clear in-page hint instead of letting the mutation fail in the save modal
  const willHaveDefault = defaultEnabled
  const violatesRequired = valueRequired && !willHaveDefault
  const requiredHint = violatesRequired
    ? 'Required attributes must have a default value. Enable the default below before turning required on.'
    : null

  // Staged save: field edits register with the page save registry; the header Save + confirm modal applies them - The
  // indexable master switch stays an immediate guarded action (its cascade has its own confirm dialog below)
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    const field = (label: string, before: string, after: string) => {
      if (before !== after) out.push({ label, before, after })
    }
    field('name', attribute.name, name)
    field('unit', attribute.unit ?? '', unit)
    field('required', display(attribute.valueRequired), display(valueRequired))
    field('filterable', display(attribute.filterable), display(filterable))
    field('searchable', display(attribute.searchable), display(searchable))
    field('public', display(attribute.isPublic), display(isPublic))
    field('facet collapsed', display(attribute.facetCollapsed), display(facetCollapsed))
    field('H1 position', attribute.h1Position, h1Position)
    field('track in Synapse', display(attribute.synapseTrack), display(synapseTrack))
    // Auto-segment is only effective while tracking is on - compare effective vs effective or a stored flag with
    // tracking off reads as a phantom edit
    field(
      'auto-create segments',
      display(attribute.synapseTrack && attribute.autoSegment),
      display(synapseTrack && autoSegment),
    )
    field('sort order', String(attribute.sortOrder), String(sortOrder))
    if (defaultDirty) {
      // Select-family drafts hold value IDs (newline-joined for
      // multiselect) - show the value NAMES in the confirm dialog
      const humanDefault = (draft: string) => {
        if (draft === '') return ''
        if (defaultContext.inputType === 'DROPDOWN' || defaultContext.inputType === 'SWATCH' || defaultContext.inputType === 'MULTISELECT') {
          return draft
            .split('\n')
            .filter(Boolean)
            .map((id) => defaultContext.values.find((v) => v.id === id)?.name ?? id)
            .join(', ')
        }
        return draft
      }
      out.push({
        label: 'default value',
        before: defaultContext.currentDefault != null ? humanDefault(initialDefault) : '',
        after: defaultEnabled ? humanDefault(defaultDraft) : '',
      })
    }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [attribute, name, unit, valueRequired, filterable, searchable, isPublic, facetCollapsed, h1Position, synapseTrack, autoSegment, sortOrder, defaultDirty, defaultEnabled, defaultDraft])

  useSaveSection({
    id: 'settings',
    title: 'Settings',
    diff,
    save: async () => {
      if (violatesRequired) throw new Error(requiredHint ?? 'Required attributes must have a default value.')
      if (name.trim() === '') throw new Error('Name is required.')
      const change = buildDefaultChange()
      await onSave(
        { id: attribute.id, name, unit, valueRequired, filterable, searchable, isPublic, facetCollapsed, isIndexable, h1Position, synapseTrack, autoSegment: synapseTrack && autoSegment, sortOrder },
        change,
      )
    },
  })

  return (
    <div className="grid gap-4">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-[1fr_160px_100px]">
            <div className="grid gap-2">
              <Label htmlFor="s-name">Name</Label>
              <Input
                id="s-name"
                required
                value={name}
                disabled={!canManage}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="s-unit">Unit</Label>
              <Input
                id="s-unit"
                placeholder="cm, kg…"
                value={unit}
                disabled={!canManage}
                onChange={(e) => setUnit(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="s-sort">Sort order</Label>
              <Input
                id="s-sort"
                type="number"
                inputMode="numeric"
                value={sortOrder}
                disabled={!canManage}
                onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
              />
            </div>
          </div>

          <div className="grid gap-3 rounded-md border p-4">
            <SwitchRow id="s-required" label="Value required"
              description="Products must provide a value for this attribute."
              checked={valueRequired} onCheckedChange={setValueRequired} disabled={!canManage} />
            <SwitchRow id="s-filterable" label="Filterable"
              description="Show in channel filter facets."
              checked={filterable} onCheckedChange={setFilterable} disabled={!canManage} />
            <SwitchRow id="s-facet-collapsed" label="Collapse facet by default"
              description={filterable
                ? 'Channels render this filter collapsed on first load. Users can still open it.'
                : 'Only applies when "Filterable" is on.'}
              checked={facetCollapsed} onCheckedChange={setFacetCollapsed} disabled={!canManage} />
            <SwitchRow id="s-attr-indexable" label="Indexable facet (master switch)"
              description="Toggling this cascades the flag to every value under this attribute (and every per-product slug override). You'll get a confirmation before anything changes."
              checked={isIndexable}
              onCheckedChange={(next) => {
                if (next === isIndexable) return
                setCascadeError(null)
                setCascadeResult(null)
                setCascadePending({ next })
              }}
              disabled={!canManage || cascading} />
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="s-h1-position" className="text-sm">
                  H1 position
                </Label>
                <p className="text-xs text-muted-foreground">
                  Where the value appears in the listing page H1 when
                  indexable. BEFORE → "Olive / Apparel"; AFTER → "Apparel / Olive".
                </p>
              </div>
              <select
                id="s-h1-position"
                className="h-8 rounded-md border bg-background px-2 text-sm"
                value={h1Position}
                disabled={!canManage}
                onChange={(e) => setH1Position(e.target.value as 'BEFORE' | 'AFTER')}
              >
                <option value="BEFORE">Before</option>
                <option value="AFTER">After</option>
              </select>
            </div>
            <SwitchRow id="s-searchable" label="Searchable"
              description="Include in full-text product search."
              checked={searchable} onCheckedChange={setSearchable} disabled={!canManage} />
            <SwitchRow id="s-visible" label="Visible to customers"
              description="Display on the product detail page."
              checked={isPublic} onCheckedChange={setIsPublic} disabled={!canManage} />
            <SwitchRow id="s-synapse-track" label="Track in Synapse"
              description="Record this attribute's values as a customer affinity when they filter by it or view products that carry it (e.g. tire size → 'they likely need this size'). Targetable from Segments."
              checked={synapseTrack} onCheckedChange={setSynapseTrack} disabled={!canManage} />
            <SwitchRow id="s-auto-segment" label="Auto-create segments"
              description="Automatically build one customer segment per popular value (e.g. 'Tire size: 205/55R16'), refreshed on a schedule and targetable by Pulse / promotions / Prism. Requires Track in Synapse."
              checked={synapseTrack && autoSegment} onCheckedChange={setAutoSegment} disabled={!canManage || !synapseTrack} />
          </div>

          <DefaultValueEditor
            inputType={defaultContext.inputType}
            values={defaultContext.values}
            enabled={defaultEnabled}
            draft={defaultDraft}
            onToggleEnabled={setDefaultEnabled}
            onDraftChange={setDefaultDraft}
            disabled={!canManage}
          />

          {violatesRequired && (
            <Alert variant="destructive">
              <AlertTitle>Default value required</AlertTitle>
              <AlertDescription>{requiredHint}</AlertDescription>
            </Alert>
          )}

          {cascadeResult && (
            <Alert>
              <AlertTitle>Cascade applied</AlertTitle>
              <AlertDescription>
                Updated indexable on {cascadeResult.values} value
                {cascadeResult.values === 1 ? '' : 's'} and{' '}
                {cascadeResult.products} product override
                {cascadeResult.products === 1 ? '' : 's'} under this attribute.
              </AlertDescription>
            </Alert>
          )}

      <AlertDialog
        open={!!cascadePending}
        onOpenChange={(v) => { if (!v && !cascading) { setCascadePending(null); setCascadeError(null) } }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {cascadePending?.next ? 'Mark this entire facet indexable?' : 'Remove indexable from this entire facet?'}
            </AlertDialogTitle>
            <AlertDialogDescription>
              This will set <code className="font-mono">isIndexable = {String(cascadePending?.next)}</code> on
              {' '}<strong>{attribute.name}</strong> AND every value / per-product
              slug override under it. The change is one transaction; per-value
              flags can still be flipped individually afterwards.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {cascadeError && (
            <Alert variant="destructive">
              <AlertTitle>Cascade failed</AlertTitle>
              <AlertDescription>{cascadeError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={cascading}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={cascading}
              onClick={async (e) => {
                e.preventDefault()
                if (!cascadePending) return
                setCascadeError(null)
                try {
                  const res = await cascadeIndexable({
                    variables: {
                      attributeId: attribute.id,
                      isIndexable: cascadePending.next,
                    },
                  })
                  const data = res.data?.setAttributeIndexable
                  if (data) {
                    setIsIndexable(data.attribute.isIndexable)
                    setCascadeResult({
                      values: data.affectedValuesCount,
                      products: data.affectedProductsCount,
                    })
                  }
                  setCascadePending(null)
                } catch (e2) {
                  setCascadeError(e2 instanceof Error ? e2.message : 'Cascade failed.')
                }
              }}
            >
              {cascading ? 'Applying…' : cascadePending?.next ? 'Yes, mark indexable' : 'Yes, remove'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

// ---------- Values card ----------

type ValueRow = {
  id: string
  slug: string
  name: string
  value: string
  sortOrder: number
  isIndexable: boolean
  // Localised descriptions surfaced from the Attribute query - The ValueEditRow component renders an editor for these alongside the
  // standard fields when editing existing values; new rows render the editor after the initial save so the value id exists
  descriptions?: { locale: string; description: string }[]
}
type ValueInput = { slug: string; name: string; value?: string; sortOrder: number; isIndexable: boolean }

function ValuesCard({
  attributeId,
  isSwatch,
  values,
  canManage,
  onAdd,
  onUpdate,
  onDelete,
}: {
  attributeId: string
  isSwatch: boolean
  values: ValueRow[]
  canManage: boolean
  onAdd: (input: ValueInput) => Promise<unknown>
  onUpdate: (id: string, input: ValueInput) => Promise<unknown>
  onDelete: (id: string) => Promise<unknown>
}) {
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editingAdd, setEditingAdd] = useState<number | null>(null)
  const [addingRow, setAddingRow] = useState(false)

  // Staged, never saved directly - row actions land here and the header Save applies the lot
  // through the confirm dialog, like every other section on this page. Embedded editors
  // (translations, channel slugs) ride along: their flush closures are captured at stage time
  // and called during the section save, after the row's own mutation
  const [edits, setEdits] = useState<Map<string, { input: ValueInput; pending: EmbeddedPending[] }>>(new Map())
  const [adds, setAdds] = useState<ValueInput[]>([])
  const [deletes, setDeletes] = useState<Set<string>>(new Set())

  const rowSummary = (v: { name: string; slug: string; value?: string | null; sortOrder: number; isIndexable: boolean }) =>
    [v.name, v.slug, v.value || '', String(v.sortOrder), v.isIndexable ? 'indexable' : 'not indexable'].join(' \u00b7 ')

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    for (const input of adds) out.push({ label: 'value', before: '', after: input.name, kind: 'add' })
    for (const [valueId, e] of edits) {
      const before = values.find((v) => v.id === valueId)
      if (!before) continue
      out.push({ label: `value "${before.name}"`, before: rowSummary(before), after: rowSummary(e.input) })
      // A row staged only for its embedded translations/slugs would otherwise diff as a no-op and
      // never reach save - name the pending flushes so the section stays dirty
      if (e.pending.some((p) => p.dirty)) {
        out.push({ label: `value "${before.name}"`, before: '', after: 'translations / channel slugs', kind: 'add' })
      }
    }
    for (const valueId of deletes) {
      const v = values.find((x) => x.id === valueId)
      if (v) out.push({ label: 'value (products referencing it may be affected)', before: v.name, after: '', kind: 'remove' })
    }
    return out
  }, [adds, edits, deletes, values])

  useSaveSection({
    id: 'values',
    title: 'Values',
    diff,
    save: async () => {
      for (const input of adds) await onAdd(input)
      for (const [valueId, e] of edits) {
        await onUpdate(valueId, e.input)
        for (const p of e.pending) {
          if (p.dirty) await p.flush()
        }
      }
      for (const valueId of deletes) await onDelete(valueId)
      setAdds([])
      setEdits(new Map())
      setDeletes(new Set())
    },
  })

  function stageEdit(valueId: string, input: ValueInput, pending: EmbeddedPending[]) {
    setEdits((prev) => new Map(prev).set(valueId, { input, pending }))
    setEditingId(null)
  }
  function stageDelete(valueId: string) {
    setDeletes((prev) => new Set(prev).add(valueId))
    // a staged edit of a row now marked for deletion would only confuse the diff
    setEdits((prev) => {
      if (!prev.has(valueId)) return prev
      const next = new Map(prev)
      next.delete(valueId)
      return next
    })
  }
  function unstageDelete(valueId: string) {
    setDeletes((prev) => {
      const next = new Set(prev)
      next.delete(valueId)
      return next
    })
  }

  return (
    <div className="grid gap-3">
      {canManage && !addingRow && (
        <div className="flex justify-end">
          <Button size="sm" variant="outline" onClick={() => { setEditingId(null); setEditingAdd(null); setAddingRow(true) }}>
            <PlusIcon className="size-4" /> Add value
          </Button>
        </div>
      )}
      {values.length === 0 && adds.length === 0 && !addingRow ? (
        <p className="rounded-md border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
          No values yet.
        </p>
      ) : (
        <div className="divide-y rounded-md border bg-background">
          {values.map((v) => {
            const staged = edits.get(v.id)
            const shown = staged ? { ...v, ...staged.input, value: staged.input.value ?? '' } : v
            return editingId === v.id ? (
              <ValueEditRow
                key={v.id}
                attributeId={attributeId}
                initial={v}
                draft={staged?.input}
                isSwatch={isSwatch}
                onStage={(input, pending) => stageEdit(v.id, input, pending)}
                onCancel={() => setEditingId(null)}
              />
            ) : (
              <ValueDisplayRow
                key={v.id}
                value={shown}
                isSwatch={isSwatch}
                canManage={canManage && !deletes.has(v.id)}
                pending={!!staged}
                deleted={deletes.has(v.id)}
                onEdit={() => { setAddingRow(false); setEditingAdd(null); setEditingId(v.id) }}
                onDelete={() => stageDelete(v.id)}
                onRestore={() => unstageDelete(v.id)}
              />
            )
          })}
          {adds.map((input, i) =>
            editingAdd === i ? (
              <ValueEditRow
                key={`__add-${i}`}
                attributeId={attributeId}
                draft={input}
                isSwatch={isSwatch}
                onStage={(next) => {
                  setAdds((prev) => prev.map((p, j) => (j === i ? next : p)))
                  setEditingAdd(null)
                }}
                onCancel={() => setEditingAdd(null)}
                isNew
              />
            ) : (
              <ValueDisplayRow
                key={`__add-${i}`}
                value={{ id: `__add-${i}`, ...input, value: input.value ?? '' }}
                isSwatch={isSwatch}
                canManage={canManage}
                pending
                onEdit={() => { setAddingRow(false); setEditingId(null); setEditingAdd(i) }}
                onDelete={() => setAdds((prev) => prev.filter((_, j) => j !== i))}
              />
            ),
          )}
          {addingRow && (
            <ValueEditRow
              key="__new__"
              attributeId={attributeId}
              isSwatch={isSwatch}
              onStage={(input) => {
                setAdds((prev) => [...prev, input])
                setAddingRow(false)
              }}
              onCancel={() => setAddingRow(false)}
              isNew
            />
          )}
        </div>
      )}
    </div>
  )
}

function ValueDisplayRow({
  value,
  isSwatch,
  canManage,
  pending,
  deleted,
  onEdit,
  onDelete,
  onRestore,
}: {
  value: Omit<ValueRow, 'descriptions'>
  isSwatch: boolean
  canManage: boolean
  pending?: boolean
  deleted?: boolean
  onEdit: () => void
  onDelete: () => void
  onRestore?: () => void
}) {
  return (
    <div className={cn('flex items-center gap-3 px-6 py-3', deleted && 'opacity-60')}>
      {isSwatch && (
        <span
          className="size-5 shrink-0 rounded-full border"
          style={{ backgroundColor: value.value || undefined }}
        />
      )}
      <div className="min-w-0 flex-1 grid gap-0.5">
        <span className={cn('text-sm font-medium leading-none', deleted && 'line-through')}>{value.name}</span>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <code className="font-mono">{value.slug}</code>
          {!isSwatch && value.value && (
            <>
              <span>·</span>
              <span>{value.value}</span>
            </>
          )}
          {isSwatch && value.value && (
            <>
              <span>·</span>
              <code className="font-mono">{value.value}</code>
            </>
          )}
        </div>
      </div>
      {value.isIndexable && (
        <span
          className="shrink-0 rounded border border-emerald-700/40 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-emerald-300"
          title="Filtered listing renders as <a href> and is allowed to be indexed by Google"
        >
          Indexable
        </span>
      )}
      <span className="shrink-0 font-mono text-xs text-muted-foreground w-6 text-right">
        {value.sortOrder}
      </span>
      {pending && !deleted && (
        <span className="shrink-0 rounded border border-amber-600/40 bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-amber-600 dark:text-amber-400">
          pending - applied on Save
        </span>
      )}
      {deleted ? (
        <div className="flex shrink-0 items-center gap-2">
          <span className="rounded border border-destructive/40 bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-destructive">
            removed on Save
          </span>
          <Button variant="ghost" size="sm" className="h-7" onClick={onRestore}>
            <Undo2Icon className="size-3.5" /> Restore
          </Button>
        </div>
      ) : (
        canManage && (
          <div className="flex shrink-0 gap-1">
            <Button variant="ghost" size="icon" className="size-7" onClick={onEdit} aria-label="Edit">
              <PencilIcon className="size-3.5" />
            </Button>
            <Button variant="ghost" size="icon" className="size-7" onClick={onDelete} aria-label="Delete">
              <Trash2Icon className="size-3.5" />
            </Button>
          </div>
        )
      )}
    </div>
  )
}

function ValueEditRow({
  attributeId,
  initial,
  draft,
  isSwatch,
  onStage,
  onCancel,
  isNew,
}: {
  attributeId: string
  initial?: ValueRow
  // a previously staged (unsaved) edit of this row, taking precedence over the stored fields
  draft?: ValueInput
  isSwatch: boolean
  onStage: (input: ValueInput, pending: EmbeddedPending[]) => void
  onCancel: () => void
  isNew?: boolean
}) {
  const [name, setName] = useState(draft?.name ?? initial?.name ?? '')
  const [slug, setSlug] = useState(draft?.slug ?? initial?.slug ?? '')
  const [slugTouched, setSlugTouched] = useState(!isNew || !!draft)
  const [rawValue, setRawValue] = useState(draft?.value ?? initial?.value ?? '')
  const [sortOrder, setSortOrder] = useState(draft?.sortOrder ?? initial?.sortOrder ?? 0)
  const [isIndexable, setIsIndexable] = useState(draft?.isIndexable ?? initial?.isIndexable ?? false)

  // Embedded staged editors (translations, channel slugs) report their pending {dirty, flush} here; this row's Save
  // flushes them together with the row fields so the form has exactly one save action
  const translationsPending = useRef<EmbeddedPending | null>(null)
  const channelSlugsPending = useRef<EmbeddedPending | null>(null)

  function onNameChange(v: string) {
    setName(v)
    if (!slugTouched) setSlug(slugify(v))
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    // Nothing is written here - the staged input plus any embedded pendings (translations,
    // channel slugs) travel up and apply during the section save, through the header Save
    const pending: EmbeddedPending[] = []
    if (translationsPending.current) pending.push(translationsPending.current)
    if (channelSlugsPending.current) pending.push(channelSlugsPending.current)
    onStage({ slug, name, value: rawValue || undefined, sortOrder, isIndexable }, pending)
  }

  return (
    <form onSubmit={onSubmit} className="px-6 py-3 grid gap-3 bg-muted/30">
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-[1fr_1fr_1fr_80px]">
        <div className="grid gap-1.5">
          <Label className="text-xs">Name</Label>
          <Input
            className="h-8 text-sm"
            required
            placeholder="Red"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            autoFocus={isNew}
          />
        </div>
        <div className="grid gap-1.5">
          <Label className="text-xs">Slug</Label>
          <Input
            className="h-8 font-mono text-xs"
            required
            placeholder="red"
            value={slug}
            onChange={(e) => { setSlug(e.target.value); setSlugTouched(true) }}
            onBlur={() => setSlug((s) => slugify(s))}
            title={isNew
              ? undefined
              : 'Editing this changes URL filters like ?attr_color=<slug> - old links may stop matching.'}
          />
        </div>
        <div className="grid gap-1.5">
          <Label className="text-xs">{isSwatch ? 'Hex colour' : 'Value'}</Label>
          <div className="flex items-center gap-1.5">
            {isSwatch && rawValue && (
              <span className="size-7 shrink-0 rounded border" style={{ backgroundColor: rawValue }} />
            )}
            <Input
              className="h-8 font-mono text-xs"
              placeholder={isSwatch ? '#ff0000' : 'optional'}
              value={rawValue}
              onChange={(e) => setRawValue(e.target.value)}
            />
          </div>
        </div>
        <div className="grid gap-1.5">
          <Label className="text-xs">Order</Label>
          <Input
            className="h-8 text-sm"
            type="number"
            inputMode="numeric"
            value={sortOrder}
            onChange={(e) => setSortOrder(Number(e.target.value) || 0)}
          />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id={`val-indexable-${initial?.id ?? 'new'}`}
          checked={isIndexable}
          onCheckedChange={setIsIndexable}
        />
        <Label htmlFor={`val-indexable-${initial?.id ?? 'new'}`} className="text-xs">
          Indexable on the channel
          <span className="ml-2 font-normal text-muted-foreground">
            (renders facet link as &lt;a href&gt;, allows Google to index the
            filtered listing)
          </span>
        </Label>
      </div>

      {/* Description editor only when editing an existing value -
          new rows need the value id before the per-(value, locale)
          mutation can fire, and that id comes back from the initial
          AddAttributeValue call - Operator saves the row first, then
          comes back to add descriptions */}
      {initial && (
        <AttributeValueDescriptionEditor
          valueId={initial.id}
          attributeId={attributeId}
          descriptions={initial.descriptions ?? []}
          canManage
          onPendingChange={(p) => { translationsPending.current = p }}
        />
      )}

      {/* Channel slug overrides - same existing-row-only constraint as
          the description editor above - Staged; flushed by this row's
          Save together with the translation edits */}
      {initial && (
        <ValueChannelSlugEditor
          valueId={initial.id}
          globalSlug={slug}
          canManage
          onPendingChange={(p) => { channelSlugsPending.current = p }}
        />
      )}

      <div className="flex items-center gap-2">
        <Button type="submit" size="sm">
          <CheckIcon className="size-3.5" />
          {isNew ? 'Add' : 'Done'}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onCancel}>
          <XIcon className="size-3.5" />
          Cancel
        </Button>
        <span className="text-xs text-muted-foreground">Applied on Save, with the rest of the page.</span>
      </div>
    </form>
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

// ---------- Default value editor ----------

// Internal draft shape per input type:
//   PLAIN_TEXT / DATE / DATE_TIME: string
//   NUMERIC:                       string (parsed on save)
//   BOOLEAN:                       'true' | 'false'
//   DROPDOWN / SWATCH:             value-id string
//   MULTISELECT:                   newline-separated value-id strings
type DefaultDraft = string

const SERIALIZE_INVALID = Symbol('invalid-default-value')

function serializeDefault(inputType: import('@/graphql/schema.types').AttributeInputType, value: unknown): DefaultDraft {
  if (value == null) return inputType === 'BOOLEAN' ? 'false' : ''
  switch (inputType) {
    case 'BOOLEAN':
      return value === true ? 'true' : 'false'
    case 'NUMERIC':
      return typeof value === 'number' ? String(value) : ''
    case 'MULTISELECT':
      return Array.isArray(value) ? value.filter((v) => typeof v === 'string').join('\n') : ''
    default:
      return typeof value === 'string' ? value : ''
  }
}

function decodeDefault(
  inputType: import('@/graphql/schema.types').AttributeInputType,
  draft: DefaultDraft,
): unknown | typeof SERIALIZE_INVALID {
  switch (inputType) {
    case 'BOOLEAN':
      return draft === 'true'
    case 'NUMERIC': {
      const n = Number(draft)
      return Number.isFinite(n) ? n : SERIALIZE_INVALID
    }
    case 'MULTISELECT': {
      const ids = draft.split('\n').map((s) => s.trim()).filter(Boolean)
      return ids.length === 0 ? SERIALIZE_INVALID : ids
    }
    case 'PLAIN_TEXT':
    case 'DATE':
    case 'DATE_TIME':
    case 'DROPDOWN':
    case 'SWATCH':
      return draft.trim() ? draft.trim() : SERIALIZE_INVALID
    default:
      return SERIALIZE_INVALID
  }
}

function sameSerializedDefault(a: DefaultDraft, b: DefaultDraft): boolean {
  return a === b
}

function DefaultValueEditor({
  inputType,
  values,
  enabled,
  draft,
  onToggleEnabled,
  onDraftChange,
  disabled,
}: {
  inputType: import('@/graphql/schema.types').AttributeInputType
  values: Array<{ id: string; name: string; value: string }>
  enabled: boolean
  draft: DefaultDraft
  onToggleEnabled: (v: boolean) => void
  onDraftChange: (v: DefaultDraft) => void
  disabled?: boolean
}) {
  const selectedSet = (() => {
    if (inputType !== 'MULTISELECT') return new Set<string>()
    return new Set(draft.split('\n').map((s) => s.trim()).filter(Boolean))
  })()

  function toggleMulti(id: string) {
    const next = new Set(selectedSet)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onDraftChange(Array.from(next).join('\n'))
  }

  return (
    <div className="grid gap-3 rounded-md border p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="grid gap-0.5">
          <Label htmlFor="s-default-toggle" className="text-sm">Default value</Label>
          <p className="text-xs text-muted-foreground">
            Auto-applied to new products of types that include this attribute. Admins still override per
            product on the product detail page.
          </p>
        </div>
        <Switch
          id="s-default-toggle"
          checked={enabled}
          onCheckedChange={onToggleEnabled}
          disabled={disabled}
        />
      </div>

      {enabled && (
        <div className="grid gap-2">
          {inputType === 'BOOLEAN' && (
            <div className="flex items-center gap-3">
              <Switch
                id="s-default-bool"
                checked={draft === 'true'}
                onCheckedChange={(v) => onDraftChange(v ? 'true' : 'false')}
                disabled={disabled}
              />
              <Label htmlFor="s-default-bool" className="text-sm">
                {draft === 'true' ? 'True' : 'False'}
              </Label>
            </div>
          )}
          {(inputType === 'PLAIN_TEXT' || inputType === 'NUMERIC' || inputType === 'DATE' || inputType === 'DATE_TIME') && (
            <Input
              id="s-default-scalar"
              type={
                inputType === 'NUMERIC' ? 'number'
                  : inputType === 'DATE' ? 'date'
                    : inputType === 'DATE_TIME' ? 'datetime-local'
                      : 'text'
              }
              step={inputType === 'NUMERIC' ? 'any' : undefined}
              value={draft}
              onChange={(e) => onDraftChange(e.target.value)}
              disabled={disabled}
            />
          )}
          {(inputType === 'DROPDOWN' || inputType === 'SWATCH') && (
            <select
              className="h-9 rounded-md border bg-background px-3 text-sm"
              value={draft}
              onChange={(e) => onDraftChange(e.target.value)}
              disabled={disabled}
            >
              <option value="">- Pick a value -</option>
              {values.map((v) => (
                <option key={v.id} value={v.id}>{v.name || v.value}</option>
              ))}
            </select>
          )}
          {inputType === 'MULTISELECT' && (
            <div className="grid gap-1.5">
              {values.length === 0 ? (
                <p className="text-xs text-muted-foreground">Add values below before setting a default.</p>
              ) : (
                values.map((v) => {
                  const checked = selectedSet.has(v.id)
                  return (
                    <label key={v.id} className="flex cursor-pointer items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => toggleMulti(v.id)}
                        disabled={disabled}
                      />
                      <span>{v.name || v.value}</span>
                    </label>
                  )
                })
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
