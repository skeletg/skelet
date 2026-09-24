import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { ChevronDownIcon, ChevronRightIcon, PlusIcon, Trash2Icon } from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'
import { LoadingState } from '@/components/ui/loading-state'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FieldError } from '@/components/form/fields'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfigField, type ConfigFieldData } from '@/features/settings/ConfigField'
import { ConfigSectionsDocument, SetConfigValueDocument } from '@/features/settings/Settings.gql'
import { fieldIsInherited } from '@/features/settings/provenance'
import { useConfigDrafts } from '@/features/settings/useConfigDrafts'
import type { ConfigScopeInput } from '@/graphql/schema.types'
import {
  AttributesDocument,
  SetAttributeIndexableDocument,
  SetAttributeValuesIndexableDocument,
} from '@/features/catalog/Catalog.gql'
import { RunAdminActionDocument } from '@/features/tools/Tools.gql'
import { CrawlAuditDocument } from '@/features/seo/LinkMap.gql'
import {
  combinationEstimate,
  hasSelectableValues,
  type FacetAttr,
  type FacetValue,
} from './facet-indexing'
import {
  COMBINATIONS_PATH,
  FACET_INDEXING_LABELS,
  MAX_SLOTS,
  facetIndexingChanges,
  facetIndexingDiff,
  facetIndexingSchema,
  toFacetIndexingValues,
  type FacetIndexingValues,
} from './schemas/facetIndexing'

// Facet-indexing SEO console: the seo/facets config group (hidden from the generic
// settings page, same deal as the search section) plus an overview of which facets and
// values are index-opted-in. Anatomy of the ruleset: single-facet pages index per
// attribute/value flags; multi-facet pages only when the combination is listed here

type Scope = { mode: 'GLOBAL' | 'CHANNEL'; channelId?: string; locale?: string }

export function FacetSeoPage() {
  const { scope, scopeInput, effectiveChannelId } = useConfigScope()
  const skipQuery = scope.mode === 'CHANNEL' && !effectiveChannelId
  const configQuery = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    skip: skipQuery,
  })
  const attrsQuery = useQuery(AttributesDocument)

  const facetGroup = useMemo(
    () =>
      (configQuery.data?.configSections ?? [])
        .find((s) => s.slug === 'seo')
        ?.groups.find((g) => g.slug === 'facets') ?? null,
    [configQuery.data],
  )

  // Facet candidates mirror what the storefront can render as a filter panel
  const facetAttrs = useMemo(
    () =>
      (attrsQuery.data?.attributes ?? [])
        .filter((a) => a.filterable && a.isPublic)
        .sort((a, b) => a.name.localeCompare(b.name)),
    [attrsQuery.data],
  )

  if (configQuery.loading || attrsQuery.loading) return <LoadingState />

  if (skipQuery) {
    return (
      <DetailPageLayout toc={false}>
        <DetailPageHeader backTo={null} title="Facet indexing" />
        <DetailPageBody>
          <Alert className="m-6">
            <AlertTitle>Pick a channel</AlertTitle>
            <AlertDescription>
              Facet indexing is configured per channel - choose one in the header, or switch to Global defaults.
            </AlertDescription>
          </Alert>
        </DetailPageBody>
      </DetailPageLayout>
    )
  }

  const scopeKey = scope.mode === 'GLOBAL' ? 'global' : `channel:${effectiveChannelId}`
  // Keyed on the scope - the combinations and guards are that scope's values
  return <FacetIndexingForm key={scopeKey} scopeKey={scopeKey} attrs={facetAttrs} fields={facetGroup?.fields ?? []} />
}

// The opt-ins and combinations are one form behind the header Save; the guard fields are config
// values and stage through useConfigDrafts into the same registry, so one review covers all three
function FacetIndexingForm({ scopeKey, attrs, fields }: { scopeKey: string; attrs: FacetAttr[]; fields: ConfigFieldData[] }) {
  const permissions = usePermissions()
  const { scope, scopeInput, selectedChannel, effectiveChannelId } = useConfigScope()
  const canManageConfig = scope.mode === 'GLOBAL' ? permissions.has('config.manage_defaults') : permissions.has('config.manage')
  const canManageCatalog = permissions.has('catalog.manage')

  const combosField = fields.find((f) => f.path === COMBINATIONS_PATH) ?? null
  const guardFields = fields.filter((f) => f.path !== COMBINATIONS_PATH)

  const refetchAttrs = [{ query: AttributesDocument }]
  const [setIndexable] = useMutation(SetAttributeIndexableDocument, { refetchQueries: refetchAttrs })
  const [setValuesIndexable] = useMutation(SetAttributeValuesIndexableDocument, { refetchQueries: refetchAttrs })
  const [setConfigValue] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })
  // Set after a save that flipped a scalar facet - its buckets carry the flag from the last index
  // run, so the change is invisible to crawlers until a reindex
  const [scalarPending, setScalarPending] = useState(false)

  const baseline = useMemo(() => toFacetIndexingValues(attrs, combosField?.value ?? ''), [attrs, combosField?.value])
  const diff = useMemo(() => facetIndexingDiff(attrs), [attrs])
  const scopeLabel = scope.mode === 'GLOBAL' ? 'Global defaults' : (selectedChannel?.name ?? 'Channel')

  const record = useRecordForm({
    id: 'facet-indexing',
    title: 'Facet indexing',
    label: `Facet indexing · ${scopeLabel}`,
    isEdit: true,
    baseline,
    schema: facetIndexingSchema,
    diff,
    persist: { routeKey: `facet-indexing:${scopeKey}` },
    submit: async (v) => {
      const changes = facetIndexingChanges(baseline, v)
      await Promise.all(changes.facets.map((f) => setIndexable({ variables: { attributeId: f.id, isIndexable: f.isIndexable } })))
      // One call per direction rather than per value
      if (changes.valuesOn.length > 0) await setValuesIndexable({ variables: { valueIds: changes.valuesOn, isIndexable: true } })
      if (changes.valuesOff.length > 0) await setValuesIndexable({ variables: { valueIds: changes.valuesOff, isIndexable: false } })
      if (changes.combos !== null) {
        await setConfigValue({ variables: { path: COMBINATIONS_PATH, value: changes.combos, scope: scopeInput } })
      }
      if (changes.facets.some((f) => !hasSelectableValues(attrs.find((a) => a.id === f.id)?.inputType ?? ''))) {
        setScalarPending(true)
      }
    },
  })

  return (
    <RecordFormPage
      record={record}
      backTo={null}
      title="Facet indexing"
      badges={
        scope.mode === 'GLOBAL' ? (
          <Badge variant="outline">Platform defaults</Badge>
        ) : (
          <Badge variant="secondary">{selectedChannel?.name ?? 'Channel'} overrides</Badge>
        )
      }
      meta={<span>Which filter pages search engines may index - single filters follow the per-value flags, combinations must be listed</span>}
      createLabel="Save changes"
      labels={FACET_INDEXING_LABELS}
      hideSave={!canManageCatalog && !canManageConfig}
    >
      {effectiveChannelId ? <CrawlRealityStrip channelId={effectiveChannelId} /> : null}

      <DetailPageSection
        title="Facets"
        description="The master opt-in per facet. Expand a facet to opt its individual values in or out - a filter page is only indexable when both the facet and the value it filters on carry the flag."
      >
        <AttributeIndexSection form={record.form} attrs={attrs} canManage={canManageCatalog} scalarPending={scalarPending} onReindexed={() => setScalarPending(false)} />
      </DetailPageSection>

      <DetailPageSection
        title="Indexable combinations"
        description="Filter pages combining several facets stay noindex unless their combination is listed - each listing is a deliberate landing-page family, so crawlers never walk the full combination space."
      >
        {combosField ? (
          <CombinationsSection form={record.form} attrs={attrs} canManage={canManageConfig} />
        ) : (
          <p className="text-sm text-muted-foreground">The combinations setting is missing - is the backend up to date?</p>
        )}
      </DetailPageSection>

      <DetailPageSection
        title="Guards & robots tags"
        description="Extra conditions a filter page must meet before it indexes, and what the robots meta says once the verdict is in."
      >
        <GuardFieldsSection fields={guardFields} scope={scope} scopeInput={scopeInput} canManage={canManageConfig} />
      </DetailPageSection>

      <DetailPageSection
        title="Elsewhere"
        description="Neighbouring controls that shape the same crawl surface but live on their own pages."
      >
        <ul className="grid gap-2 text-sm">
          <li>
            {/* Section-precise: SearchSettings renders its groups as DetailPageSections,
                so the Product Sort group answers to its slugified title */}
            <Link to="/search-engine#product-sort" className="font-medium hover:underline">
              Sort URLs
            </Link>
            <span className="text-muted-foreground">
              {' '}
              - whether a <code className="font-mono text-xs">/s/</code> URL may index is a per-option switch under Product Sort.
            </span>
          </li>
          <li>
            {/* Settings has no anchors - it narrows through the route instead,
                /settings/<section>/<group> */}
            <Link to="/settings/seo/sitemap" className="font-medium hover:underline">
              Sitemap
            </Link>
            <span className="text-muted-foreground">
              {' '}
              - nothing emits facet URLs into a sitemap; indexable ones are found by internal linking. Paste exceptions into Extra
              URLs, or suppress paths with Exclude paths.
            </span>
          </li>
          <li>
            <Link to="/categories" className="font-medium hover:underline">
              Category &amp; collection overrides
            </Link>
            <span className="text-muted-foreground">
              {' '}
              - hiding a facet on one page also removes it from indexing there, so these rules are the ceiling rather than the
              guarantee. Open a category or collection, then its Storefront filters section.
            </span>
          </li>
        </ul>
      </DetailPageSection>
    </RecordFormPage>
  )
}

type FacetForm = ReturnType<typeof useRecordForm<FacetIndexingValues>>['form']

/**
 * What the last crawl actually found, next to what the rules intend. Two cheap reads of
 * the existing crawlAudit: totalPages for FILTER, and again with issue NOINDEX. Never
 * issueCounts - that block is whole-crawl, not kind-filtered.
 * The crawler stamps FILTER for a /f/ OR /s/ segment, hence the wording
 */
function CrawlRealityStrip({ channelId }: { channelId: string }) {
  const all = useQuery(CrawlAuditDocument, {
    variables: { channelId, kind: 'FILTER' as const, limit: 1 },
  })
  const noindexed = useQuery(CrawlAuditDocument, {
    variables: { channelId, kind: 'FILTER' as const, issue: 'NOINDEX' as const, limit: 1 },
  })
  const total = all.data?.crawlAudit?.totalPages
  if (all.loading || total == null) return null
  const off = noindexed.data?.crawlAudit?.totalPages ?? 0

  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 border-b px-6 py-3 text-sm">
      {total === 0 ? (
        <span className="text-muted-foreground">
          The last crawl found no filter or sort URLs.
        </span>
      ) : (
        <span className="text-muted-foreground">
          Last crawl reached{' '}
          <span className="font-mono tabular-nums text-foreground">{total.toLocaleString()}</span>{' '}
          filter &amp; sort URLs,{' '}
          <span className="font-mono tabular-nums text-foreground">{off.toLocaleString()}</span> of
          them noindex.
        </span>
      )}
      <Link to="/seo/link-map" className="font-medium hover:underline">
        Open the link map
      </Link>
    </div>
  )
}


/**
 * Every storefront facet with its index opt-ins. The master switch is staged here, and
 * a selectable facet expands in place to opt its values in or out in bulk, rather than only
 * pointing at where that is done.
 */
function AttributeIndexSection({
  form,
  attrs,
  canManage,
  scalarPending,
  onReindexed,
}: {
  form: FacetForm
  attrs: FacetAttr[]
  canManage: boolean
  scalarPending: boolean
  onReindexed: () => void
}) {
  const [runAction, { loading: reindexing }] = useMutation(RunAdminActionDocument)
  const [expanded, setExpanded] = useState<string | null>(null)
  const facets = useStore(form.store, (s) => s.values.facets)
  const values = useStore(form.store, (s) => s.values.values)

  const setValues = (ids: string[], next: boolean) =>
    form.setFieldValue('values', (prev) => ({ ...prev, ...Object.fromEntries(ids.map((id) => [id, next])) }))

  if (attrs.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No filterable public attributes yet - mark attributes as filterable to build the
        storefront sidebar first.
      </p>
    )
  }

  return (
    <div className="grid gap-3">
      {scalarPending && (
        <Alert>
          <AlertTitle>Reindex to publish the scalar change</AlertTitle>
          <AlertDescription className="flex flex-wrap items-center gap-3">
            <span>
              Scalar facets carry their indexable flag in the search index, so the switch
              you just saved reaches crawlers on the next reindex. Selectable facets and
              their values apply immediately.
            </span>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={reindexing}
              onClick={async () => {
                try {
                  await runAction({ variables: { slug: 'search.reindex', args: {} } })
                  toast.success('Reindex started')
                  onReindexed()
                } catch (err) {
                  toast.error(err instanceof Error ? err.message : 'Could not start the reindex')
                }
              }}
            >
              {reindexing ? 'Starting…' : 'Reindex now'}
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <ul className="divide-y rounded-md border">
        {attrs.map((a) => {
          const on = facets[a.id] ?? a.isIndexable
          const selectable = hasSelectableValues(a.inputType)
          const ixValues = a.values.filter((v) => values[v.id] ?? v.isIndexable).length
          const isOpen = expanded === a.id
          return (
            <li key={a.id}>
              <div className="flex items-center gap-3 px-3 py-2">
                <span className="min-w-0 flex-1">
                  <Link to={`/catalog/attributes/${a.id}`} className="block truncate text-sm font-medium hover:underline">
                    {a.name}
                  </Link>
                  <span className="block truncate font-mono text-[11px] text-muted-foreground">{a.slug}</span>
                </span>
                {selectable ? (
                  <button
                    type="button"
                    onClick={() => setExpanded(isOpen ? null : a.id)}
                    aria-expanded={isOpen}
                    className="hidden shrink-0 items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:inline-flex"
                  >
                    {isOpen ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />}
                    {ixValues}/{a.values.length} values indexable
                  </button>
                ) : (
                  <span className="hidden shrink-0 text-xs text-muted-foreground sm:block">scalar - all buckets follow the switch</span>
                )}
                <Badge variant="outline" className="hidden shrink-0 md:inline-flex">
                  H1 {a.h1Position.toLowerCase()}
                </Badge>
                <Switch
                  checked={on}
                  onCheckedChange={(v) => form.setFieldValue('facets', (prev) => ({ ...prev, [a.id]: v }))}
                  disabled={!canManage}
                  aria-label={`Facet ${a.name} indexable`}
                />
              </div>
              {isOpen && selectable && <ValueOptInList values={a.values} current={values} canManage={canManage} onChange={setValues} />}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

/**
 * Value-level opt-in for one facet: search, bulk apply to whatever the search matched,
 * per-row switches. Staged into the page's form, saved as one bulk mutation per direction.
 */
function ValueOptInList({
  values,
  current,
  canManage,
  onChange,
}: {
  values: FacetValue[]
  current: Record<string, boolean>
  canManage: boolean
  onChange: (ids: string[], next: boolean) => void
}) {
  const [query, setQuery] = useState('')
  const q = query.trim().toLowerCase()
  const shown = q ? values.filter((v) => v.name.toLowerCase().includes(q) || v.slug.toLowerCase().includes(q)) : values

  return (
    <div className="border-t bg-muted/30 px-3 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={`Search ${values.length} values…`} className="h-8 w-56" />
        <span className="text-xs text-muted-foreground">{q ? `${shown.length} shown` : `${values.length} values`}</span>
        <span className="ml-auto flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canManage || shown.length === 0}
            onClick={() => onChange(shown.map((v) => v.id), true)}
          >
            {q ? 'Index matching' : 'Index all'}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!canManage || shown.length === 0}
            onClick={() => onChange(shown.map((v) => v.id), false)}
          >
            {q ? 'Exclude matching' : 'Exclude all'}
          </Button>
        </span>
      </div>
      {shown.length === 0 ? (
        <p className="mt-3 text-xs text-muted-foreground">Nothing matches &ldquo;{query.trim()}&rdquo;</p>
      ) : (
        <ul className="mt-3 grid gap-1 sm:grid-cols-2 lg:grid-cols-3">
          {shown.map((v) => {
            const on = current[v.id] ?? v.isIndexable
            const dirty = on !== v.isIndexable
            return (
              <li key={v.id} className={`flex items-center gap-2 rounded-md px-2 py-1 ${dirty ? 'bg-amber-500/10' : ''}`}>
                <Switch
                  checked={on}
                  onCheckedChange={(next) => onChange([v.id], next)}
                  disabled={!canManage}
                  aria-label={`Value ${v.name} indexable`}
                />
                <span className="min-w-0 flex-1 truncate text-sm" title={v.slug}>
                  {v.name}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

/**
 * Builder over the seo/facets/indexable_combinations value - rows of facet picks
 * serialize to the `slug + slug` line format the storefront parses.
 */
function CombinationsSection({ form, attrs, canManage }: { form: FacetForm; attrs: FacetAttr[]; canManage: boolean }) {
  const rows = useStore(form.store, (s) => s.values.combos)

  // Only facets whose master switch is on make useful combination members
  const options = attrs.filter((a) => a.isIndexable)

  const estimate = (row: string[]) => combinationEstimate(row, attrs)
  const totalEstimate = rows.reduce((sum, r) => sum + (estimate(r) ?? 0), 0)

  const setRows = (update: (prev: string[][]) => string[][]) => form.setFieldValue('combos', update)
  const setSlot = (row: number, slot: number, slug: string) =>
    setRows((prev) => prev.map((r, i) => (i === row ? r.map((s, j) => (j === slot ? slug : s)) : r)))

  return (
    <fieldset disabled={!canManage} className="grid gap-3">
      {rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No combinations yet - every multi-filter page is noindex. Add one to open a
          landing-page family like brand + size.
        </p>
      )}
      <ul className="grid gap-2">
        {rows.map((row, i) => {
          const rowEstimate = estimate(row)
          return (
            <li key={i} className="grid gap-1 rounded-md border p-2">
              <div className="flex flex-wrap items-center gap-2">
                {row.map((slug, j) => (
                  <Select key={j} value={slug || undefined} onValueChange={(v) => setSlot(i, j, v)} disabled={!canManage}>
                    <SelectTrigger className="w-44" aria-label={`Combination ${i + 1} facet ${j + 1}`}>
                      <SelectValue placeholder="Pick a facet" />
                    </SelectTrigger>
                    <SelectContent>
                      {options
                        .filter((o) => o.slug === slug || !row.includes(o.slug))
                        .map((o) => (
                          <SelectItem key={o.slug} value={o.slug}>
                            {o.name}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                ))}
                {row.length < MAX_SLOTS && (
                  <Button type="button" variant="outline" size="sm" onClick={() => setRows((prev) => prev.map((r, x) => (x === i ? [...r, ''] : r)))}>
                    <PlusIcon className="size-3.5" />
                    Facet
                  </Button>
                )}
                <span className="ml-auto flex items-center gap-2">
                  <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                    {rowEstimate == null ? 'scalar - not countable' : `up to ${rowEstimate.toLocaleString()} URLs`}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="text-muted-foreground hover:text-destructive"
                    aria-label="Remove combination"
                    onClick={() => setRows((prev) => prev.filter((_, x) => x !== i))}
                  >
                    <Trash2Icon className="size-4" />
                  </Button>
                </span>
              </div>
              <form.Field name={`combos[${i}]`}>{(f) => <FieldError field={f} />}</form.Field>
            </li>
          )
        })}
      </ul>
      <div>
        <Button type="button" variant="outline" size="sm" disabled={options.length < 2} onClick={() => setRows((prev) => [...prev, ['', '']])}>
          <PlusIcon className="mr-1 size-3.5" />
          Add combination
        </Button>
        {options.length < 2 && (
          <p className="mt-2 text-xs text-muted-foreground">Combinations need at least two indexable facets - switch them on above first.</p>
        )}
        {totalEstimate > 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            These combinations open up to <span className="font-mono tabular-nums">{totalEstimate.toLocaleString()}</span> crawlable
            URLs before the min-results guard trims them.
          </p>
        )}
      </div>
    </fieldset>
  )
}

/**
 * The rest of the seo/facets group rendered generically - a key registered into the
 * group on the backend shows up here with no dashboard change, same trick as the
 * search settings page. Staged through the shared config drafts into the page's registry.
 */
function GuardFieldsSection({
  fields,
  scope,
  scopeInput,
  canManage,
}: {
  fields: ConfigFieldData[]
  scope: Scope
  scopeInput: ConfigScopeInput | undefined
  canManage: boolean
}) {
  const sections = useMemo(
    () => [{ slug: 'seo', label: 'Guards', groups: [{ slug: 'facets', label: 'Facets', fields }] }],
    [fields],
  )
  const { drafts, onFieldChange, onReset, resettingPath, resetError } = useConfigDrafts({ sections, scopeInput })

  if (fields.length === 0) {
    return <p className="text-sm text-muted-foreground">No guard settings registered.</p>
  }

  return (
    <div className="grid gap-4">
      {resetError && (
        <Alert variant="destructive">
          <AlertTitle>Couldn't reset</AlertTitle>
          <AlertDescription>{resetError}</AlertDescription>
        </Alert>
      )}
      {fields.map((f) => (
        <ConfigField
          key={f.path}
          field={f}
          draft={drafts[f.path]}
          onChange={(v) => onFieldChange(f.path, v)}
          onReset={() => onReset(f.path)}
          resetting={resettingPath === f.path}
          disabled={!canManage}
          inherited={fieldIsInherited(f, scope.mode, scope.channelId, scope.locale ?? '')}
        />
      ))}
    </div>
  )
}
