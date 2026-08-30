import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
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
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfigField } from '@/features/settings/ConfigField'
import {
  ConfigSectionsDocument,
  ResetConfigValueDocument,
  SetConfigValueDocument,
} from '@/features/settings/Settings.gql'
import { fieldIsInherited } from '@/features/settings/provenance'
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

// Facet-indexing SEO console: the seo/facets config group (hidden from the generic
// settings page, same deal as the search section) plus an overview of which facets and
// values are index-opted-in. Anatomy of the ruleset: single-facet pages index per
// attribute/value flags; multi-facet pages only when the combination is listed here

const COMBINATIONS_PATH = 'seo/facets/indexable_combinations'
const MAX_SLOTS = 4

export function FacetSeoPage() {
  const permissions = usePermissions()
  const { scope, scopeInput, selectedChannel, effectiveChannelId } = useConfigScope()
  const canManageConfig =
    scope.mode === 'GLOBAL'
      ? permissions.has('config.manage_defaults')
      : permissions.has('config.manage')
  const canManageCatalog = permissions.has('catalog.manage')

  const skipQuery = scope.mode === 'CHANNEL' && !effectiveChannelId
  const configQuery = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
    skip: skipQuery,
  })
  const attrsQuery = useQuery(AttributesDocument)

  const { contextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  useUnsavedChangesGuard(isDirty, 'Facet indexing')

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

  const facetGroup = useMemo(
    () =>
      (configQuery.data?.configSections ?? [])
        .find((s) => s.slug === 'seo')
        ?.groups.find((g) => g.slug === 'facets') ?? null,
    [configQuery.data],
  )
  const combosField = facetGroup?.fields.find((f) => f.path === COMBINATIONS_PATH) ?? null
  const guardFields = (facetGroup?.fields ?? []).filter((f) => f.path !== COMBINATIONS_PATH)

  // Facet candidates mirror what the storefront can render as a filter panel
  const facetAttrs = useMemo(
    () =>
      (attrsQuery.data?.attributes ?? [])
        .filter((a) => a.filterable && a.isPublic)
        .sort((a, b) => a.name.localeCompare(b.name)),
    [attrsQuery.data],
  )

  const scopeBadge =
    scope.mode === 'GLOBAL' ? (
      <Badge variant="default">Platform defaults</Badge>
    ) : (
      <Badge variant="secondary">{selectedChannel?.name ?? 'Channel'} overrides</Badge>
    )

  if (configQuery.loading || attrsQuery.loading) return <LoadingState />

  return (
    <SaveRegistryContext.Provider value={contextValue}>
      <DetailPageLayout toc={false}>
        <DetailPageHeader
          backTo={null}
          title="Facet indexing"
          badges={scopeBadge}
          meta={
            <span>
              Which filter pages search engines may index - single filters follow the
              per-value flags, combinations must be listed
            </span>
          }
          actions={
            <Button
              type="button"
              size="sm"
              disabled={!isDirty || savingAll}
              onClick={() => {
                setSaveError(null)
                setConfirmingSave(true)
              }}
            >
              {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
            </Button>
          }
        />

        <DetailPageBody>
          {skipQuery ? (
            <Alert className="m-6">
              <AlertTitle>Pick a channel</AlertTitle>
              <AlertDescription>
                Facet indexing is configured per channel - choose one in the header, or
                switch to Global defaults.
              </AlertDescription>
            </Alert>
          ) : (
            <>
              {effectiveChannelId ? (
                <CrawlRealityStrip channelId={effectiveChannelId} />
              ) : null}

              <DetailPageSection
                title="Facets"
                description="The master opt-in per facet. Expand a facet to opt its individual values in or out - a filter page is only indexable when both the facet and the value it filters on carry the flag."
              >
                <AttributeIndexSection attrs={facetAttrs} canManage={canManageCatalog} />
              </DetailPageSection>

              <DetailPageSection
                title="Indexable combinations"
                description="Filter pages combining several facets stay noindex unless their combination is listed - each listing is a deliberate landing-page family, so crawlers never walk the full combination space."
              >
                {combosField ? (
                  <CombinationsSection
                    field={combosField}
                    attrs={facetAttrs}
                    scopeInput={scopeInput}
                    canManage={canManageConfig}
                  />
                ) : (
                  <p className="text-sm text-muted-foreground">
                    The combinations setting is missing - is the backend up to date?
                  </p>
                )}
              </DetailPageSection>

              <DetailPageSection
                title="Guards & robots tags"
                description="Extra conditions a filter page must meet before it indexes, and what the robots meta says once the verdict is in."
              >
                <GuardFieldsSection
                  fields={guardFields}
                  scope={scope}
                  scopeInput={scopeInput}
                  canManage={canManageConfig}
                />
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
                      - whether a <code className="font-mono text-xs">/s/</code> URL may index is a
                      per-option switch under Product Sort.
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
                      - nothing emits facet URLs into a sitemap; indexable ones are found by
                      internal linking. Paste exceptions into Extra URLs, or suppress paths with
                      Exclude paths.
                    </span>
                  </li>
                  <li>
                    <Link to="/categories" className="font-medium hover:underline">
                      Category &amp; collection overrides
                    </Link>
                    <span className="text-muted-foreground">
                      {' '}
                      - hiding a facet on one page also removes it from indexing there, so these
                      rules are the ceiling rather than the guarantee. Open a category or
                      collection, then its Storefront filters section.
                    </span>
                  </li>
                </ul>
              </DetailPageSection>
            </>
          )}
        </DetailPageBody>

        <SaveConfirmDialog
          open={confirmingSave}
          scopeLabel={`Facet indexing · ${scope.mode === 'GLOBAL' ? 'Global defaults' : (selectedChannel?.name ?? 'Channel')}`}
          sections={dirtySections}
          saving={savingAll}
          error={saveError}
          onCancel={() => {
            if (!savingAll) {
              setConfirmingSave(false)
              setSaveError(null)
            }
          }}
          onConfirm={onConfirmSave}
        />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

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
  attrs,
  canManage,
}: {
  attrs: FacetAttr[]
  canManage: boolean
}) {
  const refetch = [{ query: AttributesDocument }]
  const [setIndexable] = useMutation(SetAttributeIndexableDocument, { refetchQueries: refetch })
  const [setValuesIndexable] = useMutation(SetAttributeValuesIndexableDocument, {
    refetchQueries: refetch,
  })
  const [runAction, { loading: reindexing }] = useMutation(RunAdminActionDocument)
  const [drafts, setDrafts] = useState<Record<string, boolean>>({})
  const [valueDrafts, setValueDrafts] = useState<Record<string, boolean>>({})
  const [expanded, setExpanded] = useState<string | null>(null)
  // Set after a save that flipped a scalar facet - its buckets carry the flag from the
  // last index run, so the change is invisible to crawlers until a reindex
  const [scalarPending, setScalarPending] = useState(false)

  const changed = attrs.filter((a) => drafts[a.id] !== undefined && drafts[a.id] !== a.isIndexable)
  const allValues = useMemo(
    () => attrs.flatMap((a) => a.values.map((v) => ({ ...v, attr: a }))),
    [attrs],
  )
  const changedValues = allValues.filter(
    (v) => valueDrafts[v.id] !== undefined && valueDrafts[v.id] !== v.isIndexable,
  )

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = changed.map((a) => ({
      label: a.name,
      before: a.isIndexable ? 'indexable' : 'not indexable',
      after: drafts[a.id] ? 'indexable' : 'not indexable',
    }))
    // Values roll up per facet - a 40-row diff would drown the confirm dialog
    const byAttr = new Map<string, { on: number; off: number; name: string }>()
    for (const v of changedValues) {
      const e = byAttr.get(v.attr.id) ?? { on: 0, off: 0, name: v.attr.name }
      if (valueDrafts[v.id]) e.on += 1
      else e.off += 1
      byAttr.set(v.attr.id, e)
    }
    for (const [, e] of byAttr) {
      out.push({
        label: `${e.name} values`,
        before: `${e.on + e.off} changing`,
        after: [e.on ? `${e.on} indexable` : '', e.off ? `${e.off} not indexable` : '']
          .filter(Boolean)
          .join(', '),
      })
    }
    return out
    // both lists derive from drafts + attrs
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drafts, valueDrafts, attrs])

  useSaveSection({
    id: 'facet-attributes',
    title: 'Facet opt-ins',
    diff,
    save: async () => {
      await Promise.all(
        changed.map((a) =>
          setIndexable({ variables: { attributeId: a.id, isIndexable: drafts[a.id]! } }),
        ),
      )
      // One call per direction rather than per value
      for (const on of [true, false]) {
        const ids = changedValues.filter((v) => valueDrafts[v.id] === on).map((v) => v.id)
        if (ids.length > 0) {
          await setValuesIndexable({ variables: { valueIds: ids, isIndexable: on } })
        }
      }
      if (changed.some((a) => !hasSelectableValues(a.inputType))) setScalarPending(true)
      setDrafts({})
      setValueDrafts({})
    },
  })

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
                  setScalarPending(false)
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
          const on = drafts[a.id] ?? a.isIndexable
          const selectable = hasSelectableValues(a.inputType)
          const ixValues = a.values.filter((v) => valueDrafts[v.id] ?? v.isIndexable).length
          const isOpen = expanded === a.id
          return (
            <li key={a.id}>
              <div className="flex items-center gap-3 px-3 py-2">
                <span className="min-w-0 flex-1">
                  <Link
                    to={`/catalog/attributes/${a.id}`}
                    className="block truncate text-sm font-medium hover:underline"
                  >
                    {a.name}
                  </Link>
                  <span className="block truncate font-mono text-[11px] text-muted-foreground">
                    {a.slug}
                  </span>
                </span>
                {selectable ? (
                  <button
                    type="button"
                    onClick={() => setExpanded(isOpen ? null : a.id)}
                    aria-expanded={isOpen}
                    className="hidden shrink-0 items-center gap-1 rounded-md px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:inline-flex"
                  >
                    {isOpen ? (
                      <ChevronDownIcon className="size-3.5" />
                    ) : (
                      <ChevronRightIcon className="size-3.5" />
                    )}
                    {ixValues}/{a.values.length} values indexable
                  </button>
                ) : (
                  <span className="hidden shrink-0 text-xs text-muted-foreground sm:block">
                    scalar - all buckets follow the switch
                  </span>
                )}
                <Badge variant="outline" className="hidden shrink-0 md:inline-flex">
                  H1 {a.h1Position.toLowerCase()}
                </Badge>
                <Switch
                  checked={on}
                  onCheckedChange={(v) => setDrafts((prev) => ({ ...prev, [a.id]: v }))}
                  disabled={!canManage}
                  aria-label={`Facet ${a.name} indexable`}
                />
              </div>
              {isOpen && selectable && (
                <ValueOptInList
                  values={a.values}
                  drafts={valueDrafts}
                  canManage={canManage}
                  onChange={(ids, next) =>
                    setValueDrafts((prev) => {
                      const out = { ...prev }
                      for (const id of ids) out[id] = next
                      return out
                    })
                  }
                />
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}

/**
 * Value-level opt-in for one facet: search, bulk apply to whatever the search matched,
 * per-row switches. Staged into the parent's diff, saved as one bulk mutation.
 */
function ValueOptInList({
  values,
  drafts,
  canManage,
  onChange,
}: {
  values: FacetValue[]
  drafts: Record<string, boolean>
  canManage: boolean
  onChange: (ids: string[], next: boolean) => void
}) {
  const [query, setQuery] = useState('')
  const q = query.trim().toLowerCase()
  const shown = q
    ? values.filter(
        (v) => v.name.toLowerCase().includes(q) || v.slug.toLowerCase().includes(q),
      )
    : values

  return (
    <div className="border-t bg-muted/30 px-3 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={`Search ${values.length} values…`}
          className="h-8 w-56"
        />
        <span className="text-xs text-muted-foreground">
          {q ? `${shown.length} shown` : `${values.length} values`}
        </span>
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
            const on = drafts[v.id] ?? v.isIndexable
            const dirty = drafts[v.id] !== undefined && drafts[v.id] !== v.isIndexable
            return (
              <li
                key={v.id}
                className={`flex items-center gap-2 rounded-md px-2 py-1 ${dirty ? 'bg-amber-500/10' : ''}`}
              >
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

type CfgField = {
  path: string
  value: string
}

function parseCombos(raw: string): string[][] {
  return raw
    .split('\n')
    .map((line) =>
      line
        .split('+')
        .map((s) => s.trim())
        .filter(Boolean),
    )
    .filter((combo) => combo.length >= 2)
}

function serializeCombos(rows: string[][]): string {
  return rows
    .map((row) => row.filter(Boolean))
    .filter((row) => row.length >= 2)
    .map((row) => row.join(' + '))
    .join('\n')
}

/**
 * Builder over the seo/facets/indexable_combinations value - rows of facet picks
 * serialize to the `slug + slug` line format the storefront parses.
 */
function CombinationsSection({
  field,
  attrs,
  scopeInput,
  canManage,
}: {
  field: CfgField
  attrs: FacetAttr[]
  scopeInput: ConfigScopeInput | undefined
  canManage: boolean
}) {
  const [setConfigValue] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })

  const stored = useMemo(() => parseCombos(field.value), [field.value])
  const [rows, setRows] = useState<string[][]>(stored)
  // Re-seed when the stored value changes (load or post-save refetch)
  const [prevStored, setPrevStored] = useState(stored)
  if (JSON.stringify(prevStored) !== JSON.stringify(stored)) {
    setPrevStored(stored)
    setRows(stored)
  }

  const serialized = serializeCombos(rows)
  const dirty = serialized !== serializeCombos(stored)

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      dirty
        ? [
            {
              label: 'Indexable combinations',
              before: serializeCombos(stored).replace(/\n/g, ' · ') || 'none',
              after: serialized.replace(/\n/g, ' · ') || 'none',
            },
          ]
        : [],
    [dirty, stored, serialized],
  )

  useSaveSection({
    id: 'facet-combinations',
    title: 'Indexable combinations',
    diff,
    save: async () => {
      await setConfigValue({
        variables: { path: COMBINATIONS_PATH, value: serialized, scope: scopeInput },
      })
    },
  })

  // Only facets whose master switch is on make useful combination members
  const options = attrs.filter((a) => a.isIndexable)

  const estimate = (row: string[]) => combinationEstimate(row, attrs)
  const totalEstimate = rows.reduce((sum, r) => sum + (estimate(r) ?? 0), 0)

  const setSlot = (row: number, slot: number, slug: string) => {
    setRows((prev) =>
      prev.map((r, i) => (i === row ? r.map((s, j) => (j === slot ? slug : s)) : r)),
    )
  }

  return (
    <div className="grid gap-3">
      {rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No combinations yet - every multi-filter page is noindex. Add one to open a
          landing-page family like brand + size.
        </p>
      )}
      <ul className="grid gap-2">
        {rows.map((row, i) => (
          <li key={i} className="flex flex-wrap items-center gap-2 rounded-md border p-2">
            {row.map((slug, j) => (
              <Select
                key={j}
                value={slug || undefined}
                onValueChange={(v) => setSlot(i, j, v)}
                disabled={!canManage}
              >
                <SelectTrigger className="w-44">
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
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={!canManage}
                onClick={() =>
                  setRows((prev) => prev.map((r, x) => (x === i ? [...r, ''] : r)))
                }
              >
                <PlusIcon className="size-3.5" />
                Facet
              </Button>
            )}
            <span className="ml-auto flex items-center gap-2">
              <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                {estimate(row) == null
                  ? 'scalar - not countable'
                  : `up to ${estimate(row)!.toLocaleString()} URLs`}
              </span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-muted-foreground hover:text-destructive"
                disabled={!canManage}
                aria-label="Remove combination"
                onClick={() => setRows((prev) => prev.filter((_, x) => x !== i))}
              >
                <Trash2Icon className="size-4" />
              </Button>
            </span>
          </li>
        ))}
      </ul>
      <div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!canManage || options.length < 2}
          onClick={() => setRows((prev) => [...prev, ['', '']])}
        >
          <PlusIcon className="mr-1 size-3.5" />
          Add combination
        </Button>
        {options.length < 2 && (
          <p className="mt-2 text-xs text-muted-foreground">
            Combinations need at least two indexable facets - switch them on above first.
          </p>
        )}
        {totalEstimate > 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            These combinations open up to{' '}
            <span className="font-mono tabular-nums">{totalEstimate.toLocaleString()}</span>{' '}
            crawlable URLs before the min-results guard trims them.
          </p>
        )}
      </div>
    </div>
  )
}

/**
 * The rest of the seo/facets group rendered generically - a key registered into the
 * group on the backend shows up here with no dashboard change, same trick as the
 * search settings page.
 */
function GuardFieldsSection({
  fields,
  scope,
  scopeInput,
  canManage,
}: {
  fields: React.ComponentProps<typeof ConfigField>['field'][]
  scope: { mode: 'GLOBAL' | 'CHANNEL'; channelId?: string; locale?: string }
  scopeInput: ConfigScopeInput | undefined
  canManage: boolean
}) {
  const [setConfigValue] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })
  const [resetConfigValue] = useMutation(ResetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })
  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [resettingPath, setResettingPath] = useState<string | null>(null)

  const dirtyFields = fields.filter(
    (f) => drafts[f.path] !== undefined && drafts[f.path] !== f.value,
  )
  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      dirtyFields.map((f) => ({
        label: f.label,
        before: f.value || '(empty)',
        after: drafts[f.path] || '(empty)',
      })),
    // dirtyFields derives from drafts+fields
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [drafts, fields],
  )

  useSaveSection({
    id: 'facet-guards',
    title: 'Guards & robots tags',
    diff,
    save: async () => {
      await Promise.all(
        dirtyFields.map((f) =>
          setConfigValue({
            variables: { path: f.path, value: drafts[f.path]!, scope: scopeInput },
          }),
        ),
      )
      setDrafts({})
    },
  })

  if (fields.length === 0) {
    return <p className="text-sm text-muted-foreground">No guard settings registered.</p>
  }

  return (
    <div className="grid gap-4">
      {fields.map((f) => (
        <ConfigField
          key={f.path}
          field={f}
          draft={drafts[f.path]}
          onChange={(v) =>
            setDrafts((prev) => {
              const next = { ...prev }
              if (v === undefined) delete next[f.path]
              else next[f.path] = v
              return next
            })
          }
          onReset={async () => {
            setResettingPath(f.path)
            try {
              await resetConfigValue({ variables: { path: f.path, scope: scopeInput } })
            } finally {
              setResettingPath(null)
            }
          }}
          resetting={resettingPath === f.path}
          disabled={!canManage}
          inherited={fieldIsInherited(f, scope.mode, scope.channelId, scope.locale ?? '')}
        />
      ))}
    </div>
  )
}
