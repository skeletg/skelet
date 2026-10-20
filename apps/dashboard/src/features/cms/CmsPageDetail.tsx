import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
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
import { useSaveConfirm } from '@/shared/saving/useSaveConfirm'
import { useSectionForm } from '@/shared/saving/useSectionForm'
import {
  AiStatusDocument,
  CloneCmsPageDocument,
  CmsPageDocument,
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
  type CmsPageRowFragment,
  CmsPagesDocument,
} from './Cms.gql'
import { PageBlockCanvas } from './PageBlockCanvas'
import { CmsPreviewPanel } from './CmsPreviewPanel'
import { parseDocument, serializeDocument, type CmsBlock } from './page-blocks'
import type { FieldValue } from './DynamicFieldRenderer'
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
import { pageContentDiff, pageContentSchema, toPageContentValues } from './schemas/page'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

const parseBlocks = (body: unknown) => parseDocument(body)?.blocks ?? []

export function CmsPageDetail() {
  const { id = '' } = useParams()
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  // Locale is attached via the `Locale` request header (see lib/apollo.ts)
  const variables = { id }
  const { data, loading, error, refetch } = useQuery(CmsPageDocument, {
    variables,
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const page = data?.cmsPage

  if (loading && !page) return <LoadingState label="Loading page…" />
  if (error) return <LoadError title="Couldn't load page" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  if (!page) return <p className="text-sm text-muted-foreground">Page not found.</p>

  // The header's locale; in GLOBAL mode (no effective locale) the first existing translation, shown
  // read-only
  const locale = effectiveLocale || page.content?.locale || page.allContent[0]?.locale || globalDefaultLocale
  // Keyed on the locale - the layout, title, SEO and translatable fields are that locale's content
  return <CmsPageEditor key={`${page.id}:${locale}`} page={page} locale={locale} canEditTranslations={!!effectiveLocale} variables={variables} />
}

function CmsPageEditor({
  page: p,
  locale: editingLocale,
  canEditTranslations,
  variables,
}: {
  page: CmsPageRowFragment
  locale: string
  canEditTranslations: boolean
  variables: { id: string }
}) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const { enabled: featureEnabled } = useFeatures()
  const canManage = has('cms.manage')
  const canApprove = has('cms.approve')
  const { selectedChannel } = useConfigScope()

  const refetchList = [CmsPagesDocument]
  const refetchPage = [{ query: CmsPageDocument, variables }]

  const { data: ptData } = useQuery(CmsPageTypeDocument, { variables: { id: p.pageTypeId ?? '' }, skip: !p.pageTypeId })
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
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; msg: string } | null>(null)

  const displayTitle = p.content?.title?.trim() || p.allContent[0]?.title?.trim() || p.slug

  // Page save registry: every editable section (basics, content, selections, …) stages its edits and
  // the header Save applies them through the SaveConfirmDialog diff modal (docs/conventions.md)
  const registry = useSaveRegistry(`CMS page · ${displayTitle}`)
  const confirm = useSaveConfirm(registry.saveAll)

  // Split assigned attributes: scalar (non-translatable, non-select/relationship) for `data`,
  // translatable for the content's `data`, select-types for the selection picker, and relationship
  // types for the relationship picker. Conditional visibility hides an attribute whose condition isn't
  // met by the SAVED page values; no condition -> always visible
  const ptAttributes = pageType?.attributes ?? []
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
    const cur = ((p.data ?? {}) as Record<string, unknown>)[sa.slug]
    return cur != null && String(cur) === want
  }
  const visibleAttrs = ptAttributes.filter(conditionMet)
  const isField = (a: (typeof ptAttributes)[number]) => !['SELECT', 'MULTISELECT', 'RELATIONSHIP'].includes(a.attribute.inputType)
  const scalarAttrs = visibleAttrs.filter((a) => !a.attribute.isTranslatable && isField(a))
  const translatableAttrs = visibleAttrs.filter((a) => a.attribute.isTranslatable && isField(a))
  const selectAttrs = visibleAttrs.filter((a) => a.attribute.inputType === 'SELECT' || a.attribute.inputType === 'MULTISELECT')
  const relationshipAttrs = visibleAttrs.filter((a) => a.attribute.inputType === 'RELATIONSHIP')

  // The locale's content - its block layout is read by three places: the builder section, the content
  // section and the live preview, so the form lives here rather than in ContentCard. In GLOBAL scope it's
  // a read-only view of the closest locale: nothing staged, no draft
  const contentBaseline = useMemo(() => toPageContentValues(p.allContent, editingLocale, parseBlocks), [p.allContent, editingLocale])
  const { form: contentForm, draft: contentDraft } = useSectionForm({
    id: 'content',
    title: `Content · ${editingLocale}`,
    baseline: contentBaseline,
    schema: pageContentSchema,
    diff: pageContentDiff(translatableAttrs),
    routeKey: `cms-page-content:${p.id}:${editingLocale}`,
    registry: registry.contextValue,
    enabled: canEditTranslations,
    submit: async (v) => {
      await upsertContent({
        variables: {
          input: {
            pageId: p.id,
            locale: editingLocale,
            title: v.title.trim(),
            // The layout persists as the `{version:1, blocks}` Document - `body` is a JSON scalar
            body: serializeDocument(v.blocks as CmsBlock[]),
            metaTitle: v.metaTitle,
            metaDescription: v.metaDescription,
            data: v.data as Record<string, FieldValue>,
          },
        },
      })
    },
  })
  const blocks = useStore(contentForm.store, (s) => s.values.blocks) as CmsBlock[]
  // When the layout is clean the preview shows the real server render (data-backed grids fetch real
  // catalog data); when dirty it switches to the live client tree
  const blocksDirty = JSON.stringify(blocks) !== JSON.stringify(contentBaseline.blocks)

  // In-context editing: clicking a block in the live preview posts its id back here. selectNonce bumps
  // on every click (even re-clicking the same block) so the canvas re-opens and re-scrolls to it
  const [selectedBlockId, setSelectedBlockId] = useState<string | null>(null)
  const [selectNonce, setSelectNonce] = useState(0)

  const allLocales = Array.from(new Set([...p.allContent.map((c) => c.locale), editingLocale].filter(Boolean))).sort()

  async function run(fn: () => Promise<unknown>, failure: string, ok?: string) {
    setFeedback(null)
    try {
      await fn()
      if (ok) setFeedback({ kind: 'ok', msg: ok })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : failure })
    }
  }
  async function onDelete() {
    await deletePage({ variables: { id: p.id } })
    navigate('/cms/pages', { replace: true })
  }
  async function onDuplicate() {
    await run(async () => {
      const res = await clonePage({ variables: { id: p.id } })
      const newId = res.data?.cloneCMSPage?.id
      if (newId) navigate(`/cms/pages/${newId}`)
    }, 'Duplicate failed.')
  }

  // Neutral chip for the active locale - the primary token is for brand, not state
  const localeChip = (active: boolean) =>
    active
      ? 'inline-flex h-7 items-center rounded-md border border-foreground/40 bg-muted px-2 text-xs font-medium text-foreground'
      : 'inline-flex h-7 items-center rounded-md border px-2 text-xs text-muted-foreground'

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
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
              {pageType && <Badge variant="outline">{pageType.content?.name ?? pageType.slug}</Badge>}
              <Badge variant="outline" className="gap-1">
                <WorkflowIcon className="size-3" />
                {workflowLabel(p.workflowState)}
              </Badge>
            </>
          }
          meta={<code className="font-mono">{p.urlPath}</code>}
          actions={
            canManage && (
              <>
                <Button type="button" variant="outline" size="sm" onClick={onDuplicate} disabled={cloning}>
                  <CopyIcon className="size-4" />
                  {cloning ? 'Duplicating…' : 'Duplicate'}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                  onClick={() => setConfirmDelete(true)}
                  disabled={deleting}
                >
                  <Trash2Icon className="size-4" />
                  Delete
                </Button>
                <Button type="button" size="sm" disabled={!registry.isDirty || confirm.saving} onClick={confirm.open}>
                  {confirm.saving ? 'Saving…' : registry.isDirty ? 'Save changes' : 'No changes'}
                </Button>
              </>
            )
          }
        />

        <DetailPageBody>
          {/* Sections ordered content-first: the editing surfaces an author reaches for most
              (layout -> content -> fields) lead; publishing and governance follow */}

          {/* ---- Page layout (the builder centerpiece) ---- */}
          <DetailPageSection
            title="Page layout"
            description="Build the page from blocks. Click or drag a block from the palette, reorder by dragging, edit the selected block in the inspector - the live preview on the right updates as you go."
            trailing={
              <div className="flex items-center gap-2">
                <span className="inline-flex h-7 items-center rounded-md border px-2 text-xs text-muted-foreground">
                  {blocks.length} block{blocks.length === 1 ? '' : 's'}
                </span>
                <span className={localeChip(true)} title="Layout edits apply to this locale's content">
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
              onChange={(next) => contentForm.setFieldValue('blocks', next)}
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
                {/* Which locales have content - the locale itself is picked in the header */}
                {allLocales.map((loc) => (
                  <span key={loc} className={localeChip(loc === editingLocale)}>
                    {loc}
                  </span>
                ))}
                {canEditTranslations && !p.allContent.some((c) => c.locale === editingLocale) && (
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
              form={contentForm}
              draft={contentDraft}
              canManage={canManage}
              canEditTranslations={canEditTranslations}
              translatableAttrs={translatableAttrs}
              aiEnabled={aiEnabled}
              onTranslate={async (sourceLocale) => {
                await translatePage({ variables: { pageId: p.id, sourceLocale, targetLocale: editingLocale } })
              }}
              onSaveSeo={async (s) => {
                // updateCMSPage replaces the whole metadata map, so keep any non-seo keys and re-send every
                // seo.* value in one write - they all live in metadata, so separate saves would clobber
                const seoKeys = ['seo.ogImage', 'seo.canonical', 'seo.nofollow', 'seo.jsonld', 'seo.schema']
                const metadata = (p.metadata ?? []).filter((m) => !seoKeys.includes(m.key)).map((m) => ({ key: m.key, value: m.value }))
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

          <DetailPageSection title="Basics & non-translated fields" description="Slug change auto-creates a 301 redirect.">
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
            <DetailPageSection title="Selections" description="Pick options for select / multiselect attributes.">
              <SelectionsCard
                pageId={p.id}
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
            <DetailPageSection title="Relationships" description="Link this page to other entities. Selection order is preserved.">
              <RelationshipsCard
                pageId={p.id}
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
            <CmsPageTagsSection pageId={p.id} tags={p.tags} canManage={canManage} refetchVariables={variables} />
          </DetailPageSection>

          <DetailPageSection
            title="Channels"
            description="The page is global. Publish it per channel, and optionally set it as that channel's homepage (served at /)."
          >
            <CmsPageChannelsSection pageId={p.id} listings={p.channelListings} canManage={canManage} refetchVariables={variables} />
          </DetailPageSection>

          {/* Lifecycle - publish/unpublish are non-field actions and apply immediately, outside the
              staged-save flow */}
          <DetailPageSection title="Status">
            {feedback && (
              <Alert variant={feedback.kind === 'ok' ? 'default' : 'destructive'}>
                <AlertTitle>{feedback.kind === 'ok' ? 'Saved' : 'Error'}</AlertTitle>
                <AlertDescription>{feedback.msg}</AlertDescription>
              </Alert>
            )}
            <div className="flex flex-wrap items-center gap-3">
              <Button
                type="button"
                onClick={() => run(() => publishMut({ variables: { id: p.id } }), 'Publish failed.')}
                disabled={!canManage || p.isPublished}
              >
                <CheckCircle2Icon className="size-4" /> Publish
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => run(() => unpublishMut({ variables: { id: p.id } }), 'Unpublish failed.')}
                disabled={!canManage || !p.isPublished}
              >
                <CircleOffIcon className="size-4" /> Unpublish
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => run(() => rebuildMut({ variables: { id: p.id } }), 'Rebuild failed.', 'Page rebuilt in the storefront cache.')}
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

          {/* Prism is the Signals suite, with no transactional exception - with it off the resolvers
              refuse, so the section would only render an error */}
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

          <DetailPageSection title="History & scheduling" description="Schedule a future go-live, and restore an earlier saved version.">
            <CmsPageHistorySchedule
              pageId={p.id}
              locale={editingLocale}
              scheduledAt={p.scheduledAt as string | null}
              canManage={canManage}
              refetchVariables={variables}
            />
          </DetailPageSection>

          <DetailPageSection title="Comments" description="Collaboration notes for this page. Resolve a comment once it's addressed.">
            <CmsPageCommentsSection pageId={p.id} canManage={canManage} />
          </DetailPageSection>
          <PluginSections target="cms-page" entity={p} mode="edit" />
        </DetailPageBody>

        <ConfirmDialog
          open={confirmDelete}
          onOpenChange={setConfirmDelete}
          title="Delete page?"
          description="This permanently removes the page, its translations, and selections. URL redirects are preserved."
          confirmLabel="Delete"
          busyLabel="Deleting…"
          onConfirm={onDelete}
        />

        <SaveConfirmDialog
          {...confirm.dialogProps}
          scopeLabel={`CMS page · ${displayTitle}${canEditTranslations ? ` · ${editingLocale}` : ''}`}
          sections={registry.dirtySections}
        />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}
