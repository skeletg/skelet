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
  BlogCategoriesDocument,
  BlogCategoryDocument,
  CreateBlogCategoryDocument,
  DeleteBlogCategoryContentDocument,
  DeleteBlogCategoryDocument,
  UpdateBlogCategoryDocument,
  UpsertBlogCategoryContentDocument,
  type BlogCategoryNodeFragment,
} from './Blog.gql'

const BACK_TO = '/blog/taxonomy?tab=categories'

// BlogCategoryEditor is the standalone create + edit page for a single blog
// category. `/blog/categories/new` (or no id) is create mode; `/blog/categories/:id`
// loads the category and edits it - This outer component owns the fetch + gating;
// the inner form is key-remounted per category so its state hydrates from props
// via useState initializers (no set-state-in-effect)
export function BlogCategoryEditor() {
  const { id } = useParams()
  const isNew = !id || id === 'new'

  const { data, loading, error, refetch } = useQuery(BlogCategoryDocument, {
    variables: { id: id ?? '' },
    skip: isNew,
  })
  const category = data?.blogCategory

  if (isNew) return <CategoryForm key="new" />
  if (loading) return <LoadingState label="Loading category…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load category"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!category) return <p className="text-sm text-muted-foreground">Category not found.</p>
  // Locale list rides the key so removing a translation re-seeds the form
  return (
    <CategoryForm
      key={`${category.id}:${category.allContent.map((c) => c.locale).join(',')}`}
      category={category}
    />
  )
}

function CategoryForm({ category }: { category?: BlogCategoryNodeFragment }) {
  const isNew = !category
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const currentName = category?.content?.name ?? category?.allContent[0]?.name ?? ''

  const refetchAfterWrite = category
    ? [{ query: BlogCategoriesDocument }, { query: BlogCategoryDocument, variables: { id: category.id } }]
    : [{ query: BlogCategoriesDocument }]

  const [createCat, { loading: creating }] = useMutation(CreateBlogCategoryDocument, {
    refetchQueries: [{ query: BlogCategoriesDocument }],
  })
  const [updateCat] = useMutation(UpdateBlogCategoryDocument, { refetchQueries: refetchAfterWrite })
  const [upsertContent] = useMutation(UpsertBlogCategoryContentDocument, { refetchQueries: refetchAfterWrite })
  const [deleteCat, { loading: deleting }] = useMutation(DeleteBlogCategoryDocument, {
    refetchQueries: [{ query: BlogCategoriesDocument }],
  })
  // Detail query only - the list resolvers return allContent: [], so
  // refetching a list here would blank the locale pills
  const [deleteContent] = useMutation(DeleteBlogCategoryContentDocument, {
    refetchQueries: category ? [{ query: BlogCategoryDocument, variables: { id: category.id } }] : [],
    awaitRefetchQueries: true,
  })

  const [slug, setSlug] = useState(category?.slug ?? '')
  const [slugTouched, setSlugTouched] = useState(!isNew)
  const [name, setName] = useState(currentName)
  const [isPublished, setIsPublished] = useState(category?.isPublished ?? false)
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
  const original = { slug: category?.slug ?? '', name: currentName, isPublished: category?.isPublished ?? false }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const categoryDiff = useMemo(
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
    id: 'category',
    title: 'Category',
    diff: categoryDiff,
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
    if (!slug.trim() || !name.trim()) {
      setSaveError('Slug and name are required.')
      return
    }
    setSaving(true)
    try {
      if (isNew) {
        await createCat({
          variables: {
            input: { slug: slug.trim(), isPublished, locale: scopedLocale, name: name.trim() },
          },
        })
      } else {
        await updateCat({
          variables: {
            input: {
              id: category.id,
              slug: slug.trim() !== category.slug ? slug.trim() : null,
              isPublished,
            },
          },
        })
        if (name.trim() !== currentName) {
          await upsertContent({
            variables: {
              input: { categoryId: category.id, locale: scopedLocale, name: name.trim() },
            },
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
    if (!category) return
    setDeleteError(null)
    try {
      await deleteCat({ variables: { id: category.id } })
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
          title={isNew ? 'New category' : name || category.slug || 'Category'}
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
          meta={!isNew ? <code className="font-mono">{category.urlPath}</code> : null}
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
                {saving || creating ? 'Saving…' : isNew ? 'Create category' : 'Save changes'}
              </Button>
            </>
          }
        />

        <DetailPageBody>
          <DetailPageSection title="Basics" description="Slug change auto-creates a 301 redirect.">
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
                  placeholder="News"
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
                  placeholder="news"
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
                  locales={category.allContent.map((c) => c.locale)}
                  entityLabel="category"
                  canManage={canManage}
                  onRemove={async () => {
                    await deleteContent({
                      variables: { categoryId: category.id, locale: scopedLocale },
                    })
                  }}
                />
              )}
            </div>
          </DetailPageSection>
        </DetailPageBody>

        <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete this category?</AlertDialogTitle>
              <AlertDialogDescription>
                This category must have no children. This action cannot be undone.
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
          scopeLabel={`Category · ${name || category?.slug || ''} · ${scopedLocale}`}
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
