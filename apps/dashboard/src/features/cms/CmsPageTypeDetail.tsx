import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { Input } from '@/components/ui/input'
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
import { Label } from '@/components/ui/label'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useConfigScope } from '@/lib/config-scope'
import {
  AssignCmsAttributeDocument,
  CmsAttributeGroupsDocument,
  CmsAttributesDocument,
  CmsPageTypesDocument,
  DeleteCmsPageTypeDocument,
  SetCmsPageTypeChannelRootDocument,
  SetCmsPageTypeDefaultBlocksDocument,
  UnassignCmsAttributeDocument,
  UpdateCmsPageTypeDocument,
  UpdateCmsPageTypeStructureDocument,
  UpsertCmsPageTypeContentDocument,
  type CmsPageTypeRowFragment,
} from './Cms.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { PageBlockCanvas } from './PageBlockCanvas'
import { PageTypePagesPanel } from './CmsPageTypePages'
import { parseDocument, serializeDocument, type CmsBlock } from './page-blocks'

function typeName(t: CmsPageTypeRowFragment) {
  return t.content?.name ?? t.slug
}

// CmsPageTypeDetail is the standalone /cms/page-types/:id editor - Page types are low-volume, so it reads the full list
// and finds the type by id rather than adding a dedicated query
export function CmsPageTypeDetail() {
  const { id = '' } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')

  const { data, loading, error, refetch } = useQuery(CmsPageTypesDocument)
  const type = (data?.cmsPageTypes ?? []).find((t) => t.id === id)

  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // All four panels stage into this registry and the ONE header Save applies every dirty one
  // through a single review dialog, rather than each carrying a Save of its own
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveAllError, setSaveAllError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveAllError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveAllError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }
  const [deleteType, { loading: deleting }] = useMutation(DeleteCmsPageTypeDocument, {
    refetchQueries: [{ query: CmsPageTypesDocument }],
  })

  async function onDelete() {
    if (!type) return
    setDeleteError(null)
    try {
      await deleteType({ variables: { id: type.id } })
      navigate('/cms/page-types', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (loading) return <LoadingState label="Loading page type…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load page type"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!type)
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/cms/page-types" backLabel="Back to page types" title="Page type not found" />
        <DetailPageBody>
          <DetailPageSection>
            <p className="text-sm text-muted-foreground">
              This page type doesn't exist or was deleted.
            </p>
          </DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    )

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/cms/page-types"
        backLabel="Back to page types"
        title={typeName(type)}
        badges={
          <>
            <code className="font-mono text-xs text-muted-foreground">{type.slug}</code>
            {type.isSystem && <Badge variant="secondary">system</Badge>}
            {type.isSingleton && <Badge variant="secondary">singleton</Badge>}
            {type.rootSlug && <Badge variant="outline">/{type.rootSlug}</Badge>}
          </>
        }
        meta={type.content?.description ? <span>{type.content.description}</span> : undefined}
        actions={
          canManage ? (
            <>
              {!type.isSystem && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => {
                    setDeleteError(null)
                    setConfirmDelete(true)
                  }}
                  disabled={deleting}
                >
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
              )}
              <Button
                type="button"
                size="sm"
                disabled={!isDirty || savingAll}
                onClick={() => setConfirmingSave(true)}
              >
                {savingAll ? 'Saving…' : 'Save changes'}
              </Button>
            </>
          ) : null
        }
      />

      <DetailPageBody>
        <DetailPageSection
          title="Pages"
          description="All pages of this type. Search, sort, and filter by status or tag; open one to edit it."
        >
          <PageTypePagesPanel pageTypeId={type.id} canManage={canManage} />
        </DetailPageSection>

        <DetailPageSection
          title="Naming & display"
          description="The label and icon this type shows under in the CMS, plus its per-locale name."
        >
          <PageTypeDetailsPanel type={type} canManage={canManage} />
        </DetailPageSection>

        <DetailPageSection
          title="URL & behavior"
          description="Give pages of this type a URL root (per-channel optional), and control single-instance behavior."
        >
          <PageTypeStructurePanel type={type} canManage={canManage} />
        </DetailPageSection>

        <DetailPageSection
          title="SEO defaults"
          description="Pages of this type inherit these unless they set their own. Use {title} in the meta-title pattern."
        >
          <PageTypeSEOPanel type={type} canManage={canManage} />
        </DetailPageSection>

        <DetailPageSection
          title="Default content template"
          description="New pages of this type start pre-filled with these blocks (block IDs are regenerated per page)."
        >
          <PageTypeTemplatePanel type={type} canManage={canManage} />
        </DetailPageSection>

        <DetailPageSection
          title="Attributes"
          description="Attributes assigned to this type become editable fields on every page of this type."
        >
          <PageTypeAttributesPanel type={type} canManage={canManage} />
        </DetailPageSection>
      </DetailPageBody>

      <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {typeName(type)}?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the page type and its attribute assignments. It must have no pages.
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
              onClick={(e) => {
                e.preventDefault()
                onDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete page type'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Page type · ${typeName(type)}`}
        sections={dirtySections}
        saving={savingAll}
        error={saveAllError}
        onCancel={() => { if (!savingAll) { setConfirmingSave(false); setSaveAllError(null) } }}
        onConfirm={onConfirmSave}
      />
    </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// PageTypeStructurePanel edits the URL root (global + per-channel overrides)
// and the singleton flag - Uses the partial-update mutation so it only writes
// PageTypeDetailsPanel owns the presentation bits: the sidebar icon and
// ordering (updateCMSPageType) plus the localized name/description
// (upsertCMSPageTypeContent, one row per locale)
function PageTypeDetailsPanel({
  type,
  canManage,
}: {
  type: CmsPageTypeRowFragment
  canManage: boolean
}) {
  const refetch = [{ query: CmsPageTypesDocument }]
  const [updateType] = useMutation(UpdateCmsPageTypeDocument, { refetchQueries: refetch })
  const [upsertContent] = useMutation(UpsertCmsPageTypeContentDocument, { refetchQueries: refetch })
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  const locale = effectiveLocale || globalDefaultLocale || 'en'
  const existing = type.allContent.find((c) => c.locale === locale)

  const [icon, setIcon] = useState(type.icon ?? '')
  const [position, setPosition] = useState(String(type.position))
  const [name, setName] = useState(existing?.name ?? '')
  const [description, setDescription] = useState(existing?.description ?? '')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [ok, setOk] = useState(false)

  useSaveSection({
    id: 'page-type-details',
    title: 'Details',
    diff: fieldDiff(
      {
        icon: type.icon ?? '',
        position: String(type.position),
        name: existing?.name ?? '',
        description: existing?.description ?? '',
      },
      { icon, position, name, description },
      [
        { label: 'Icon', value: (o) => o.icon },
        { label: 'Position', value: (o) => o.position },
        { label: 'Name', value: (o) => o.name },
        { label: 'Description', value: (o) => o.description },
      ],
    ),
    save: () => save(),
  })

  async function save() {
    setError(null)
    setOk(false)
    setSaving(true)
    try {
      await updateType({
        variables: {
          input: { id: type.id, icon: icon.trim() || null, position: Number(position) || 0 },
        },
      })
      // name is required by the mutation, so only send content when set
      if (name.trim()) {
        await upsertContent({
          variables: {
            input: {
              pageTypeId: type.id,
              locale,
              name: name.trim(),
              description: description.trim() || null,
            },
          },
        })
      }
      setOk(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
      throw e // the header dialog keeps itself open and surfaces the message
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="grid gap-4">
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-3 sm:grid-cols-[1fr_8rem]">
        <div className="grid gap-1.5">
          <Label htmlFor="pt-name">Name ({locale})</Label>
          <Input
            id="pt-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={type.slug}
            disabled={!canManage}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="pt-position">Position</Label>
          <Input
            id="pt-position"
            type="number"
            value={position}
            onChange={(e) => setPosition(e.target.value)}
            disabled={!canManage}
          />
        </div>
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="pt-desc">Description ({locale})</Label>
        <Input
          id="pt-desc"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          disabled={!canManage}
        />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="pt-icon">Icon</Label>
        <Input
          id="pt-icon"
          value={icon}
          onChange={(e) => setIcon(e.target.value)}
          placeholder="lucide icon name, e.g. FileText"
          disabled={!canManage}
        />
      </div>
      {type.allContent.length > 0 && (
        <p className="text-xs text-muted-foreground">
          Translations stored: {type.allContent.map((c) => c.locale).join(', ')}
        </p>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          {/* No per-panel Save - the page header applies every staged panel through one dialog */}
          {saving ? <span className="text-xs text-muted-foreground">Saving…</span> : null}
          {ok && <span className="text-xs text-emerald-600">Saved.</span>}
        </div>
      )}
    </div>
  )
}

// the fields it owns (SEO defaults are a separate panel)
function PageTypeStructurePanel({
  type,
  canManage,
}: {
  type: CmsPageTypeRowFragment
  canManage: boolean
}) {
  const refetch = [{ query: CmsPageTypesDocument }]
  const { data: chData } = useQuery(ChannelsDocument)
  const channels = (chData?.channels ?? []).filter((c) => c.isActive)
  const [updateStructure] = useMutation(UpdateCmsPageTypeStructureDocument, { refetchQueries: refetch })
  const [setChannelRoot] = useMutation(SetCmsPageTypeChannelRootDocument, { refetchQueries: refetch })

  const currentRoots: Record<string, string> = {}
  for (const cr of type.channelRoots) currentRoots[cr.channelId] = cr.rootSlug

  const [rootSlug, setRootSlug] = useState(type.rootSlug ?? '')
  const [isSingleton, setIsSingleton] = useState(type.isSingleton)
  const [roots, setRoots] = useState<Record<string, string>>(currentRoots)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [ok, setOk] = useState(false)

  useSaveSection({
    id: 'page-type-structure',
    title: 'Structure',
    diff: fieldDiff(
      {
        rootSlug: type.rootSlug ?? '',
        isSingleton: type.isSingleton,
        roots: Object.entries(currentRoots).map(([k, v]) => `${k}=${v}`).sort().join(', '),
      },
      {
        rootSlug,
        isSingleton,
        roots: Object.entries(roots).map(([k, v]) => `${k}=${(v ?? '').trim()}`).sort().join(', '),
      },
      [
        { label: 'Root slug', value: (o) => o.rootSlug },
        { label: 'Singleton', value: (o) => o.isSingleton },
        { label: 'Per-channel roots', value: (o) => o.roots },
      ],
    ),
    save: () => save(),
  })

  async function save() {
    setError(null)
    setOk(false)
    setSaving(true)
    try {
      await updateStructure({ variables: { input: { id: type.id, rootSlug, isSingleton } } })
      for (const c of channels) {
        const next = (roots[c.id] ?? '').trim()
        if (next !== (currentRoots[c.id] ?? '')) {
          await setChannelRoot({ variables: { pageTypeId: type.id, channelId: c.id, rootSlug: next } })
        }
      }
      setOk(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
      throw e // the header dialog keeps itself open and surfaces the message
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="grid max-w-xl gap-4">
      <div className="grid gap-1.5">
        <Label htmlFor="root-slug" className="text-sm font-medium">URL root</Label>
        <Input
          id="root-slug"
          value={rootSlug}
          onChange={(e) => setRootSlug(e.target.value)}
          placeholder="e.g. help - pages live at /help/<slug>"
          disabled={!canManage}
          className="h-9"
        />
        <span className="text-xs text-muted-foreground">
          Empty = bare paths. Pages of this type are served at <code>/{rootSlug || '<slug>'}{rootSlug ? '/<slug>' : ''}</code>.
        </span>
      </div>

      {channels.length > 1 && (
        <div className="grid gap-1.5">
          <Label className="text-sm font-medium">Per-channel root overrides</Label>
          <ul className="grid gap-1.5">
            {channels.map((c) => (
              <li key={c.id} className="flex items-center gap-2">
                <span className="w-32 shrink-0 truncate text-sm">{c.name}</span>
                <Input
                  value={roots[c.id] ?? ''}
                  onChange={(e) => setRoots((prev) => ({ ...prev, [c.id]: e.target.value }))}
                  placeholder={rootSlug || '(global root)'}
                  disabled={!canManage}
                  className="h-9"
                />
              </li>
            ))}
          </ul>
          <span className="text-xs text-muted-foreground">Blank = use the global root above on that channel.</span>
        </div>
      )}

      <div className="flex items-center gap-2">
        <Switch id="singleton" checked={isSingleton} onCheckedChange={setIsSingleton} disabled={!canManage} />
        <Label htmlFor="singleton" className="text-sm">Singleton - allow only one page of this type</Label>
      </div>

      {error && (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          {/* No per-panel Save - the page header applies every staged panel through one dialog */}
          {saving ? <span className="text-xs text-muted-foreground">Saving…</span> : null}
          {ok && <span className="text-xs text-emerald-600">Saved.</span>}
        </div>
      )}
    </div>
  )
}

// PageTypeSEOPanel edits the inherited meta-title pattern + noindex default
function PageTypeSEOPanel({
  type,
  canManage,
}: {
  type: CmsPageTypeRowFragment
  canManage: boolean
}) {
  const [updateStructure] = useMutation(UpdateCmsPageTypeStructureDocument, {
    refetchQueries: [{ query: CmsPageTypesDocument }],
  })
  const [metaTitle, setMetaTitle] = useState(type.defaultMetaTitle ?? '')
  const [noindex, setNoindex] = useState(type.defaultSeoNoindex)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [ok, setOk] = useState(false)

  useSaveSection({
    id: 'page-type-seo',
    title: 'SEO defaults',
    diff: fieldDiff(
      { metaTitle: type.defaultMetaTitle ?? '', noindex: type.defaultSeoNoindex },
      { metaTitle, noindex },
      [
        { label: 'Default meta title', value: (o) => o.metaTitle },
        { label: 'Default noindex', value: (o) => o.noindex },
      ],
    ),
    save: () => save(),
  })

  async function save() {
    setError(null)
    setOk(false)
    setSaving(true)
    try {
      await updateStructure({
        variables: { input: { id: type.id, defaultMetaTitle: metaTitle, defaultSeoNoindex: noindex } },
      })
      setOk(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
      throw e // the header dialog keeps itself open and surfaces the message
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="grid max-w-xl gap-4">
      <div className="grid gap-1.5">
        <Label htmlFor="default-meta-title" className="text-sm font-medium">Default meta title</Label>
        <Input
          id="default-meta-title"
          value={metaTitle}
          onChange={(e) => setMetaTitle(e.target.value)}
          placeholder="e.g. {title} - Help Center"
          disabled={!canManage}
          className="h-9"
        />
      </div>
      <div className="flex items-center gap-2">
        <Switch id="default-noindex" checked={noindex} onCheckedChange={setNoindex} disabled={!canManage} />
        <Label htmlFor="default-noindex" className="text-sm">Default to noindex (hide from search engines)</Label>
      </div>
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          {/* No per-panel Save - the page header applies every staged panel through one dialog */}
          {saving ? <span className="text-xs text-muted-foreground">Saving…</span> : null}
          {ok && <span className="text-xs text-emerald-600">Saved.</span>}
        </div>
      )}
    </div>
  )
}

// PageTypeTemplatePanel authors the starter block document seeded into new
// pages of this type, reusing the page builder canvas
function PageTypeTemplatePanel({
  type,
  canManage,
}: {
  type: CmsPageTypeRowFragment
  canManage: boolean
}) {
  const [setDefaultBlocks] = useMutation(SetCmsPageTypeDefaultBlocksDocument, {
    refetchQueries: [{ query: CmsPageTypesDocument }],
  })
  const [blocks, setBlocks] = useState<CmsBlock[]>(() => parseDocument(type.defaultBlocks)?.blocks ?? [])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // A block document can't diff field-by-field, so it reports its block count and type order -
  // enough to see the starter template changed, and the save thunk reads live state
  useSaveSection({
    id: 'page-type-template',
    title: 'Starter template',
    diff: fieldDiff(
      { blocks: (parseDocument(type.defaultBlocks)?.blocks ?? []).map((b) => b.type).join(', ') },
      { blocks: blocks.map((b) => b.type).join(', ') },
      [{ label: 'Starter blocks', value: (o) => o.blocks }],
    ),
    save: () => save(),
  })
  const [ok, setOk] = useState(false)

  async function save() {
    setError(null)
    setOk(false)
    setSaving(true)
    try {
      const doc = blocks.length > 0 ? serializeDocument(blocks) : null
      await setDefaultBlocks({ variables: { pageTypeId: type.id, blocks: doc } })
      setOk(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Save failed.')
      throw e // the header dialog keeps itself open and surfaces the message
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="grid gap-3">
      <PageBlockCanvas blocks={blocks} onChange={setBlocks} disabled={!canManage} />
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Save failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      {canManage && (
        <div className="flex items-center gap-3">
          {/* No per-panel Save - the page header applies every staged panel through one dialog */}
          {saving ? <span className="text-xs text-muted-foreground">Saving…</span> : null}
          {ok && <span className="text-xs text-emerald-600">Saved.</span>}
        </div>
      )}
    </div>
  )
}

// PageTypeAttributesPanel manages the attributes assigned to a page type: the assigned list (with remove) plus an
// attribute picker + Required switch to assign new ones - Reused from the former inline page-types console
function PageTypeAttributesPanel({
  type,
  canManage,
}: {
  type: CmsPageTypeRowFragment
  canManage: boolean
}) {
  const { data: attrsData } = useQuery(CmsAttributesDocument)
  const allAttrs = attrsData?.cmsAttributes ?? []
  const [assignMut] = useMutation(AssignCmsAttributeDocument, {
    refetchQueries: [{ query: CmsPageTypesDocument }, { query: CmsAttributesDocument }],
  })
  const [unassignMut] = useMutation(UnassignCmsAttributeDocument, {
    refetchQueries: [{ query: CmsPageTypesDocument }, { query: CmsAttributesDocument }],
  })

  const [picked, setPicked] = useState('')
  const [required, setRequired] = useState(false)
  // Groups are the form-layout buckets an assigned attribute can sit in
  const [groupId, setGroupId] = useState('')
  const { data: groupsData } = useQuery(CmsAttributeGroupsDocument)
  const groups = groupsData?.cmsAttributeGroups ?? []
  const [condAttr, setCondAttr] = useState('')
  const [condValue, setCondValue] = useState('')
  const assignedIds = new Set(type.attributes.map((a) => a.attribute.id))
  const candidates = allAttrs.filter((a) => !assignedIds.has(a.id))
  // Possible condition sources = already-assigned select/boolean attributes
  const conditionSources = type.attributes.filter((pta) =>
    ['SELECT', 'MULTISELECT', 'BOOLEAN'].includes(pta.attribute.inputType),
  )
  const attrName = (id: string) =>
    type.attributes.find((pta) => pta.attribute.id === id)?.attribute.content?.name ??
    type.attributes.find((pta) => pta.attribute.id === id)?.attribute.slug ??
    id

  return (
    <div className="grid gap-3">
      <Label className="text-xs uppercase tracking-wide text-muted-foreground">Assigned attributes</Label>
      {type.attributes.length === 0 ? (
        <p className="text-sm text-muted-foreground">No attributes assigned.</p>
      ) : (
        <ul className="grid gap-1">
          {type.attributes.map((pta, i) => {
            const aname = pta.attribute.content?.name ?? pta.attribute.slug
            return (
              <li
                key={pta.attribute.id}
                className="flex items-center gap-2 rounded-md border px-2 py-1.5"
              >
                <span className="text-xs text-muted-foreground">#{i + 1}</span>
                <span className="flex-1 truncate text-sm font-medium">{aname}</span>
                <code className="text-xs text-muted-foreground">{pta.attribute.slug}</code>
                <Badge variant="outline" className="text-xs">
                  {pta.attribute.inputType.toLowerCase()}
                </Badge>
                {pta.isRequired && (
                  <Badge variant="secondary" className="text-xs">
                    required
                  </Badge>
                )}
                {pta.groupId && (
                  <Badge variant="outline" className="text-xs">
                    {groups.find((g) => g.id === pta.groupId)?.slug ?? 'group'}
                  </Badge>
                )}
                {pta.conditionAttributeId && (
                  <Badge variant="outline" className="text-xs" title="Conditional visibility">
                    if {attrName(pta.conditionAttributeId)} = {pta.conditionValue}
                  </Badge>
                )}
                {canManage && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-muted-foreground hover:text-destructive"
                    onClick={() =>
                      unassignMut({
                        variables: { pageTypeId: type.id, attributeId: pta.attribute.id },
                      })
                    }
                  >
                    <Trash2Icon className="size-3.5" />
                  </Button>
                )}
              </li>
            )
          })}
        </ul>
      )}
      {canManage && candidates.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <Select value={picked} onValueChange={setPicked}>
            <SelectTrigger className="w-64">
              <SelectValue placeholder="Add attribute…" />
            </SelectTrigger>
            <SelectContent>
              {candidates.map((a) => (
                <SelectItem key={a.id} value={a.id}>
                  {a.content?.name ?? a.slug}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="flex items-center gap-2">
            <Switch checked={required} onCheckedChange={setRequired} />
            <Label className="text-xs">Required</Label>
          </div>
          {groups.length > 0 && (
            <Select value={groupId} onValueChange={setGroupId}>
              <SelectTrigger className="h-8 w-40">
                <SelectValue placeholder="(no group)" />
              </SelectTrigger>
              <SelectContent>
                {groups.map((g) => (
                  <SelectItem key={g.id} value={g.id}>
                    {g.slug}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          {conditionSources.length > 0 && (
            <div className="flex items-center gap-2">
              <Label className="text-xs text-muted-foreground">Show only if</Label>
              <Select value={condAttr} onValueChange={setCondAttr}>
                <SelectTrigger className="h-8 w-40">
                  <SelectValue placeholder="(always)" />
                </SelectTrigger>
                <SelectContent>
                  {conditionSources.map((s) => (
                    <SelectItem key={s.attribute.id} value={s.attribute.id}>
                      {s.attribute.content?.name ?? s.attribute.slug}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {condAttr && (
                <Input
                  value={condValue}
                  onChange={(e) => setCondValue(e.target.value)}
                  placeholder="= value (slug / true)"
                  className="h-8 w-36 text-xs"
                />
              )}
            </div>
          )}
          <Button
            size="sm"
            disabled={!picked}
            onClick={async () => {
              if (!picked) return
              await assignMut({
                variables: {
                  input: {
                    pageTypeId: type.id,
                    attributeId: picked,
                    position: type.attributes.length,
                    isRequired: required,
                    groupId: groupId || null,
                    conditionAttributeId: condAttr || null,
                    conditionValue: condAttr ? condValue : null,
                  },
                },
              })
              setPicked('')
              setRequired(false)
              setCondAttr('')
              setCondValue('')
            }}
          >
            <PlusIcon className="size-4" />
            Assign
          </Button>
        </div>
      )}
    </div>
  )
}
