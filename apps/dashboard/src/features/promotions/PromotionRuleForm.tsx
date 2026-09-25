import { useMemo, useRef, useState, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useStore } from '@tanstack/react-form'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { fieldErrorText } from '@/components/form/field-errors'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext } from '@/shared/saving/saveRegistry'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { RULE_LABELS, contentChange, ruleDiff, ruleSchema, toRuleInput, toRuleValues, type RuleRow, type RuleValues } from './schemas/rule'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ChevronDownIcon,
  ChevronUpIcon,
  CircleCheckIcon,
  CirclePauseIcon,
  LayersIcon,
  Loader2Icon,
  OctagonAlertIcon,
  PencilIcon,
  PlusIcon,
  ShieldIcon,
  TagIcon,
  Trash2Icon,
  ZapIcon,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  DetailPageHeader,
  DetailPageBody,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { ColorPicker } from '@/components/ColorPicker'
import { MediaPicker } from '@/components/MediaPicker'
import type { ColumnDef } from '@/components/data-table/tanstack'

import { TranslationLocales } from '@/components/TranslationLocales'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  DeletePromotionRuleContentDocument,
  UpsertPromotionRuleContentDocument,
} from './Promotions.gql'
import { CodeGeneratorDialog } from './CodeGeneratorDialog'
import { SchemaForm, type JSONSchema } from './SchemaForm'
import {
  ACTION_CATEGORY_ORDER,
  CATEGORIES,
  CONDITION_CATEGORY_ORDER,
  categorize,
  summariseType,
  type CategoryKey,
  type CategoryMeta,
} from './typeCategory'
import { PluginSections } from '@/plugins/detail-sections'
import { FORM_WIDGETS } from './widgets/registry'
import {
  ACTION_TYPES_QUERY,
  CONDITION_TYPES_QUERY,
  CREATE_PROMOTION_RULE,
  DELETE_PROMOTION_CODE,
  PROMOTION_RULE_QUERY,
  UPDATE_PROMOTION_CODE,
  UPDATE_PROMOTION_RULE,
} from './ruleDocuments'

// ---- Types ----

type PromotionStopMode = 'NONE' | 'SAME_ACTION_TYPE' | 'ALL'

// slugify turns "Free Shipping (DACH)!" into "free-shipping-dach" - Used by the create flow to derive a slug from the name as the operator
// types, so they don't have to repeat themselves - Edit pages keep the existing slug - URL stability matters more than re-derivation
function slugify(s: string): string {
  return s
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

type ConditionRow = {
  type: string
  // Decoded params: SchemaForm reads/writes the JSON object directly
  params: unknown
  paramsVersion?: number
  sortOrder?: number
}


// Per-locale storefront copy for the rule (label + terms)
type ContentRow = {
  locale: string
  label: string
  terms: string
  updatedAt: string
}

type CodeRow = {
  id: string
  code: string
  maxUses: number | null
  perCustomerLimit: number | null
  usedCount: number
  isActive: boolean
}

type TypeDescriptor = {
  slug: string
  label: string
  description: string
  paramsSchema: unknown
}

// ---- Page wrappers ----

export function PromotionRuleCreatePage() {
  return <PromotionRuleEditor mode="create" />
}

export function PromotionRuleEditPage({
  ruleId,
  analyticsSlot,
}: { ruleId?: string; analyticsSlot?: ReactNode } = {}) {
  const params = useParams<{ id?: string }>()
  const id = ruleId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery<{
    promotionRule: {
      id: string
      slug: string
      name: string
      description: string
      publicLabel: string
      channelId: string | null
      requiresCode: boolean
      priority: number
      stopAfterApply: PromotionStopMode
      exclusivityGroup: string
      validFrom: string | null
      validTo: string | null
      maxUses: number | null
      perCustomerLimit: number | null
      usedCount: number
      isActive: boolean
      labelMediaId: string | null
      bannerMediaId: string | null
      pillText: string
      pillTextColor: string
      pillBgColor: string
      contents: ContentRow[]
      conditions: Array<{ id: string; type: string; params: unknown; paramsVersion: number; sortOrder: number }>
      actions: Array<{ id: string; type: string; params: unknown; paramsVersion: number; sortOrder: number }>
      codes: { items: CodeRow[]; totalCount: number }
    } | null
  }>(PROMOTION_RULE_QUERY, { variables: { id }, skip: !id, fetchPolicy: 'network-only' })

  if (loading) {
    return (
      <div className="flex items-center gap-2 px-6 py-6 text-sm text-muted-foreground">
        <Loader2Icon className="size-4 animate-spin" /> Loading…
      </div>
    )
  }
  if (error) return <p className="px-6 py-6 text-sm text-destructive">{error.message}</p>
  if (!data?.promotionRule) return <p className="px-6 py-6 text-sm text-muted-foreground">Promotion not found.</p>
  return (
    <PromotionRuleEditor
      mode="edit"
      rule={data.promotionRule}
      onSaved={refetch}
      analyticsSlot={analyticsSlot}
    />
  )
}

// ---- Editor ----

type ExistingRule = Parameters<typeof toRuleValues>[0] & {
  id: string
  usedCount: number
  codes: { items: CodeRow[]; totalCount: number }
}

// The storefront copy fields are the header locale's, so the form is keyed on it
function PromotionRuleEditor(props: { mode: 'create' | 'edit'; rule?: ExistingRule; onSaved?: () => void; analyticsSlot?: ReactNode }) {
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  const contentLocale = effectiveLocale || globalDefaultLocale
  return <RuleEditor key={contentLocale} {...props} contentLocale={contentLocale} />
}

// The whole rule is one form behind the header Save: a new rule writes straight through, an existing
// one reviews its changes in the confirm dialog first. Codes are rows with their own actions
function RuleEditor({
  mode,
  rule,
  onSaved,
  analyticsSlot,
  contentLocale,
}: {
  mode: 'create' | 'edit'
  rule?: ExistingRule
  onSaved?: () => void
  analyticsSlot?: ReactNode
  contentLocale: string
}) {
  const isEdit = mode === 'edit' && !!rule
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('promotions.manage')

  // Type catalogs for the condition / action pickers
  const { data: condTypeData } = useQuery<{ promotionConditionTypes: TypeDescriptor[] }>(CONDITION_TYPES_QUERY)
  const { data: actTypeData } = useQuery<{ promotionActionTypes: TypeDescriptor[] }>(ACTION_TYPES_QUERY)
  const conditionTypes = useMemo(() => condTypeData?.promotionConditionTypes ?? [], [condTypeData])
  const actionTypes = useMemo(() => actTypeData?.promotionActionTypes ?? [], [actTypeData])

  const [createRule] = useMutation(CREATE_PROMOTION_RULE)
  const [updateRule] = useMutation(UPDATE_PROMOTION_RULE)
  const [upsertContent] = useMutation(UpsertPromotionRuleContentDocument)
  const [deleteContent] = useMutation(DeletePromotionRuleContentDocument, {
    refetchQueries: rule ? [{ query: PROMOTION_RULE_QUERY, variables: { id: rule.id } }] : [],
    awaitRefetchQueries: true,
  })
  // UI state, not the admin's work - an existing slug is never re-derived from the name, for URL stability
  const [slugTouched, setSlugTouched] = useState(isEdit)
  const createdId = useRef('')

  const baseline = useMemo(() => toRuleValues(rule, contentLocale), [rule, contentLocale])
  const record = useRecordForm({
    id: 'promotion',
    title: 'Promotion',
    label: isEdit ? `Promotion · ${rule.name}` : 'New promotion',
    isEdit,
    baseline,
    schema: ruleSchema,
    diff: ruleDiff,
    persist: { routeKey: `promotion:${rule?.id ?? 'new'}:${contentLocale}` },
    submit: async (v) => {
      if (!rule) {
        const res = await createRule({ variables: { input: toRuleInput(v) } })
        const created = (res.data as { createPromotionRule: { id: string } } | undefined)?.createPromotionRule
        if (!created) throw new Error('Create succeeded but no promotion id was returned.')
        createdId.current = created.id
        return
      }
      await updateRule({ variables: { input: { id: rule.id, ...toRuleInput(v) } } })
      const content = contentChange(baseline, v)
      if (contentLocale && content) await upsertContent({ variables: { input: { ruleId: rule.id, locale: contentLocale, ...content } } })
    },
    onSaved: isEdit ? onSaved : () => navigate(`/promotions/${createdId.current}`),
  })
  const { form, registry, confirm, draft, submitError } = record
  const values = useStore(form.store, (s) => s.values)
  const submitting = useStore(form.store, (s) => s.isSubmitting)

  const setRows = (field: 'conditions' | 'actions', update: (rows: RuleRow[]) => RuleRow[]) => form.setFieldValue(field, update)
  const contents = rule?.contents ?? []

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageHeader
        backTo="/promotions"
        backLabel="Back to promotions"
        title={rule?.name ?? 'New promotion'}
        actions={
          canManage && (
            <Button type="button" size="sm" onClick={record.save} disabled={submitting || (isEdit && !registry.isDirty)}>
              {submitting ? <Loader2Icon className="size-4 animate-spin" /> : null}
              {isEdit ? 'Save changes' : 'Create'}
            </Button>
          )
        }
      />
      <DetailPageBody>
        {analyticsSlot}
        {draft.restoredAt !== null && (
          <div className="px-6 py-4">
            <RestoredDraftBanner draft={draft} labels={RULE_LABELS} />
          </div>
        )}
        {submitError ? (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Create failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          </div>
        ) : null}

        <fieldset disabled={!canManage} className="contents">
          <DetailPageSection title="General" description="Identity and lifecycle.">
            {/* The two most operationally-impactful flags - does the customer need a code, is the rule
                live - sit above the identity block, where they're easy to find on a fresh form */}
            <div className="grid gap-3 md:grid-cols-2">
              <ToggleCard
                icon={values.requiresCode ? <TagIcon className="size-5" /> : <ZapIcon className="size-5" />}
                title={values.requiresCode ? 'Customer enters a code' : 'Applies automatically'}
                description={
                  values.requiresCode
                    ? 'Customer must type a coupon code on the cart for this rule to fire.'
                    : 'Engine applies this rule whenever its conditions match - no code needed.'
                }
                checked={values.requiresCode}
                onCheckedChange={(v) => form.setFieldValue('requiresCode', v)}
                id="requiresCode"
              />
              <ToggleCard
                icon={values.isActive ? <CircleCheckIcon className="size-5" /> : <CirclePauseIcon className="size-5" />}
                title={values.isActive ? 'Live' : 'Paused'}
                description={
                  values.isActive ? 'Rule is currently evaluated against carts.' : 'Rule is saved but the engine ignores it. Toggle on when ready.'
                }
                checked={values.isActive}
                onCheckedChange={(v) => form.setFieldValue('isActive', v)}
                id="isActive"
                tone={values.isActive ? 'emerald' : 'muted'}
              />
            </div>

            {/* Identity - what humans read in the dashboard, and the stable identifier URLs and the engine use */}
            <div className="grid items-start gap-3 md:grid-cols-2">
              <form.AppField
                name="name"
                listeners={{
                  onChange: ({ value }) => {
                    if (!slugTouched) form.setFieldValue('slug', slugify(value))
                  },
                }}
              >
                {(f) => (
                  <f.TextField id="name" label="Name" placeholder="e.g. 10% off first order" hint="Human-readable label shown on the rule list and dashboard." />
                )}
              </form.AppField>
              <form.AppField name="slug">
                {(f) => (
                  <f.TextField
                    id="slug"
                    label="Slug"
                    placeholder="auto-derived from name"
                    onValueChange={() => setSlugTouched(true)}
                    hint="Stable identifier for URLs, logs, and the engine's audit trail. Auto-syncs from the name until you edit it."
                  />
                )}
              </form.AppField>
              <form.AppField name="description">
                {(f) => (
                  <f.TextareaField
                    id="description"
                    label="Description (internal)"
                    rows={2}
                    className="md:col-span-2"
                    placeholder="Why this rule exists - for your team, not the customer."
                  />
                )}
              </form.AppField>
              <form.AppField name="publicLabel">
                {(f) => (
                  <f.TextField
                    id="publicLabel"
                    label="Public label (visible to customer)"
                    className="md:col-span-2"
                    placeholder="e.g. 10% off your first order"
                  />
                )}
              </form.AppField>
            </div>
          </DetailPageSection>

          {isEdit ? (
            <DetailPageSection
              title="Storefront translation"
              description="Per-locale label and terms shown to shoppers. Locales without a row fall back to the public label above."
            >
              <div className="grid gap-4 md:max-w-2xl">
                <form.AppField name="contentLabel">
                  {(f) => (
                    <f.TextField
                      id="contentLabel"
                      label={`Label (${contentLocale || 'no locale selected'})`}
                      placeholder={values.publicLabel || 'e.g. 10% off your first order'}
                      disabled={!contentLocale}
                    />
                  )}
                </form.AppField>
                <form.AppField name="contentTerms">
                  {(f) => (
                    <f.TextareaField
                      id="contentTerms"
                      label="Terms (optional)"
                      rows={2}
                      placeholder="Small print shown under the offer."
                      disabled={!contentLocale}
                    />
                  )}
                </form.AppField>
                <TranslationLocales
                  locale={contentLocale}
                  locales={contents.map((c) => c.locale)}
                  entityLabel="promotion"
                  canManage={canManage}
                  onRemove={async () => {
                    await deleteContent({ variables: { ruleId: rule.id, locale: contentLocale } })
                  }}
                />
              </div>
            </DetailPageSection>
          ) : null}

          <DetailPageSection
            title="Promotional creative"
            description="Optional imagery and badges advertising this rule across the channel. Targeting is auto-derived from the rule's product-level conditions below."
          >
            <div className="grid gap-4 md:grid-cols-2">
              <MediaPicker
                value={values.labelMediaId || null}
                onChange={(id) => form.setFieldValue('labelMediaId', id ?? '')}
                label="Label image (PLP corner sticker)"
                description="Overlaid on product listing cards. Use a transparent PNG / SVG so it sits cleanly over the product photo."
                mimeGroup="IMAGE"
              />
              <MediaPicker
                value={values.bannerMediaId || null}
                onChange={(id) => form.setFieldValue('bannerMediaId', id ?? '')}
                label="Banner image (PDP hero strip)"
                description="Rendered between the gallery and the price on product detail pages. Landscape orientation works best."
                mimeGroup="IMAGE"
                previewClassName="size-24 w-40"
              />
            </div>

            {/* Pill badge - a text alternative to the label image; channels use it when the rule has no
                label media, so an operator picks image OR text */}
            <div className="grid gap-3 rounded border bg-card p-4">
              <div className="grid gap-1">
                <p className="text-sm font-medium">Pill badge (text fallback)</p>
                <p className="text-xs text-muted-foreground">
                  Stylable text badge shown on PLP cards when there is no label image. Short copy works best - "10% OFF", "NEW", "MEMBERS ONLY". Leave
                  blank to skip.
                </p>
              </div>
              <form.AppField name="pillText">{(f) => <f.TextField id="pillText" label="Pill text" placeholder="e.g. 10% OFF" maxLength={40} />}</form.AppField>
              <div className="grid gap-3 md:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label>Text color</Label>
                  <ColorPicker value={values.pillTextColor} onChange={(v) => form.setFieldValue('pillTextColor', v || '#ffffff')} ariaLabel="Pill text color" />
                </div>
                <div className="grid gap-1.5">
                  <Label>Background color</Label>
                  <ColorPicker value={values.pillBgColor} onChange={(v) => form.setFieldValue('pillBgColor', v || '#dc2626')} ariaLabel="Pill background color" />
                </div>
              </div>
              {values.pillText.trim() ? (
                <div className="flex items-center gap-3 pt-1">
                  <span className="text-xs text-muted-foreground">Preview:</span>
                  <span
                    className="inline-flex items-center rounded-full px-3 py-1 text-xs font-semibold uppercase tracking-wide"
                    style={{ backgroundColor: values.pillBgColor, color: values.pillTextColor }}
                  >
                    {values.pillText}
                  </span>
                </div>
              ) : null}
            </div>
          </DetailPageSection>

          <DetailPageSection title="Stacking & priority" description="Where this rule sits in the evaluation chain.">
            <div className="grid items-start gap-3 md:grid-cols-2">
              <form.AppField name="priority">
                {(f) => (
                  <f.NumberField
                    id="priority"
                    label="Priority"
                    hint="Lower = runs first. Drag rules on the list page to reorder visually; this number is auto-assigned in increments of 10."
                  />
                )}
              </form.AppField>
              <form.AppField name="exclusivityGroup">
                {(f) => (
                  <f.TextField
                    id="exclusivityGroup"
                    label="Exclusivity group"
                    placeholder="e.g. sale-event-q4 (leave empty to always stack)"
                    hint="Rules sharing a group are mutually exclusive - only one fires."
                  />
                )}
              </form.AppField>
              <div className="grid gap-1.5 md:col-span-2">
                <Label>When this rule fires…</Label>
                <StopModeRadio value={values.stopAfterApply} onChange={(v) => form.setFieldValue('stopAfterApply', v)} />
              </div>
            </div>
          </DetailPageSection>

          <DetailPageSection
            title="When this rule fires"
            description="All conditions below must match. Use Combine (AND/OR/NOT) for boolean logic between groups."
          >
            <SubrowList
              kind="condition"
              rows={values.conditions}
              types={conditionTypes}
              onMove={(from, to) => setRows('conditions', (rows) => moveItem(rows, from, to))}
              onChange={(idx, next) => setRows('conditions', (rows) => rows.map((r, i) => (i === idx ? next : r)))}
              onRemove={(idx) => setRows('conditions', (rows) => rows.filter((_, i) => i !== idx))}
              onAdd={() => setRows('conditions', (rows) => [...rows, { type: '', params: {} }])}
            />
            <RowErrors form={form} name="conditions" count={values.conditions.length} />
          </DetailPageSection>

          <DetailPageSection title="Then do this" description="Actions run top-to-bottom when the rule fires.">
            <SubrowList
              kind="action"
              rows={values.actions}
              types={actionTypes}
              onMove={(from, to) => setRows('actions', (rows) => moveItem(rows, from, to))}
              onChange={(idx, next) => setRows('actions', (rows) => rows.map((r, i) => (i === idx ? next : r)))}
              onRemove={(idx) => setRows('actions', (rows) => rows.filter((_, i) => i !== idx))}
              onAdd={() => setRows('actions', (rows) => [...rows, { type: '', params: {} }])}
            />
            <RowErrors form={form} name="actions" count={values.actions.length} />
          </DetailPageSection>

          <DetailPageSection title="Validity & limits" description="Time window + usage caps. Per-code overrides are set on each code below.">
            <div className="grid items-start gap-3 md:grid-cols-2">
              <form.AppField name="validFrom">{(f) => <f.TextField id="validFrom" label="Valid from" type="datetime-local" />}</form.AppField>
              <form.AppField name="validTo">{(f) => <f.TextField id="validTo" label="Valid to" type="datetime-local" />}</form.AppField>
              <form.AppField name="maxUses">
                {(f) => <f.TextField id="maxUses" label="Max uses (aggregate, across all codes)" type="number" placeholder="unlimited" />}
              </form.AppField>
              <form.AppField name="perCustomerLimit">
                {(f) => <f.TextField id="perCustomerLimit" label="Per-customer limit" type="number" placeholder="unlimited" />}
              </form.AppField>
            </div>
          </DetailPageSection>
        </fieldset>

        {isEdit ? <CodesSection ruleId={rule.id} codes={rule.codes.items} onChanged={onSaved} /> : null}
        <PluginSections target="promotion" entity={rule ?? null} mode={isEdit ? 'edit' : 'create'} />
      </DetailPageBody>
      <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={record.label} sections={registry.dirtySections} />
    </SaveRegistryContext.Provider>
  )
}

// A row left without a type - each subrow's own error, listed under its list
function RowErrors({ form, name, count }: { form: ReturnType<typeof useRecordForm<RuleValues>>['form']; name: 'conditions' | 'actions'; count: number }) {
  return (
    <>
      {Array.from({ length: count }, (_, i) => (
        <form.Field key={i} name={`${name}[${i}].type`}>
          {(f) => {
            const text = fieldErrorText(f)
            return text ? <p className="text-xs text-destructive">{`Row ${i + 1}: ${text}`}</p> : null
          }}
        </form.Field>
      ))}
    </>
  )
}

// ---- Subrow list + components ----

// moveItem returns a copy of `arr` with the element at `from` moved to `to` - Used by the up/down arrow controls on
// each subrow so the admin can reorder conditions / actions without typing sort_order numbers
function moveItem<T>(arr: T[], from: number, to: number): T[] {
  if (from === to || from < 0 || to < 0 || from >= arr.length || to >= arr.length) return arr
  const copy = arr.slice()
  const [item] = copy.splice(from, 1)
  copy.splice(to, 0, item)
  return copy
}

// SubrowList renders the conditions / actions section body: a list of categorised subrows with visual connectors ("AND"
// between conditions, numbered steps between actions) plus an empty state and a single "Add" button at the bottom
function SubrowList({
  kind,
  rows,
  types,
  onChange,
  onRemove,
  onMove,
  onAdd,
}: {
  kind: 'condition' | 'action'
  rows: ConditionRow[]
  types: TypeDescriptor[]
  onChange: (idx: number, next: ConditionRow) => void
  onRemove: (idx: number) => void
  onMove: (from: number, to: number) => void
  onAdd: () => void
}) {
  if (rows.length === 0) {
    return (
      <div className="grid gap-3">
        <EmptyState kind={kind} onAdd={onAdd} />
      </div>
    )
  }
  return (
    <div className="grid gap-2">
      {rows.map((row, idx) => (
        <div key={idx} className="grid gap-2">
          {idx > 0 ? <Connector kind={kind} index={idx} /> : null}
          <RuleSubrow
            kind={kind}
            row={row}
            total={rows.length}
            types={types}
            onChange={(next) => onChange(idx, next)}
            onRemove={() => onRemove(idx)}
            onMoveUp={idx > 0 ? () => onMove(idx, idx - 1) : undefined}
            onMoveDown={idx < rows.length - 1 ? () => onMove(idx, idx + 1) : undefined}
          />
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={onAdd}
        className="w-fit"
      >
        <PlusIcon className="size-4" /> Add {kind}
      </Button>
    </div>
  )
}

function Connector({ kind, index }: { kind: 'condition' | 'action'; index: number }) {
  return (
    <div className="flex items-center gap-2 pl-3 text-[10px] uppercase tracking-widest text-muted-foreground">
      <div className="h-px flex-1 bg-border" />
      <span className="rounded bg-muted px-1.5 py-0.5">
        {kind === 'condition' ? 'AND' : `Step ${index + 1}`}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}

function EmptyState({
  kind,
  onAdd,
}: {
  kind: 'condition' | 'action'
  onAdd: () => void
}) {
  return (
    <div className="grid gap-3 rounded border border-dashed p-6 text-center">
      <p className="text-sm font-medium">
        {kind === 'condition'
          ? 'No conditions - this rule will fire on every cart.'
          : 'No actions yet - pick what this rule should do.'}
      </p>
      <p className="text-xs text-muted-foreground">
        {kind === 'condition'
          ? 'Add a condition to restrict when this rule applies (subtotal range, customer group, products, time-of-day, …).'
          : 'Examples: % off, fixed amount off, free shipping, free gift, BOGO.'}
      </p>
      <div>
        <Button type="button" variant="default" size="sm" onClick={onAdd}>
          <PlusIcon className="size-4" /> Add {kind}
        </Button>
      </div>
    </div>
  )
}

// RuleSubrow is a single condition / action card - Two modes:
//   - Unpicked (no type chosen yet): shows a categorised picker
//   - Filled: shows a header strip (category icon + summary +
//     edit/remove + up/down) and the SchemaForm body for params
//
// The header is read-only ergonomics: changing TYPE after picking
// resets params (the new type's schema almost certainly doesn't
// accept the old fields), so the type lives behind an "Edit type"
// affordance that explicitly warns of the reset
function RuleSubrow({
  kind,
  row,
  total,
  types,
  onChange,
  onRemove,
  onMoveUp,
  onMoveDown,
}: {
  kind: 'condition' | 'action'
  row: ConditionRow
  total: number
  types: TypeDescriptor[]
  onChange: (next: ConditionRow) => void
  onRemove: () => void
  onMoveUp?: () => void
  onMoveDown?: () => void
}) {
  const current = types.find((t) => t.slug === row.type)
  const schema = current?.paramsSchema as JSONSchema | undefined
  const cat = row.type ? categorize(row.type, kind) : null
  const summary = current ? summariseType(row.type, current.label, row.params) : ''

  // Unpicked: the picker is the whole card - Once a type is selected,
  // the picker collapses into the header strip
  if (!row.type) {
    return (
      <TypePicker
        kind={kind}
        types={types}
        onPick={(slug) => onChange({ ...row, type: slug, params: {} })}
        onCancel={total > 0 ? onRemove : undefined}
      />
    )
  }

  const Icon = cat?.icon
  return (
    <div className="grid gap-3 rounded-lg border bg-card shadow-sm">
      <header className="flex flex-wrap items-center gap-2 border-b bg-muted/40 px-3 py-2">
        {Icon ? (
          <div className="grid size-7 shrink-0 place-items-center rounded bg-background text-muted-foreground">
            <Icon className="size-4" />
          </div>
        ) : null}
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{summary || current?.label || row.type}</p>
          <p className="truncate text-xs text-muted-foreground">
            {current?.label ?? row.type}
            {cat ? <> · {cat.label}</> : null}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {onMoveUp ? (
            <Button type="button" variant="ghost" size="icon" className="size-7" onClick={onMoveUp} aria-label="Move up">
              <ChevronUpIcon className="size-4" />
            </Button>
          ) : null}
          {onMoveDown ? (
            <Button type="button" variant="ghost" size="icon" className="size-7" onClick={onMoveDown} aria-label="Move down">
              <ChevronDownIcon className="size-4" />
            </Button>
          ) : null}
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={() => {
              if (window.confirm(`Change ${kind} type? This will clear the current parameters.`)) {
                onChange({ ...row, type: '', params: {} })
              }
            }}
            aria-label="Change type"
          >
            <PencilIcon className="size-4" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-7"
            onClick={onRemove}
            aria-label={`Remove ${kind}`}
          >
            <Trash2Icon className="size-4" />
          </Button>
        </div>
      </header>
      <div className="px-3 pb-3">
        {schema ? (
          <SchemaForm
            schema={schema}
            value={row.params ?? {}}
            onChange={(next) => onChange({ ...row, params: next })}
            widgets={FORM_WIDGETS}
          />
        ) : (
          <p className="text-xs text-muted-foreground">
            This type is unknown (plugin uninstalled?). Remove the row or pick
            a different type.
          </p>
        )}
      </div>
    </div>
  )
}

// TypePicker is the categorised dropdown shown when a subrow doesn't
// yet have a type - Groups every registered slug into the buckets
// defined in typeCategory.ts (Cart / Customer / Products / … for
// conditions; Cart discount / Line discount / Shipping / Gift / Cap
// for actions) so the operator scans by category rather than reading
// a flat list of 13+ slugs
//
// Empty value vs "select" placeholder: Select needs a non-empty value
// to display the placeholder, so the trigger button stays in the
// "Choose…" state until the user picks - Cancel removes the empty row
// entirely if the parent provides onCancel (which it does when at
// least one other row exists - the first empty row of a fresh
// section has nothing to cancel back to)
function TypePicker({
  kind,
  types,
  onPick,
  onCancel,
}: {
  kind: 'condition' | 'action'
  types: TypeDescriptor[]
  onPick: (slug: string) => void
  onCancel?: () => void
}) {
  // Group types by category - The category order is defined in typeCategory.ts so new built-ins / plugin slugs land in
  // the intended visual slot without changes here
  const order = kind === 'condition' ? CONDITION_CATEGORY_ORDER : ACTION_CATEGORY_ORDER
  const buckets = new Map<CategoryKey, { meta: CategoryMeta; items: TypeDescriptor[] }>()
  for (const cat of order) {
    buckets.set(cat, { meta: CATEGORIES[cat], items: [] })
  }
  for (const t of types) {
    const meta = categorize(t.slug, kind)
    const bucket = buckets.get(meta.key)
    if (bucket) bucket.items.push(t)
  }

  return (
    <div className="grid gap-3 rounded-lg border border-dashed bg-muted/30 p-4">
      <div className="flex items-center gap-3">
        <Select value={undefined} onValueChange={(slug) => slug && onPick(slug)}>
          <SelectTrigger className="flex-1">
            <SelectValue placeholder={`Pick a ${kind} type…`} />
          </SelectTrigger>
          <SelectContent className="max-h-96">
            {order.map((catKey, catIdx) => {
              const bucket = buckets.get(catKey)
              if (!bucket || bucket.items.length === 0) return null
              const Icon = bucket.meta.icon
              return (
                <div key={catKey}>
                  {catIdx > 0 ? <SelectSeparator /> : null}
                  <SelectGroup>
                    <SelectLabel className="flex items-center gap-2">
                      <Icon className="size-3.5" />
                      {bucket.meta.label}
                    </SelectLabel>
                    {bucket.items.map((t) => (
                      <SelectItem key={t.slug} value={t.slug}>
                        <div className="flex flex-col">
                          <span>{t.label}</span>
                          {t.description ? (
                            <span className="text-xs text-muted-foreground">{t.description}</span>
                          ) : null}
                        </div>
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </div>
              )
            })}
          </SelectContent>
        </Select>
        {onCancel ? (
          <Button type="button" variant="ghost" size="icon" onClick={onCancel} aria-label="Cancel">
            <Trash2Icon className="size-4" />
          </Button>
        ) : null}
      </div>
    </div>
  )
}

// ---- Codes ----

function CodesSection({
  ruleId,
  codes,
  onChanged,
}: {
  ruleId: string
  codes: CodeRow[]
  onChanged?: () => void
}) {
  const [openGen, setOpenGen] = useState(false)
  const [deleteCode] = useMutation<{ deletePromotionCode: boolean }, { id: string }>(DELETE_PROMOTION_CODE)
  const [updateCode] = useMutation<
    { updatePromotionCode: { id: string; isActive: boolean } },
    { input: { id: string; isActive: boolean } }
  >(UPDATE_PROMOTION_CODE)
  // Codes use the shared DataTable so search / sort / pagination /
  // bulk-select feel identical to every other admin list (products,
  // orders, customers) - The column defs declare their own
  // DataTableColumnHeader for sortable headers, and the
  // facet-filter "Active" lets operators isolate enabled vs disabled
  // codes when a rule has many
  const columns = useMemo<ColumnDef<CodeRow>[]>(
    () => [
      {
        accessorKey: 'code',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Code" />,
        cell: ({ row }) => (
          <span className="font-mono text-sm">{row.original.code}</span>
        ),
      },
      {
        accessorKey: 'usedCount',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Usage" />,
        cell: ({ row }) => {
          const used = row.original.usedCount
          const max = row.original.maxUses
          return (
            <span className="font-mono text-xs tabular-nums">
              {used}
              {max != null ? ` / ${max}` : ''}
            </span>
          )
        },
      },
      {
        accessorKey: 'isActive',
        header: 'Active',
        cell: ({ row }) => (
          // Click to flip - disabling a code stops it being redeemable
          // without destroying its redemption history
          <button
            type="button"
            title={row.original.isActive ? 'Disable this code' : 'Enable this code'}
            onClick={async () => {
              await updateCode({
                variables: {
                  input: { id: row.original.id, isActive: !row.original.isActive },
                },
              })
              onChanged?.()
            }}
          >
            {row.original.isActive ? (
              <Badge variant="secondary">Active</Badge>
            ) : (
              <Badge variant="outline">Disabled</Badge>
            )}
          </button>
        ),
      },
      {
        id: 'actions',
        cell: ({ row }) => (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="size-8"
            onClick={async () => {
              if (!window.confirm(`Delete code ${row.original.code}?`)) return
              await deleteCode({ variables: { id: row.original.id } })
              onChanged?.()
            }}
            aria-label="Delete code"
          >
            <Trash2Icon className="size-4" />
          </Button>
        ),
      },
    ],
    [deleteCode, updateCode, onChanged],
  )

  return (
    <DetailPageSection
      title="Codes"
      description="Codes attached to this rule. Generate batches with the pattern grammar [A] letter, [N] digit, [*] alphanumeric."
    >
      <DataTable
        columns={columns}
        data={codes}
        getRowId={(row) => row.id}
        emptyState="No codes yet. Generate a batch to get started."
        searchPlaceholder="Search code…"
        initialSorting={[{ id: 'code', desc: false }]}
        filters={[
          {
            column: 'isActive',
            title: 'Status',
            options: [
              { label: 'Active', value: 'true' },
              { label: 'Disabled', value: 'false' },
            ],
          },
        ]}
        toolbarRightSlot={
          <div className="flex items-center gap-2">
            <Button type="button" size="sm" variant="outline" onClick={() => setOpenGen(true)}>
              <PlusIcon className="size-4" /> Generate codes
            </Button>
          </div>
        }
      />
      <CodeGeneratorDialog
        ruleId={ruleId}
        open={openGen}
        onOpenChange={setOpenGen}
        onGenerated={onChanged}
      />
    </DetailPageSection>
  )
}

// ToggleCard is the prominent on/off card used for the high-impact
// flags (Requires code / Active) at the top of the General section
// Renders as a horizontal card with an icon, contextual title +
// description that flips based on state, and a switch - Visually more
// substantial than a labelled Switch - these flags change rule
// behaviour, so they earn the real estate
function ToggleCard({
  icon,
  title,
  description,
  checked,
  onCheckedChange,
  id,
  tone,
}: {
  icon: React.ReactNode
  title: string
  description: string
  checked: boolean
  onCheckedChange: (next: boolean) => void
  id: string
  tone?: 'emerald' | 'muted'
}) {
  const toneClass =
    tone === 'emerald'
      ? 'border-emerald-500/40 bg-emerald-50/40 dark:bg-emerald-950/20'
      : tone === 'muted'
        ? 'border-muted-foreground/20 bg-muted/40'
        : checked
          ? 'border-foreground/20 bg-accent/30'
          : 'border-border'
  return (
    <label
      htmlFor={id}
      className={`flex cursor-pointer items-start gap-3 rounded-lg border p-4 transition hover:border-foreground/30 ${toneClass}`}
    >
      <div className="grid size-9 shrink-0 place-items-center rounded-md bg-background text-foreground">
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold">{title}</p>
        <p className="mt-0.5 text-xs text-muted-foreground">{description}</p>
      </div>
      <Switch
        id={id}
        checked={checked}
        onCheckedChange={onCheckedChange}
        className="mt-1 shrink-0"
      />
    </label>
  )
}

// StopModeRadio renders the three stacking modes as click-cards
// instead of a small Select - Each card spells out the implication so
// an operator picking "Halt lower priorities" understands they're
// disabling other promotions, not just slotting in a flag - Maps
// 1-to-1 with promotion.StopMode on the backend
function StopModeRadio({
  value,
  onChange,
}: {
  value: PromotionStopMode
  onChange: (next: PromotionStopMode) => void
}) {
  const options: Array<{
    value: PromotionStopMode
    title: string
    description: string
    icon: React.ReactNode
    selectedRing: string
  }> = [
    {
      value: 'NONE',
      title: 'Stack with others',
      description:
        'Every other promotion still fires. Default for most discounts.',
      icon: <LayersIcon className="size-5" />,
      selectedRing: 'ring-emerald-500/60 border-emerald-500/40',
    },
    {
      value: 'SAME_ACTION_TYPE',
      title: 'Block same action type',
      description:
        'Rules at strictly lower priority with the same action are skipped. Peers stay; unrelated actions - free shipping, gifts - pass through.',
      icon: <ShieldIcon className="size-5" />,
      selectedRing: 'ring-amber-500/60 border-amber-500/40',
    },
    {
      value: 'ALL',
      title: 'Halt lower priorities',
      description:
        'No rule at strictly lower priority fires after this. Peers at the same priority still fire - pair two unrelated overrides at the same priority.',
      icon: <OctagonAlertIcon className="size-5" />,
      selectedRing: 'ring-destructive/60 border-destructive/40',
    },
  ]
  return (
    <div className="grid gap-3 md:grid-cols-3">
      {options.map((opt) => {
        const selected = value === opt.value
        return (
          <button
            type="button"
            key={opt.value}
            onClick={() => onChange(opt.value)}
            aria-pressed={selected}
            className={`group relative flex flex-col gap-2 rounded-lg border p-4 text-left transition hover:border-foreground/40 ${
              selected
                ? `bg-accent/40 shadow-sm ring-2 ${opt.selectedRing}`
                : 'border-border'
            }`}
          >
            <div className="flex items-center justify-between gap-2">
              <div
                className={`grid size-10 place-items-center rounded-md bg-background text-foreground transition ${
                  selected ? '' : 'opacity-70 group-hover:opacity-100'
                }`}
              >
                {opt.icon}
              </div>
              {selected ? (
                <span className="text-[10px] uppercase tracking-widest text-muted-foreground">
                  Selected
                </span>
              ) : null}
            </div>
            <span className="text-sm font-semibold">{opt.title}</span>
            <span className="text-xs text-muted-foreground">{opt.description}</span>
          </button>
        )
      })}
    </div>
  )
}
