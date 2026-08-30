import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Trash2Icon } from 'lucide-react'
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
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
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
  BlogAuthorDocument,
  BlogAuthorsDocument,
  CreateBlogAuthorDocument,
  DeleteBlogAuthorContentDocument,
  DeleteBlogAuthorDocument,
  UpdateBlogAuthorDocument,
  UpsertBlogAuthorContentDocument,
  type BlogAuthorRowFragment,
} from './Blog.gql'

const BACK_TO = '/blog/taxonomy?tab=authors'

// Lexical-shaped rich-text node carrying the bio as a single plain-text
// paragraph - matches the original inline editor's bio handling
function bioFromText(text: string) {
  return text
    ? { root: { children: [{ type: 'paragraph', children: [{ type: 'text', text }] }] } }
    : null
}

// Pull whatever plain text we can out of a stored lexical bio for display
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function bioToText(bio: any): string {
  const children = bio?.root?.children
  if (!Array.isArray(children)) return ''
  const parts: string[] = []
  for (const block of children) {
    for (const node of block?.children ?? []) {
      if (typeof node?.text === 'string') parts.push(node.text)
    }
  }
  return parts.join('\n')
}

// BlogAuthorEditor is the standalone create + edit page for a single blog
// author. `/blog/authors/new` (or no id) is create mode; `/blog/authors/:id`
// loads and edits - The outer component fetches + gates; the inner form is
// key-remounted per author so its state hydrates from props (no
// set-state-in-effect)
export function BlogAuthorEditor() {
  const { id } = useParams()
  const isNew = !id || id === 'new'

  const { data, loading, error, refetch } = useQuery(BlogAuthorDocument, {
    variables: { id: id ?? '' },
    skip: isNew,
  })
  const author = data?.blogAuthor

  if (isNew) return <AuthorForm key="new" />
  if (loading) return <LoadingState label="Loading author…" />
  if (error)
    return (
      <LoadError
        title="Couldn't load author"
        message={error.message}
        onRetry={() => void refetch()}
        retrying={loading}
      />
    )
  if (!author) return <p className="text-sm text-muted-foreground">Author not found.</p>
  // Locale list rides the key so removing a translation re-seeds the form
  return (
    <AuthorForm
      key={`${author.id}:${author.allContent.map((c) => c.locale).join(',')}`}
      author={author}
    />
  )
}

function AuthorForm({ author }: { author?: BlogAuthorRowFragment }) {
  const isNew = !author
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { scope, selectedChannel, globalDefaultLocale } = useConfigScope()
  const scopedLocale =
    scope.mode === 'CHANNEL'
      ? scope.locale || selectedChannel?.defaultLocale || globalDefaultLocale
      : globalDefaultLocale

  const refetchAfterWrite = author
    ? [{ query: BlogAuthorsDocument }, { query: BlogAuthorDocument, variables: { id: author.id } }]
    : [{ query: BlogAuthorsDocument }]

  const [createAuthor, { loading: creating }] = useMutation(CreateBlogAuthorDocument, {
    refetchQueries: [{ query: BlogAuthorsDocument }],
  })
  const [updateAuthor] = useMutation(UpdateBlogAuthorDocument, { refetchQueries: refetchAfterWrite })
  const [upsertContent] = useMutation(UpsertBlogAuthorContentDocument, { refetchQueries: refetchAfterWrite })
  const [deleteAuthor, { loading: deleting }] = useMutation(DeleteBlogAuthorDocument, {
    refetchQueries: [{ query: BlogAuthorsDocument }],
  })
  // Detail query only - the list resolvers return allContent: [], so
  // refetching a list here would blank the locale pills
  const [deleteContent] = useMutation(DeleteBlogAuthorContentDocument, {
    refetchQueries: author ? [{ query: BlogAuthorDocument, variables: { id: author.id } }] : [],
    awaitRefetchQueries: true,
  })

  const [slug, setSlug] = useState(author?.slug ?? '')
  const [slugTouched, setSlugTouched] = useState(!isNew)
  const [displayName, setDisplayName] = useState(author?.displayName ?? '')
  const [email, setEmail] = useState(author?.email ?? '')
  const [bio, setBio] = useState(bioToText(author?.content?.bio ?? author?.allContent[0]?.bio))
  const [saveError, setSaveError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  // Header Save reviews the change list before writing - docs/conventions.md
  const { contextValue: saveContextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [confirmError, setConfirmError] = useState<string | null>(null)

  function onNameChange(next: string) {
    setDisplayName(next)
    if (isNew && !slugTouched) setSlug(slugify(next))
  }

  // Localized name lives per-locale, so the scope label names the locale being edited
  const draft = { slug, displayName, email, bio }
  const original = { slug: author?.slug ?? '', displayName: author?.displayName ?? '', email: author?.email ?? '', bio: bioToText(author?.content?.bio ?? author?.allContent[0]?.bio) }
  // Keyed on the serialized pair: both are fresh object literals every render, so the memo
  // needs their CONTENT - and a dep list takes plain identifiers, not calls
  const originalKey = JSON.stringify(original)
  const draftKey = JSON.stringify(draft)
  const authorDiff = useMemo(
    () =>
      !isNew
        ? fieldDiff(original, draft, [
            { label: 'Slug', value: (o) => o.slug },
            { label: 'Display name', value: (o) => o.displayName },
            { label: 'Email', value: (o) => o.email },
            { label: 'Bio', value: (o) => o.bio },
          ])
        : [],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [isNew, originalKey, draftKey],
  )

  useSaveSection({
    id: 'author',
    title: 'Author',
    diff: authorDiff,
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
    if (!slug.trim() || !displayName.trim()) {
      setSaveError('Slug and display name are required.')
      return
    }
    setSaving(true)
    try {
      if (isNew) {
        await createAuthor({
          variables: {
            input: {
              slug: slug.trim(),
              displayName: displayName.trim(),
              email: email.trim() || null,
              locale: scopedLocale,
              name: displayName.trim(),
              bio: bioFromText(bio.trim()),
            },
          },
        })
      } else {
        await updateAuthor({
          variables: {
            input: {
              id: author.id,
              slug: slug.trim() !== author.slug ? slug.trim() : null,
              displayName: displayName.trim(),
              email: email.trim() || null,
              clearEmail: !email.trim(),
            },
          },
        })
        await upsertContent({
          variables: {
            input: {
              authorId: author.id,
              locale: scopedLocale,
              name: displayName.trim(),
              bio: bioFromText(bio.trim()),
            },
          },
        })
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
    if (!author) return
    setDeleteError(null)
    try {
      await deleteAuthor({ variables: { id: author.id } })
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
          title={isNew ? 'New author' : displayName || author.slug || 'Author'}
          meta={!isNew && email ? <span>{email}</span> : null}
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
                {saving || creating ? 'Saving…' : isNew ? 'Create author' : 'Save changes'}
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
                <Label htmlFor="display-name">Display name</Label>
                <Input
                  id="display-name"
                  value={displayName}
                  onChange={(e) => onNameChange(e.target.value)}
                  placeholder="Jane Doe"
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
                  placeholder="jane-doe"
                  disabled={!canManage}
                />
                {isNew && !slugTouched && displayName && (
                  <p className="text-xs text-muted-foreground">
                    Generated from the name - type here to override.
                  </p>
                )}
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="email">Email (optional)</Label>
                <Input
                  id="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="jane@example.com"
                  disabled={!canManage}
                />
              </div>
              <div className="grid gap-1.5">
                <Label htmlFor="bio">Bio ({scopedLocale}, plain text)</Label>
                <Textarea
                  id="bio"
                  value={bio}
                  onChange={(e) => setBio(e.target.value)}
                  rows={4}
                  disabled={!canManage}
                />
              </div>
              {!isNew && (
                <TranslationLocales
                  locale={scopedLocale}
                  locales={author.allContent.map((c) => c.locale)}
                  entityLabel="author"
                  canManage={canManage}
                  onRemove={async () => {
                    await deleteContent({ variables: { authorId: author.id, locale: scopedLocale } })
                  }}
                />
              )}
            </div>
          </DetailPageSection>
        </DetailPageBody>

        <AlertDialog open={confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(false)}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete this author?</AlertDialogTitle>
              <AlertDialogDescription>
                Posts referencing this author lose their byline. This action cannot be undone.
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
          scopeLabel={`Author · ${displayName || author?.slug || ''} · ${scopedLocale}`}
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
