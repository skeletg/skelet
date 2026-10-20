import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import {
  ArchiveIcon,
  ArrowLeftIcon,
  CheckCircle2Icon,
  CircleOffIcon,
  ClockIcon,
  EyeIcon,
  Trash2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { DetailPageLayout } from '@/components/layout/detail-page'
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
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  FormRichTextEditor,
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
import { TranslationLocales } from '@/components/TranslationLocales'
import {
  ArchiveBlogPostDocument,
  BlogCategoriesDocument,
  BlogPostDocument,
  BlogTagsDocument,
  DeleteBlogPostContentDocument,
  DeleteBlogPostDocument,
  DeleteBlogPostChannelListingDocument,
  PublishBlogPostDocument,
  ScheduleBlogPostDocument,
  SetBlogPostCategoriesDocument,
  SetBlogPostChannelListingDocument,
  SetBlogPostTagsDocument,
  UnpublishBlogPostDocument,
  UpdateBlogPostDocument,
  UpsertBlogPostContentDocument,
  type BlogPostQuery,
  BlogPostsDocument,
} from './Blog.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { slugify } from '@/lib/slug'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { RestoredDraftBanner } from '@/shared/saving/RestoredDraftBanner'
import { SaveRegistryContext, useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { changed } from '@/shared/saving/diff'
import { BlogPreviewPanel } from './BlogPreviewPanel'
import { POST_LABELS, postChanges, postDiff, postSchema, toPostValues } from './schemas/post'
import { readImageDimensions } from '@/lib/image'
import { putToSignedUrl } from '@/lib/upload'

function statusLabel(status: string) {
  return status.charAt(0) + status.slice(1).toLowerCase()
}

function statusVariant(status: string) {
  switch (status) {
    case 'PUBLISHED':
      return 'secondary'
    case 'SCHEDULED':
      return 'outline'
    case 'ARCHIVED':
      return 'destructive'
    default:
      return 'outline'
  }
}

type Post = NonNullable<BlogPostQuery['blogPost']>

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
  const [deletePost, { loading: deleting }] = useMutation(DeleteBlogPostDocument, { refetchQueries: refetchList })
  const [deleteContent] = useMutation(DeleteBlogPostContentDocument, { refetchQueries: refetchPost, awaitRefetchQueries: true })
  // Media-library upload pair for the body's Insert Image - images go into a per-post directory in the
  // shared library; the editor inserts a figure/caption node with the returned URL
  const [requestMediaUpload] = useMutation<RequestMediaAssetUploadMutation, RequestMediaAssetUploadMutationVariables>(RequestMediaAssetUploadDocument)
  const [confirmMediaUpload] = useMutation<ConfirmMediaAssetUploadMutation, ConfirmMediaAssetUploadMutationVariables>(ConfirmMediaAssetUploadDocument)

  const { data: catsData } = useQuery(BlogCategoriesDocument)
  const { data: tagsData } = useQuery(BlogTagsDocument)
  const flatCategories = useMemo(() => flattenCategories(catsData?.blogCategories ?? []), [catsData])
  const allTags = useMemo(() => tagsData?.blogTags ?? [], [tagsData])

  const [scheduleAt, setScheduleAt] = useState('')
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

  // Header auto-hides on scroll-down inside the layout's scroll container, comes back on scroll-up
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const lastScrollY = useRef(0)
  const [headerVisible, setHeaderVisible] = useState(true)
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const onScroll = () => {
      const y = el.scrollTop
      if (y < 32) {
        setHeaderVisible(true)
        lastScrollY.current = y
        return
      }
      const delta = y - lastScrollY.current
      // A small dead-zone so jitters don't toggle the header
      if (delta > 6) setHeaderVisible(false)
      else if (delta < -6) setHeaderVisible(true)
      lastScrollY.current = y
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
  }, [])

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
      setConfirmDelete(false)
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

  const toggle = (field: 'categoryIds' | 'tagIds', id: string) =>
    form.setFieldValue(field, (prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

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

  return (
    <SaveRegistryContext.Provider value={registry.contextValue}>
      <DetailPageLayout
        scrollRef={scrollRef}
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
          ) : null
        }
      >
        {/* Sticky action header - auto-hides on scroll-down, returns on scroll-up. translate-y, so the
            rest of the layout doesn't jump when it disappears */}
        <div
          className={
            'sticky top-0 z-10 border-b bg-background/80 backdrop-blur transition-transform duration-200 supports-[backdrop-filter]:bg-background/60 ' +
            (headerVisible ? 'translate-y-0' : '-translate-y-full')
          }
        >
          <div className="flex flex-wrap items-center gap-3 px-6 py-3">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <Button asChild variant="ghost" size="icon" className="-ml-2 size-8 shrink-0 text-muted-foreground hover:text-foreground" aria-label="Back to posts">
                  <Link to="/blog/posts">
                    <ArrowLeftIcon className="size-4" />
                  </Link>
                </Button>
                <h1 className="min-w-0 truncate text-xl font-semibold tracking-tight">{displayTitle}</h1>
                <Badge variant={statusVariant(post.status)}>{statusLabel(post.status)}</Badge>
                {post.publishedAt && (
                  <Badge variant="outline" className="gap-1">
                    <ClockIcon className="size-3" />
                    {new Date(post.publishedAt).toLocaleString()}
                  </Badge>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                <code className="font-mono">{post.slug}</code>
                {post.author && (
                  <>
                    <span>·</span>
                    <span>by {post.author.displayName}</span>
                  </>
                )}
                <span>·</span>
                <span>
                  {post.allContent.length} locale{post.allContent.length === 1 ? '' : 's'}
                </span>
                <span>·</span>
                <span>
                  {post.channelListings.filter((l) => l.isPublished).length} store
                  {post.channelListings.filter((l) => l.isPublished).length === 1 ? '' : 's'}
                </span>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button type="button" size="sm" variant="outline" onClick={() => setPreviewOpen(true)} title="Open live preview side panel">
                <EyeIcon className="size-4" /> Preview
              </Button>
              {canManage && (
                <>
                  {post.status === 'PUBLISHED' ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => runAction(() => unpublishMut({ variables: { id: post.id } }), 'Unpublish failed.')}
                      disabled={lifecycleBusy}
                    >
                      <CircleOffIcon className="size-4" /> Unpublish
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => runAction(() => publishMut({ variables: { id: post.id } }), 'Publish failed.')}
                      disabled={lifecycleBusy}
                    >
                      <CheckCircle2Icon className="size-4" /> Publish
                    </Button>
                  )}
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                    onClick={() => setConfirmDelete(true)}
                  >
                    <Trash2Icon className="size-4" />
                    Delete
                  </Button>
                  <Button type="button" size="sm" disabled={!registry.isDirty || submitting} onClick={record.save}>
                    {submitting ? 'Saving…' : registry.isDirty ? `Save changes${totalDirty > 0 ? ` (${totalDirty})` : ''}` : 'No changes'}
                  </Button>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Body - edge-to-edge sections divided by horizontal rules, no box wrappers */}
        <div className="divide-y pb-16">
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

          {/* ---- Schedule + Archive (Publish/Unpublish live in the header) ---- */}
          <section className="px-6 py-6">
            <div className="mb-3">
              <h2 className="text-base font-semibold">Schedule & lifecycle</h2>
              <p className="text-sm text-muted-foreground">Pick a future publish date or archive the post.</p>
            </div>
            <div className="flex flex-wrap items-end gap-3">
              <div className="flex items-end gap-2">
                <div className="grid gap-1">
                  <Label htmlFor="schedule" className="text-xs">
                    Schedule for
                  </Label>
                  <Input
                    id="schedule"
                    type="datetime-local"
                    value={scheduleAt}
                    onChange={(e) => setScheduleAt(e.target.value)}
                    className="w-56"
                    disabled={!canManage}
                  />
                </div>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() =>
                    runAction(() => scheduleMut({ variables: { id: post.id, publishedAt: new Date(scheduleAt).toISOString() } }), 'Schedule failed.')
                  }
                  disabled={!canManage || lifecycleBusy || !scheduleAt}
                >
                  <ClockIcon className="size-4" /> Schedule
                </Button>
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => runAction(() => archiveMut({ variables: { id: post.id } }), 'Archive failed.')}
                disabled={!canManage || lifecycleBusy || post.status === 'ARCHIVED'}
              >
                <ArchiveIcon className="size-4" /> Archive
              </Button>
            </div>
          </section>

          <fieldset disabled={!canManage} className="contents">
            {/* ---- Basics ---- */}
            <section className="px-6 py-6">
              <div className="mb-3">
                <h2 className="text-base font-semibold">Basics</h2>
                <p className="text-sm text-muted-foreground">Slug change creates a 301 redirect from the old path.</p>
              </div>
              <div className="grid gap-4">
                <form.AppField name="slug">{(f) => <f.TextField id="slug" label="Slug" onValueChange={() => setSlugTouched(true)} />}</form.AppField>
                <form.AppField name="readingTime">{(f) => <f.TextField id="reading-time" label="Reading time (min)" type="number" />}</form.AppField>
                <form.Field name="seoNoindex">
                  {(f) => (
                    <div className="flex items-center gap-2">
                      <Switch id="seo-noindex" checked={f.state.value} onCheckedChange={f.handleChange} />
                      <Label htmlFor="seo-noindex">Hide from search engines (noindex)</Label>
                    </div>
                  )}
                </form.Field>
              </div>
            </section>

            {/* ---- Content (per locale) ---- */}
            <section className="px-6 py-6">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h2 className="text-base font-semibold">Content</h2>
                  <p className="text-sm text-muted-foreground">Title, body, excerpt, and SEO meta - per locale.</p>
                </div>
                {/* Which locales have content, and the remove action for the one being edited - the
                    active locale is the header's; there is no in-form picker */}
                <TranslationLocales
                  locale={canEditTranslations ? locale : ''}
                  locales={localesWithContent}
                  entityLabel="post"
                  canManage={canManage}
                  defaultLocaleLocked
                  onRemove={onRemoveTranslation}
                />
              </div>
              <div className="grid gap-4">
                <form.AppField
                  name="title"
                  listeners={{
                    // Fills a post's missing slug from its title - never overwrites one it has
                    onChange: ({ value }) => {
                      if (!slugTouched && !form.getFieldValue('slug')) form.setFieldValue('slug', slugify(value))
                    },
                  }}
                >
                  {(f) => <f.TextField id="title" label="Title" />}
                </form.AppField>
                <div className="grid gap-1.5">
                  <Label>Excerpt</Label>
                  <form.Field name="excerpt">
                    {(f) => (
                      <FormRichTextEditor
                        value={(f.state.value as RichTextValue) ?? undefined}
                        fallbackText=""
                        disabled={!canManage}
                        placeholder="Short summary (used in feeds and listings)…"
                        onChange={(v) => f.handleChange(v)}
                      />
                    )}
                  </form.Field>
                </div>
                <div className="grid gap-1.5">
                  <Label>Body</Label>
                  <form.Field name="body">
                    {(f) => (
                      <FormRichTextEditor
                        value={(f.state.value as RichTextValue) ?? undefined}
                        fallbackText=""
                        disabled={!canManage}
                        placeholder="Write the post…"
                        onChange={(v) => f.handleChange(v)}
                        onUploadImage={canManage ? uploadBodyImage : undefined}
                      />
                    )}
                  </form.Field>
                </div>
                <form.AppField name="metaTitle">
                  {(f) => (
                    <f.TextField
                      id="meta-title"
                      label="Meta title"
                      placeholder={values.title || 'Post title'}
                      note={<LengthHint length={values.metaTitle.length} max={60} />}
                    />
                  )}
                </form.AppField>
                <form.AppField name="metaDescription">
                  {(f) => (
                    <f.TextareaField
                      id="meta-description"
                      label="Meta description"
                      rows={3}
                      placeholder="Short search-result description…"
                      note={<LengthHint length={values.metaDescription.length} max={160} />}
                    />
                  )}
                </form.AppField>
              </div>
            </section>

            {/* ---- Categories + Tags ---- */}
            <section className="px-6 py-6">
              <div className="mb-3">
                <h2 className="text-base font-semibold">Categories & tags</h2>
                <p className="text-sm text-muted-foreground">Toggle to attach. Applied with the header Save.</p>
              </div>
              <div className="grid gap-6">
                <div className="grid gap-2">
                  <Label className="text-xs uppercase tracking-wide text-muted-foreground">Categories</Label>
                  {flatCategories.length === 0 ? (
                    <p className="text-sm text-muted-foreground">No categories yet.</p>
                  ) : (
                    <ul className="grid gap-1 md:grid-cols-2">
                      {flatCategories.map((c) => (
                        <li key={c.id} className="flex items-center gap-2 rounded-md border px-2 py-1.5">
                          <Checkbox id={`cat-${c.id}`} checked={values.categoryIds.includes(c.id)} onCheckedChange={() => toggle('categoryIds', c.id)} />
                          <Label htmlFor={`cat-${c.id}`} className="flex-1 cursor-pointer text-sm font-normal">
                            {c.label}
                          </Label>
                          <code className="text-xs text-muted-foreground">{c.urlPath}</code>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
                <div className="grid gap-2">
                  <Label className="text-xs uppercase tracking-wide text-muted-foreground">Tags</Label>
                  {allTags.length === 0 ? (
                    <p className="text-sm text-muted-foreground">No tags yet.</p>
                  ) : (
                    <ul className="flex flex-wrap gap-2">
                      {allTags.map((t) => {
                        const checked = values.tagIds.includes(t.id)
                        return (
                          <li key={t.id}>
                            <Button type="button" variant={checked ? 'secondary' : 'outline'} size="sm" aria-pressed={checked} onClick={() => toggle('tagIds', t.id)}>
                              #{t.content?.name ?? t.slug}
                            </Button>
                          </li>
                        )
                      })}
                    </ul>
                  )}
                </div>
              </div>
            </section>
          </fieldset>

          <PostStoresSection postId={post.id} listings={post.channelListings} canManage={canManage} refetchVariables={variables} />

          <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>Delete post?</AlertDialogTitle>
                <AlertDialogDescription>This permanently removes the post and all its translations. URL redirects are preserved.</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={onDelete} disabled={deleting}>
                  {deleting ? 'Deleting…' : 'Delete'}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>

          <SaveConfirmDialog {...confirm.dialogProps} scopeLabel={`Locale ${locale}`} sections={registry.dirtySections} />
        </div>
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

// Search engines truncate past these lengths - a count beside the field, red once over
function LengthHint({ length, max }: { length: number; max: number }) {
  return <span className={`text-xs tabular-nums ${length > max ? 'text-destructive' : 'text-muted-foreground'}`}>{length}/{max}</span>
}

type FlatCategory = { id: string; label: string; urlPath: string }

// Structural shape of the category tree nodes this flattener walks; the generated query types inline each nesting
// level, so a recursive hand-written type is the practical way to accept the whole tree
type CategoryTreeNode = {
  id: string
  slug: string
  urlPath: string
  content?: { name: string } | null
  children?: CategoryTreeNode[] | null
}

function flattenCategories(roots: CategoryTreeNode[], depth = 0, out: FlatCategory[] = []): FlatCategory[] {
  for (const c of roots) {
    const name = c.content?.name ?? c.slug
    out.push({
      id: c.id,
      label: `${'– '.repeat(depth)}${name}`,
      urlPath: c.urlPath,
    })
    if (c.children?.length) flattenCategories(c.children, depth + 1, out)
  }
  return out
}

// PostStoresCard renders one row per active channel, exposing the is_published / seo_noindex listing toggles and a
// "remove from store" action - Stores without a listing display as off; toggling on creates the listing on first save
function PostStoresSection({
  postId,
  listings,
  canManage,
  refetchVariables,
}: {
  postId: string
  listings: { channelId: string; isPublished: boolean; seoNoindex: boolean }[]
  canManage: boolean
  refetchVariables: { id: string; locale?: string; includeUnpublished: boolean }
}) {
  const { data, loading } = useQuery(ChannelsDocument)
  const stores = data?.channels ?? []
  const refetch = [{ query: BlogPostDocument, variables: refetchVariables }]
  const [setListing, { loading: saving }] = useMutation(SetBlogPostChannelListingDocument, {
    refetchQueries: refetch,
  })
  const [deleteListing, { loading: deleting }] = useMutation(DeleteBlogPostChannelListingDocument, {
    refetchQueries: refetch,
  })

  const byStore = new Map(listings.map((l) => [l.channelId, l]))
  const active = stores.filter((s) => s.isActive)

  // Noindex is an SEO field, so it stages for the header Save - Published stays immediate, it's the post's lifecycle
  const [noindexEdits, setNoindexEdits] = useState<Map<string, boolean>>(new Map())
  const noindexDiff = active
    .map((sv) =>
      noindexEdits.has(sv.id) ? changed(`Store · ${sv.name} · noindex`, !!byStore.get(sv.id)?.seoNoindex, noindexEdits.get(sv.id)) : null,
    )
    .filter((e): e is SectionDiffEntry => e !== null)
  useSaveSection({
    id: 'store-noindex',
    title: 'Available on stores',
    diff: noindexDiff,
    save: async () => {
      for (const [channelId, seoNoindex] of noindexEdits) {
        const listing = byStore.get(channelId)
        if (!listing || listing.seoNoindex === seoNoindex) continue
        await setListing({ variables: { input: { postId, channelId, isPublished: listing.isPublished, seoNoindex } } })
      }
      setNoindexEdits(new Map())
    },
  })

  return (
    <section className="px-6 py-6">
      <div className="mb-3">
        <h2 className="text-base font-semibold">Available on stores</h2>
        <p className="text-sm text-muted-foreground">
          The post is global. Toggle a store on to publish it there, then optionally hide it from search.
        </p>
      </div>
      <div className="grid gap-2">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading stores…</p>
        ) : active.length === 0 ? (
          <p className="text-sm text-muted-foreground">No active channels configured.</p>
        ) : (
          <ul className="grid gap-2">
            {active.map((sv) => {
              const listing = byStore.get(sv.id)
              const enabled = !!listing?.isPublished
              const noindex = !!listing?.seoNoindex
              return (
                <li key={sv.id} className="flex flex-wrap items-center gap-3 rounded-md border px-3 py-2">
                  <div className="min-w-40 flex-1">
                    <p className="text-sm font-medium">{sv.name}</p>
                    <code className="text-xs text-muted-foreground">{sv.slug}</code>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Switch
                      id={`pub-${sv.id}`}
                      checked={enabled}
                      onCheckedChange={(v) =>
                        setListing({
                          variables: {
                            input: {
                              postId,
                              channelId: sv.id,
                              isPublished: v,
                              seoNoindex: noindex,
                            },
                          },
                        })
                      }
                      disabled={!canManage || saving}
                    />
                    <Label htmlFor={`pub-${sv.id}`} className="text-xs">
                      Published
                    </Label>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Switch
                      id={`ni-${sv.id}`}
                      checked={noindexEdits.get(sv.id) ?? noindex}
                      onCheckedChange={(v) => setNoindexEdits((prev) => new Map(prev).set(sv.id, v))}
                      disabled={!canManage || saving || !listing}
                    />
                    <Label htmlFor={`ni-${sv.id}`} className="text-xs">
                      Noindex
                    </Label>
                  </div>
                  {listing && canManage && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        deleteListing({ variables: { postId, channelId: sv.id } })
                      }
                      disabled={deleting}
                    >
                      Remove
                    </Button>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </section>
  )
}
