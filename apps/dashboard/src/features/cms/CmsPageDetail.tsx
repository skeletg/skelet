import { useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { toast } from 'sonner'
import { CopyIcon, HistoryIcon, MessageSquareIcon, MoreHorizontalIcon, PencilIcon, Trash2Icon } from 'lucide-react'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
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
  CmsPageCommentsDocument,
  CmsPageDocument,
  TranslateCmsPageDocument,
  CmsPageTypeDocument,
  DeleteCmsPageDocument,
  SetCmsPageRelationshipsDocument,
  SetCmsPageSelectionsDocument,
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
import { SeoCard } from './page-detail/SeoCard'
import { SelectionsCard } from './page-detail/SelectionsCard'
import { RelationshipsCard } from './page-detail/RelationshipsCard'
import { CmsPageExperimentSection } from './page-detail/ExperimentSection'
import { CmsPageComments } from './page-detail/Comments'
import { CmsPageTagsSection } from './page-detail/TagsSection'
import { CmsPageChannelsSection } from './page-detail/ChannelsSection'
import { CmsPageHistory } from './page-detail/History'
import { PublishMenu, PublishStatus } from './page-detail/PublishMenu'
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
  const { selectedChannel, channels, setScope } = useConfigScope()

  const refetchList = [CmsPagesDocument]
  const refetchPage = [{ query: CmsPageDocument, variables }]

  const { data: ptData } = useQuery(CmsPageTypeDocument, { variables: { id: p.pageTypeId ?? '' }, skip: !p.pageTypeId })
  const pageType = ptData?.cmsPageType

  const [updatePage] = useMutation(UpdateCmsPageDocument, { refetchQueries: refetchPage })
  const [upsertContent] = useMutation(UpsertCmsPageContentDocument, { refetchQueries: refetchPage })
  const [setSelectionsMut] = useMutation(SetCmsPageSelectionsDocument, { refetchQueries: refetchPage })
  const [setRelationshipsMut] = useMutation(SetCmsPageRelationshipsDocument, { refetchQueries: refetchPage })
  const [deletePage, { loading: deleting }] = useMutation(DeleteCmsPageDocument, { refetchQueries: refetchList })
  const [clonePage, { loading: cloning }] = useMutation(CloneCmsPageDocument, { refetchQueries: refetchList })
  const { data: aiStatusData } = useQuery(AiStatusDocument)
  const aiEnabled = aiStatusData?.aiStatus.enabled ?? false
  const [translatePage] = useMutation(TranslateCmsPageDocument, { refetchQueries: refetchPage })
  const { data: commentsData } = useQuery(CmsPageCommentsDocument, { variables: { pageId: p.id } })
  const openComments = (commentsData?.cmsPageComments ?? []).filter((c) => !c.resolved).length

  const [confirmDelete, setConfirmDelete] = useState(false)
  const [panel, setPanel] = useState<'history' | 'comments' | null>(null)

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

  async function onDelete() {
    await deletePage({ variables: { id: p.id } })
    navigate('/cms/pages', { replace: true })
  }
  async function onDuplicate() {
    try {
      const res = await clonePage({ variables: { id: p.id } })
      const newId = res.data?.cloneCMSPage?.id
      if (newId) navigate(`/cms/pages/${newId}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Duplicate failed.')
    }
  }

  // Global scope shows the closest locale read-only; offer a scope that can edit it, preferring a channel
  // the page is already listed on
  const listed = new Set(p.channelListings.map((l) => l.channelId))
  const editChannel = channels.find((c) => c.isActive && listed.has(c.id)) ?? channels.find((c) => c.isActive)
  const editLocale = editChannel && (editChannel.locales.some((l) => l.code === editingLocale) ? editingLocale : editChannel.defaultLocale)
  const newTranslation = canEditTranslations && !p.allContent.some((c) => c.locale === editingLocale)
  const chip = 'inline-flex h-7 items-center rounded-md border px-2 text-xs text-muted-foreground'

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
          badges={p.workflowState === 'IN_REVIEW' ? <Badge variant="warning">In review</Badge> : null}
          meta={
            <>
              <code className="font-mono">{p.urlPath}</code>
              {pageType && <span>· {pageType.content?.name ?? pageType.slug}</span>}
            </>
          }
          actions={
            <>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={openComments > 0 ? `Comments, ${openComments} open` : 'Comments'}
                title="Comments"
                onClick={() => setPanel('comments')}
              >
                <MessageSquareIcon className="size-4" />
                {openComments > 0 && <span className="text-xs tabular-nums">{openComments}</span>}
              </Button>
              <Button type="button" variant="ghost" size="sm" aria-label="Version history" title="Version history" onClick={() => setPanel('history')}>
                <HistoryIcon className="size-4" />
              </Button>
              {canManage ? (
                <>
                  <PublishMenu page={p} canApprove={canApprove} dirty={registry.isDirty} refetchVariables={variables} />
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="ghost" size="sm" aria-label="More actions" disabled={cloning || deleting}>
                        <MoreHorizontalIcon className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onSelect={() => void onDuplicate()}>
                        <CopyIcon /> Duplicate
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setConfirmDelete(true)}>
                        <Trash2Icon /> Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                  <Button type="button" size="sm" disabled={!registry.isDirty || confirm.saving} onClick={confirm.open}>
                    {confirm.saving ? 'Saving…' : registry.isDirty ? 'Save changes' : 'No changes'}
                  </Button>
                </>
              ) : (
                <PublishStatus page={p} />
              )}
            </>
          }
        />

        <DetailPageBody>
          {!canEditTranslations && (
            <div className="flex flex-wrap items-center justify-between gap-3 bg-muted/40 px-6 py-3 text-sm">
              <span className="text-muted-foreground">
                The layout and content are per language, so global scope shows the <code className="font-mono">{editingLocale}</code> copy read-only.
              </span>
              {editChannel && editLocale && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setScope({ mode: 'CHANNEL', channelId: editChannel.id, locale: editLocale })}
                >
                  <PencilIcon className="size-4" />
                  Edit in {editChannel.name} · {editLocale}
                </Button>
              )}
            </div>
          )}

          <DetailPageSection title="Layout" trailing={<span className={chip}>{blocks.length} block{blocks.length === 1 ? '' : 's'}</span>}>
            <PageBlockCanvas
              blocks={blocks}
              onChange={(next) => contentForm.setFieldValue('blocks', next)}
              disabled={!canManage || !canEditTranslations}
              focusBlockId={selectedBlockId}
              focusNonce={selectNonce}
            />
          </DetailPageSection>

          <DetailPageSection title="Content" trailing={newTranslation ? <span className={`${chip} border-dashed`}>New translation</span> : null}>
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
            />
          </DetailPageSection>

          <DetailPageSection title="SEO">
            <SeoCard
              page={p}
              locale={editingLocale}
              form={contentForm}
              canManage={canManage}
              canEditTranslations={canEditTranslations}
              aiEnabled={aiEnabled}
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

          <DetailPageSection title="Page settings">
            <BasicsCard
              page={p}
              scalarAttrs={scalarAttrs}
              canManage={canManage}
              onSave={async (input) => {
                await updatePage({ variables: { input } })
              }}
            />
            {selectAttrs.length > 0 && (
              <SelectionsCard
                pageId={p.id}
                selectAttrs={selectAttrs}
                selections={p.selections}
                locale={editingLocale}
                canManage={canManage}
                onSave={async (attributeId, valueIds) => {
                  await setSelectionsMut({ variables: { pageId: p.id, attributeId, valueIds } })
                }}
              />
            )}
            {relationshipAttrs.length > 0 && (
              <RelationshipsCard
                pageId={p.id}
                relationshipAttrs={relationshipAttrs}
                relationships={p.relationships}
                canManage={canManage}
                onSave={async (attributeId, refs) => {
                  await setRelationshipsMut({ variables: { pageId: p.id, attributeId, refs } })
                }}
              />
            )}
            <div className="grid gap-2">
              <span className="text-sm font-medium">Tags</span>
              <CmsPageTagsSection pageId={p.id} tags={p.tags} canManage={canManage} refetchVariables={variables} />
            </div>
          </DetailPageSection>

          <DetailPageSection title="Channels" description="The page goes live on a channel it's published on, once the page itself is published.">
            <CmsPageChannelsSection pageId={p.id} listings={p.channelListings} canManage={canManage} refetchVariables={variables} />
          </DetailPageSection>

          {/* Prism is the Signals suite and binds per channel - with Signals off the resolvers refuse */}
          {featureEnabled('signals') && selectedChannel && (
            <DetailPageSection title="A/B test" description="Serve alternative layouts of this page to a share of visitors.">
              <CmsPageExperimentSection
                page={p}
                locale={editingLocale}
                channelId={selectedChannel.id}
                canManage={canManage && canEditTranslations}
                refetchVariables={variables}
              />
            </DetailPageSection>
          )}
          <PluginSections target="cms-page" entity={p} mode="edit" />
        </DetailPageBody>

        <Dialog open={panel === 'history'} onOpenChange={(o) => !o && setPanel(null)}>
          <DialogContent className="flex h-[min(85vh,46rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
            <DialogHeader className="gap-1 border-b px-5 py-4 pr-12">
              <DialogTitle className="flex items-center gap-2">
                <HistoryIcon className="size-5 text-muted-foreground" />
                Version history
              </DialogTitle>
              <DialogDescription>
                Saved versions of the <code className="font-mono">{editingLocale}</code> content. Restoring one saves it as a new
                version, so it can be undone.
              </DialogDescription>
            </DialogHeader>
            {panel === 'history' && (
              <CmsPageHistory pageId={p.id} locale={editingLocale} canManage={canManage} refetchVariables={variables} />
            )}
          </DialogContent>
        </Dialog>

        <Dialog open={panel === 'comments'} onOpenChange={(o) => !o && setPanel(null)}>
          <DialogContent className="flex max-h-[min(80vh,40rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-lg">
            <DialogHeader className="gap-1 border-b px-5 py-4 pr-12">
              <DialogTitle className="flex items-center gap-2">
                <MessageSquareIcon className="size-5 text-muted-foreground" />
                Comments
              </DialogTitle>
              <DialogDescription>Notes for whoever works on this page. Resolve one once it's handled.</DialogDescription>
            </DialogHeader>
            {panel === 'comments' && <CmsPageComments pageId={p.id} canManage={canManage} />}
          </DialogContent>
        </Dialog>

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
