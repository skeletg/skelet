import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Loader2Icon, RefreshCwIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { AttributesDocument } from '@/features/catalog/Catalog.gql'
import { LoadError } from '@/shared/errors/LoadError'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import { useSaveConfirm } from '@/shared/saving/useSaveConfirm'
import { ConfigField } from '@/features/settings/ConfigField'
import {
  CatalogAttrProvider,
  type CatalogAttrInfo,
} from '@/features/settings/SearchAttributesEditor'
import { ConfigSectionsDocument } from '@/features/settings/Settings.gql'
import { useConfigDrafts } from '@/features/settings/useConfigDrafts'
import { RunAdminActionDocument } from '@/features/tools/Tools.gql'

// SearchSettings adopts the product-detail visual shape: sticky header with the save action and a vertically-stacked, divider-separated
// body of named sections - Keeps the page feeling like a first-class editor rather than a generic settings panel
//
// The fields are config-registry values, staged and saved through useConfigDrafts exactly like the
// Settings page's - the header Save reviews them in the standard confirm dialog

const SECTION_SLUG = 'search'
const REINDEX_ACTION_SLUG = 'search.reindex'

const GROUP_LABELS: Record<string, { title: string; description: string }> = {
  general: {
    title: 'General',
    description: 'Pagination clamps applied to both product and blog search requests.',
  },
  products: {
    title: 'Product Search',
    description:
      'Per-attribute tuning for the product index. Mixed match types fan out into a bool.should query; uniform settings stay on the faster multi_match path.',
  },
  products_sort: {
    title: 'Product Sort',
    description:
      'Sort options offered on browse pages. First entry is the default. Attribute fields are drawn from Filterable catalog attributes; numeric attributes sort lexicographically.',
  },
  blog: {
    title: 'Blog Search',
    description: 'Per-attribute tuning for blog posts. Same fan-out rules as products.',
  },
  mapping: {
    title: 'Index mapping',
    description:
      'Index-time settings. Changes only take effect after running a reindex - the live index keeps serving its current mapping until then.',
  },
}


// Search config is GLOBAL only - single OpenSearch cluster - so we skip the channel scope picker the
// generic Settings page uses
const SCOPE_INPUT = { kind: 'DEFAULT' as const }

// The registry lives here and the drafts one level down - useConfigDrafts registers through context,
// so it has to run inside the Provider this renders
export function SearchSettings() {
  const registry = useSaveRegistry('Search settings')
  const confirm = useSaveConfirm(registry.saveAll)
  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout>
        <SearchSettingsBody isDirty={registry.isDirty} saving={confirm.saving} onSave={confirm.open} />
        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel="Search engine" sections={registry.dirtySections} />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

function SearchSettingsBody({ isDirty, saving, onSave }: { isDirty: boolean; saving: boolean; onSave: () => void }) {
  const { data, loading, error, refetch } = useQuery(ConfigSectionsDocument, {
    variables: { scope: SCOPE_INPUT },
    notifyOnNetworkStatusChange: true,
  })

  // Catalog attributes marked Searchable=true auto-merge into the product attributes table - Done client-side so the editor always shows a
  // current view of "what's actually searched" without server-side bookkeeping on top of the JSON config
  const attributesQuery = useQuery(AttributesDocument)
  const searchableCatalogAttrs = useMemo<CatalogAttrInfo[]>(() => {
    const list = attributesQuery.data?.attributes ?? []
    return list
      .filter((a) => a.searchable)
      .map((a) => ({ slug: a.slug, name: a.name, inputType: String(a.inputType) }))
  }, [attributesQuery.data])

  const [runAction, { loading: reindexing }] = useMutation(RunAdminActionDocument)

  const searchSection = useMemo(
    () => data?.configSections.find((s) => s.slug === SECTION_SLUG) ?? null,
    [data],
  )
  const sections = useMemo(() => (searchSection ? [searchSection] : undefined), [searchSection])
  const { drafts, onFieldChange, onReset, resettingPath, resetError } = useConfigDrafts({ sections, scopeInput: SCOPE_INPUT })

  const [reindexTarget, setReindexTarget] = useState<'products' | 'blog' | 'all'>('all')
  const [reindexResult, setReindexResult] = useState<{ ok: boolean; message: string } | null>(null)

  const dirtyMappingPending = (searchSection?.groups ?? [])
    .filter((g) => g.slug === 'mapping')
    .some((g) => g.fields.some((f) => drafts[f.path] !== undefined && drafts[f.path] !== f.value))

  async function onReindex() {
    setReindexResult(null)
    try {
      const res = await runAction({
        variables: { slug: REINDEX_ACTION_SLUG, args: { target: reindexTarget } },
      })
      const r = res.data?.runAdminAction
      if (!r) {
        setReindexResult({ ok: false, message: 'No response from reindex action.' })
        return
      }
      setReindexResult({ ok: r.success, message: r.output || 'Done.' })
    } catch (e) {
      setReindexResult({ ok: false, message: e instanceof Error ? e.message : 'Reindex failed.' })
    }
  }

  if (loading && !searchSection) {
    return <p className="p-8 text-center text-sm text-muted-foreground">Loading search settings…</p>
  }
  if (error) {
    return (
      <LoadError
        title="Couldn't load search settings"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  }
  if (!searchSection) {
    return (
      <div className="p-6">
        <Alert>
          <AlertTitle>Search section not registered</AlertTitle>
          <AlertDescription>
            The backend isn't reporting a <code>search</code> config section. Check that the
            search domain's <code>init()</code> registered cleanly at boot.
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  return (
    <>
      <DetailPageHeader
        title="Search Engine"
        badges={
          <>
            <Badge variant="outline" className="gap-1 text-muted-foreground">
              OpenSearch
            </Badge>
            {dirtyMappingPending ? (
              <Badge variant="secondary" className="gap-1">
                Mapping change staged - reindex required
              </Badge>
            ) : null}
          </>
        }
        meta={
          <>
            <span>Per-attribute tuning</span>
            <span>·</span>
            <span>Query-time settings apply live</span>
            <span>·</span>
            <span>Mapping changes require reindex</span>
          </>
        }
        actions={
          <Button type="button" size="sm" disabled={!isDirty || saving} onClick={onSave}>
            {saving ? 'Saving…' : 'Save changes'}
          </Button>
        }
      />

      <DetailPageBody>
        {searchSection.groups.map((group) => {
          const labels = GROUP_LABELS[group.slug] ?? { title: group.label, description: '' }
          const isMapping = group.slug === 'mapping'
          return (
            <DetailPageSection
              key={group.slug}
              title={labels.title}
              description={labels.description}
              trailing={
                isMapping ? (
                  <div className="flex items-center gap-2">
                    <Select
                      value={reindexTarget}
                      onValueChange={(v) => setReindexTarget(v as 'products' | 'blog' | 'all')}
                      disabled={reindexing}
                    >
                      <SelectTrigger size="sm" className="w-44">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="all">All indexes</SelectItem>
                        <SelectItem value="products">Products only</SelectItem>
                        <SelectItem value="blog">Blog posts only</SelectItem>
                      </SelectContent>
                    </Select>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void onReindex()}
                      disabled={reindexing}
                      className="h-8"
                    >
                      {reindexing ? (
                        <Loader2Icon className="size-4 animate-spin" />
                      ) : (
                        <RefreshCwIcon className="size-4" />
                      )}
                      Reindex now
                    </Button>
                  </div>
                ) : null
              }
            >
              {isMapping && reindexResult ? (
                <Alert variant={reindexResult.ok ? 'default' : 'destructive'}>
                  <AlertTitle>{reindexResult.ok ? 'Reindex complete' : 'Reindex failed'}</AlertTitle>
                  <AlertDescription className="font-mono text-xs">
                    {reindexResult.message}
                  </AlertDescription>
                </Alert>
              ) : null}

              {group.fields.length === 0 ? (
                <p className="text-sm italic text-muted-foreground">No fields in this group.</p>
              ) : (
                <CatalogAttrProvider
                  attributes={searchableCatalogAttrs}
                  forPath="search/products/attributes"
                >
                  {/* An inset block, NOT a full-bleed list: the fields used to run edge to edge
                      with a border-y, which is the same rule DetailPageBody draws between
                      sections, so every field read as a section of its own. Indenting them inside
                      a bordered block leaves the full-width rules meaning "section" and nothing
                      else. ConfigField brings its own padding and provenance rail */}
                  <div className="divide-y rounded-md border">
                    {group.fields.map((field) => (
                      <ConfigField
                        key={field.path}
                        field={field}
                        draft={drafts[field.path]}
                        onChange={(value) => onFieldChange(field.path, value)}
                        onReset={() => onReset(field.path)}
                        resetting={resettingPath === field.path}
                        disabled={saving}
                      />
                    ))}
                  </div>
                </CatalogAttrProvider>
              )}
            </DetailPageSection>
          )
        })}
      </DetailPageBody>

      {resetError ? (
        <div className="fixed bottom-6 right-6 z-50 w-full max-w-sm animate-in slide-in-from-bottom-4 fade-in">
          <Alert variant="destructive" className="shadow-2xl">
            <AlertTitle>Couldn't reset</AlertTitle>
            <AlertDescription>{resetError}</AlertDescription>
          </Alert>
        </div>
      ) : null}
    </>
  )
}
