import { useMemo, useRef, useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
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
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { FieldError } from '@/components/form/fields'
import { DetailPageSection } from '@/components/layout/detail-page'
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
  type AttributeQuery,
} from './Catalog.gql'
import {
  AttributeDescriptionEditor,
  AttributeValueDescriptionEditor,
  type EmbeddedPending,
} from './AttributeDescriptionEditor'
import { ValueChannelSlugEditor } from './ValueChannelSlugEditor'
import { AttributeSearchTuningCard } from './AttributeSearchTuningCard'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { AttributeInputTypeBadge } from './AttributeInputTypeBadge'
import { INPUT_TYPES_WITH_VALUES } from './attributeInputTypes'
import { PluginSections } from '@/plugins/detail-sections'
import type { AttributeInputType } from '@/graphql/schema.types'
import {
  ATTRIBUTE_SETTINGS_LABELS,
  attributeSettingsDiff,
  attributeSettingsSchema,
  toAttributeSettingsInput,
  toAttributeSettingsValues,
  type AttributeSettingsValues,
} from './schemas/attributeSettings'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

type Attribute = NonNullable<AttributeQuery['attribute']>

export function AttributeDetail() {
  const { id = '' } = useParams()
  const attributeQuery = useQuery(AttributeDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const attribute = attributeQuery.data?.attribute

  if (attributeQuery.loading && !attribute) return <LoadingState label="Loading attribute…" />
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
  return <AttributeForm attribute={attribute} />
}

// The settings are the page's form; the public description, search tuning and value rows stage
// through their own sections into the same registry, so the one header Save reviews them all
function AttributeForm({ attribute }: { attribute: Attribute }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('catalog.manage')
  const id = attribute.id

  const refetch = [{ query: AttributeDocument, variables: { id } }, { query: AttributesDocument }]
  const [deleteAttribute, { loading: deleting }] = useMutation(DeleteAttributeDocument, {
    refetchQueries: [{ query: AttributesDocument }],
  })
  const [updateAttribute] = useMutation(UpdateAttributeDocument, { refetchQueries: refetch })
  const [addValue] = useMutation(AddAttributeValueDocument, { refetchQueries: refetch })
  const [updateValue] = useMutation(UpdateAttributeValueDocument, { refetchQueries: refetch })
  const [deleteValue] = useMutation(DeleteAttributeValueDocument, { refetchQueries: refetch })

  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const baseline = useMemo(() => toAttributeSettingsValues(attribute), [attribute])
  const schema = useMemo(() => attributeSettingsSchema(attribute.inputType), [attribute.inputType])
  const diff = useMemo(() => attributeSettingsDiff(attribute.inputType, attribute.values), [attribute.inputType, attribute.values])

  const record = useRecordForm({
    id: 'settings',
    title: 'Settings',
    label: `Attribute · ${attribute.name}`,
    isEdit: true,
    baseline,
    schema,
    diff,
    persist: { routeKey: `attribute:${id}` },
    submit: async (v) => {
      await updateAttribute({ variables: { input: toAttributeSettingsInput(baseline, v, attribute.inputType, id) } })
    },
  })

  async function onDeleteAttribute() {
    await deleteAttribute({ variables: { id } })
    navigate('/catalog/attributes', { replace: true })
  }

  const supportsValues = INPUT_TYPES_WITH_VALUES.has(attribute.inputType)
  const isSwatch = attribute.inputType === 'SWATCH'

  return (
    <RecordFormPage
      record={record}
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
        canManage && (
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
        )
      }
      createLabel="Save changes"
      labels={ATTRIBUTE_SETTINGS_LABELS}
      hideSave={!canManage}
    >
      <DetailPageSection title="Settings" description="Name, unit, and behaviour flags.">
        <SettingsFields form={record.form} attribute={attribute} canManage={canManage} />
      </DetailPageSection>

      <DetailPageSection
        title="Public description"
        description="Short locale-aware copy rendered next to the facet panel header. Sparse - translate only the locales you care about; the rest fall back to nothing."
      >
        <AttributeDescriptionEditor attributeId={id} descriptions={attribute.descriptions ?? []} canManage={canManage} />
      </DetailPageSection>

      <DetailPageSection
        title="Search tuning"
        description="How this attribute participates in the channel product search. Stored centrally - saving here updates this attribute's row in /search-engine's Product Search table."
      >
        <AttributeSearchTuningCard attributeSlug={attribute.slug} searchable={attribute.searchable} />
      </DetailPageSection>

      {supportsValues && (
        <DetailPageSection
          title="Values"
          description={isSwatch ? 'Swatch options - name shown to customers, value holds the hex colour.' : 'Pre-defined options for this attribute.'}
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

      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={setConfirmingDelete}
        title={<>Delete {attribute.name}?</>}
        description="Permanently removes this attribute, all its values, and its assignments to any product types."
        confirmLabel="Delete attribute"
        busyLabel="Deleting…"
        destructive
        onConfirm={onDeleteAttribute}
      />
    </RecordFormPage>
  )
}

// ---------- Settings ----------

type SettingsForm = ReturnType<typeof useRecordForm<AttributeSettingsValues>>['form']

function SettingsFields({ form, attribute, canManage }: { form: SettingsForm; attribute: Attribute; canManage: boolean }) {
  const filterable = useStore(form.store, (s) => s.values.filterable)
  const synapseTrack = useStore(form.store, (s) => s.values.synapseTrack)

  // The indexable master switch is an action, not a field: flipping it cascades to every value and
  // per-product slug override, so it gets its own confirm and writes the moment that's accepted
  const [cascadePending, setCascadePending] = useState<{ next: boolean } | null>(null)
  const [cascadeResult, setCascadeResult] = useState<{ values: number; products: number } | null>(null)
  const [cascadeIndexable, { loading: cascading }] = useMutation(SetAttributeIndexableDocument, {
    refetchQueries: [{ query: AttributeDocument, variables: { id: attribute.id } }, { query: AttributesDocument }],
  })

  const switchRow = (name: 'valueRequired' | 'filterable' | 'facetCollapsed' | 'searchable' | 'isPublic' | 'synapseTrack', id: string, label: string, description: string) => (
    <form.AppField name={name}>
      {(f) => <f.SwitchField id={id} label={label} description={description} bordered={false} disabled={!canManage} />}
    </form.AppField>
  )

  return (
    <fieldset disabled={!canManage} className="grid gap-4">
      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-[1fr_160px_100px]">
        <form.AppField name="name">{(f) => <f.TextField id="s-name" label="Name" />}</form.AppField>
        <form.AppField name="unit">{(f) => <f.TextField id="s-unit" label="Unit" placeholder="cm, kg…" />}</form.AppField>
        <form.AppField name="sortOrder">{(f) => <f.NumberField id="s-sort" label="Sort order" />}</form.AppField>
      </div>

      <div className="grid gap-3 rounded-md border p-4">
        {switchRow('valueRequired', 's-required', 'Value required', 'Products must provide a value for this attribute.')}
        {switchRow('filterable', 's-filterable', 'Filterable', 'Show in channel filter facets.')}
        {switchRow(
          'facetCollapsed',
          's-facet-collapsed',
          'Collapse facet by default',
          filterable ? 'Channels render this filter collapsed on first load. Users can still open it.' : 'Only applies when "Filterable" is on.',
        )}
        <div className="flex items-start justify-between gap-4">
          <div className="grid gap-0.5">
            <Label htmlFor="s-attr-indexable" className="text-sm">
              Indexable facet (master switch)
            </Label>
            <p className="text-xs text-muted-foreground">
              Toggling this cascades the flag to every value under this attribute (and every per-product slug override). You'll get a
              confirmation before anything changes.
            </p>
          </div>
          <Switch
            id="s-attr-indexable"
            checked={attribute.isIndexable}
            onCheckedChange={(next) => {
              setCascadeResult(null)
              setCascadePending({ next })
            }}
            disabled={!canManage || cascading}
          />
        </div>
        <form.Field name="h1Position">
          {(f) => (
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="s-h1-position" className="text-sm">
                  H1 position
                </Label>
                <p className="text-xs text-muted-foreground">
                  Where the value appears in the listing page H1 when indexable. BEFORE → "Olive / Apparel"; AFTER → "Apparel / Olive".
                </p>
              </div>
              <select
                id="s-h1-position"
                className="h-8 rounded-md border bg-background px-2 text-sm"
                value={f.state.value}
                onChange={(e) => f.handleChange(e.target.value as 'BEFORE' | 'AFTER')}
              >
                <option value="BEFORE">Before</option>
                <option value="AFTER">After</option>
              </select>
            </div>
          )}
        </form.Field>
        {switchRow('searchable', 's-searchable', 'Searchable', 'Include in full-text product search.')}
        {switchRow('isPublic', 's-visible', 'Visible to customers', 'Display on the product detail page.')}
        {switchRow(
          'synapseTrack',
          's-synapse-track',
          'Track in Synapse',
          "Record this attribute's values as a customer affinity when they filter by it or view products that carry it (e.g. tire size → 'they likely need this size'). Targetable from Segments.",
        )}
        <form.Field name="autoSegment">
          {(f) => (
            <div className="flex items-start justify-between gap-4">
              <div className="grid gap-0.5">
                <Label htmlFor="s-auto-segment" className="text-sm">
                  Auto-create segments
                </Label>
                <p className="text-xs text-muted-foreground">
                  Automatically build one customer segment per popular value (e.g. 'Tire size: 205/55R16'), refreshed on a schedule and
                  targetable by Pulse / promotions / Prism. Requires Track in Synapse.
                </p>
              </div>
              <Switch id="s-auto-segment" checked={synapseTrack && f.state.value} onCheckedChange={f.handleChange} disabled={!canManage || !synapseTrack} />
            </div>
          )}
        </form.Field>
      </div>

      <DefaultValueFields form={form} inputType={attribute.inputType} values={attribute.values} disabled={!canManage} />

      {cascadeResult && (
        <Alert>
          <AlertTitle>Cascade applied</AlertTitle>
          <AlertDescription>
            Updated indexable on {cascadeResult.values} value{cascadeResult.values === 1 ? '' : 's'} and {cascadeResult.products} product
            override{cascadeResult.products === 1 ? '' : 's'} under this attribute.
          </AlertDescription>
        </Alert>
      )}

      <ConfirmDialog
        open={!!cascadePending}
        onOpenChange={(v) => !v && setCascadePending(null)}
        title={cascadePending?.next ? 'Mark this entire facet indexable?' : 'Remove indexable from this entire facet?'}
        description={
          <>
            This will set <code className="font-mono">isIndexable = {String(cascadePending?.next)}</code> on{' '}
            <strong>{attribute.name}</strong> AND every value / per-product slug override under it. The change is one transaction;
            per-value flags can still be flipped individually afterwards.
          </>
        }
        confirmLabel={cascadePending?.next ? 'Yes, mark indexable' : 'Yes, remove'}
        busyLabel="Applying…"
        onConfirm={async () => {
          if (!cascadePending) return
          const res = await cascadeIndexable({ variables: { attributeId: attribute.id, isIndexable: cascadePending.next } })
          const data = res.data?.setAttributeIndexable
          if (data) setCascadeResult({ values: data.affectedValuesCount, products: data.affectedProductsCount })
        }}
      />
    </fieldset>
  )
}

// The default value, shaped by the input type - the draft is text for every type (a value id, or
// newline-joined ids for a multiselect) and the schema decodes it (schemas/attributeSettings)
function DefaultValueFields({
  form,
  inputType,
  values,
  disabled,
}: {
  form: SettingsForm
  inputType: AttributeInputType
  values: Array<{ id: string; name: string; value: string }>
  disabled: boolean
}) {
  const hasDefault = useStore(form.store, (s) => s.values.hasDefault)
  return (
    <div className="grid gap-3 rounded-md border p-4">
      <form.AppField name="hasDefault">
        {(f) => (
          <f.SwitchField
            id="s-default-toggle"
            label="Default value"
            description="Auto-applied to new products of types that include this attribute. Admins still override per product on the product detail page."
            bordered={false}
            disabled={disabled}
          />
        )}
      </form.AppField>

      {hasDefault && (
        <form.Field name="defaultDraft">
          {(f) => {
            const draft = f.state.value
            const selected = new Set(draft.split('\n').map((s) => s.trim()).filter(Boolean))
            return (
              <div className="grid gap-2">
                {inputType === 'BOOLEAN' && (
                  <div className="flex items-center gap-3">
                    <Switch id="s-default-bool" checked={draft === 'true'} onCheckedChange={(v) => f.handleChange(v ? 'true' : 'false')} />
                    <Label htmlFor="s-default-bool" className="text-sm">
                      {draft === 'true' ? 'True' : 'False'}
                    </Label>
                  </div>
                )}
                {(inputType === 'PLAIN_TEXT' || inputType === 'NUMERIC' || inputType === 'DATE' || inputType === 'DATE_TIME') && (
                  <Input
                    id="s-default-scalar"
                    aria-label="Default value"
                    type={inputType === 'NUMERIC' ? 'number' : inputType === 'DATE' ? 'date' : inputType === 'DATE_TIME' ? 'datetime-local' : 'text'}
                    step={inputType === 'NUMERIC' ? 'any' : undefined}
                    value={draft}
                    onChange={(e) => f.handleChange(e.target.value)}
                    onBlur={f.handleBlur}
                  />
                )}
                {(inputType === 'DROPDOWN' || inputType === 'SWATCH') && (
                  <select
                    aria-label="Default value"
                    className="h-9 rounded-md border bg-background px-3 text-sm"
                    value={draft}
                    onChange={(e) => f.handleChange(e.target.value)}
                  >
                    <option value="">- Pick a value -</option>
                    {values.map((v) => (
                      <option key={v.id} value={v.id}>
                        {v.name || v.value}
                      </option>
                    ))}
                  </select>
                )}
                {inputType === 'MULTISELECT' && (
                  <div className="grid gap-1.5">
                    {values.length === 0 ? (
                      <p className="text-xs text-muted-foreground">Add values below before setting a default.</p>
                    ) : (
                      values.map((v) => (
                        <label key={v.id} className="flex cursor-pointer items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            checked={selected.has(v.id)}
                            onChange={() => {
                              const next = new Set(selected)
                              if (next.has(v.id)) next.delete(v.id)
                              else next.add(v.id)
                              f.handleChange(Array.from(next).join('\n'))
                            }}
                          />
                          <span>{v.name || v.value}</span>
                        </label>
                      ))
                    )}
                  </div>
                )}
                <FieldError field={f} />
              </div>
            )
          }}
        </form.Field>
      )}
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
          className="shrink-0 rounded border border-success/40 bg-success-soft px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-success"
          title="Filtered listing renders as <a href> and is allowed to be indexed by Google"
        >
          Indexable
        </span>
      )}
      <span className="shrink-0 font-mono text-xs text-muted-foreground w-6 text-right">
        {value.sortOrder}
      </span>
      {pending && !deleted && (
        <span className="shrink-0 rounded border border-warning/40 bg-warning-soft px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wider text-warning">
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

