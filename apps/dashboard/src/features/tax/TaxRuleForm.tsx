import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { gql } from '@apollo/client'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { LoadError } from '@/shared/errors/LoadError'
import { SchemaForm, type JSONSchema } from '@/features/promotions/SchemaForm'
import { FORM_WIDGETS } from '@/features/promotions/widgets/registry'
import { PluginSections } from '@/plugins/detail-sections'

const TAX_RULE_QUERY = gql`
  query TaxRule($id: ID!) {
    taxRule(id: $id) {
      id
      channelId
      name
      priority
      exclusivityGroup
      sortOrder
      isActive
      conditions {
        type
        params
        paramsVersion
        sortOrder
      }
      effects {
        type
        params
        paramsVersion
        sortOrder
      }
    }
  }
`

const TYPE_QUERIES = gql`
  query TaxRuleTypes {
    taxConditionTypes {
      slug
      label
      description
      paramsSchema
    }
    taxEffectTypes {
      slug
      label
      description
      paramsSchema
    }
  }
`

const CREATE_TAX_RULE = gql`
  mutation CreateTaxRule($input: CreateTaxRuleInput!) {
    createTaxRule(input: $input) {
      id
    }
  }
`

const UPDATE_TAX_RULE = gql`
  mutation UpdateTaxRule($input: UpdateTaxRuleInput!) {
    updateTaxRule(input: $input) {
      id
    }
  }
`

type TypeDescriptor = {
  slug: string
  label: string
  description: string
  paramsSchema: JSONSchema
}

type RowKind = { type: string; params: unknown; paramsVersion?: number }

type TaxRule = {
  id: string
  channelId: string
  name: string
  priority: number
  exclusivityGroup: string
  sortOrder: number
  isActive: boolean
  conditions: RowKind[]
  effects: RowKind[]
}

export function TaxRuleCreatePage() {
  const [params] = useSearchParams()
  const channelId = params.get('channelId') ?? ''
  return <TaxRuleFormPage mode="create" defaultChannelId={channelId} />
}

export function TaxRuleEditPage({ taxRuleId }: { taxRuleId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = taxRuleId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery<{ taxRule: TaxRule | null }, { id: string }>(
    TAX_RULE_QUERY,
    { variables: { id }, skip: !id },
  )
  if (loading) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading rule…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError
          title="Couldn't load tax rule"
          message={error.message}
          onRetry={() => void refetch()}
          retrying={loading}
        />
      </div>
    )
  }
  if (!data?.taxRule) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Rule not found.</p>
  }
  return <TaxRuleFormPage mode="edit" rule={data.taxRule} />
}

function TaxRuleFormPage({
  mode,
  rule,
  defaultChannelId,
}: {
  mode: 'create' | 'edit'
  rule?: TaxRule
  defaultChannelId?: string
}) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const { data: typeData } = useQuery<{
    taxConditionTypes: TypeDescriptor[]
    taxEffectTypes: TypeDescriptor[]
  }>(TYPE_QUERIES)
  const conditionTypes = useMemo(() => typeData?.taxConditionTypes ?? [], [typeData])
  const effectTypes = useMemo(() => typeData?.taxEffectTypes ?? [], [typeData])

  const [channelId] = useState(rule?.channelId ?? defaultChannelId ?? '')
  const [name, setName] = useState(rule?.name ?? '')
  const [priority, setPriority] = useState(rule?.priority ?? 0)
  const [exclusivityGroup, setExclusivityGroup] = useState(rule?.exclusivityGroup ?? '')
  const [sortOrder, setSortOrder] = useState(rule?.sortOrder ?? 0)
  const [isActive, setIsActive] = useState(rule?.isActive ?? true)
  const [conditions, setConditions] = useState<RowKind[]>(
    rule?.conditions.map((c) => ({ type: c.type, params: c.params ?? {} })) ?? [],
  )
  const [effects, setEffects] = useState<RowKind[]>(
    rule?.effects.map((e) => ({ type: e.type, params: e.params ?? {} })) ?? [],
  )
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const [createRule, { loading: creating }] = useMutation(CREATE_TAX_RULE)
  const [updateRule, { loading: updating }] = useMutation(UPDATE_TAX_RULE)
  const loading = creating || updating

  // Conditions and effects are rows, so they diff as a count plus their serialized types rather
  // than field-by-field - enough for the reviewer to see that the rule's logic changed
  const draft = {
    name, priority, exclusivityGroup, sortOrder, isActive,
    conditions: conditions.map((c) => c.type).join(', '),
    effects: effects.map((e) => e.type).join(', '),
  }
  const original = {
    name: rule?.name ?? '',
    priority: rule?.priority ?? 0,
    exclusivityGroup: rule?.exclusivityGroup ?? '',
    sortOrder: rule?.sortOrder ?? 0,
    isActive: rule?.isActive ?? true,
    conditions: (rule?.conditions ?? []).map((c) => c.type).join(', '),
    effects: (rule?.effects ?? []).map((e) => e.type).join(', '),
  }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const ruleDiff = useMemo(
    () =>
      isEdit
        ? fieldDiff(original, draft, [
            { label: 'Name', value: (o) => o.name },
            { label: 'Priority', value: (o) => o.priority },
            { label: 'Exclusivity group', value: (o) => o.exclusivityGroup },
            { label: 'Sort order', value: (o) => o.sortOrder },
            { label: 'Active', value: (o) => o.isActive },
            { label: 'Conditions', value: (o) => o.conditions },
            { label: 'Effects', value: (o) => o.effects },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isEdit, originalKey, draftKey],
  )

  useSaveSection({ id: 'tax-rule', title: 'Tax rule', diff: ruleDiff, save: () => applyChanges() })

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

  async function applyChanges() {
    setSubmitError(null)
    const conditionsInput = conditions.map((c, i) => ({
      type: c.type,
      params: c.params ?? {},
      sortOrder: i,
    }))
    const effectsInput = effects.map((eff, i) => ({
      type: eff.type,
      params: eff.params ?? {},
      sortOrder: i,
    }))
    try {
      if (isEdit && rule) {
        await updateRule({
          variables: {
            input: { id: rule.id, name, priority, exclusivityGroup, sortOrder, isActive, conditions: conditionsInput, effects: effectsInput },
          },
        })
      } else {
        if (!channelId) {
          setSubmitError('channelId missing - open from the tax-rules list page.')
          return
        }
        await createRule({
          variables: {
            input: { channelId, name, priority, exclusivityGroup, sortOrder, isActive, conditions: conditionsInput, effects: effectsInput },
          },
        })
      }
      navigate('/tax')
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Save failed.')
      throw err // the dialog keeps itself open and surfaces the message
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/tax"
        backLabel="Tax rules"
        title={isEdit ? rule?.name || '(unnamed)' : 'New tax rule'}
        meta="Conditions must all match; effects apply in order. An exclusivity group makes the most specific rule win."
        actions={
          <>
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to="/tax">Cancel</Link>
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={loading || (isEdit && !isDirty)}
              onClick={() => (isEdit ? setConfirmingSave(true) : void applyChanges())}
            >
              {loading ? 'Saving…' : isEdit ? 'Save changes' : 'Create rule'}
            </Button>
          </>
        }
      />

      <div>
        <DetailPageBody>
          <DetailPageSection title="Rule">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="name">Display name</Label>
                <Input id="name" value={name} onChange={(e) => setName(e.target.value)} placeholder="California sales tax" required />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="group">Exclusivity group</Label>
                <Input
                  id="group"
                  value={exclusivityGroup}
                  onChange={(e) => setExclusivityGroup(e.target.value)}
                  placeholder="jurisdiction (empty = always stacks)"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="priority">Priority (lower = earlier)</Label>
                <Input id="priority" type="number" inputMode="numeric" value={priority} onChange={(e) => setPriority(Number(e.target.value) || 0)} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="sort">Sort order</Label>
                <Input id="sort" type="number" inputMode="numeric" value={sortOrder} onChange={(e) => setSortOrder(Number(e.target.value) || 0)} />
              </div>
            </div>
            <div className="flex items-center gap-2 rounded-md border p-3">
              <Checkbox id="active" checked={isActive} onCheckedChange={(v) => setIsActive(!!v)} />
              <Label htmlFor="active" className="cursor-pointer">
                Active - inactive rules are skipped at order placement.
              </Label>
            </div>
          </DetailPageSection>

          <RuleRowsSection
            title="Conditions"
            subtitle="All must match for the rule to apply."
            rows={conditions}
            setRows={setConditions}
            types={conditionTypes}
          />

          <RuleRowsSection
            title="Effects"
            subtitle="Applied in order to compute tax."
            rows={effects}
            setRows={setEffects}
            types={effectTypes}
          />

          {submitError && (
            <div className="px-6 py-4">
              <Alert variant="destructive">
                <AlertTitle>{isEdit ? 'Update failed' : 'Create failed'}</AlertTitle>
                <AlertDescription>{submitError}</AlertDescription>
              </Alert>
            </div>
          )}
        </DetailPageBody>
      </div>
      {/* Outside the form on purpose - plugin submit handlers must not trip it */}
      <DetailPageBody className="pb-0">
        <PluginSections target="tax-rule" entity={rule ?? null} mode={isEdit ? 'edit' : 'create'} />
      </DetailPageBody>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Tax rule · ${rule?.name || '(unnamed)'}`}
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

function RuleRowsSection({
  title,
  subtitle,
  rows,
  setRows,
  types,
}: {
  title: string
  subtitle: string
  rows: RowKind[]
  setRows: React.Dispatch<React.SetStateAction<RowKind[]>>
  types: TypeDescriptor[]
}) {
  function addRow() {
    const first = types[0]
    if (!first) return
    setRows((prev) => [...prev, { type: first.slug, params: {} }])
  }
  function updateRow(i: number, next: Partial<RowKind>) {
    setRows((prev) => prev.map((r, idx) => (idx === i ? { ...r, ...next } : r)))
  }
  function removeRow(i: number) {
    setRows((prev) => prev.filter((_, idx) => idx !== i))
  }

  return (
    <DetailPageSection
      title={title}
      description={subtitle}
      trailing={
        <Button type="button" variant="outline" size="sm" className="gap-1" onClick={addRow} disabled={types.length === 0}>
          <PlusIcon className="size-3.5" />
          Add
        </Button>
      }
    >
      {rows.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-6 text-center text-xs text-muted-foreground">
          None yet.
        </p>
      ) : (
        <ul className="grid gap-3">
          {rows.map((row, i) => {
            const desc = types.find((t) => t.slug === row.type)
            return (
              <li key={i} className="grid gap-3 rounded-md border p-3">
                <div className="flex items-center gap-2">
                  <select
                    className="h-8 flex-1 rounded-md border bg-background px-2 text-sm"
                    value={row.type}
                    onChange={(e) => updateRow(i, { type: e.target.value, params: {} })}
                  >
                    {types.map((t) => (
                      <option key={t.slug} value={t.slug}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                  <Button type="button" variant="ghost" size="icon" className="size-7 text-muted-foreground hover:text-destructive" onClick={() => removeRow(i)}>
                    <Trash2Icon className="size-3.5" />
                  </Button>
                </div>
                {desc?.description && <p className="text-xs text-muted-foreground">{desc.description}</p>}
                {/* Same registry promotions use - tax conditions name the same countries and
                    customer groups, so they get the same pickers rather than a bare text box */}
                {desc?.paramsSchema && (
                  <SchemaForm
                    schema={desc.paramsSchema}
                    value={row.params}
                    onChange={(v) => updateRow(i, { params: v })}
                    widgets={FORM_WIDGETS}
                  />
                )}
              </li>
            )
          })}
        </ul>
      )}
    </DetailPageSection>
  )
}
