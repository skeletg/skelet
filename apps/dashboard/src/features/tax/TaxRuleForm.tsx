import { useMemo } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { LoadError } from '@/shared/errors/LoadError'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { SchemaForm, type JSONSchema } from '@/features/promotions/SchemaForm'
import { FORM_WIDGETS } from '@/features/promotions/widgets/registry'
import { PluginSections } from '@/plugins/detail-sections'
import {
  CreateTaxRuleDocument,
  TaxRuleDocument,
  TaxRuleTypesDocument,
  UpdateTaxRuleDocument,
} from './Tax.gql'
import {
  TAX_RULE_LABELS,
  taxRuleDiff,
  taxRuleSchema,
  toTaxRuleInput,
  toTaxRuleValues,
  type ExistingTaxRule,
  type RuleRow,
  type TaxRuleValues,
} from './schemas/taxRule'

type TypeDescriptor = { slug: string; label: string; description: string; paramsSchema: unknown }

export function TaxRuleCreatePage() {
  const [params] = useSearchParams()
  return <TaxRuleFormPage mode="create" channelId={params.get('channelId') ?? ''} />
}

export function TaxRuleEditPage({ taxRuleId }: { taxRuleId?: string } = {}) {
  const params = useParams<{ id?: string }>()
  const id = taxRuleId ?? params.id ?? ''
  const { data, loading, error, refetch } = useQuery(TaxRuleDocument, { variables: { id }, skip: !id })
  if (loading) return <p className="px-6 py-6 text-sm text-muted-foreground">Loading rule…</p>
  if (error) {
    return (
      <div className="px-6 py-6">
        <LoadError title="Couldn't load tax rule" message={error.message} onRetry={() => void refetch()} retrying={loading} />
      </div>
    )
  }
  if (!data?.taxRule) {
    return <p className="px-6 py-6 text-sm text-muted-foreground">Rule not found.</p>
  }
  return <TaxRuleFormPage mode="edit" rule={data.taxRule} channelId={data.taxRule.channelId} />
}

function TaxRuleFormPage({ mode, rule, channelId }: { mode: 'create' | 'edit'; rule?: ExistingTaxRule; channelId: string }) {
  const navigate = useNavigate()
  const isEdit = mode === 'edit'

  const { data: typeData } = useQuery(TaxRuleTypesDocument)
  const conditionTypes = typeData?.taxConditionTypes ?? []
  const effectTypes = typeData?.taxEffectTypes ?? []

  const [createRule] = useMutation(CreateTaxRuleDocument)
  const [updateRule] = useMutation(UpdateTaxRuleDocument)

  const baseline = useMemo(() => toTaxRuleValues(rule), [rule])
  const record = useRecordForm({
    id: 'tax-rule',
    title: 'Tax rule',
    label: isEdit ? `Tax rule · ${rule?.name || '(unnamed)'}` : 'New tax rule',
    isEdit,
    baseline,
    schema: taxRuleSchema,
    diff: taxRuleDiff,
    persist: { routeKey: `tax-rule:${rule?.id ?? `new:${channelId}`}` },
    submit: async (value) => {
      const input = toTaxRuleInput(value)
      if (rule) {
        await updateRule({ variables: { input: { id: rule.id, ...input } } })
        return
      }
      if (!channelId) throw new Error('channelId missing - open this from the tax rules list page.')
      await createRule({ variables: { input: { channelId, ...input } } })
    },
    onSaved: () => navigate('/tax'),
  })
  const { form, draft } = record
  const clash = (field: keyof TaxRuleValues) => <ClashNote draft={draft} field={field} />

  return (
    <RecordFormPage
      record={record}
      backTo="/tax"
      backLabel="Tax rules"
      title={isEdit ? rule?.name || '(unnamed)' : 'New tax rule'}
      meta="Conditions must all match; effects apply in order. An exclusivity group makes the most specific rule win."
      createLabel="Create rule"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/tax">Cancel</Link>
        </Button>
      }
      labels={TAX_RULE_LABELS}
      // Outside the form on purpose - plugin submit handlers must not trip it
      after={<PluginSections target="tax-rule" entity={rule ?? null} mode={isEdit ? 'edit' : 'create'} />}
    >
      <DetailPageSection title="Rule">
        <div className="grid items-start gap-4 sm:grid-cols-2">
          <form.AppField name="name">
            {(f) => <f.TextField id="name" label="Display name" placeholder="California sales tax" note={clash('name')} />}
          </form.AppField>
          <form.AppField name="exclusivityGroup">
            {(f) => (
              <f.TextField
                id="group"
                label="Exclusivity group"
                placeholder="jurisdiction (empty = always stacks)"
                note={clash('exclusivityGroup')}
              />
            )}
          </form.AppField>
          <form.AppField name="priority">
            {(f) => <f.NumberField id="priority" label="Priority (lower = earlier)" note={clash('priority')} />}
          </form.AppField>
          <form.AppField name="sortOrder">
            {(f) => <f.NumberField id="sort" label="Sort order" note={clash('sortOrder')} />}
          </form.AppField>
        </div>
        <div className="rounded-md border p-3">
          <form.AppField name="isActive">
            {(f) => <f.CheckboxField id="active" label="Active - inactive rules are skipped at order placement." />}
          </form.AppField>
        </div>
      </DetailPageSection>

      <form.Field name="conditions">
        {(f) => (
          <RuleRowsSection
            title="Conditions"
            subtitle="All must match for the rule to apply."
            rows={f.state.value}
            onChange={f.handleChange}
            types={conditionTypes}
            note={clash('conditions')}
          />
        )}
      </form.Field>

      <form.Field name="effects">
        {(f) => (
          <RuleRowsSection
            title="Effects"
            subtitle="Applied in order to compute tax."
            rows={f.state.value}
            onChange={f.handleChange}
            types={effectTypes}
            note={clash('effects')}
          />
        )}
      </form.Field>
    </RecordFormPage>
  )
}

function RuleRowsSection({
  title,
  subtitle,
  rows,
  onChange,
  types,
  note,
}: {
  title: string
  subtitle: string
  rows: RuleRow[]
  onChange: (rows: RuleRow[]) => void
  types: TypeDescriptor[]
  note?: React.ReactNode
}) {
  const updateRow = (i: number, next: Partial<RuleRow>) =>
    onChange(rows.map((r, idx) => (idx === i ? { ...r, ...next } : r)))

  return (
    <DetailPageSection
      title={title}
      description={subtitle}
      trailing={
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="gap-1"
          disabled={types.length === 0}
          onClick={() => types[0] && onChange([...rows, { type: types[0].slug, params: {} }])}
        >
          <PlusIcon className="size-3.5" />
          Add
        </Button>
      }
    >
      {rows.length === 0 ? (
        <p className="rounded-md border border-dashed px-4 py-6 text-center text-xs text-muted-foreground">None yet.</p>
      ) : (
        <ul className="grid gap-3">
          {rows.map((row, i) => {
            const desc = types.find((t) => t.slug === row.type)
            return (
              <li key={i} className="grid gap-3 rounded-md border p-3">
                <div className="flex items-center gap-2">
                  <select
                    aria-label={`${title} ${i + 1} type`}
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
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Remove ${title.toLowerCase()} ${i + 1}`}
                    className="size-7 text-muted-foreground hover:text-destructive"
                    onClick={() => onChange(rows.filter((_, idx) => idx !== i))}
                  >
                    <Trash2Icon className="size-3.5" />
                  </Button>
                </div>
                {desc?.description && <p className="text-xs text-muted-foreground">{desc.description}</p>}
                {/* Same registry promotions use - tax conditions name the same countries and
                    customer groups, so they get the same pickers rather than a bare text box */}
                {desc?.paramsSchema != null && (
                  <SchemaForm
                    schema={desc.paramsSchema as JSONSchema}
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
      {note}
    </DetailPageSection>
  )
}
