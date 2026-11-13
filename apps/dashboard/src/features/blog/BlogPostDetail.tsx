import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { ArrowLeftIcon, CheckCircle2Icon, CircleOffIcon, EyeIcon, PanelRightIcon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { DetailPageLayout } from '@/components/layout/detail-page'
import { WORKBENCH } from '@/components/layout/workbench-styles'
import { fieldErrorText } from '@/components/form/field-errors'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { Button } from '@/components/ui/button'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  FormRichTextEditor,
  richTextToPlainText,
  type RichTextImageUploadResult,
  type RichTextValue,
} from '@/features/catalog/LexicalRichTextEditor'
import {
  ConfirmMediaAssetUploadDocument,
  RequestMediaAssetUploadDocument,
  type ConfirmMediaAssetUploadMutation,
  type ConfirmMediaAssetUploadMutationVariables,
  type RequestMediaAssetUploadMutation,
  type RequestMediaAssetUploadMutationVariables,
} from '@/features/media/MediaLibrary.gql'
import {
  ArchiveBlogPostDocument,
  BlogCategoriesDocument,
  BlogPostDocument,
  BlogTagsDocument,
  DeleteBlogPostContentDocument,
  DeleteBlogPostDocument,
  PublishBlogPostDocument,
  ScheduleBlogPostDocument,
  SetBlogPostCategoriesDocument,
  SetBlogPostTagsDocument,
  UnpublishBlogPostDocument,
  UpdateBlogPostDocument,
  UpsertBlogPostContentDocument,
  BlogPostsDocument,
} from './Blog.gql'
import { slugify } from '@/lib/slug'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { SaveRegistryContext } from '@/shared/saving/saveRegistry'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { cn } from '@/lib/utils'
import { BlogPreviewPanel } from './BlogPreviewPanel'
import { flattenCategories } from './post/categories'
import { PostCapsule } from './post/PostCapsule'
import { PostPanel } from './post/PostPanel'
import { StoreNoindexSave } from './post/PostStores'
import { STATUS_DOT, statusLabel } from './post/status'
import type { Post } from './post/types'
import { usePageZoom } from './post/zoom'
import { POST_LABELS, postChanges, postDiff, postSchema, toPostValues } from './schemas/post'
import { readImageDimensions } from '@/lib/image'
import { putToSignedUrl } from '@/lib/upload'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

// The locale the form edits: the header's, or in GLOBAL mode the default locale when the post has
// content for it. Falling through to allContent[0] alone is alphabetical, which hides recent work
function editingLocaleFor(post: Post, scopedLocale: string | undefined, globalDefaultLocale: string): string {
  const withContent = post.allContent.map((c) => c.locale)
  const preferred =
    globalDefaultLocale && withContent.includes(globalDefaultLocale)
      ? globalDefaultLocale
      : (post.content?.locale ?? post.allContent[0]?.locale ?? '')
  return scopedLocale ?? post.content?.locale ?? (preferred || globalDefaultLocale)
}

export function BlogPostDetail() {
  const { id = '' } = useParams()
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  // Header drives the locale being edited - in GLOBAL mode (no effective locale) existing translations
  // still render so the post can be reviewed
  const scopedLocale = effectiveLocale || undefined
  // Locale is attached via the `Locale` request header (see lib/apollo.ts)
  const variables = { id, includeUnpublished: true }
  const { data, loading, error, refetch } = useQuery(BlogPostDocument, {
    variables,
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const post = data?.blogPost

  // After a translation is removed the form follows the content that's left - until the post or the
  // header scope changes, which re-derive it
  const [override, setOverride] = useState<string | null>(null)
  const scopeKey = `${post?.id}:${scopedLocale}:${globalDefaultLocale}`
  const [prevScopeKey, setPrevScopeKey] = useState(scopeKey)
  if (prevScopeKey !== scopeKey) {
    setPrevScopeKey(scopeKey)
    setOverride(null)
  }

  if (loading && !post) return <LoadingState label="Loading post…" />
  if (error) return <LoadError title="Couldn't load post" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  if (!post) return <p className="text-sm text-muted-foreground">Post not found.</p>

  const locale = override ?? editingLocaleFor(post, scopedLocale, globalDefaultLocale)
  // Keyed on the locale - title, body, excerpt and SEO are that locale's content
  return (
    <PostForm
      key={`${post.id}:${locale}`}
      post={post}
      locale={locale}
      canEditTranslations={!!effectiveLocale}
      variables={variables}
      onLocaleRemoved={(remaining) => setOverride(remaining)}
    />
  )
}

// The post's settings, the locale's content and its categories and tags are one form behind the
// header Save; publishing, scheduling, archiving and store listings act at once
function PostForm({
  post,
  locale,
  canEditTranslations,
  variables,
  onLocaleRemoved,
}: {
  post: Post
  locale: string
  canEditTranslations: boolean
  variables: { id: string; includeUnpublished: boolean }
  onLocaleRemoved: (next: string | null) => void
}) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('blog.manage')

  const refetchList = [BlogPostsDocument]
  const refetchPost = [{ query: BlogPostDocument, variables }]
  const [updatePost] = useMutation(UpdateBlogPostDocument, { refetchQueries: refetchPost })
  const [upsertContent] = useMutation(UpsertBlogPostContentDocument, { refetchQueries: refetchPost })
  const [setCategories] = useMutation(SetBlogPostCategoriesDocument, { refetchQueries: refetchPost })
  const [setTags] = useMutation(SetBlogPostTagsDocument, { refetchQueries: refetchPost })
  const [publishMut, { loading: publishing }] = useMutation(PublishBlogPostDocument, { refetchQueries: [...refetchPost, ...refetchList] })
  const [scheduleMut, { loading: scheduling }] = useMutation(ScheduleBlogPostDocument, { refetchQueries: [...refetchPost, ...refetchList] })
  const [unpublishMut, { loading: unpublishing }] = useMutation(UnpublishBlogPostDocument, { refetchQueries: [...refetchPost, ...refetchList] })
  const [archiveMut, { loading: archiving }] = useMutation(ArchiveBlogPostDocument, { refetchQueries: [...refetchPost, ...refetchList] })
  const [deletePost] = useMutation(DeleteBlogPostDocument, { refetchQueries: refetchList })
  const [deleteContent] = useMutation(DeleteBlogPostContentDocument, { refetchQueries: refetchPost, awaitRefetchQueries: true })
  // Media-library upload pair for the body's Insert Image - images go into a per-post directory in the
  // shared library; the editor inserts a figure/caption node with the returned URL
  const [requestMediaUpload] = useMutation<RequestMediaAssetUploadMutation, RequestMediaAssetUploadMutationVariables>(RequestMediaAssetUploadDocument)
  const [confirmMediaUpload] = useMutation<ConfirmMediaAssetUploadMutation, ConfirmMediaAssetUploadMutationVariables>(ConfirmMediaAssetUploadDocument)

  const { data: catsData } = useQuery(BlogCategoriesDocument)
  const { data: tagsData } = useQuery(BlogTagsDocument)
  const flatCategories = useMemo(() => flattenCategories(catsData?.blogCategories ?? []), [catsData])
  const allTags = useMemo(() => tagsData?.blogTags ?? [], [tagsData])

  const [confirmDelete, setConfirmDelete] = useState(false)
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; msg: string } | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  // UI state, not the admin's work - an existing slug is never rewritten from the title
  const [slugTouched, setSlugTouched] = useState(!!post.slug)

  const baseline = useMemo(() => toPostValues(post, locale), [post, locale])
  const schema = useMemo(() => postSchema(baseline), [baseline])
  const diff = useMemo(
    () =>
      postDiff({
        category: (id) => flatCategories.find((c) => c.id === id)?.label.replace(/^(– )+/, '') ?? post.categories.find((c) => c.id === id)?.slug ?? id,
        tag: (id) => {
          const t = allTags.find((x) => x.id === id)
          return t ? (t.content?.name ?? t.slug) : (post.tags.find((x) => x.id === id)?.slug ?? id)
        },
      }),
    [flatCategories, allTags, post.categories, post.tags],
  )

  const record = useRecordForm({
    id: 'post',
    title: `Post (${locale})`,
    label: `Blog post · ${post.slug}`,
    isEdit: true,
    baseline,
    schema,
    diff,
    persist: { routeKey: `blog-post:${post.id}:${locale}` },
    submit: async (v) => {
      const changes = postChanges(baseline, v)
      if (changes.settings) await updatePost({ variables: { input: { id: post.id, ...changes.settings } } })
      if (changes.content) await upsertContent({ variables: { input: { postId: post.id, locale, ...changes.content } } })
      if (changes.categoryIds) await setCategories({ variables: { postId: post.id, categoryIds: changes.categoryIds } })
      if (changes.tagIds) await setTags({ variables: { postId: post.id, tagIds: changes.tagIds } })
    },
    onSaved: () => setFeedback({ kind: 'ok', msg: 'Saved.' }),
  })
  const { form, registry, confirm, draft } = record
  const values = useStore(form.store, (s) => s.values)
  const submitting = useStore(form.store, (s) => s.isSubmitting)
  const totalDirty = registry.dirtySections.reduce((n, s) => n + s.diff.length, 0)

  // The formatting bar is drawn by the body's editor, into this element floating over the page
  const [bar, setBar] = useState<HTMLElement | null>(null)
  const [panelOpen, setPanelOpen] = useState(true)
  const [zoom, setZoom] = usePageZoom()
  const [noindexEdits, setNoindexEdits] = useState<Map<string, boolean>>(new Map())

  const localesWithContent = post.allContent.map((c) => c.locale)
  const displayTitle = post.content?.title?.trim() || post.allContent[0]?.title?.trim() || post.slug
  const lifecycleBusy = publishing || scheduling || unpublishing || archiving

  async function runAction(fn: () => Promise<unknown>, failure: string) {
    try {
      await fn()
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : failure })
    }
  }

  async function onDelete() {
    try {
      await deletePost({ variables: { id: post.id } })
      navigate('/blog/posts', { replace: true })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Delete failed.' })
    }
  }

  // Immediate, like publish/archive - then follow the content that's left so the form isn't sitting
  // on an empty locale
  async function onRemoveTranslation() {
    await deleteContent({ variables: { postId: post.id, locale } })
    const remaining = localesWithContent.filter((loc) => loc !== locale).sort()
    setFeedback({ kind: 'ok', msg: `Removed the ${locale} translation.` })
    onLocaleRemoved(remaining[0] ?? null)
  }

  // request -> S3 PUT -> confirm, then the shape the editor needs for a figure/caption block. Scoped
  // under blog/posts/<postId> so a per-post bulk delete stays possible
  async function uploadBodyImage(file: File): Promise<RichTextImageUploadResult | null> {
    if (!file.type.startsWith('image/')) throw new Error('Only image files can be uploaded.')
    const altText = window.prompt('Image alt text (optional)', '')?.trim() ?? ''
    const dims = await readImageDimensions(file)
    const requested = await requestMediaUpload({
      variables: { input: { path: `blog/posts/${post.id}`, filename: file.name, mimeType: file.type, sizeBytes: file.size, altText } },
    })
    const payload = requested.data?.requestMediaAssetUpload
    if (!payload) throw new Error('Upload request did not return a signed URL.')
    await putToSignedUrl(payload.uploadUrl, file, 'Image upload')
    const confirmed = await confirmMediaUpload({ variables: { assetId: payload.asset.id, width: dims.width, height: dims.height } })
    const asset = confirmed.data?.confirmMediaAssetUpload
    if (!asset) throw new Error('Image upload confirmation failed.')
    return { altText: asset.altText || altText, height: asset.height ?? dims.height, width: asset.width ?? dims.width, src: asset.url }
  }

  const readingMinutes = Number.parseInt(values.readingTime, 10)
  const written = richTextToPlainText(values.body as RichTextValue).trim()
  const words = written ? written.split(/\s+/).length : 0
  const readEstimate = Math.max(1, Math.round(words / WORDS_A_MINUTE))

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <StoreNoindexSave postId={post.id} listings={post.channelListings} refetchVariables={variables} edits={noindexEdits} onSaved={() => setNoindexEdits(new Map())} />
      <DetailPageLayout
        className={WORKBENCH}
        rightSidebar={
          previewOpen ? (
            <BlogPreviewPanel
              onClose={() => setPreviewOpen(false)}
              postId={post.id}
              locale={locale || undefined}
              state={{
                id: post.id,
                slug: values.slug || post.slug,
                publishedAt: post.publishedAt ?? null,
                readingTimeMinutes: Number.isFinite(readingMinutes) ? readingMinutes : null,
                seoNoindex: values.seoNoindex,
                author: post.author ? { id: post.author.id, slug: post.author.slug, displayName: post.author.displayName } : null,
                content: {
                  locale,
                  title: values.title,
                  excerpt: values.excerpt as RichTextValue,
                  body: values.body as RichTextValue,
                  metaTitle: values.metaTitle || null,
                  metaDescription: values.metaDescription || null,
                },
                // urlPath is computed server-side and isn't in the dashboard's fragment - the channel's
                // initial fetch supplies it; the live push just refreshes the editable fields
                categories: post.categories.map((c) => ({ id: c.id, slug: c.slug, content: c.content ? { name: c.content.name } : null })),
                tags: post.tags.map((t) => ({ id: t.id, slug: t.slug, content: t.content ? { name: t.content.name } : null })),
              }}
            />
          ) : panelOpen ? (
            <PostPanel
              post={post}
              form={form}
              values={values}
              locale={locale}
              canManage={canManage}
              canEditTranslations={canEditTranslations}
              categories={flatCategories}
              tags={allTags}
              busy={lifecycleBusy}
              variables={variables}
              noindexEdits={noindexEdits}
              onNoindex={setNoindexEdits}
              onSchedule={(at) => runAction(() => scheduleMut({ variables: { id: post.id, publishedAt: new Date(at).toISOString() } }), 'Schedule failed.')}
              onArchive={() => runAction(() => archiveMut({ variables: { id: post.id } }), 'Archive failed.')}
              onDelete={() => setConfirmDelete(true)}
              onRemoveTranslation={onRemoveTranslation}
              onSlugTyped={() => setSlugTouched(true)}
            />
          ) : null
        }
      >
        <div className="flex min-h-full flex-col">
          <div className="sticky top-0 z-10">
            <div className="flex min-h-[49px] flex-wrap items-center gap-3 border-b border-foreground/10 bg-background/85 px-4 py-2 backdrop-blur-md">
              <div className="flex min-w-0 flex-1 items-center gap-1.5">
                <Button asChild variant="ghost" size="icon" className="-ml-1 size-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground" aria-label="Back to posts">
                  <Link to="/blog/posts">
                    <ArrowLeftIcon className="size-4" />
                  </Link>
                </Button>
                <h1 className="min-w-0 truncate text-sm font-medium tracking-tight">{values.title.trim() || displayTitle}</h1>
                <span className="flex shrink-0 items-center gap-1.5 rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
                  <span aria-hidden className={cn('size-1.5 rounded-full', STATUS_DOT[post.status] ?? 'bg-muted-foreground/50')} />
                  {statusLabel(post.status)}
                </span>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant={panelOpen && !previewOpen ? 'secondary' : 'outline'}
                  aria-pressed={panelOpen && !previewOpen}
                  onClick={() => {
                    setPanelOpen(previewOpen ? true : !panelOpen)
                    setPreviewOpen(false)
                  }}
                >
                  <PanelRightIcon className="size-4" /> Details
                </Button>
                <Button type="button" size="sm" variant={previewOpen ? 'secondary' : 'outline'} aria-pressed={previewOpen} onClick={() => setPreviewOpen(!previewOpen)} title="Open live preview side panel">
                  <EyeIcon className="size-4" /> Preview
                </Button>
                {canManage && (
                  <>
                    {post.status === 'PUBLISHED' ? (
                      <Button type="button" size="sm" variant="outline" onClick={() => runAction(() => unpublishMut({ variables: { id: post.id } }), 'Unpublish failed.')} disabled={lifecycleBusy}>
                        <CircleOffIcon className="size-4" /> Unpublish
                      </Button>
                    ) : (
                      <Button type="button" size="sm" variant="outline" onClick={() => runAction(() => publishMut({ variables: { id: post.id } }), 'Publish failed.')} disabled={lifecycleBusy}>
                        <CheckCircle2Icon className="size-4" /> Publish
                      </Button>
                    )}
                    <Button type="button" size="sm" disabled={!registry.isDirty || submitting} onClick={record.save}>
                      {submitting ? 'Saving…' : registry.isDirty ? `Save changes${totalDirty > 0 ? ` (${totalDirty})` : ''}` : 'No changes'}
                    </Button>
                  </>
                )}
              </div>
            </div>
            <div className="pointer-events-none flex justify-center px-4 pt-3">
              <div
                ref={setBar}
                role="toolbar"
                aria-label="Formatting"
                className="pointer-events-auto max-w-full rounded-full border border-foreground/10 bg-background/90 px-2 py-1 shadow-[0_1px_2px_rgb(0_0_0/0.06),0_10px_30px_-12px_rgb(0_0_0/0.25)] backdrop-blur-md empty:hidden"
              />
            </div>
          </div>

          {draft.offer !== null && (
            <div className="px-6 pt-4">
              <RestoredDraftBanner draft={draft} labels={POST_LABELS} />
            </div>
          )}
          {feedback && (
            <div className="px-6 pt-4">
              <Alert variant={feedback.kind === 'ok' ? 'default' : 'destructive'}>
                <AlertTitle>{feedback.kind === 'ok' ? 'Saved' : 'Error'}</AlertTitle>
                <AlertDescription>{feedback.msg}</AlertDescription>
              </Alert>
            </div>
          )}

          <div className="@container flex-1 px-4 pt-6 pb-10">
            <article
              aria-label="Post"
              style={{ zoom: zoom / 100 }}
              className="mx-auto min-h-[60rem] w-full max-w-[50rem] rounded-2xl bg-background px-8 py-12 shadow-[0_1px_1px_rgb(0_0_0/0.04),0_32px_80px_-32px_rgb(0_0_0/0.28)] ring-1 ring-foreground/5 @3xl:px-24 @3xl:py-20"
            >
              <form.Field
                name="title"
                listeners={{
                  // Fills a post's missing slug from its title - never overwrites one it has
                  onChange: ({ value }) => {
                    if (!slugTouched && !form.getFieldValue('slug')) form.setFieldValue('slug', slugify(value))
                  },
                }}
              >
                {(f) => (
                  <div className="mb-3 grid gap-1">
                    {/* The title wraps and grows with what is typed: an unseen copy of the text in the same cell sets the height */}
                    <div data-copy={`${f.state.value} `} className={cn('grid after:invisible after:col-start-1 after:row-start-1 after:break-words after:whitespace-pre-wrap after:content-[attr(data-copy)]', TITLE_TYPE)}>
                      <textarea
                        id="title"
                        aria-label="Title"
                        rows={1}
                        value={f.state.value}
                        disabled={!canManage}
                        placeholder="Untitled post"
                        onChange={(e) => f.handleChange(e.target.value.replace(/\n/g, ' '))}
                        onBlur={f.handleBlur}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') e.preventDefault()
                        }}
                        className="col-start-1 row-start-1 w-full resize-none overflow-hidden bg-transparent font-[inherit] tracking-[inherit] text-foreground outline-none placeholder:text-muted-foreground/40"
                      />
                    </div>
                    {fieldErrorText(f) ? <p className="text-sm text-destructive">{fieldErrorText(f)}</p> : null}
                  </div>
                )}
              </form.Field>
              <p className="mb-9 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-muted-foreground">
                {post.author ? <span>{post.author.displayName}</span> : null}
                {post.publishedAt ? <span>{new Date(post.publishedAt).toLocaleDateString(undefined, { day: 'numeric', month: 'long', year: 'numeric' })}</span> : null}
                {words > 0 ? <span>{readEstimate} min read</span> : null}
              </p>
              <form.Field name="body">
                {(f) => (
                  <FormRichTextEditor
                    value={(f.state.value as RichTextValue) ?? undefined}
                    fallbackText=""
                    disabled={!canManage}
                    placeholder="Start writing…"
                    page={{ toolbar: bar }}
                    onChange={(v) => f.handleChange(v)}
                    onUploadImage={canManage ? uploadBodyImage : undefined}
                  />
                )}
              </form.Field>
            </article>
          </div>

          <PostCapsule
            words={words}
            readEstimate={readEstimate}
            onUseEstimate={canManage && values.readingTime !== String(readEstimate) ? () => form.setFieldValue('readingTime', String(readEstimate)) : undefined}
            locale={locale}
            unsaved={registry.isDirty ? totalDirty : null}
            zoom={zoom}
            onZoom={setZoom}
          />
        </div>

        <ConfirmDialog
          open={confirmDelete}
          onOpenChange={setConfirmDelete}
          title="Delete post?"
          description="This permanently removes the post and all its translations. URL redirects are preserved."
          confirmLabel="Delete"
          // Not awaited: the dialog closes at once and a failure shows in the page feedback, as it always has
          onConfirm={() => void onDelete()}
        />

        <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={`Locale ${locale}`} sections={registry.dirtySections} />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// Shared by the title and the unseen copy that sizes it, which must wrap exactly alike
const TITLE_TYPE = 'text-[2.6rem] leading-[1.1] font-semibold tracking-[-0.025em]'

const WORDS_A_MINUTE = 220
