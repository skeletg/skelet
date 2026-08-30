import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
} from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ImageIcon,
  PlusIcon,
  ShapesIcon,
  Trash2Icon,
  XIcon,
} from 'lucide-react'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { LoadError } from '@/shared/errors/LoadError'
import {
  AddProductToCollectionDocument,
  ClearCollectionCoverDocument,
  CollectionDocument,
  CollectionProductPickerDocument,
  CollectionsDocument,
  type CollectionDetail as CollectionRecord,
  type CollectionProduct,
  ConfirmCollectionCoverUploadDocument,
  RemoveProductFromCollectionDocument,
  ReorderCollectionProductsDocument,
  DeleteCollectionContentDocument,
  DeleteCollectionDocument,
  type MetadataEntry,
  RequestCollectionCoverUploadDocument,
  UpdateCollectionDocument,
  UpsertCollectionContentDocument,
} from './Collections.gql'
import { LexicalRichTextEditor, richTextToPlainText, type RichTextValue } from './LexicalRichTextEditor'
import { UrlRewritesSection } from './UrlRewritesSection'
import { AutocompletePicker, type Option } from '@/features/promotions/widgets/AutocompletePicker'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import {
  SaveRegistryContext,
  useSaveRegistry,
  useSaveSection,
  type SectionDiffEntry,
} from '@/shared/saving/saveRegistry'
import { LoadingState } from '@/components/ui/loading-state'
import { PluginSections } from '@/plugins/detail-sections'
import { AttributesDocument } from './Catalog.gql'
import { SetCollectionFacetOverridesDocument } from './Collections.gql'
import { FacetOverridesEditor } from './FacetOverridesEditor'
import {
  sortFacetCandidates,
  useFacetOverridesDraft,
  type FacetOverrideValue,
} from './facet-overrides-draft'

type CollectionRefetch = Array<{ query: typeof CollectionDocument; variables: { id: string } }>

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

function resolveCollectionName(collection: CollectionRecord) {
  // Prefer the server-resolved locale (collection.name, populated from collection.contentLocale's row) so Global view
  // shows the default locale instead of whichever translation Postgres returned first
  const resolved = collection.name?.trim()
  if (resolved) return resolved
  const fallback = collection.content.find((entry) => entry.name.trim())?.name.trim()
  return fallback || collection.slug
}

function resolveProductName(product: CollectionProduct) {
  return product.name?.trim() || product.sku?.trim() || product.id
}

function cleanMetadata(entries: MetadataEntry[]): MetadataEntry[] {
  return entries
    .map((entry) => ({ key: entry.key.trim(), value: entry.value }))
    .filter((entry) => entry.key || entry.value)
}

function findDuplicateKey(entries: MetadataEntry[]): string | null {
  const seen = new Set<string>()
  for (const entry of entries) {
    if (!entry.key) continue
    if (seen.has(entry.key)) return entry.key
    seen.add(entry.key)
  }
  return null
}

function clip(value: string, max = 80): string {
  return value.length > max ? `${value.slice(0, max - 1)}…` : value
}

function metadataDiffEntries(
  prefix: string,
  before: MetadataEntry[],
  after: MetadataEntry[],
): SectionDiffEntry[] {
  const cleanedAfter = cleanMetadata(after)
  const beforeMap = new Map(before.map((entry) => [entry.key, entry.value]))
  const afterMap = new Map(cleanedAfter.map((entry) => [entry.key, entry.value]))
  const out: SectionDiffEntry[] = []
  for (const [key, value] of afterMap) {
    const prev = beforeMap.get(key)
    if (prev !== value) {
      out.push({ label: `${prefix} · ${key || '(no key)'}`, before: clip(prev ?? ''), after: clip(value) })
    }
  }
  for (const [key, value] of beforeMap) {
    if (!afterMap.has(key)) out.push({ label: `${prefix} · ${key}`, before: clip(value), after: '' })
  }
  return out
}

export function CollectionDetail() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const { has } = usePermissions()
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const canManage = has('catalog.manage')
  // Locale comes from the `Locale` request header (set by the Apollo
  // localeLink); we still derive `scopedLocale` for client-side rendering
  // - picking the active translation row to seed the form state
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || undefined
      : undefined
  const collectionVariables = { id }

  const collectionQuery = useQuery(CollectionDocument, {
    variables: collectionVariables,
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const collection = collectionQuery.data?.collection

  const collectionRefetch = [{ query: CollectionDocument, variables: collectionVariables }]
  const [deleteCollection, { loading: deleting }] = useMutation(DeleteCollectionDocument, {
    refetchQueries: [{ query: CollectionsDocument }],
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Page save registry: every editable section (basics, cover, content, metadata) stages its edits and the header Save
  // applies them through the SaveConfirmDialog diff modal - the dashboard-wide convention (see docs/conventions.md)
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

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

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteCollection({ variables: { id } })
      navigate('/collections', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  if (collectionQuery.loading) {
    return <LoadingState label="Loading collection…" />
  }
  if (collectionQuery.error) {
    return (
      <LoadError
        title="Couldn't load collection"
        message={collectionQuery.error.message}
        onRetry={() => void collectionQuery.refetch()}
        retrying={collectionQuery.loading}
      />
    )
  }
  if (!collection) {
    return <p className="text-sm text-muted-foreground">Collection not found.</p>
  }

  const displayName = resolveCollectionName(collection)
  const activeLocale =
    scopedLocale ?? collection.contentLocale ?? collection.content[0]?.locale ?? globalDefaultLocale

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/collections"
        backLabel="Back to collections"
        title={displayName}
        badges={
          <Badge variant={collection.isPublished ? 'default' : 'outline'}>
            {collection.isPublished ? 'Published' : 'Draft'}
          </Badge>
        }
        meta={<code className="font-mono">{collection.slug}</code>}
        actions={
          canManage ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                onClick={() => { setDeleteError(null); setConfirmingDelete(true) }}
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
          ) : null
        }
      />

      <DetailPageBody>
        <CollectionContentSections
          key={`${collection.id}:${activeLocale}:${collection.updatedAt}:content`}
          collection={collection}
          locale={activeLocale}
          canManage={canManage}
          refetchQueries={collectionRefetch}
        />

        <CollectionProductsSection
          collection={collection}
          canManage={canManage}
          refetchQueries={collectionRefetch}
        />

        <CollectionFacetOverridesSection
          collection={collection}
          canManage={canManage}
          refetchQueries={collectionRefetch}
        />

        <CollectionMetadataSection
          key={`${collection.id}:${collection.updatedAt}:metadata`}
          collection={collection}
          canManage={canManage}
          refetchQueries={collectionRefetch}
        />
        <PluginSections target="collection" entity={collection ?? null} mode="edit" />
      </DetailPageBody>

      <AlertDialog
        open={confirmingDelete}
        onOpenChange={(open) => { if (!open) { setConfirmingDelete(false); setDeleteError(null) } }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete collection?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the collection record, cover image reference, translations, product
              memberships, and autogenerated public rewrite.
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
              {deleting ? 'Deleting...' : 'Delete collection'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Review-before-save modal: lists every staged change across the
          page's sections (basics, cover, content, metadata) */}
      <SaveConfirmDialog
        open={confirmingSave}
        scopeLabel={`Collection · ${displayName} · ${activeLocale}`}
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

// Owns the staged drafts for the per-locale content row (name, rich-text
// description, SEO meta tags) and registers them as ONE save section:
// UpsertCollectionContent replaces the whole (collection, locale) row, so
// splitting name/description and SEO into independently-saved sections
// would let one save clobber the other's staged fields - Renders both the
// General and SEO page sections so the UI layout stays unchanged
function CollectionContentSections({
  collection,
  locale,
  canManage,
  refetchQueries,
}: {
  collection: CollectionRecord
  locale: string
  canManage: boolean
  refetchQueries: CollectionRefetch
}) {
  const currentContent = collection.content.find((entry) => entry.locale === locale)

  // Baselines mirror the seeds below so the diff is empty until the user
  // edits - and self-clears after a save once the refetch lands
  const baseName = currentContent?.name ?? collection.name ?? ''
  const baseDescriptionText = richTextToPlainText(currentContent?.description as RichTextValue)
  const baseMetaTitle = currentContent?.metaTitle ?? collection.metaTitle ?? ''
  const baseMetaDescription = currentContent?.metaDescription ?? collection.metaDescription ?? ''
  const baseMetaKeywords = currentContent?.metaKeywords ?? collection.metaKeywords ?? ''

  const [name, setName] = useState(baseName)
  const [description, setDescription] = useState<RichTextValue>(currentContent?.description as RichTextValue)
  const [descriptionText, setDescriptionText] = useState(baseDescriptionText)
  const [metaTitle, setMetaTitle] = useState(baseMetaTitle)
  const [metaDescription, setMetaDescription] = useState(baseMetaDescription)
  const [metaKeywords, setMetaKeywords] = useState(baseMetaKeywords)

  const [confirmingContentDelete, setConfirmingContentDelete] = useState(false)
  const [contentDeleteError, setContentDeleteError] = useState<string | null>(null)

  const [upsertContent] = useMutation(UpsertCollectionContentDocument, { refetchQueries })
  const [deleteContent, { loading: deletingContent }] = useMutation(DeleteCollectionContentDocument, { refetchQueries })

  // Rich text shows its plain text in the modal; formatting-only edits (same text, different Lexical JSON) still count
  // as dirty via the serialized compare below - otherwise a bold/heading change would be unsavable ("No changes")
  const baseDescriptionJSON = JSON.stringify(currentContent?.description ?? null)
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    const field = (label: string, before: string, after: string) => {
      if (before !== after) out.push({ label, before: clip(before), after: clip(after) })
    }
    field('name', baseName, name)
    field('description', baseDescriptionText, descriptionText)
    if (
      baseDescriptionText === descriptionText &&
      JSON.stringify(description ?? null) !== baseDescriptionJSON
    ) {
      out.push({ label: 'description', before: 'previous formatting', after: 'updated formatting' })
    }
    field('meta title', baseMetaTitle, metaTitle)
    field('meta description', baseMetaDescription, metaDescription)
    field('meta keywords', baseMetaKeywords, metaKeywords)
    return out
  }, [
    baseName, name,
    baseDescriptionText, descriptionText,
    baseDescriptionJSON, description,
    baseMetaTitle, metaTitle,
    baseMetaDescription, metaDescription,
    baseMetaKeywords, metaKeywords,
  ])

  useSaveSection({
    id: 'content',
    title: `Content (${locale})`,
    diff,
    save: async () => {
      if (!name.trim()) throw new Error('Name is required for this locale before content can be saved.')
      await upsertContent({
        variables: {
          collectionId: collection.id,
          locale,
          input: {
            name: name.trim(),
            description: descriptionText.trim() ? description : undefined,
            metaTitle: metaTitle.trim() || undefined,
            metaDescription: metaDescription.trim() || undefined,
            metaKeywords: metaKeywords.trim() || undefined,
          },
        },
      })
    },
  })

  async function onDeleteContent() {
    setContentDeleteError(null)
    try {
      await deleteContent({ variables: { collectionId: collection.id, locale } })
      setConfirmingContentDelete(false)
    } catch (err) {
      setContentDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <>
      <DetailPageSection
        title="General"
        description="Slug, visibility, cover image, and public name and description."
      >
        <div className="grid gap-6">
          <CollectionBasicsBlock
            collection={collection}
            canManage={canManage}
            refetchQueries={refetchQueries}
          />
          <Separator />
          <CollectionCoverBlock
            collection={collection}
            canManage={canManage}
            refetchQueries={refetchQueries}
          />
          <Separator />
          <div className="grid gap-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h4 className="text-sm font-medium">Translation</h4>
                <p className="text-xs text-muted-foreground">
                  Channel name and description for this locale.
                </p>
              </div>
              <Badge variant="outline" className="shrink-0">{locale}</Badge>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="collection-content-name">Name</Label>
              <Input
                id="collection-content-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                disabled={!canManage}
              />
            </div>

            <div className="grid gap-2">
              <Label>Description</Label>
              <LexicalRichTextEditor
                value={description}
                fallbackText=""
                disabled={!canManage}
                placeholder="Describe the collection..."
                onChange={(next, plainText) => {
                  setDescription(next)
                  setDescriptionText(plainText)
                }}
              />
            </div>
          </div>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="SEO"
        description="Meta tags and search-engine URL paths."
        trailing={<Badge variant="outline">{locale}</Badge>}
      >
        <div className="grid gap-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="collection-content-meta-title">Meta title</Label>
              <Input
                id="collection-content-meta-title"
                value={metaTitle}
                onChange={(event) => setMetaTitle(event.target.value)}
                disabled={!canManage || deletingContent}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="collection-content-meta-keywords">Meta keywords</Label>
              <Input
                id="collection-content-meta-keywords"
                value={metaKeywords}
                onChange={(event) => setMetaKeywords(event.target.value)}
                disabled={!canManage || deletingContent}
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label htmlFor="collection-content-meta-description">Meta description</Label>
            <Textarea
              id="collection-content-meta-description"
              rows={4}
              value={metaDescription}
              onChange={(event) => setMetaDescription(event.target.value)}
              disabled={!canManage || deletingContent}
            />
          </div>

          {canManage && (
            <div className="flex justify-end">
              <Button
                variant="outline"
                onClick={() => { setContentDeleteError(null); setConfirmingContentDelete(true) }}
                disabled={!currentContent || deletingContent}
              >
                <Trash2Icon className="size-4" />
                Delete translation
              </Button>
            </div>
          )}

          <Separator />

          <div>
            <h4 className="text-sm font-medium">URL paths</h4>
            <p className="text-xs text-muted-foreground">
              Canonical path and redirects for this collection.
            </p>
          </div>
          <UrlRewritesSection
            entityId={collection.id}
            entityType="COLLECTION"
            entityLabel="collection"
            canManage={canManage}
          />
        </div>
      </DetailPageSection>

      <AlertDialog
        open={confirmingContentDelete}
        onOpenChange={(open) => { if (!open) { setConfirmingContentDelete(false); setContentDeleteError(null) } }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {locale} translation?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the locale-specific name, description, and SEO fields for this
              collection.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {contentDeleteError && (
            <Alert variant="destructive">
              <AlertTitle>Delete failed</AlertTitle>
              <AlertDescription>{contentDeleteError}</AlertDescription>
            </Alert>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletingContent}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={onDeleteContent} disabled={deletingContent}>
              {deletingContent ? 'Deleting...' : 'Delete translation'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function CollectionBasicsBlock({
  collection,
  canManage,
  refetchQueries,
}: {
  collection: CollectionRecord
  canManage: boolean
  refetchQueries: CollectionRefetch
}) {
  const [slug, setSlug] = useState(collection.slug)
  const [isPublished, setIsPublished] = useState(collection.isPublished)
  const [seoNoindex, setSeoNoindex] = useState(collection.seoNoindex)
  const [updateCollection] = useMutation(UpdateCollectionDocument, { refetchQueries })

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    if (slug !== collection.slug) out.push({ label: 'slug', before: collection.slug, after: slug })
    if (isPublished !== collection.isPublished) {
      out.push({
        label: 'status',
        before: collection.isPublished ? 'Published' : 'Draft',
        after: isPublished ? 'Published' : 'Draft',
      })
    }
    if (seoNoindex !== collection.seoNoindex) {
      out.push({ label: 'noindex', before: String(collection.seoNoindex), after: String(seoNoindex) })
    }
    return out
  }, [collection.slug, collection.isPublished, collection.seoNoindex, slug, isPublished, seoNoindex])

  useSaveSection({
    id: 'basics',
    title: 'General',
    diff,
    save: async () => {
      await updateCollection({
        variables: {
          input: {
            id: collection.id,
            slug: slug.trim() || undefined,
            isPublished,
            seoNoindex,
          },
        },
      })
    },
  })

  return (
    <div className="grid gap-4">
      <div className="grid gap-2">
        <Label htmlFor="collection-detail-slug">Slug</Label>
        <Input
          id="collection-detail-slug"
          value={slug}
          onChange={(event) => setSlug(event.target.value)}
          disabled={!canManage}
        />
      </div>
      <div className="flex items-center justify-between gap-4 py-1">
        <div>
          <Label htmlFor="collection-published" className="text-sm">Published</Label>
          <p className="text-xs text-muted-foreground">
            Make this collection visible on channels.
          </p>
        </div>
        <Switch
          id="collection-published"
          checked={isPublished}
          onCheckedChange={setIsPublished}
          disabled={!canManage}
        />
      </div>
      <div className="flex items-center justify-between gap-4 py-1">
        <div>
          <Label htmlFor="collection-noindex" className="text-sm">Noindex</Label>
          <p className="text-xs text-muted-foreground">
            Ask search engines not to index this collection.
          </p>
        </div>
        <Switch
          id="collection-noindex"
          checked={seoNoindex}
          onCheckedChange={setSeoNoindex}
          disabled={!canManage}
        />
      </div>
    </div>
  )
}

// Cover edits are staged like every other field: picking a file or hitting
// "Clear cover" only marks the change locally; the actual signed-URL upload
// (or clear mutation) runs from this section's save() when the page Save is
// confirmed - Staged state is cleared in save() itself because the upload
// doesn't bump collection.updatedAt-keyed remounts reliably
function CollectionCoverBlock({
  collection,
  canManage,
  refetchQueries,
}: {
  collection: CollectionRecord
  canManage: boolean
  refetchQueries: CollectionRefetch
}) {
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [altText, setAltText] = useState(collection.coverImage?.altText ?? collection.name ?? '')
  const [pendingFile, setPendingFile] = useState<File | null>(null)
  const [pendingClear, setPendingClear] = useState(false)
  const [fileError, setFileError] = useState<string | null>(null)
  const [requestUpload] = useMutation(RequestCollectionCoverUploadDocument)
  const [confirmUpload] = useMutation(ConfirmCollectionCoverUploadDocument, { refetchQueries })
  const [clearCover] = useMutation(ClearCollectionCoverDocument, { refetchQueries })

  const previewUrl = useMemo(
    () => (pendingFile ? URL.createObjectURL(pendingFile) : null),
    [pendingFile],
  )
  useEffect(() => () => { if (previewUrl) URL.revokeObjectURL(previewUrl) }, [previewUrl])

  const currentLabel = collection.coverImage
    ? collection.coverImage.url.split('/').pop() || 'current cover'
    : ''

  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    if (pendingFile) {
      out.push({ label: 'cover image', before: currentLabel, after: pendingFile.name })
      const baseAlt = collection.coverImage?.altText ?? ''
      if (altText.trim() !== baseAlt) {
        out.push({ label: 'cover alt text', before: baseAlt, after: altText.trim() })
      }
    } else if (pendingClear) {
      out.push({ label: 'cover image', before: currentLabel, after: '' })
    }
    return out
  }, [pendingFile, pendingClear, currentLabel, altText, collection.coverImage?.altText])

  useSaveSection({
    id: 'cover',
    title: 'Cover image',
    diff,
    save: async () => {
      if (pendingClear) {
        await clearCover({ variables: { collectionId: collection.id } })
        setPendingClear(false)
        return
      }
      if (!pendingFile) return
      const file = pendingFile
      const [{ width, height }, upload] = await Promise.all([
        readImageDimensions(file),
        requestUpload({
          variables: {
            input: {
              collectionId: collection.id,
              filename: file.name,
              mimeType: file.type,
              sizeBytes: file.size,
              altText: altText.trim() || undefined,
            },
          },
        }),
      ])
      const payload = upload.data?.requestCollectionCoverUpload
      if (!payload) throw new Error('Upload request did not return a signed URL.')
      const uploaded = await fetch(payload.uploadUrl, {
        method: 'PUT',
        headers: { 'Content-Type': file.type },
        body: file,
      })
      if (!uploaded.ok) throw new Error(`Cover upload failed with HTTP ${uploaded.status}.`)
      await confirmUpload({ variables: { collectionId: collection.id, width, height } })
      setPendingFile(null)
    },
  })

  function onFileChange(event: ChangeEvent<HTMLInputElement>) {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file) return
    if (!file.type.startsWith('image/')) {
      setFileError('Only image files can be uploaded here.')
      return
    }
    setFileError(null)
    setPendingClear(false)
    setPendingFile(file)
  }

  function discardStaged() {
    setPendingFile(null)
    setPendingClear(false)
    setFileError(null)
  }

  const displayedImage = previewUrl ?? (pendingClear ? null : collection.coverImage?.url ?? null)
  const displayedAlt = previewUrl ? altText : collection.coverImage?.altText ?? ''

  return (
    <div className="grid gap-4">
      <div>
        <h4 className="text-sm font-medium">Cover image</h4>
        <p className="text-xs text-muted-foreground">
          Hero image shown on the channel category and listing pages. A staged
          image uploads when you save the page.
        </p>
      </div>
      {displayedImage ? (
        <div className="overflow-hidden rounded-lg border bg-muted/20 w-full">
          <img
            src={displayedImage}
            alt={displayedAlt}
            className="h-40 w-full object-cover"
          />
        </div>
      ) : (
        <div className="flex h-40 w-full items-center justify-center rounded-lg border border-dashed bg-muted/20 text-sm text-muted-foreground">
          <div className="flex items-center gap-2">
            <ImageIcon className="size-4" />
            {pendingClear ? 'Cover removal staged' : 'No cover image'}
          </div>
        </div>
      )}

      {pendingFile ? (
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Badge variant="outline">Staged</Badge>
          <span>{pendingFile.name}</span>
        </div>
      ) : collection.coverImage && !pendingClear ? (
        <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Badge variant="outline">{collection.coverImage.status}</Badge>
          <span>{collection.coverImage.width}×{collection.coverImage.height}</span>
          <span>·</span>
          <span>{collection.coverImage.mimeType}</span>
        </div>
      ) : null}

      <div className="grid gap-2">
        <Label htmlFor="collection-cover-alt">Alt text</Label>
        <Input
          id="collection-cover-alt"
          value={altText}
          onChange={(event) => setAltText(event.target.value)}
          disabled={!canManage}
        />
        <p className="text-xs text-muted-foreground">
          Alt text is saved when a new cover image is uploaded.
        </p>
      </div>

      {fileError && (
        <Alert variant="destructive">
          <AlertTitle>Cover image</AlertTitle>
          <AlertDescription>{fileError}</AlertDescription>
        </Alert>
      )}

      {canManage && (
        <div className="flex flex-wrap justify-end gap-2">
          {(pendingFile || pendingClear) && (
            <Button variant="ghost" onClick={discardStaged}>
              Discard staged change
            </Button>
          )}
          {collection.coverImage && !pendingClear && (
            <Button
              variant="outline"
              onClick={() => { setPendingFile(null); setPendingClear(true) }}
            >
              <XIcon className="size-4" />
              Clear cover
            </Button>
          )}
          <Button onClick={() => fileInputRef.current?.click()}>
            <ImageIcon className="size-4" />
            {pendingFile ? 'Choose different image' : collection.coverImage ? 'Replace cover' : 'Upload cover'}
          </Button>
        </div>
      )}

      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={onFileChange}
      />
    </div>
  )
}

// Collection membership - Ordering is meaningful - the storefront renders a
// collection in this order - so rows carry move up/down alongside remove
// Writes go straight through (no save-registry staging) because each mutation
// returns the re-ordered server list, which is the authoritative answer
/**
 * CollectionFacetOverridesSection - per-collection storefront filter presentation,
 * same drag-and-drop editor and save contract as the category twin
 */
function CollectionFacetOverridesSection({
  collection,
  canManage,
  refetchQueries,
}: {
  collection: { id: string; facetOverrides: FacetOverrideValue[] }
  canManage: boolean
  refetchQueries: CollectionRefetch
}) {
  const { data: attrData } = useQuery(AttributesDocument)
  const [setOverrides] = useMutation(SetCollectionFacetOverridesDocument, { refetchQueries })

  const candidates = useMemo(
    () =>
      sortFacetCandidates(
        (attrData?.attributes ?? []).filter((a) => a.filterable && a.isPublic),
      ),
    [attrData],
  )

  const draft = useFacetOverridesDraft(collection.facetOverrides, candidates)

  useSaveSection({
    id: 'facet-overrides',
    title: 'Storefront filters',
    diff: draft.sectionDiff,
    save: async () => {
      await setOverrides({
        variables: { collectionId: collection.id, overrides: draft.payload },
      })
    },
  })

  if (candidates.length === 0) return null

  return (
    <DetailPageSection
      title="Storefront filters"
      description="Order and visibility of the filter sidebar on this collection's page. Hidden filters stay filterable through direct links."
    >
      <FacetOverridesEditor
        rows={draft.rows}
        customized={draft.customized}
        canManage={canManage}
        onReorder={draft.reorder}
        onToggleHidden={draft.toggleHidden}
        onReset={draft.reset}
      />
    </DetailPageSection>
  )
}

function CollectionProductsSection({
  collection,
  canManage,
  refetchQueries,
}: {
  collection: CollectionRecord
  canManage: boolean
  refetchQueries: CollectionRefetch
}) {
  const rows = collection.products
  const [productId, setProductId] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const { data: pickerData, loading: pickerLoading } = useQuery(
    CollectionProductPickerDocument,
    { skip: !canManage, fetchPolicy: 'cache-first' },
  )
  // Refetch rather than trusting cache normalization: the mutations return a Collection whose products list is a
  // different shape from the one the page query asked for, so the cached entity doesn't reliably pick up the change
  const [addProduct] = useMutation(AddProductToCollectionDocument, { refetchQueries })
  const [removeProduct] = useMutation(RemoveProductFromCollectionDocument, { refetchQueries })
  const [reorderProducts] = useMutation(ReorderCollectionProductsDocument, { refetchQueries })

  const options = useMemo<Option[]>(
    () =>
      (pickerData?.products ?? [])
        .filter((p) => !rows.some((r) => r.id === p.id))
        .map((p) => ({ value: p.id, label: p.name || p.sku || p.id, hint: p.sku ?? undefined })),
    [pickerData, rows],
  )

  async function run(fn: () => Promise<unknown>) {
    setError(null)
    setBusy(true)
    try {
      await fn()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not update the collection.')
    } finally {
      setBusy(false)
    }
  }

  // Move by swapping with the neighbour and sending the whole id list, which
  // is what reorderCollectionProducts expects
  function move(index: number, delta: number) {
    const next = rows.map((r) => r.id)
    const target = index + delta
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target], next[index]]
    void run(() =>
      reorderProducts({ variables: { collectionId: collection.id, productIds: next } }),
    )
  }

  return (
    <DetailPageSection
      title="Products"
      description="Products in this collection, in the order the storefront shows them."
    >
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Update failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {canManage && (
        <div className="grid max-w-2xl grid-cols-[1fr_auto] items-end gap-2">
          <div className="grid gap-1.5">
            <Label>Add a product</Label>
            <AutocompletePicker
              options={options}
              value={productId}
              onChange={(v) => setProductId((v as string) ?? '')}
              placeholder="Search products…"
              emptyMessage="No products left to add."
              loading={pickerLoading}
            />
          </div>
          <Button
            type="button"
            disabled={!productId || busy}
            onClick={() =>
              void run(async () => {
                await addProduct({
                  variables: { collectionId: collection.id, productId },
                })
                setProductId('')
              })
            }
          >
            Add
          </Button>
        </div>
      )}

      {rows.length === 0 ? (
        <div className="rounded-lg border border-dashed bg-muted/20 p-5 text-sm text-muted-foreground">
          No products are assigned to this collection yet.
        </div>
      ) : (
        <div className="overflow-hidden rounded-md border">
          <table className="w-full text-sm">
            <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
              <tr>
                <th className="w-12 px-3 py-2 font-medium">#</th>
                <th className="px-3 py-2 font-medium">Product</th>
                <th className="px-3 py-2 font-medium">SKU</th>
                <th className="px-3 py-2 text-right font-medium">Price</th>
                {canManage && <th className="w-28 px-3 py-2" />}
              </tr>
            </thead>
            <tbody>
              {rows.map((product, index) => (
                <tr key={product.id} className="border-t">
                  <td className="px-3 py-2 text-muted-foreground tabular-nums">{index + 1}</td>
                  <td className="px-3 py-2">
                    <div className="flex min-w-0 items-center gap-3">
                      {product.thumbnail?.url ? (
                        <img
                          src={product.thumbnail.url}
                          alt={product.thumbnail.altText}
                          className="size-8 shrink-0 rounded-md border object-cover"
                        />
                      ) : (
                        <div className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted/25 text-muted-foreground">
                          <ShapesIcon className="size-3.5" />
                        </div>
                      )}
                      <Link
                        to={`/products/${product.id}`}
                        className="truncate font-medium hover:underline"
                      >
                        {resolveProductName(product)}
                      </Link>
                    </div>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">
                    {product.sku ?? '-'}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums">
                    {formatCents(product.defaultPrice, product.defaultCurrencyCode)}
                  </td>
                  {canManage && (
                    <td className="px-3 py-2">
                      <div className="flex justify-end gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="size-7"
                          title="Move up"
                          disabled={index === 0 || busy}
                          onClick={() => move(index, -1)}
                        >
                          <ArrowUpIcon className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="size-7"
                          title="Move down"
                          disabled={index === rows.length - 1 || busy}
                          onClick={() => move(index, 1)}
                        >
                          <ArrowDownIcon className="size-3.5" />
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="size-7 text-destructive"
                          title="Remove from collection"
                          disabled={busy}
                          onClick={() =>
                            void run(() =>
                              removeProduct({
                                variables: {
                                  collectionId: collection.id,
                                  productId: product.id,
                                },
                              }),
                            )
                          }
                        >
                          <Trash2Icon className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </DetailPageSection>
  )
}

function CollectionMetadataSection({
  collection,
  canManage,
  refetchQueries,
}: {
  collection: CollectionRecord
  canManage: boolean
  refetchQueries: CollectionRefetch
}) {
  const [publicRows, setPublicRows] = useState<MetadataEntry[]>(collection.metadata)
  const [privateRows, setPrivateRows] = useState<MetadataEntry[]>(collection.privateMetadata)
  const [updateCollection] = useMutation(UpdateCollectionDocument, { refetchQueries })

  const diff = useMemo<SectionDiffEntry[]>(
    () => [
      ...metadataDiffEntries('metadata', collection.metadata, publicRows),
      ...metadataDiffEntries('private metadata', collection.privateMetadata, privateRows),
    ],
    [collection.metadata, collection.privateMetadata, publicRows, privateRows],
  )

  useSaveSection({
    id: 'metadata',
    title: 'Custom data',
    diff,
    save: async () => {
      const cleanedPublic = cleanMetadata(publicRows)
      const cleanedPrivate = cleanMetadata(privateRows)
      if (cleanedPublic.some((entry) => !entry.key) || cleanedPrivate.some((entry) => !entry.key)) {
        throw new Error('Metadata entries with a value need a key.')
      }
      const publicDuplicate = findDuplicateKey(cleanedPublic)
      if (publicDuplicate) throw new Error(`Duplicate metadata key: "${publicDuplicate}"`)
      const privateDuplicate = findDuplicateKey(cleanedPrivate)
      if (privateDuplicate) throw new Error(`Duplicate private metadata key: "${privateDuplicate}"`)

      await updateCollection({
        variables: {
          input: {
            id: collection.id,
            metadata: cleanedPublic,
            privateMetadata: cleanedPrivate,
          },
        },
      })
    },
  })

  return (
    <DetailPageSection
      title="Custom data"
      description="Public and private key-value pairs for integrations and workflows."
    >
      <div className="grid gap-6">
        <KeyValueMetadataEditor
          title="Metadata"
          description="Publicly exposable values available to channels and APIs."
          rows={publicRows}
          disabled={!canManage}
          onChange={setPublicRows}
        />
        <Separator />
        <KeyValueMetadataEditor
          title="Private metadata"
          description="Internal-only values for admin processes and backend integrations."
          rows={privateRows}
          disabled={!canManage}
          onChange={setPrivateRows}
        />
      </div>
    </DetailPageSection>
  )
}

function KeyValueMetadataEditor({
  title,
  description,
  rows,
  disabled,
  onChange,
}: {
  title: string
  description: string
  rows: MetadataEntry[]
  disabled: boolean
  onChange: (rows: MetadataEntry[]) => void
}) {
  function addRow() {
    onChange([...rows, { key: '', value: '' }])
  }

  function removeRow(index: number) {
    onChange(rows.filter((_, idx) => idx !== index))
  }

  function updateRow(index: number, patch: Partial<MetadataEntry>) {
    onChange(rows.map((row, idx) => (idx === index ? { ...row, ...patch } : row)))
  }

  return (
    <section className="grid gap-3">
      <div className="flex items-center justify-between gap-3">
        <div className="grid gap-1">
          <h3 className="text-sm font-medium">{title}</h3>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>
        <Button variant="outline" size="sm" onClick={addRow} disabled={disabled}>
          <PlusIcon className="size-4" />
          Add entry
        </Button>
      </div>

      <div className="grid gap-2">
        {rows.length === 0 && (
          <div className="rounded-md border border-dashed bg-muted/20 p-4 text-sm text-muted-foreground">
            No entries yet.
          </div>
        )}

        {rows.map((row, index) => (
          <div key={`${title}-${index}`} className="grid gap-2 rounded-md border p-3 md:grid-cols-[minmax(0,180px)_minmax(0,1fr)_auto] md:items-start">
            <Input
              value={row.key}
              placeholder="key"
              disabled={disabled}
              onChange={(event) => updateRow(index, { key: event.target.value })}
            />
            <Input
              value={row.value}
              placeholder="value"
              disabled={disabled}
              onChange={(event) => updateRow(index, { value: event.target.value })}
            />
            <Button
              variant="ghost"
              size="icon"
              className="size-9 text-destructive hover:text-destructive"
              disabled={disabled}
              onClick={() => removeRow(index)}
            >
              <Trash2Icon className="size-4" />
            </Button>
          </div>
        ))}
      </div>
    </section>
  )
}
