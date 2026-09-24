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
import { DetailPageSection } from '@/components/layout/detail-page'
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
  RequestCollectionCoverUploadDocument,
  UpdateCollectionDocument,
  UpsertCollectionContentDocument,
} from './Collections.gql'
import { LexicalRichTextEditor, type RichTextValue } from './LexicalRichTextEditor'
import { UrlRewritesSection } from './UrlRewritesSection'
import { AutocompletePicker, type Option } from '@/features/promotions/widgets/AutocompletePicker'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
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
import { COLLECTION_LABELS, collectionChanges, collectionDiff, collectionSchema, toCollectionValues } from './schemas/collection'

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

export function CollectionDetail() {
  const { id = '' } = useParams()
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  // Locale comes from the `Locale` request header (set by the Apollo localeLink); we still derive
  // `scopedLocale` for client-side rendering - picking the active translation row to seed the form
  const scopedLocale = scope.mode === 'CHANNEL' ? scope.locale || selectedChannel?.defaultLocale || undefined : undefined

  const collectionQuery = useQuery(CollectionDocument, {
    variables: { id },
    skip: !id,
    notifyOnNetworkStatusChange: true,
  })
  const collection = collectionQuery.data?.collection

  if (collectionQuery.loading && !collection) return <LoadingState label="Loading collection…" />
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
  if (!collection) return <p className="text-sm text-muted-foreground">Collection not found.</p>

  const locale = scopedLocale ?? collection.contentLocale ?? collection.content[0]?.locale ?? globalDefaultLocale
  // Keyed on the locale too - the name, description and SEO fields are that locale's translation
  return <CollectionForm key={`${collection.id}:${locale}`} collection={collection} locale={locale} />
}

// Slug, flags, the translation and metadata are one form behind the header Save; the cover upload and
// the storefront filter order stage as their own sections into the same registry, and products and
// URL paths write as they're edited
function CollectionForm({ collection, locale }: { collection: CollectionRecord; locale: string }) {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('catalog.manage')
  const refetchQueries: CollectionRefetch = [{ query: CollectionDocument, variables: { id: collection.id } }]

  const [updateCollection] = useMutation(UpdateCollectionDocument, { refetchQueries })
  const [upsertContent] = useMutation(UpsertCollectionContentDocument, { refetchQueries })
  const [deleteContent, { loading: deletingContent }] = useMutation(DeleteCollectionContentDocument, { refetchQueries })
  const [deleteCollection, { loading: deleting }] = useMutation(DeleteCollectionDocument, {
    refetchQueries: [{ query: CollectionsDocument }],
  })

  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [confirmingContentDelete, setConfirmingContentDelete] = useState(false)
  const [contentDeleteError, setContentDeleteError] = useState<string | null>(null)

  const baseline = useMemo(() => toCollectionValues(collection, locale), [collection, locale])
  const schema = useMemo(() => collectionSchema(baseline), [baseline])
  const displayName = resolveCollectionName(collection)
  const hasTranslation = collection.content.some((c) => c.locale === locale)

  const record = useRecordForm({
    id: 'collection',
    title: `Collection (${locale})`,
    label: `Collection · ${displayName} · ${locale}`,
    isEdit: true,
    baseline,
    schema,
    diff: collectionDiff,
    persist: { routeKey: `collection:${collection.id}:${locale}` },
    submit: async (v) => {
      const changes = collectionChanges(baseline, v)
      if (changes.update) await updateCollection({ variables: { input: { id: collection.id, ...changes.update } } })
      if (changes.content) await upsertContent({ variables: { collectionId: collection.id, locale, input: changes.content } })
    },
  })
  const { form } = record

  async function onDelete() {
    setDeleteError(null)
    try {
      await deleteCollection({ variables: { id: collection.id } })
      navigate('/collections', { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

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
    <RecordFormPage
      record={record}
      backTo="/collections"
      backLabel="Back to collections"
      title={displayName}
      badges={
        collection.isPublished ? (
          <Badge variant="secondary">Published</Badge>
        ) : (
          <Badge variant="outline" className="text-muted-foreground">
            Draft
          </Badge>
        )
      }
      meta={<code className="font-mono">{collection.slug}</code>}
      actions={
        canManage && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="text-destructive hover:bg-destructive/10 hover:text-destructive"
            onClick={() => {
              setDeleteError(null)
              setConfirmingDelete(true)
            }}
            disabled={deleting}
          >
            <Trash2Icon className="size-4" />
            Delete
          </Button>
        )
      }
      createLabel="Save changes"
      labels={COLLECTION_LABELS}
      hideSave={!canManage}
    >
      <DetailPageSection title="General" description="Slug, visibility, cover image, and public name and description.">
        <div className="grid gap-6">
          <fieldset disabled={!canManage} className="grid gap-4">
            <form.AppField name="slug">{(f) => <f.TextField id="collection-detail-slug" label="Slug" />}</form.AppField>
            <form.AppField name="isPublished">
              {(f) => (
                <f.SwitchField id="collection-published" label="Published" description="Make this collection visible on channels." bordered={false} />
              )}
            </form.AppField>
            <form.AppField name="seoNoindex">
              {(f) => (
                <f.SwitchField id="collection-noindex" label="Noindex" description="Ask search engines not to index this collection." bordered={false} />
              )}
            </form.AppField>
          </fieldset>
          <Separator />
          <CollectionCoverBlock collection={collection} canManage={canManage} refetchQueries={refetchQueries} />
          <Separator />
          <fieldset disabled={!canManage} className="grid gap-4">
            <div className="flex items-start justify-between gap-3">
              <div>
                <h4 className="text-sm font-medium">Translation</h4>
                <p className="text-xs text-muted-foreground">Channel name and description for this locale.</p>
              </div>
              <Badge variant="outline" className="shrink-0">
                {locale}
              </Badge>
            </div>
            <form.AppField name="name">{(f) => <f.TextField id="collection-content-name" label="Name" />}</form.AppField>
            <div className="grid gap-2">
              <Label>Description</Label>
              <form.Field name="description">
                {(f) => (
                  <LexicalRichTextEditor
                    value={f.state.value as RichTextValue}
                    fallbackText=""
                    disabled={!canManage}
                    placeholder="Describe the collection..."
                    onChange={(next, plainText) => {
                      f.handleChange(next)
                      form.setFieldValue('descriptionText', plainText)
                    }}
                  />
                )}
              </form.Field>
            </div>
          </fieldset>
        </div>
      </DetailPageSection>

      <CollectionProductsSection collection={collection} canManage={canManage} refetchQueries={refetchQueries} />

      <DetailPageSection title="SEO" description="Meta tags and search-engine URL paths." trailing={<Badge variant="outline">{locale}</Badge>}>
        <div className="grid gap-4">
          <fieldset disabled={!canManage || deletingContent} className="grid gap-4">
            <div className="grid items-start gap-4 md:grid-cols-2">
              <form.AppField name="metaTitle">{(f) => <f.TextField id="collection-content-meta-title" label="Meta title" />}</form.AppField>
              <form.AppField name="metaKeywords">
                {(f) => <f.TextField id="collection-content-meta-keywords" label="Meta keywords" />}
              </form.AppField>
            </div>
            <form.AppField name="metaDescription">
              {(f) => <f.TextareaField id="collection-content-meta-description" label="Meta description" rows={4} />}
            </form.AppField>
          </fieldset>

          {canManage && (
            <div className="flex justify-end">
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setContentDeleteError(null)
                  setConfirmingContentDelete(true)
                }}
                disabled={!hasTranslation || deletingContent}
              >
                <Trash2Icon className="size-4" />
                Delete translation
              </Button>
            </div>
          )}

          <Separator />

          <div>
            <h4 className="text-sm font-medium">URL paths</h4>
            <p className="text-xs text-muted-foreground">Canonical path and redirects for this collection.</p>
          </div>
          <UrlRewritesSection entityId={collection.id} entityType="COLLECTION" entityLabel="collection" canManage={canManage} />
        </div>
      </DetailPageSection>

      <CollectionFacetOverridesSection collection={collection} canManage={canManage} refetchQueries={refetchQueries} />

      <DetailPageSection title="Custom data" description="Public and private key-value pairs for integrations and workflows.">
        <fieldset disabled={!canManage} className="grid gap-6">
          <section className="grid gap-3">
            <div className="grid gap-1">
              <h3 className="text-sm font-medium">Metadata</h3>
              <p className="text-sm text-muted-foreground">Publicly exposable values available to channels and APIs.</p>
            </div>
            <form.AppField name="metadata">{(f) => <f.MetadataField />}</form.AppField>
          </section>
          <Separator />
          <section className="grid gap-3">
            <div className="grid gap-1">
              <h3 className="text-sm font-medium">Private metadata</h3>
              <p className="text-sm text-muted-foreground">Internal-only values for admin processes and backend integrations.</p>
            </div>
            <form.AppField name="privateMetadata">{(f) => <f.MetadataField label="Private metadata" />}</form.AppField>
          </section>
        </fieldset>
      </DetailPageSection>

      <PluginSections target="collection" entity={collection} mode="edit" />

      <AlertDialog
        open={confirmingContentDelete}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmingContentDelete(false)
            setContentDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {locale} translation?</AlertDialogTitle>
            <AlertDialogDescription>This removes the locale-specific name, description, and SEO fields for this collection.</AlertDialogDescription>
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

      <AlertDialog
        open={confirmingDelete}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmingDelete(false)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete collection?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the collection record, cover image reference, translations, product memberships, and autogenerated public
              rewrite.
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
    </RecordFormPage>
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

