import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  CheckCircle2Icon,
  CircleOffIcon,
  CopyIcon,
  RotateCcwIcon,
  Trash2Icon,
  WorkflowIcon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
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
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { useFeatures } from '@/shared/features/useFeatures'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry } from '@/shared/saving/saveRegistry'
import {
  AiStatusDocument,
  CloneCmsPageDocument,
  CmsPageDocument,
  CmsPagesDocument,
  TranslateCmsPageDocument,
  CmsPageTypeDocument,
  DeleteCmsPageDocument,
  PublishCmsPageDocument,
  RebuildCmsPageDocument,
  SetCmsPageRelationshipsDocument,
  SetCmsPageSelectionsDocument,
  UnpublishCmsPageDocument,
  UpdateCmsPageDocument,
  UpsertCmsPageContentDocument,
} from './Cms.gql'
import { PageBlockCanvas } from './PageBlockCanvas'
import { CmsPreviewPanel } from './CmsPreviewPanel'
import { parseDocument, type CmsBlock } from './page-blocks'
import { BasicsCard } from './page-detail/BasicsCard'
import { ContentCard } from './page-detail/ContentCard'
import { SelectionsCard } from './page-detail/SelectionsCard'
import { RelationshipsCard } from './page-detail/RelationshipsCard'
import { CmsPageExperimentSection } from './page-detail/ExperimentSection'
import { CmsPageWorkflowSection, CmsPageCommentsSection } from './page-detail/WorkflowSection'
import { CmsPageTagsSection } from './page-detail/TagsSection'
import { CmsPageChannelsSection } from './page-detail/ChannelsSection'
import { CmsPageHistorySchedule } from './page-detail/HistorySchedule'
import { workflowLabel } from './page-detail/shared'
import { PluginSections } from '@/plugins/detail-sections'

export function CmsPageDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const { enabled: featureEnabled } = useFeatures()
  const canManage = has('cms.manage')
  const canApprove = has('cms.approve')
  const { effectiveLocale, globalDefaultLocale, selectedChannel } = useConfigScope()
  // Header drives the locale being edited - In GLOBAL mode (no effective locale) translation editing is disabled; we
  // still render whatever translation exists so the page can be reviewed
  const scopedLocale = effectiveLocale || undefined
  const canEditTranslations = !!effectiveLocale

  // Locale is attached via the `Locale` request header (see lib/apollo.ts)
  const variables = { id }

  const { data, loading, error, refetch } = useQuery(CmsPageDocument, {
    variables,
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const page = data?.cmsPage
  const refetchListVariables = { filter: { onlyPublished: false }, limit: 200, offset: 0 }
  const refetchList = [{ query: CmsPagesDocument, variables: refetchListVariables }]
  const refetchPage = [{ query: CmsPageDocument, variables }]

  const { data: ptData } = useQuery(CmsPageTypeDocument, {
    variables: { id: page?.pageTypeId ?? '' },
    skip: !page?.pageTypeId,
  })
  const pageType = ptData?.cmsPageType

  const [updatePage] = useMutation(UpdateCmsPageDocument, { refetchQueries: refetchPage })
  const [upsertContent] = useMutation(UpsertCmsPageContentDocument, { refetchQueries: refetchPage })
  const [publishMut] = useMutation(PublishCmsPageDocument, { refetchQueries: [...refetchPage, ...refetchList] })
  const [unpublishMut] = useMutation(UnpublishCmsPageDocument, { refetchQueries: [...refetchPage, ...refetchList] })
  const [rebuildMut, { loading: rebuilding }] = useMutation(RebuildCmsPageDocument)
  const [setSelectionsMut] = useMutation(SetCmsPageSelectionsDocument, { refetchQueries: refetchPage })
  const [setRelationshipsMut] = useMutation(SetCmsPageRelationshipsDocument, { refetchQueries: refetchPage })
  const [deletePage, { loading: deleting }] = useMutation(DeleteCmsPageDocument, { refetchQueries: refetchList })
  const [clonePage, { loading: cloning }] = useMutation(CloneCmsPageDocument, { refetchQueries: refetchList })
  const { data: aiStatusData } = useQuery(AiStatusDocument)
  const aiEnabled = aiStatusData?.aiStatus.enabled ?? false
  const [translatePage] = useMutation(TranslateCmsPageDocument, { refetchQueries: refetchPage })

  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; msg: string } | null>(null)

  // Page save registry: every editable section (basics, content, selections) stages its edits and the header Save
  // applies them through the SaveConfirmDialog diff modal - the dashboard-wide convention (see docs/conventions.md)
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)


  // The page-layout block document is LIFTED here (not owned by ContentCard)
  // so the builder section AND the preview panel read the same in-flight state:
  // the canvas mutates `blocks`, the preview panel streams it to the iframe,
  // and ContentCard stages it for save - Seeded from the locale's persisted
  // body; reset when the page identity or edited locale changes (NOT on the
  // page object - unrelated refetches would wipe staged edits mid-edit)
  const [blocks, setBlocks] = useState<CmsBlock[]>([])

  // In-context editing: clicking a block in the live preview posts its id back here. selectNonce bumps on every click
  // (even re-clicking the same block) so the canvas re-opens + re-scrolls to it each time
  const [selectedBlockId, setSelectedBlockId] = useState<string | null>(null)
  const [selectNonce, setSelectNonce] = useState(0)

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

  // editingLocale tracks the header's effectiveLocale exactly - When the header is in GLOBAL mode (no effective locale)
  // we fall back to the first existing translation purely for read-only display
  const editingLocale =
    scopedLocale ?? page?.content?.locale ?? page?.allContent[0]?.locale ?? globalDefaultLocale

  const allLocales = useMemo(() => {
    const locales = page?.allContent
    if (!locales) return [] as string[]
    const set = new Set(locales.map((c) => c.locale))
    if (editingLocale) set.add(editingLocale)
    return Array.from(set).sort()
  }, [page?.allContent, editingLocale])

  // Does the in-flight block doc differ from what's saved for this locale?
  // Drives the live preview: when clean, the iframe shows the real server
  // render (data-backed grids fetch real catalog data); when dirty, it switches
  // to the live client tree where static blocks update live but data-backed
  // grids show a placeholder (they can't re-fetch in the browser) - Both sides
  // normalise through parseDocument, so prop key-order can't cause false dirty
  const blocksDirty = useMemo(() => {
    const savedBody = page?.allContent.find((x) => x.locale === editingLocale)?.body
    const baseline = parseDocument(savedBody)?.blocks ?? []
    return JSON.stringify(blocks) !== JSON.stringify(baseline)
  }, [blocks, page?.allContent, editingLocale])

  // Seed / reset the lifted block document when the page identity or edited
  // locale changes - A freeform (Lexical) body parses to null → start
  // the builder empty - Keyed on (id, locale) only - NOT the page object - so a
  // refetch from an unrelated action (publish toggle, another section's save)
  // doesn't wipe staged block edits mid-edit - Matches the prior ContentCard
  // effect that this lift replaces
  const pageId = page?.id
  useEffect(() => {
    if (!pageId) return
    const c = page?.allContent.find((x) => x.locale === editingLocale)
    // Syncing local draft state from the loaded page is the intended use of
    // this effect (re-seed on identity/locale change) - the same established
    // pattern as the BasicsCard/ContentCard/SelectionsCard reset effects below
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setBlocks(parseDocument(c?.body)?.blocks ?? [])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId, editingLocale])

  if (loading) return <LoadingState label="Loading page…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load page"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!page) return <p className="text-sm text-muted-foreground">Page not found.</p>

  const p = page
  const displayTitle = p.content?.title?.trim() || p.allContent[0]?.title?.trim() || p.slug

  // Split assigned attributes into scalar (non-translatable, non-select/relationship) for `data`, translatable for
  // content `data`, select-types for the selection picker, and relationship-types for the relationship picker
  const ptAttributes = pageType?.attributes ?? []

  // Conditional visibility: hide an attribute whose condition isn't met by the SAVED page values (scalars from p.data,
  // select choices from p.selections) Re-evaluates on save - No condition → always visible
  const conditionMet = (pta: (typeof ptAttributes)[number]): boolean => {
    if (!pta.conditionAttributeId) return true
    const src = ptAttributes.find((x) => x.attribute.id === pta.conditionAttributeId)
    if (!src) return true
    const want = pta.conditionValue ?? ''
    const sa = src.attribute
    if (sa.inputType === 'SELECT' || sa.inputType === 'MULTISELECT') {
      const slugs = p.selections
        .filter((s) => s.attributeId === sa.id)
        .map((s) => sa.values.find((v) => v.id === s.valueId)?.slug)
        .filter(Boolean) as string[]
      return slugs.includes(want)
    }
    const bag = (p.data ?? {}) as Record<string, unknown>
    const cur = bag[sa.slug]
    return cur != null && String(cur) === want
  }
  const visibleAttrs = ptAttributes.filter(conditionMet)

  const scalarAttrs = visibleAttrs.filter(
    (a) => !a.attribute.isTranslatable && !['SELECT', 'MULTISELECT', 'RELATIONSHIP'].includes(a.attribute.inputType),
  )
  const translatableAttrs = visibleAttrs.filter(
    (a) => a.attribute.isTranslatable && !['SELECT', 'MULTISELECT', 'RELATIONSHIP'].includes(a.attribute.inputType),
  )
  const selectAttrs = visibleAttrs.filter((a) => a.attribute.inputType === 'SELECT' || a.attribute.inputType === 'MULTISELECT')
  const relationshipAttrs = visibleAttrs.filter((a) => a.attribute.inputType === 'RELATIONSHIP')

  async function onPublish() {
    setFeedback(null)
    try {
      await publishMut({ variables: { id: p.id } })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Publish failed.' })
    }
  }
  async function onUnpublish() {
    setFeedback(null)
    try {
      await unpublishMut({ variables: { id: p.id } })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Unpublish failed.' })
    }
  }
  async function onRebuild() {
    setFeedback(null)
    try {
      await rebuildMut({ variables: { id: p.id } })
      setFeedback({ kind: 'ok', msg: 'Page rebuilt in the storefront cache.' })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Rebuild failed.' })
    }
  }
  async function onDelete() {
    setDeleteError(null)
    try {
      await deletePage({ variables: { id: p.id } })
      navigate('/cms/pages', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }
  async function onDuplicate() {
    setFeedback(null)
    try {
      const res = await clonePage({ variables: { id: p.id } })
      const newId = res.data?.cloneCMSPage?.id
      if (newId) navigate(`/cms/pages/${newId}`)
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Duplicate failed.' })
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout
      rightSidebar={
        <CmsPreviewPanel
          pageId={p.id}
          locale={editingLocale || undefined}
          blocks={blocks}
          dirty={blocksDirty}
          onSelectBlock={(id) => {
            setSelectedBlockId(id)
            setSelectNonce((n) => n + 1)
          }}
        />
      }
    >
      <DetailPageHeader
        backTo="/cms/pages"
        backLabel="Back to pages"
        title={displayTitle}
        badges={
          <>
            {p.isPublished ? (
              <Badge variant="secondary" className="gap-1">
                <CheckCircle2Icon className="size-3" />
                Published
              </Badge>
            ) : (
              <Badge variant="outline" className="gap-1 text-muted-foreground">
                <CircleOffIcon className="size-3" />
                Unpublished
              </Badge>
            )}
            {pageType && (
              <Badge variant="outline">{pageType.content?.name ?? pageType.slug}</Badge>
            )}
            <Badge variant="outline" className="gap-1">
              <WorkflowIcon className="size-3" />
              {workflowLabel(p.workflowState)}
            </Badge>
          </>
        }
        meta={<code className="font-mono">{p.urlPath}</code>}
        actions={
          <>
            {canManage && (
              <>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={onDuplicate}
                  disabled={cloning}
                >
                  <CopyIcon className="size-4" />
                  {cloning ? 'Duplicating…' : 'Duplicate'}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => { setDeleteError(null); setConfirmDelete(true) }}
                  disabled={deleting}
                >
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
                <Button
                  type="button"
                  size="sm"
                  disabled={!isDirty || savingAll}
                  onClick={() => { setSaveError(null); setConfirmingSave(true) }}
                >
                  {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
                </Button>
              </>
            )}
          </>
        }
      />

      <DetailPageBody>
        {/* Sections ordered content-first: the editing surfaces an author
            reaches for most (layout → content → fields) lead; publishing and
            governance (channels → status → workflow → A/B → history → comments)
            follow */}

        {/* ---- Page layout (the builder centerpiece) ---- */}
        {/* Promoted to its own full-width, prominent section so the canvas
            has real room to breathe (palette + canvas + inspector) instead of
            being crammed into the Content card among the title/SEO/dynamic
            fields - The block document is the lifted `blocks` state - the same
            state the live-preview panel streams to the storefront iframe */}
        <DetailPageSection
          title="Page layout"
          description="Build the page from blocks. Click or drag a block from the palette, reorder by dragging, edit the selected block in the inspector - the live preview on the right updates as you go."
          trailing={
            <div className="flex items-center gap-2">
              <span className="inline-flex h-7 items-center rounded-md border px-2 text-xs text-muted-foreground">
                {blocks.length} block{blocks.length === 1 ? '' : 's'}
              </span>
              <span
                className="inline-flex h-7 items-center rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground"
                title="Layout edits apply to this locale's content"
              >
                {editingLocale}
              </span>
            </div>
          }
        >
          {!canEditTranslations && (
            <Alert>
              <AlertTitle>Read-only in this scope</AlertTitle>
              <AlertDescription>
                Pick a channel + locale in the header to edit the layout. Below is the closest available locale's layout, read-only.
              </AlertDescription>
            </Alert>
          )}
          <PageBlockCanvas
            blocks={blocks}
            onChange={setBlocks}
            disabled={!canManage || !canEditTranslations}
            focusBlockId={selectedBlockId}
            focusNonce={selectNonce}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Content & SEO"
          description="Title, SEO, and translatable fields - per locale. The page layout is edited above."
          trailing={
            <div className="flex items-center gap-2">
              {/* Read-only badges showing which locales have content - Editing
                  the locale is done in the header - there is no in-form
                  picker any more, so this surface stays informational */}
              {allLocales.map((loc) => (
                <span
                  key={loc}
                  className={
                    loc === editingLocale
                      ? 'inline-flex h-7 items-center rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground'
                      : 'inline-flex h-7 items-center rounded-md border px-2 text-xs text-muted-foreground'
                  }
                >
                  {loc}
                </span>
              ))}
              {canEditTranslations && !allLocales.includes(editingLocale) && (
                <span className="inline-flex h-7 items-center rounded-md border border-dashed px-2 text-xs text-muted-foreground">
                  new: {editingLocale}
                </span>
              )}
            </div>
          }
        >
          <ContentCard
            page={p}
            locale={editingLocale}
            canManage={canManage}
            canEditTranslations={canEditTranslations}
            translatableAttrs={translatableAttrs}
            blocks={blocks}
            aiEnabled={aiEnabled}
            onTranslate={async (sourceLocale) => {
              await translatePage({
                variables: { pageId: p.id, sourceLocale, targetLocale: editingLocale },
              })
            }}
            onSave={async (input) => {
              await upsertContent({ variables: { input } })
            }}
            onSaveSeo={async (s) => {
              // updateCMSPage replaces the whole metadata map, so keep any non-seo keys and re-send every seo.* value
              // in one write - they all live in metadata, so saving them separately would clobber each other
              const seoKeys = ['seo.ogImage', 'seo.canonical', 'seo.nofollow', 'seo.jsonld', 'seo.schema']
              const metadata = (p.metadata ?? [])
                .filter((m) => !seoKeys.includes(m.key))
                .map((m) => ({ key: m.key, value: m.value }))
              if (s.ogImage) metadata.push({ key: 'seo.ogImage', value: s.ogImage })
              if (s.canonical) metadata.push({ key: 'seo.canonical', value: s.canonical })
              if (s.nofollow) metadata.push({ key: 'seo.nofollow', value: '1' })
              // raw JSON-LD and the builder spec are mutually exclusive - the panel sends at most one
              if (s.jsonld) metadata.push({ key: 'seo.jsonld', value: s.jsonld })
              if (s.schema) metadata.push({ key: 'seo.schema', value: s.schema })
              await updatePage({ variables: { input: { id: p.id, seoNoindex: s.noindex, metadata } } })
            }}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Basics & non-translated fields"
          description="Slug change auto-creates a 301 redirect."
        >
          <BasicsCard
            page={p}
            scalarAttrs={scalarAttrs}
            canManage={canManage}
            onSave={async (input) => {
              await updatePage({ variables: { input } })
            }}
          />
        </DetailPageSection>

        {/* ---- Select / Multiselect ---- */}
        {selectAttrs.length > 0 && (
          <DetailPageSection
            title="Selections"
            description="Pick options for select / multiselect attributes."
          >
            <SelectionsCard
              selectAttrs={selectAttrs}
              selections={p.selections}
              canManage={canManage}
              onSave={async (attributeId, valueIds) => {
                await setSelectionsMut({ variables: { pageId: p.id, attributeId, valueIds } })
              }}
            />
          </DetailPageSection>
        )}

        {/* ---- Relationships ---- */}
        {relationshipAttrs.length > 0 && (
          <DetailPageSection
            title="Relationships"
            description="Link this page to other entities. Selection order is preserved."
          >
            <RelationshipsCard
              relationshipAttrs={relationshipAttrs}
              relationships={p.relationships}
              canManage={canManage}
              onSave={async (attributeId, refs) => {
                await setRelationshipsMut({ variables: { pageId: p.id, attributeId, refs } })
              }}
            />
          </DetailPageSection>
        )}

        <DetailPageSection
          title="Tags"
          description="Assign tags from the shared library to organize and filter pages. Manage the tag list under CMS → Tags."
        >
          <CmsPageTagsSection
            pageId={p.id}
            tags={p.tags}
            canManage={canManage}
            refetchVariables={variables}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Channels"
          description="The page is global. Publish it per channel, and optionally set it as that channel's homepage (served at /)."
        >
          <CmsPageChannelsSection
            pageId={p.id}
            listings={p.channelListings}
            canManage={canManage}
            refetchVariables={variables}
          />
        </DetailPageSection>

        {/* Lifecycle - publish/unpublish are non-field actions and apply
            immediately, outside the staged-save flow */}
        <DetailPageSection title="Status">
          {feedback && (
            <Alert variant={feedback.kind === 'ok' ? 'default' : 'destructive'}>
              <AlertTitle>{feedback.kind === 'ok' ? 'Saved' : 'Error'}</AlertTitle>
              <AlertDescription>{feedback.msg}</AlertDescription>
            </Alert>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Button onClick={onPublish} disabled={!canManage || p.isPublished}>
              <CheckCircle2Icon className="size-4" /> Publish
            </Button>
            <Button variant="outline" onClick={onUnpublish} disabled={!canManage || !p.isPublished}>
              <CircleOffIcon className="size-4" /> Unpublish
            </Button>
            <Button
              variant="outline"
              onClick={onRebuild}
              disabled={!canManage || rebuilding}
              title="Invalidate this page and re-render it in the storefront cache now"
            >
              <RotateCcwIcon className="size-4" /> {rebuilding ? 'Rebuilding…' : 'Rebuild now'}
            </Button>
          </div>
        </DetailPageSection>

        <DetailPageSection
          title="Editorial workflow"
          description="Move the page through review. When approval is required (CMS settings), a page can only be published once approved."
        >
          <CmsPageWorkflowSection
            pageId={p.id}
            state={p.workflowState}
            assignedTo={p.assignedTo ?? null}
            canManage={canManage}
            canApprove={canApprove}
            refetchVariables={variables}
          />
        </DetailPageSection>

        {/* Prism is the Signals suite, with no transactional exception - with it off the resolvers refuse, so the section would only render an error */}
        {featureEnabled('signals') && (
          <DetailPageSection
            title="A/B test (Prism)"
            description="Bind a Prism experiment to serve alternative page layouts as an A/B test. Visitors are assigned server-side; the control is this page's main content."
          >
            <CmsPageExperimentSection
              page={p}
              locale={editingLocale}
              channelId={selectedChannel?.id ?? null}
              canManage={canManage && canEditTranslations}
              refetchVariables={variables}
            />
          </DetailPageSection>
        )}

        <DetailPageSection
          title="History & scheduling"
          description="Schedule a future go-live, and restore an earlier saved version."
        >
          <CmsPageHistorySchedule
            pageId={p.id}
            locale={editingLocale}
            scheduledAt={p.scheduledAt as string | null}
            canManage={canManage}
            refetchVariables={variables}
          />
        </DetailPageSection>

        <DetailPageSection
          title="Comments"
          description="Collaboration notes for this page. Resolve a comment once it's addressed."
        >
          <CmsPageCommentsSection pageId={p.id} canManage={canManage} />
        </DetailPageSection>
        <PluginSections target="cms-page" entity={p} mode="edit" />
      </DetailPageBody>

      <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete page?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the page, its translations, and selections. URL redirects are preserved.
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
            <AlertDialogAction onClick={onDelete} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Review-before-save modal: lists every staged change across the
          page's sections (basics, content, selections) */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`CMS page · ${displayTitle}${canEditTranslations ? ` · ${editingLocale}` : ''}`}
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
