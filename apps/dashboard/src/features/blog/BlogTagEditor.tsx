import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { CheckCircle2Icon, CircleOffIcon, Trash2Icon } from 'lucide-react'
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
import { Switch } from '@/components/ui/switch'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { fieldDiff } from '@/shared/saving/diff'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'
import { slugify } from '@/lib/slug'
import { TranslationLocales } from '@/components/TranslationLocales'
import {
  BlogTagDocument,
  BlogTagsDocument,
  CreateBlogTagDocument,
  DeleteBlogTagContentDocument,
  DeleteBlogTagDocument,
  UpdateBlogTagDocument,
  UpsertBlogTagContentDocument,
  type BlogTagRowFragment,
} from './Blog.gql'

const BACK_TO = '/blog/taxonomy?tab=tags'

// BlogTagEditor is the standalone create + edit page for a single blog tag
// `/blog/tags/new` (or no id) is create mode; `/blog/tags/:id` loads and edits
// The outer component fetches + gates; the inner form is key-remounted per tag
// so its state hydrates from props (no set-state-in-effect)
export function BlogTagEditor() {
  const { id } = useParams()
  const isNew = !id || id === 'new'

  const { data, loading, error, refetch } = useQuery(BlogTagDocument, {
    variables: { id: id ?? '' },
    skip: isNew,
  })
  const tag = data?.blogTag

  if (isNew) return <TagForm key="new" />
  if (loading) return <LoadingState label="Loading tag…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load tag"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!tag) return <p className="text-sm text-muted-foreground">Tag not found.</p>
  // Locale list rides the key so removing a translation re-seeds the form
  return <TagForm key={`${tag.id}:${tag.allContent.map((c) => c.locale).join(',')}`} tag={tag} />
}

function TagForm({ tag }: { tag?: BlogTagRowFragment }) {
  const isNew = !tag
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const currentName = tag?.content?.name ?? tag?.allContent[0]?.name ?? ''

  const refetchAfterWrite = tag
    ? [{ query: BlogTagsDocument }, { query: BlogTagDocument, variables: { id: tag.id } }]
    : [{ query: BlogTagsDocument }]

  const [createTag, { loading: creating }] = useMutation(CreateBlogTagDocument, {
    refetchQueries: [{ query: BlogTagsDocument }],
  })
  const [updateTag] = useMutation(UpdateBlogTagDocument, { refetchQueries: refetchAfterWrite })
  const [upsertContent] = useMutation(UpsertBlogTagContentDocument, { refetchQueries: refetchAfterWrite })
  const [deleteTag, { loading: deleting }] = useMutation(DeleteBlogTagDocument, {
    refetchQueries: [{ query: BlogTagsDocument }],
  })
  // Detail query only - the list resolvers return allContent: [], so
  // refetching a list here would blank the locale pills
  const [deleteContent] = useMutation(DeleteBlogTagContentDocument, {
    refetchQueries: tag ? [{ query: BlogTagDocument, variables: { id: tag.id } }] : [],
    awaitRefetchQueries: true,
  })

  const [slug, setSlug] = useState(tag?.slug ?? '')
  const [slugTouched, setSlugTouched] = useState(!isNew)
  const [name, setName] = useState(currentName)
  const [isPublished, setIsPublished] = useState(tag?.isPublished ?? true)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [confirmError, setConfirmError] = useState<string | null>(null)

  function onNameChange(next: string) {
    setName(next)
    if (isNew && !slugTouched) setSlug(slugify(next))
  }

  // Localized name lives per-locale, so the scope label names the locale being edited
  const draft = { slug, name, isPublished }
  const original = { slug: tag?.slug ?? '', name: currentName, isPublished: tag?.isPublished ?? true }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const tagDiff = useMemo(
    () =>
      !isNew
        ? fieldDiff(original, draft, [
            { label: 'Slug', value: (o) => o.slug },
            { label: 'Name', value: (o) => o.name },
            { label: 'Published', value: (o) => o.isPublished },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isNew, originalKey, draftKey],
  )

  useSaveSection({
    id: 'tag',
    title: 'Tag',
    diff: tagDiff,
    save: () => applyChanges(),
  })

  async function onConfirmSave() {
    setConfirmError(null)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setConfirmError(err instanceof Error ? err.message : 'Save failed.')
    }
  }

  async function applyChanges() {
    setSaveError(null)
    if (!slug.trim()) {
      setSaveError('Slug is required.')
      throw new Error('Slug is required.')
    }
    setSaving(true)
    try {
      if (isNew) {
        await createTag({
          variables: {
            input: {
              slug: slug.trim(),
              isPublished,
              locale: scopedLocale,
              name: name.trim() || slug.trim(),
            },
          },
        })
      } else {
        await updateTag({
          variables: {
            input: {
              id: tag.id,
              slug: slug.trim() !== tag.slug ? slug.trim() : null,
              isPublished,
            },
          },
        })
        const nextName = name.trim() || slug.trim()
        if (nextName !== currentName) {
          await upsertContent({
            variables: { input: { tagId: tag.id, locale: scopedLocale, name: nextName } },
          })
        }
      }
      navigate(BACK_TO)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed.')
      throw err // the dialog keeps itself open and surfaces the message
    } finally {
      setSaving(false)
    }
  }

  async function onDelete() {
    if (!tag) return
    setDeleteError(null)
    try {
      await deleteTag({ variables: { id: tag.id } })
      navigate(BACK_TO, { replace: true })
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  return (
    <SaveRegistryContext.Provider value={saveContextValue}>
      <DetailPageLayout>
        <DetailPageHeader
          backTo={BACK_TO}
          backLabel="Back to taxonomy"
          title={isNew ? 'New tag' : `#${name || tag.slug || 'tag'}`}
          badges={
            !isNew ? (
              isPublished ? (
                <Badge variant="secondary" className="gap-1">
                  <CheckCircle2Icon className="size-3" /> Published
                </Badge>
              ) : (
                <Badge variant="outline" className="gap-1 text-muted-foreground">
                  <CircleOffIcon className="size-3" /> Draft
                </Badge>
              )
            ) : null
          }
          actions={
            <>
              {!isNew && canManage && (
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
              <Button type="button" variant="outline" size="sm" asChild>
                <Link to={BACK_TO}>Cancel</Link>
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={!canManage || saving || creating || (!isNew && !isDirty)}
                onClick={() => (isNew ? void applyChanges() : setConfirmingSave(true))}
              >
                {saving || creating ? 'Saving…' : isNew ? 'Create tag' : 'Save changes'}
              </Button>
            </>
          }
        />

        <DetailPageBody>
          <DetailPageSection title="Basics">
            {saveError && (
              <Alert variant="destructive">
                <AlertTitle>Save failed</AlertTitle>
                <AlertDescription>{saveError}</AlertDescription>
              </Alert>
            )}
            <div className="grid max-w-2xl gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="name">Name ({scopedLocale})</Label>
                <Input
                  id="name"
                  value={name}
                  onChange={(e) => onNameChange(e.target.value)}
                  placeholder="Release notes"
                  disabled={!canManage}
                  autoFocus={isNew}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="slug">Slug</Label>
                <Input
                  id="slug"
                  value={slug}
                  onChange={(e) => {
                    setSlug(e.target.value)
                    setSlugTouched(true)
                  }}
                  placeholder="release-notes"
                  disabled={!canManage}
                />
                {isNew && !slugTouched && name && (
                  <p className="text-xs text-muted-foreground">
                    Generated from the name - type here to override.
                  </p>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Switch
                  id="published"
                  checked={isPublished}
                  onCheckedChange={setIsPublished}
                  disabled={!canManage}
                />
                <Label htmlFor="published">Published</Label>
              </div>
              {!isNew && (
                <TranslationLocales
                  locale={scopedLocale}
                  locales={tag.allContent.map((c) => c.locale)}
                  entityLabel="tag"
                  canManage={canManage}
                  onRemove={async () => {
                    await deleteContent({ variables: { tagId: tag.id, locale: scopedLocale } })
                  }}
                />
              )}
            </div>
          </DetailPageSection>
        </DetailPageBody>

        <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete this tag?</AlertDialogTitle>
              <AlertDialogDescription>
                This removes the tag from every post it&apos;s attached to. This action cannot be undone.
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
                {deleting ? 'Deleting…' : 'Delete'}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
        <SaveConfirmDialog
          open={confirmingSave}
          scopeLabel={`Tag · ${name || tag?.slug || ''} · ${scopedLocale}`}
          sections={dirtySections}
          saving={saving}
          error={confirmError}
          onCancel={() => { if (!saving) { setConfirmingSave(false); setConfirmError(null) } }}
          onConfirm={onConfirmSave}
        />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}
