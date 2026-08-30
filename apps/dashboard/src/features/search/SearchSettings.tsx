import { useCallback, useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Loader2Icon, RefreshCwIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
import { ConfigField, type ConfigFieldData } from '@/features/settings/ConfigField'
import {
  CatalogAttrProvider,
  type CatalogAttrInfo,
} from '@/features/settings/SearchAttributesEditor'
import {
  ConfigSectionsDocument,
  ResetConfigValueDocument,
  SetConfigValueDocument,
} from '@/features/settings/Settings.gql'
import { RunAdminActionDocument } from '@/features/tools/Tools.gql'

// SearchSettings adopts the product-detail visual shape: sticky header with the save action and a vertically-stacked, divider-separated
// body of named sections - Keeps the page feeling like a first-class editor rather than a generic settings panel

type PendingChange = { field: ConfigFieldData; before: string; after: string }
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

export function SearchSettings() {
  // Search config is GLOBAL only - single OpenSearch cluster - so we
  // skip the channel scope picker the generic Settings page uses
  const scopeInput = useMemo(() => ({ kind: 'DEFAULT' as const }), [])

  const { data, loading, error, refetch } = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
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

  const [setConfigValue] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })
  const [resetConfigValue] = useMutation(ResetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })
  const [runAction, { loading: reindexing }] = useMutation(RunAdminActionDocument)

  const searchSection = useMemo(
    () => data?.configSections.find((s) => s.slug === SECTION_SLUG) ?? null,
    [data],
  )

  const [drafts, setDrafts] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [resettingPath, setResettingPath] = useState<string | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [reindexTarget, setReindexTarget] = useState<'products' | 'blog' | 'all'>('all')
  const [reindexResult, setReindexResult] = useState<{ ok: boolean; message: string } | null>(null)

  const allFields = useMemo(() => {
    if (!searchSection) return [] as ConfigFieldData[]
    return searchSection.groups.flatMap((g) => g.fields)
  }, [searchSection])

  // Drop staged drafts if the scope input ever changes (state adjustment
  // during render, keyed on the loaded settings)
  const [prevScopeInput, setPrevScopeInput] = useState(scopeInput)
  if (prevScopeInput !== scopeInput) {
    setPrevScopeInput(scopeInput)
    setDrafts({})
    setSaveError(null)
  }

  const dirtyCount = useMemo(() => {
    let count = 0
    for (const field of allFields) {
      if (drafts[field.path] !== undefined && drafts[field.path] !== field.value) count += 1
    }
    return count
  }, [allFields, drafts])

  const dirtyMappingPending = useMemo(
    () =>
      Object.entries(drafts).some(([path, val]) => {
        const f = allFields.find((entry) => entry.path === path)
        return f && f.group === 'mapping' && val !== f.value
      }),
    [allFields, drafts],
  )

  const onFieldChange = useCallback((path: string, next: string | undefined) => {
    setDrafts((prev) => {
      const nextDrafts = { ...prev }
      if (next === undefined) delete nextDrafts[path]
      else nextDrafts[path] = next
      return nextDrafts
    })
  }, [])

  async function onReset(path: string) {
    setResettingPath(path)
    setSaveError(null)
    try {
      await resetConfigValue({ variables: { path, scope: scopeInput } })
      onFieldChange(path, undefined)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Reset failed.')
    } finally {
      setResettingPath(null)
    }
  }

  const pendingChanges = useMemo<PendingChange[]>(() => {
    const out: PendingChange[] = []
    for (const [path, after] of Object.entries(drafts)) {
      const field = allFields.find((entry) => entry.path === path)
      if (!field) continue
      if (after === field.value) continue
      out.push({ field, before: field.value, after })
    }
    return out
  }, [allFields, drafts])

  function onSaveClick() {
    if (pendingChanges.length === 0) return
    for (const { field, after } of pendingChanges) {
      if (field.type === 'JSON' || field.type === 'SEARCH_ATTRIBUTES') {
        try {
          JSON.parse(after)
        } catch {
          setSaveError(`Invalid JSON at ${field.path}`)
          return
        }
      }
    }
    setSaveError(null)
    setConfirmOpen(true)
  }

  async function onConfirmSave() {
    if (pendingChanges.length === 0) {
      setConfirmOpen(false)
      return
    }
    setSaving(true)
    setSaveError(null)
    try {
      await Promise.all(
        pendingChanges.map(({ field, after }) =>
          setConfigValue({ variables: { path: field.path, value: after, scope: scopeInput } }),
        ),
      )
      setDrafts({})
      setConfirmOpen(false)
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Save failed.')
    } finally {
      setSaving(false)
    }
  }

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
    <DetailPageLayout>
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
          <Button
            type="button"
            size="sm"
            disabled={dirtyCount === 0 || saving}
            onClick={onSaveClick}
          >
            {saving
              ? 'Saving…'
              : dirtyCount > 0
                ? `Save changes (${dirtyCount})`
                : 'No changes'}
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
                      <SelectTrigger className="h-9 w-44">
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

      {saveError ? (
        <div className="fixed bottom-6 right-6 z-50 w-full max-w-sm animate-in slide-in-from-bottom-4 fade-in">
          <Alert variant="destructive" className="shadow-2xl">
            <AlertTitle>Couldn't save</AlertTitle>
            <AlertDescription>{saveError}</AlertDescription>
          </Alert>
        </div>
      ) : null}

      <AlertDialog open={confirmOpen} onOpenChange={(open) => !saving && setConfirmOpen(open)}>
        <AlertDialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
          <AlertDialogHeader>
            <AlertDialogTitle className="text-xl">Confirm changes</AlertDialogTitle>
            <AlertDialogDescription>
              {pendingChanges.some((c) => c.field.group === 'mapping')
                ? 'Mapping changes will only take effect after running Reindex now.'
                : 'These changes apply live; no reindex is required.'}
            </AlertDialogDescription>
          </AlertDialogHeader>

          <div className="grid gap-3">
            {pendingChanges.map(({ field, before, after }) => (
              <div key={field.path} className="rounded-lg border p-3">
                <div className="mb-1 flex items-center justify-between gap-3">
                  <p className="font-medium">{field.label}</p>
                  <code className="text-[10px] text-muted-foreground">{field.path}</code>
                </div>
                <div className="grid gap-2 text-sm">
                  <div>
                    <span className="text-muted-foreground">Before:</span>{' '}
                    <code className="text-xs">{truncate(before) || <em>empty</em>}</code>
                  </div>
                  <div>
                    <span className="text-muted-foreground">After:</span>{' '}
                    <code className="text-xs">{truncate(after) || <em>empty</em>}</code>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={saving}
              onClick={(e) => {
                e.preventDefault()
                void onConfirmSave()
              }}
            >
              {saving ? 'Saving…' : 'Confirm changes'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}

function truncate(value: string): string {
  if (value.length <= 200) return value
  return value.slice(0, 200) + '…'
}
