import { useMemo, useState, useId } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
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
import { FieldError } from '@/components/form/fields'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
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
import {
  PAGE_TYPE_LABELS,
  normalizeRoot,
  pageTypeChanges,
  pageTypeDiff,
  pageTypeSchema,
  toPageTypeValues,
} from './schemas/pageType'

function typeName(t: CmsPageTypeRowFragment) {
  return t.content?.name ?? t.slug
}

// CmsPageTypeDetail is the standalone /cms/page-types/:id editor - Page types are low-volume, so it reads the full list
// and finds the type by id rather than adding a dedicated query
export function CmsPageTypeDetail() {
  const { id = '' } = useParams<{ id: string }>()
  const { data, loading, error, refetch } = useQuery(CmsPageTypesDocument)
  const type = (data?.cmsPageTypes ?? []).find((t) => t.id === id)
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  const locale = effectiveLocale || globalDefaultLocale || 'en'

  if (loading && !type) return <LoadingState label="Loading page type…" />
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
            <p className="text-sm text-muted-foreground">This page type doesn't exist or was deleted.</p>
          </DetailPageSection>
        </DetailPageBody>
      </DetailPageLayout>
    )
  // Keyed on the locale too - the name and description are the edited locale's translation
  return <PageTypeForm key={`${type.id}:${locale}`} type={type} locale={locale} />
}

// Naming, URL structure, SEO defaults and the starter template are one form behind the header
// Save; each part goes through its own mutation, only when it changed. The page list and the
// attribute rows act on their own
function PageTypeForm({ type, locale }: { type: CmsPageTypeRowFragment; locale: string }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')

  const refetchQueries = [{ query: CmsPageTypesDocument }]
  const { data: chData } = useQuery(ChannelsDocument)
  const channels = useMemo(() => (chData?.channels ?? []).filter((c) => c.isActive), [chData])
  const [updateType] = useMutation(UpdateCmsPageTypeDocument, { refetchQueries })
  // These answer with the type hydrated exactly as the list hydrates it, but still refetch: they run
  // after updateType in the same save, and its bare-row answer can otherwise land last through its
  // own refetch
  const [upsertContent] = useMutation(UpsertCmsPageTypeContentDocument, { refetchQueries })
  const [updateStructure] = useMutation(UpdateCmsPageTypeStructureDocument, { refetchQueries })
  const [setChannelRoot] = useMutation(SetCmsPageTypeChannelRootDocument, { refetchQueries })
  const [setDefaultBlocks] = useMutation(SetCmsPageTypeDefaultBlocksDocument, { refetchQueries })
  const [deleteType, { loading: deleting }] = useMutation(DeleteCmsPageTypeDocument, { refetchQueries })

  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const baseline = useMemo(
    () => toPageTypeValues(type, locale, parseDocument(type.defaultBlocks)?.blocks ?? []),
    [type, locale],
  )
  const diff = useMemo(() => pageTypeDiff((id) => channels.find((c) => c.id === id)?.name ?? id), [channels])

  const record = useRecordForm({
    id: 'page-type',
    title: 'Page type',
    label: `Page type · ${typeName(type)}`,
    isEdit: true,
    baseline,
    schema: pageTypeSchema,
    diff,
    persist: { routeKey: `cms-page-type:${type.id}:${locale}` },
    submit: async (v) => {
      const changes = pageTypeChanges(baseline, v)
      if (changes.display) {
        await updateType({ variables: { input: { id: type.id, icon: v.icon.trim() || null, position: Number(v.position) || 0 } } })
      }
      if (changes.content && v.name.trim()) {
        await upsertContent({
          variables: { input: { pageTypeId: type.id, locale, name: v.name.trim(), description: v.description.trim() || null } },
        })
      }
      if (changes.structure) {
        await updateStructure({
          variables: {
            input: {
              id: type.id,
              rootSlug: normalizeRoot(v.rootSlug),
              isSingleton: v.isSingleton,
              defaultMetaTitle: v.defaultMetaTitle,
              defaultSeoNoindex: v.defaultSeoNoindex,
            },
          },
        })
      }
      for (const r of changes.channelRoots) {
        await setChannelRoot({ variables: { pageTypeId: type.id, channelId: r.channelId, rootSlug: r.rootSlug } })
      }
      if (changes.blocks) {
        const blocks = v.blocks as CmsBlock[]
        await setDefaultBlocks({ variables: { pageTypeId: type.id, blocks: blocks.length > 0 ? serializeDocument(blocks) : null } })
      }
    },
  })
  const { form } = record
  const rootSlug = useStore(form.store, (s) => normalizeRoot(s.values.rootSlug))

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteType({ variables: { id: type.id } })
      navigate('/cms/page-types', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <RecordFormPage
      record={record}
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
        canManage &&
        !type.isSystem && (
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
        )
      }
      createLabel="Save changes"
      labels={PAGE_TYPE_LABELS}
      // The page list's search stays usable read-only, so only the form's own sections disable
      hideSave={!canManage}
      after={
        <DetailPageSection
          title="Attributes"
          description="Attributes assigned to this type become editable fields on every page of this type."
        >
          <PageTypeAttributesPanel type={type} canManage={canManage} />
        </DetailPageSection>
      }
    >
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
        <fieldset disabled={!canManage} className="grid gap-4">
          <div className="grid items-start gap-3 sm:grid-cols-[1fr_8rem]">
            <form.AppField name="name">{(f) => <f.TextField id="pt-name" label={`Name (${locale})`} placeholder={type.slug} />}</form.AppField>
            <form.AppField name="position">{(f) => <f.TextField id="pt-position" label="Position" type="number" />}</form.AppField>
          </div>
          <form.AppField name="description">{(f) => <f.TextField id="pt-desc" label={`Description (${locale})`} />}</form.AppField>
          <form.AppField name="icon">
            {(f) => <f.TextField id="pt-icon" label="Icon" placeholder="lucide icon name, e.g. FileText" />}
          </form.AppField>
          {type.allContent.length > 0 && (
            <p className="text-xs text-muted-foreground">Translations stored: {type.allContent.map((c) => c.locale).join(', ')}</p>
          )}
        </fieldset>
      </DetailPageSection>

      <DetailPageSection
        title="URL & behavior"
        description="Give pages of this type a URL root (per-channel optional), and control single-instance behavior."
      >
        <fieldset disabled={!canManage} className="grid max-w-xl gap-4">
          <form.AppField name="rootSlug">
            {(f) => (
              <f.TextField
                id="root-slug"
                label="URL root"
                placeholder="e.g. help - pages live at /help/<slug>"
                hint={
                  <>
                    Empty = bare paths. Pages of this type are served at <code>/{rootSlug || '<slug>'}{rootSlug ? '/<slug>' : ''}</code>.
                  </>
                }
              />
            )}
          </form.AppField>

          {channels.length > 1 && (
            <div className="grid gap-1.5">
              <Label className="text-sm font-medium">Per-channel root overrides</Label>
              <ul className="grid gap-1.5">
                {channels.map((c) => (
                  <li key={c.id} className="flex items-start gap-2">
                    <span className="w-32 shrink-0 truncate pt-2 text-sm">{c.name}</span>
                    <form.Field name={`channelRoots.${c.id}`}>
                      {(f) => (
                        <div className="grid flex-1 gap-1">
                          <Input
                            aria-label={`Root on ${c.name}`}
                            value={f.state.value ?? ''}
                            onChange={(e) => f.handleChange(e.target.value)}
                            onBlur={f.handleBlur}
                            placeholder={rootSlug || '(global root)'}
                            className="h-9"
                          />
                          <FieldError field={f} />
                        </div>
                      )}
                    </form.Field>
                  </li>
                ))}
              </ul>
              <span className="text-xs text-muted-foreground">Blank = use the global root above on that channel.</span>
            </div>
          )}

          <form.Field name="isSingleton">
            {(f) => (
              <div className="flex items-center gap-2">
                <Switch id="singleton" checked={f.state.value} onCheckedChange={f.handleChange} />
                <Label htmlFor="singleton" className="text-sm">
                  Singleton - allow only one page of this type
                </Label>
              </div>
            )}
          </form.Field>
        </fieldset>
      </DetailPageSection>

      <DetailPageSection
        title="SEO defaults"
        description="Pages of this type inherit these unless they set their own. Use {title} in the meta-title pattern."
      >
        <fieldset disabled={!canManage} className="grid max-w-xl gap-4">
          <form.AppField name="defaultMetaTitle">
            {(f) => <f.TextField id="default-meta-title" label="Default meta title" placeholder="e.g. {title} - Help Center" />}
          </form.AppField>
          <form.Field name="defaultSeoNoindex">
            {(f) => (
              <div className="flex items-center gap-2">
                <Switch id="default-noindex" checked={f.state.value} onCheckedChange={f.handleChange} />
                <Label htmlFor="default-noindex" className="text-sm">
                  Default to noindex (hide from search engines)
                </Label>
              </div>
            )}
          </form.Field>
        </fieldset>
      </DetailPageSection>

      <DetailPageSection
        title="Default content template"
        description="New pages of this type start pre-filled with these blocks (block IDs are regenerated per page)."
      >
        <form.Field name="blocks">
          {(f) => <PageBlockCanvas blocks={f.state.value as CmsBlock[]} onChange={(next) => f.handleChange(next)} disabled={!canManage} />}
        </form.Field>
      </DetailPageSection>

      <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {typeName(type)}?</AlertDialogTitle>
            <AlertDialogDescription>This removes the page type and its attribute assignments. It must have no pages.</AlertDialogDescription>
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
                void onDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete page type'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </RecordFormPage>
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
  const fid = useId()
  const { data: attrsData } = useQuery(CmsAttributesDocument)
  const allAttrs = attrsData?.cmsAttributes ?? []
  const [assignMut] = useMutation(AssignCmsAttributeDocument, { refetchQueries: [{ query: CmsAttributesDocument }] })
  const [unassignMut] = useMutation(UnassignCmsAttributeDocument, { refetchQueries: [{ query: CmsAttributesDocument }] })

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
            <Switch id={`${fid}-required`} checked={required} onCheckedChange={setRequired} />
            <Label htmlFor={`${fid}-required`} className="text-xs">Required</Label>
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
              <Label htmlFor={`${fid}-show-only-if`} className="text-xs text-muted-foreground">Show only if</Label>
              <Select value={condAttr} onValueChange={setCondAttr}>
                <SelectTrigger id={`${fid}-show-only-if`} className="h-8 w-40">
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
            className="h-10"
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
