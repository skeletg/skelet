import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useUnsavedChangesGuard } from '@/lib/unsaved-changes'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
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
import { Textarea } from '@/components/ui/textarea'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  LexicalRichTextEditor,
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
  BlogPostsDocument,
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
} from './Blog.gql'
import { ChannelsDocument } from '@/features/channels/Channels.gql'
import { slugify } from '@/lib/slug'
import {
  SaveConfirmDialog,
  type ChangeSection,
  type SectionDiffEntry,
} from '@/shared/saving/SaveConfirmDialog'
import { BlogPreviewPanel } from './BlogPreviewPanel'

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

export function BlogPostDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { effectiveLocale, globalDefaultLocale } = useConfigScope()
  // Header drives the locale being edited - In GLOBAL mode (no effective locale) translation editing is disabled;
  // existing translations still render read-only so the post can be reviewed
  const scopedLocale = effectiveLocale || undefined
  const canEditTranslations = !!effectiveLocale

  // Locale is attached via the `Locale` request header (see lib/apollo.ts)
  const variables = { id, includeUnpublished: true }

  const { data, loading, error, refetch } = useQuery(BlogPostDocument, {
    variables,
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const post = data?.blogPost
  const refetchListVariables = { filter: { includeUnpublished: true }, limit: 200, offset: 0 }
  const refetchList = [{ query: BlogPostsDocument, variables: refetchListVariables }]
  const refetchPost = [{ query: BlogPostDocument, variables }]

  // ---- Mutations ----
  const [updatePost] = useMutation(UpdateBlogPostDocument, { refetchQueries: refetchPost })
  const [upsertContent] = useMutation(UpsertBlogPostContentDocument, {
    refetchQueries: refetchPost,
  })
  const [publishMut, { loading: publishing }] = useMutation(PublishBlogPostDocument, {
    refetchQueries: [...refetchPost, ...refetchList],
  })
  const [scheduleMut, { loading: scheduling }] = useMutation(ScheduleBlogPostDocument, {
    refetchQueries: [...refetchPost, ...refetchList],
  })
  const [unpublishMut, { loading: unpublishing }] = useMutation(UnpublishBlogPostDocument, {
    refetchQueries: [...refetchPost, ...refetchList],
  })
  const [archiveMut, { loading: archiving }] = useMutation(ArchiveBlogPostDocument, {
    refetchQueries: [...refetchPost, ...refetchList],
  })
  // Media-library upload pair, reused for the rich-text Insert Image button below - Images go into a per-post directory in the shared media
  // library; the editor inserts a ProductImage node (despite the name - it's the generic figure/caption decorator) with the returned URL
  const [requestMediaUpload] = useMutation<
    RequestMediaAssetUploadMutation,
    RequestMediaAssetUploadMutationVariables
  >(RequestMediaAssetUploadDocument)
  const [confirmMediaUpload] = useMutation<
    ConfirmMediaAssetUploadMutation,
    ConfirmMediaAssetUploadMutationVariables
  >(ConfirmMediaAssetUploadDocument)

  const [setCategories] = useMutation(SetBlogPostCategoriesDocument, { refetchQueries: refetchPost })
  const [setTags] = useMutation(SetBlogPostTagsDocument, { refetchQueries: refetchPost })
  const [deletePost, { loading: deleting }] = useMutation(DeleteBlogPostDocument, { refetchQueries: refetchList })
  const [deleteContent] = useMutation(DeleteBlogPostContentDocument, {
    refetchQueries: refetchPost,
    awaitRefetchQueries: true,
  })

  // ---- Form state ----

  // deriveEditingLocale picks the locale the form edits on (re)load: the header's effectiveLocale, or in GLOBAL
  // mode the globalDefaultLocale when the post has content for it

  // Falling through to allContent[0] is alphabetical-by-locale-code, which hides recent work and reads as "I
  // saved content and on refresh it's gone"
  function deriveEditingLocale() {
    if (!post) return ''
    const localesWithContent = post.allContent.map((c) => c.locale)
    const preferredExisting =
      globalDefaultLocale && localesWithContent.includes(globalDefaultLocale)
        ? globalDefaultLocale
        : post.content?.locale ?? post.allContent[0]?.locale ?? ''
    return scopedLocale ?? post.content?.locale ?? preferredExisting ?? globalDefaultLocale
  }
  const contentFor = (locale: string) => post?.allContent.find((x) => x.locale === locale)

  // Drafts hydrate from the loaded post (initializers cover a cache-hit first render) and re-sync via the render-time
  // adjust blocks below, which fire on exactly the post, locale and default-locale changes
  const [slug, setSlug] = useState(post?.slug ?? '')
  const [seoNoindex, setSeoNoindex] = useState(post?.seoNoindex ?? false)
  const [readingTime, setReadingTime] = useState(post?.readingTimeMinutes ? String(post.readingTimeMinutes) : '')
  const [editingLocale, setEditingLocale] = useState<string>(deriveEditingLocale)
  const [title, setTitle] = useState(() => contentFor(editingLocale)?.title ?? '')
  const [body, setBody] = useState<RichTextValue>(() => (contentFor(editingLocale)?.body as RichTextValue) ?? null)
  const [excerpt, setExcerpt] = useState<RichTextValue>(() => (contentFor(editingLocale)?.excerpt as RichTextValue) ?? null)
  const [metaTitle, setMetaTitle] = useState(() => contentFor(editingLocale)?.metaTitle ?? '')
  const [metaDescription, setMetaDescription] = useState(() => contentFor(editingLocale)?.metaDescription ?? '')
  const [scheduleAt, setScheduleAt] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [feedback, setFeedback] = useState<{ kind: 'ok' | 'err'; msg: string } | null>(null)
  // Global save flow
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  // Live preview side panel - open + debounced state pushed to the iframe
  const [previewOpen, setPreviewOpen] = useState(false)
  // slugTouched tracks whether the user has manually edited the slug - While
  // false, typing in the title also rewrites the slug - Once true, slug stays
  // independent - Reset to true on initial post load - we never want to clobber
  // an existing slug just because someone edits the title
  const [slugTouched, setSlugTouched] = useState(true)
  // Header auto-hides on scroll-down inside the inner scroll container, comes
  // back on scroll-up - Tracks the previous Y so we can compute direction
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const lastScrollY = useRef(0)
  const [headerVisible, setHeaderVisible] = useState(true)
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const onScroll = () => {
      const y = el.scrollTop
      // Always reveal at the very top
      if (y < 32) {
        setHeaderVisible(true)
        lastScrollY.current = y
        return
      }
      const delta = y - lastScrollY.current
      // Add a small dead-zone so jitters don't toggle the header
      if (delta > 6) setHeaderVisible(false)
      else if (delta < -6) setHeaderVisible(true)
      lastScrollY.current = y
    }
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => el.removeEventListener('scroll', onScroll)
    // Re-run once `post` becomes truthy: the inner scroll container is only rendered after the loading/error/!post
    // early-returns, so on first mount scrollRef.current is null
  }, [post])

  // Re-hydrate the basics + re-pick the editing locale when the post id or the header locale scope changes - State is
  // adjusted during render, keyed on the post id, the scoped locale and the global default locale
  const [prevScope, setPrevScope] = useState({ postId: post?.id, scopedLocale, globalDefaultLocale })
  if (
    prevScope.postId !== post?.id ||
    prevScope.scopedLocale !== scopedLocale ||
    prevScope.globalDefaultLocale !== globalDefaultLocale
  ) {
    setPrevScope({ postId: post?.id, scopedLocale, globalDefaultLocale })
    if (post) {
      setSlug(post.slug)
      setSeoNoindex(post.seoNoindex)
      setReadingTime(post.readingTimeMinutes ? String(post.readingTimeMinutes) : '')
      setEditingLocale(deriveEditingLocale())
    }
  }

  // Load the current locale's content when the locale changes (adjusted during
  // render, keyed on the editing locale, the post id and its content count)
  const [prevContentKey, setPrevContentKey] = useState({
    editingLocale,
    postId: post?.id,
    contentCount: post?.allContent.length,
  })
  if (
    prevContentKey.editingLocale !== editingLocale ||
    prevContentKey.postId !== post?.id ||
    prevContentKey.contentCount !== post?.allContent.length
  ) {
    setPrevContentKey({ editingLocale, postId: post?.id, contentCount: post?.allContent.length })
    if (post) {
      const c = contentFor(editingLocale)
      setTitle(c?.title ?? '')
      setBody((c?.body as RichTextValue) ?? null)
      setExcerpt((c?.excerpt as RichTextValue) ?? null)
      setMetaTitle(c?.metaTitle ?? '')
      setMetaDescription(c?.metaDescription ?? '')
    }
  }

  // Re-sync draft category / tag selections when the post refetches
  const postCategoryIds = useMemo(
    () => (post?.categories ?? []).map((c) => c.id).slice().sort(),
    [post?.categories],
  )
  const postTagIds = useMemo(
    () => (post?.tags ?? []).map((t) => t.id).slice().sort(),
    [post?.tags],
  )
  // Draft category / tag selections - clicks update these; the global Save
  // flushes the deltas via setCategories / setTags - Re-synced during render
  // when the persisted id set changes (keyed on the joined ids, like the
  // old effects, so refetches with an identical set don't clobber drafts)
  const [draftCategoryIds, setDraftCategoryIds] = useState<string[]>(postCategoryIds)
  const [draftTagIds, setDraftTagIds] = useState<string[]>(postTagIds)
  const categoryKey = postCategoryIds.join('|')
  const [prevCategoryKey, setPrevCategoryKey] = useState(categoryKey)
  if (prevCategoryKey !== categoryKey) {
    setPrevCategoryKey(categoryKey)
    setDraftCategoryIds(postCategoryIds)
  }
  const tagKey = postTagIds.join('|')
  const [prevTagKey, setPrevTagKey] = useState(tagKey)
  if (prevTagKey !== tagKey) {
    setPrevTagKey(tagKey)
    setDraftTagIds(postTagIds)
  }

  const localesWithContent = useMemo(
    () => post?.allContent.map((c) => c.locale) ?? [],
    [post?.allContent],
  )

  const { data: catsData } = useQuery(BlogCategoriesDocument)
  const { data: tagsData } = useQuery(BlogTagsDocument)
  const flatCategories = useMemo(() => flattenCategories(catsData?.blogCategories ?? []), [catsData])
  const allTags = useMemo(() => tagsData?.blogTags ?? [], [tagsData])

  // Active locales across the whole platform - used to populate the picker for adding a translation - Already-edited locales render as pill
  // buttons; The header drives the active locale and there is no in-form picker, so the full list of active locales is not fetched here

  // ---- Diff between drafts and the loaded post ---- All memos below must run on every render - keeping them above the
  // early returns is what preserves stable hook order between the loading and loaded passes
  const localeContent = post?.allContent.find((x) => x.locale === editingLocale)
  const originalReadingTime = post?.readingTimeMinutes ? String(post.readingTimeMinutes) : ''
  const originalTitle = localeContent?.title ?? ''
  const originalBody = (localeContent?.body as RichTextValue) ?? null
  const originalExcerpt = (localeContent?.excerpt as RichTextValue) ?? null
  const originalMetaTitle = localeContent?.metaTitle ?? ''
  const originalMetaDescription = localeContent?.metaDescription ?? ''

  const basicsDiff = useMemo<SectionDiffEntry[]>(() => {
    if (!post) return []
    const out: SectionDiffEntry[] = []
    if (slug !== post.slug) out.push({ label: 'Slug', before: post.slug, after: slug })
    if (seoNoindex !== post.seoNoindex) {
      out.push({ label: 'Noindex', before: post.seoNoindex ? 'yes' : 'no', after: seoNoindex ? 'yes' : 'no' })
    }
    if (readingTime !== originalReadingTime) {
      out.push({ label: 'Reading time', before: originalReadingTime || '-', after: readingTime || '-' })
    }
    return out
  }, [post, slug, seoNoindex, readingTime, originalReadingTime])

  const contentDiff = useMemo<SectionDiffEntry[]>(() => {
    if (!post) return []
    const out: SectionDiffEntry[] = []
    if (title !== originalTitle) out.push({ label: 'Title', before: originalTitle || '-', after: title || '-' })
    if (JSON.stringify(body) !== JSON.stringify(originalBody)) {
      out.push({ label: 'Body', before: 'previous', after: 'updated' })
    }
    if (JSON.stringify(excerpt) !== JSON.stringify(originalExcerpt)) {
      out.push({ label: 'Excerpt', before: 'previous', after: 'updated' })
    }
    if (metaTitle !== originalMetaTitle) {
      out.push({ label: 'Meta title', before: originalMetaTitle || '-', after: metaTitle || '-' })
    }
    if (metaDescription !== originalMetaDescription) {
      out.push({ label: 'Meta description', before: originalMetaDescription || '-', after: metaDescription || '-' })
    }
    return out
  }, [post, title, originalTitle, body, originalBody, excerpt, originalExcerpt, metaTitle, originalMetaTitle, metaDescription, originalMetaDescription])

  const categoriesDiff = useMemo<SectionDiffEntry[]>(() => {
    if (!post) return []
    const out: SectionDiffEntry[] = []
    const orig = new Set(postCategoryIds)
    const draft = new Set(draftCategoryIds)
    const labelById = new Map(flatCategories.map((c) => [c.id, c.label]))
    const lookup = (id: string) => labelById.get(id) ?? post.categories.find((c) => c.id === id)?.slug ?? id
    for (const id of draft) if (!orig.has(id)) out.push({ label: 'Add', before: '-', after: lookup(id) })
    for (const id of orig) if (!draft.has(id)) out.push({ label: 'Remove', before: lookup(id), after: '-' })
    return out
  }, [post, postCategoryIds, draftCategoryIds, flatCategories])

  const tagsDiff = useMemo<SectionDiffEntry[]>(() => {
    if (!post) return []
    const out: SectionDiffEntry[] = []
    const orig = new Set(postTagIds)
    const draft = new Set(draftTagIds)
    const labelById = new Map(allTags.map((t) => [t.id, t.content?.name ?? t.slug]))
    const lookup = (id: string) => labelById.get(id) ?? post.tags.find((t) => t.id === id)?.slug ?? id
    for (const id of draft) if (!orig.has(id)) out.push({ label: 'Add', before: '-', after: lookup(id) })
    for (const id of orig) if (!draft.has(id)) out.push({ label: 'Remove', before: lookup(id), after: '-' })
    return out
  }, [post, postTagIds, draftTagIds, allTags])

  const changeSections = useMemo<ChangeSection[]>(() => {
    const out: ChangeSection[] = []
    if (basicsDiff.length) out.push({ id: 'basics', title: 'Post settings', diff: basicsDiff })
    if (contentDiff.length) out.push({ id: 'content', title: `Content (${editingLocale})`, diff: contentDiff })
    if (categoriesDiff.length) out.push({ id: 'categories', title: 'Categories', diff: categoriesDiff })
    if (tagsDiff.length) out.push({ id: 'tags', title: 'Tags', diff: tagsDiff })
    return out
  }, [basicsDiff, contentDiff, categoriesDiff, tagsDiff, editingLocale])

  const flushSave = useCallback(async () => {
    if (!post) return
    if (basicsDiff.length) {
      await updatePost({
        variables: {
          input: {
            id: post.id,
            slug: slug !== post.slug ? slug : null,
            seoNoindex,
            readingTimeMinutes: readingTime ? Number(readingTime) : null,
          },
        },
      })
    }
    if (contentDiff.length) {
      await upsertContent({
        variables: {
          input: {
            postId: post.id,
            locale: editingLocale,
            title,
            body,
            excerpt,
            metaTitle,
            metaDescription,
          },
        },
      })
    }
    if (categoriesDiff.length) {
      await setCategories({ variables: { postId: post.id, categoryIds: draftCategoryIds } })
    }
    if (tagsDiff.length) {
      await setTags({ variables: { postId: post.id, tagIds: draftTagIds } })
    }
  }, [
    post, basicsDiff.length, contentDiff.length, categoriesDiff.length, tagsDiff.length,
    slug, seoNoindex, readingTime, editingLocale, title, body, excerpt,
    metaTitle, metaDescription, draftCategoryIds, draftTagIds,
    updatePost, upsertContent, setCategories, setTags,
  ])

  // Above the early returns - a hook can't sit next to isDirty further down, and the label falls
  // back to the workspace tab's own title
  useUnsavedChangesGuard(changeSections.length > 0)

  if (loading) return <LoadingState label="Loading post…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load post"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!post) return <p className="text-sm text-muted-foreground">Post not found.</p>

  const p = post // narrow once for the JSX below
  const displayTitle =
    p.content?.title?.trim() || p.allContent[0]?.title?.trim() || p.slug

  const isDirty = changeSections.length > 0
  const totalDirty = changeSections.reduce((n, s) => n + s.diff.length, 0)

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await flushSave()
      setConfirmingSave(false)
      setFeedback({ kind: 'ok', msg: 'Saved.' })
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
    } finally {
      setSavingAll(false)
    }
  }

  async function onPublish() {
    try {
      await publishMut({ variables: { id: p.id } })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Publish failed.' })
    }
  }
  async function onSchedule() {
    if (!scheduleAt) return
    try {
      await scheduleMut({ variables: { id: p.id, publishedAt: new Date(scheduleAt).toISOString() } })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Schedule failed.' })
    }
  }
  async function onUnpublish() {
    try {
      await unpublishMut({ variables: { id: p.id } })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Unpublish failed.' })
    }
  }
  async function onArchive() {
    try {
      await archiveMut({ variables: { id: p.id } })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Archive failed.' })
    }
  }
  async function onDelete() {
    try {
      await deletePost({ variables: { id: p.id } })
      navigate('/blog/posts', { replace: true })
    } catch (err) {
      setFeedback({ kind: 'err', msg: err instanceof Error ? err.message : 'Delete failed.' })
      setConfirmDelete(false)
    }
  }

  // Immediate action, like publish/archive above - the staged Save flow
  // only covers edits to the fields
  async function onRemoveTranslation() {
    await deleteContent({ variables: { postId: p.id, locale: editingLocale } })
    // Follow the content that's left so the form isn't sitting on an empty locale
    const remaining = localesWithContent.filter((loc) => loc !== editingLocale)
    if (remaining.length) setEditingLocale(remaining.slice().sort()[0])
    setFeedback({ kind: 'ok', msg: `Removed the ${editingLocale} translation.` })
  }

  function toggleCategory(catId: string) {
    setDraftCategoryIds((prev) =>
      prev.includes(catId) ? prev.filter((id) => id !== catId) : [...prev, catId],
    )
  }
  function toggleTag(tagId: string) {
    setDraftTagIds((prev) =>
      prev.includes(tagId) ? prev.filter((id) => id !== tagId) : [...prev, tagId],
    )
  }

  const lifecycleBusy = publishing || scheduling || unpublishing || archiving

  // uploadBodyImage handles the request → S3 PUT → confirm pair the
  // media-library exposes, then returns the shape the rich-text
  // editor needs to mint a figure/caption block - Path scopes images
  // under blog/posts/<postId> so per-post bulk delete is feasible
  // later - Alt text is prompted up front and stamped on the asset
  // (also used as <figcaption>)
  async function uploadBodyImage(file: File): Promise<RichTextImageUploadResult | null> {
    if (!post) return null
    if (!file.type.startsWith('image/')) {
      throw new Error('Only image files can be uploaded.')
    }
    const altText = window.prompt('Image alt text (optional)', '')?.trim() ?? ''
    const dims = await readImageDimensions(file)
    const requested = await requestMediaUpload({
      variables: {
        input: {
          path: `blog/posts/${post.id}`,
          filename: file.name,
          mimeType: file.type,
          sizeBytes: file.size,
          altText,
        },
      },
    })
    const payload = requested.data?.requestMediaAssetUpload
    if (!payload) throw new Error('Upload request did not return a signed URL.')

    const uploaded = await fetch(payload.uploadUrl, {
      method: 'PUT',
      headers: { 'Content-Type': file.type },
      body: file,
    })
    if (!uploaded.ok) {
      throw new Error(`Image upload failed with HTTP ${uploaded.status}.`)
    }
    const confirmed = await confirmMediaUpload({
      variables: {
        assetId: payload.asset.id,
        width: dims.width,
        height: dims.height,
      },
    })
    const asset = confirmed.data?.confirmMediaAssetUpload
    if (!asset) throw new Error('Image upload confirmation failed.')
    return {
      altText: asset.altText || altText,
      height: asset.height ?? dims.height,
      width: asset.width ?? dims.width,
      src: asset.url,
    }
  }

  return (
    // DetailPageLayout supplies the full-bleed wrapper + scroll
    // container shared with ProductDetail and the other detail pages
    // rightSidebar is rendered as a sibling of the main scroll column
    // when the preview panel is open. scrollRef is forwarded so this
    // page's hand-rolled auto-hide header keeps tracking the same
    // scroll element the layout creates
    <DetailPageLayout
      scrollRef={scrollRef}
      rightSidebar={
        previewOpen && post ? (
          <BlogPreviewPanel
            onClose={() => setPreviewOpen(false)}
            postId={post.id}
            locale={editingLocale || undefined}
            state={{
              id: post.id,
              slug: slug || post.slug,
              publishedAt: post.publishedAt ?? null,
              readingTimeMinutes: (() => {
                const n = readingTime ? Number.parseInt(readingTime, 10) : NaN
                return Number.isFinite(n) ? n : null
              })(),
              seoNoindex,
              author: post.author
                ? {
                    id: post.author.id,
                    slug: post.author.slug,
                    displayName: post.author.displayName,
                  }
                : null,
              content: {
                locale: editingLocale,
                title,
                excerpt,
                body,
                metaTitle: metaTitle || null,
                metaDescription: metaDescription || null,
              },
              // urlPath is computed server-side and not in the dashboard's BlogPostRow fragment - the channel's initial
              // server fetch supplies it; the live push just refreshes the editable fields
              categories: post.categories.map((c) => ({
                id: c.id,
                slug: c.slug,
                content: c.content ? { name: c.content.name } : null,
              })),
              tags: post.tags.map((t) => ({
                id: t.id,
                slug: t.slug,
                content: t.content ? { name: t.content.name } : null,
              })),
            }}
          />
        ) : null
      }
    >
        {/* Sticky action header - auto-hides on scroll-down, returns on
            scroll-up - Uses translate-y so the rest of the layout doesn't
            jump when it disappears */}
        <div
          className={
            'sticky top-0 z-10 border-b bg-background/80 backdrop-blur transition-transform duration-200 supports-[backdrop-filter]:bg-background/60 ' +
            (headerVisible ? 'translate-y-0' : '-translate-y-full')
          }
        >
          <div className="flex flex-wrap items-center gap-3 px-6 py-3">
            <div className="flex min-w-0 flex-1 flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  asChild
                  variant="ghost"
                  size="icon"
                  className="-ml-2 size-8 shrink-0 text-muted-foreground hover:text-foreground"
                  aria-label="Back to posts"
                >
                  <Link to="/blog/posts">
                    <ArrowLeftIcon className="size-4" />
                  </Link>
                </Button>
                <h1 className="min-w-0 truncate text-xl font-semibold tracking-tight">{displayTitle}</h1>
                <Badge variant={statusVariant(p.status)}>{statusLabel(p.status)}</Badge>
                {p.publishedAt && (
                  <Badge variant="outline" className="gap-1">
                    <ClockIcon className="size-3" />
                    {new Date(p.publishedAt).toLocaleString()}
                  </Badge>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                <code className="font-mono">{p.slug}</code>
                {p.author && (
                  <>
                    <span>·</span>
                    <span>by {p.author.displayName}</span>
                  </>
                )}
                <span>·</span>
                <span>{p.allContent.length} locale{p.allContent.length === 1 ? '' : 's'}</span>
                <span>·</span>
                <span>{p.channelListings.filter((l) => l.isPublished).length} store{p.channelListings.filter((l) => l.isPublished).length === 1 ? '' : 's'}</span>
              </div>
            </div>
            <div className="flex shrink-0 items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => setPreviewOpen(true)}
                title="Open live preview side panel"
              >
                <EyeIcon className="size-4" /> Preview
              </Button>
              {canManage && (
                <>
                  {p.status === 'PUBLISHED' ? (
                    <Button size="sm" variant="outline" onClick={onUnpublish} disabled={lifecycleBusy}>
                      <CircleOffIcon className="size-4" /> Unpublish
                    </Button>
                  ) : (
                    <Button size="sm" variant="outline" onClick={onPublish} disabled={lifecycleBusy}>
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
                  <Button
                    type="button"
                    size="sm"
                    disabled={!isDirty || savingAll}
                    onClick={() => { setSaveError(null); setConfirmingSave(true) }}
                  >
                    {isDirty
                      ? `Save changes${totalDirty > 0 ? ` (${totalDirty})` : ''}`
                      : 'No changes'}
                  </Button>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Body - edge-to-edge sections divided by horizontal rules, no
            box wrappers (matches ProductDetail) */}
        <div className="divide-y pb-16">

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
            <Button variant="outline" onClick={onSchedule} disabled={!canManage || lifecycleBusy || !scheduleAt}>
              <ClockIcon className="size-4" /> Schedule
            </Button>
          </div>
          <Button
            variant="outline"
            onClick={onArchive}
            disabled={!canManage || lifecycleBusy || p.status === 'ARCHIVED'}
          >
            <ArchiveIcon className="size-4" /> Archive
          </Button>
        </div>
      </section>

      {/* ---- Basics ---- */}
      <section className="px-6 py-6">
        <div className="mb-3">
          <h2 className="text-base font-semibold">Basics</h2>
          <p className="text-sm text-muted-foreground">Slug change creates a 301 redirect from the old path.</p>
        </div>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="slug">Slug</Label>
            <Input
              id="slug"
              value={slug}
              onChange={(e) => {
                setSlug(e.target.value)
                setSlugTouched(true)
              }}
              disabled={!canManage}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="reading-time">Reading time (min)</Label>
            <Input
              id="reading-time"
              type="number"
              value={readingTime}
              onChange={(e) => setReadingTime(e.target.value)}
              disabled={!canManage}
            />
          </div>
          <div className="flex items-center gap-2">
            <Switch
              id="seo-noindex"
              checked={seoNoindex}
              onCheckedChange={setSeoNoindex}
              disabled={!canManage}
            />
            <Label htmlFor="seo-noindex">Hide from search engines (noindex)</Label>
          </div>
        </div>
      </section>

      {/* ---- Content (locale-tabbed) ---- */}
      <section className="px-6 py-6">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-base font-semibold">Content</h2>
            <p className="text-sm text-muted-foreground">Title, body, excerpt, and SEO meta - per locale.</p>
          </div>
          {/* Which locales have content + the remove action for the one
              being edited - The active edit locale is the header's
              effectiveLocale - there is no in-form locale picker */}
          <TranslationLocales
            locale={canEditTranslations ? editingLocale : ''}
            locales={localesWithContent}
            entityLabel="post"
            canManage={canManage}
            defaultLocaleLocked
            onRemove={onRemoveTranslation}
          />
        </div>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="title">Title</Label>
            <Input
              id="title"
              value={title}
              onChange={(e) => {
                const next = e.target.value
                setTitle(next)
                // Only auto-fill the slug if the editor opened with an empty slug field (rare for existing posts; handy when fixing a
                // row that never got one) - Once the admin edits slug manually we mark it touched and stop interfering
                if (!slugTouched && (!slug || slug.length === 0)) setSlug(slugify(next))
              }}
              disabled={!canManage}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>Excerpt</Label>
            {/*
              Key forces a remount when the underlying content changes
              (different post, different locale, or after-save when
              updatedAt advances) - LexicalRichTextEditor captures `value`
              on mount and never reacts to prop changes - without this
              key, the editor sticks at whatever was in `excerpt` state
              the first time it rendered, which on page load is null
              (state default) before the useEffect populates from
              allContent
            */}
            <LexicalRichTextEditor
              key={`excerpt:${post?.id}:${editingLocale}:${localeContent?.updatedAt ?? 'new'}`}
              value={(localeContent?.excerpt as RichTextValue) ?? undefined}
              fallbackText=""
              disabled={!canManage}
              placeholder="Short summary (used in feeds and listings)…"
              onChange={(v) => setExcerpt(v)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>Body</Label>
            <LexicalRichTextEditor
              key={`body:${post?.id}:${editingLocale}:${localeContent?.updatedAt ?? 'new'}`}
              value={(localeContent?.body as RichTextValue) ?? undefined}
              fallbackText=""
              disabled={!canManage}
              placeholder="Write the post…"
              onChange={(v) => setBody(v)}
              onUploadImage={canManage ? uploadBodyImage : undefined}
            />
          </div>
          <div className="grid gap-4">
            <div className="grid gap-2">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="meta-title">Meta title</Label>
                <span className={`text-xs tabular-nums ${metaTitle.length > 60 ? 'text-destructive' : 'text-muted-foreground'}`}>
                  {metaTitle.length}/60
                </span>
              </div>
              <Input
                id="meta-title"
                value={metaTitle}
                onChange={(e) => setMetaTitle(e.target.value)}
                disabled={!canManage}
                placeholder={title || 'Post title'}
              />
            </div>
            <div className="grid gap-2">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="meta-description">Meta description</Label>
                <span className={`text-xs tabular-nums ${metaDescription.length > 160 ? 'text-destructive' : 'text-muted-foreground'}`}>
                  {metaDescription.length}/160
                </span>
              </div>
              <Textarea
                id="meta-description"
                value={metaDescription}
                onChange={(e) => setMetaDescription(e.target.value)}
                rows={3}
                disabled={!canManage}
                placeholder="Short search-result description…"
              />
            </div>
          </div>
        </div>
      </section>

      {/* ---- Categories + Tags ---- */}
      <section className="px-6 py-6">
        <div className="mb-3">
          <h2 className="text-base font-semibold">Categories & tags</h2>
          <p className="text-sm text-muted-foreground">Toggle to attach. Saves immediately.</p>
        </div>
        <div className="grid gap-6">
          <div className="grid gap-2">
            <Label className="text-xs uppercase tracking-wide text-muted-foreground">Categories</Label>
            {flatCategories.length === 0 ? (
              <p className="text-sm text-muted-foreground">No categories yet.</p>
            ) : (
              <ul className="grid gap-1 md:grid-cols-2">
                {flatCategories.map((c) => {
                  const checked = draftCategoryIds.includes(c.id)
                  return (
                    <li key={c.id} className="flex items-center gap-2 rounded-md border px-2 py-1.5">
                      <Checkbox
                        id={`cat-${c.id}`}
                        checked={checked}
                        onCheckedChange={() => toggleCategory(c.id)}
                        disabled={!canManage}
                      />
                      <Label htmlFor={`cat-${c.id}`} className="flex-1 cursor-pointer text-sm font-normal">
                        {c.label}
                      </Label>
                      <code className="text-xs text-muted-foreground">{c.urlPath}</code>
                    </li>
                  )
                })}
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
                  const checked = draftTagIds.includes(t.id)
                  const label = t.content?.name ?? t.slug
                  return (
                    <li key={t.id}>
                      <Button
                        type="button"
                        variant={checked ? 'default' : 'outline'}
                        size="sm"
                        onClick={() => toggleTag(t.id)}
                        disabled={!canManage}
                      >
                        #{label}
                      </Button>
                    </li>
                  )
                })}
              </ul>
            )}
          </div>
        </div>
      </section>

      <PostStoresSection postId={p.id} listings={p.channelListings} canManage={canManage} refetchVariables={variables} />

      <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete post?</AlertDialogTitle>
            <AlertDialogDescription>
              This permanently removes the post and all its translations. URL redirects are preserved.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={onDelete} disabled={deleting}>
              {deleting ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Locale ${editingLocale}`}
        sections={changeSections}
        saving={savingAll}
        error={saveError}
        onCancel={() => { setConfirmingSave(false); setSaveError(null) }}
        onConfirm={onConfirmSave}
      />
        </div>
    </DetailPageLayout>
  )
}

// readImageDimensions probes a File for its natural size so the upload-confirm step can stamp width/height onto the
// media asset Local copy of the ProductDetail helper - small enough that sharing would cost more than duplicating
function readImageDimensions(file: File): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    const url = URL.createObjectURL(file)
    image.onload = () => {
      URL.revokeObjectURL(url)
      resolve({ width: image.naturalWidth, height: image.naturalHeight })
    }
    image.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Could not read image dimensions.'))
    }
    image.src = url
  })
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
                      checked={noindex}
                      onCheckedChange={(v) =>
                        setListing({
                          variables: {
                            input: {
                              postId,
                              channelId: sv.id,
                              isPublished: enabled,
                              seoNoindex: v,
                            },
                          },
                        })
                      }
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
