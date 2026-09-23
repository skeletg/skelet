import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { Trash2Icon } from 'lucide-react'
import { DetailPageSection } from '@/components/layout/detail-page'
import { TranslationLocales } from '@/components/TranslationLocales'
import { Button } from '@/components/ui/button'
import { LoadingState } from '@/components/ui/loading-state'
import { useEditingLocale } from '@/lib/editing-locale'
import { slugify } from '@/lib/slug'
import { usePermissions } from '@/shared/auth/usePermissions'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { LoadError } from '@/shared/errors/LoadError'
import { ClashNote } from '@/shared/saving/RestoredDraftBanner'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
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
import { AUTHOR_LABELS, authorDiff, authorSchema, bioFromText, toAuthorValues, type AuthorValues } from './schemas/author'

const BACK_TO = '/blog/taxonomy?tab=authors'

// The standalone create + edit page for one blog author - `/blog/authors/new` creates,
// `/blog/authors/:id` loads and edits. The form is key-remounted per author and per stored locale
// list, so removing a translation re-seeds it from the refetched record
export function BlogAuthorEditor() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const { data, loading, error, refetch } = useQuery(BlogAuthorDocument, { variables: { id: id ?? '' }, skip: isNew })
  const author = data?.blogAuthor

  if (isNew) return <AuthorForm key="new" />
  if (loading) return <LoadingState label="Loading author…" />
  if (error) {
    return <LoadError title="Couldn't load author" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }
  if (!author) return <p className="text-sm text-muted-foreground">Author not found.</p>
  return <AuthorForm key={`${author.id}:${author.allContent.map((c) => c.locale).join(',')}`} author={author} />
}

function AuthorForm({ author }: { author?: BlogAuthorRowFragment }) {
  const isNew = !author
  const navigate = useNavigate()
  const canManage = usePermissions().has('blog.manage')
  const locale = useEditingLocale()
  const [slugTouched, setSlugTouched] = useState(!isNew)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const lists = [{ query: BlogAuthorsDocument }]
  const afterWrite = author ? [...lists, { query: BlogAuthorDocument, variables: { id: author.id } }] : lists
  const [createAuthor] = useMutation(CreateBlogAuthorDocument, { refetchQueries: lists })
  const [updateAuthor] = useMutation(UpdateBlogAuthorDocument, { refetchQueries: afterWrite })
  const [upsertContent] = useMutation(UpsertBlogAuthorContentDocument, { refetchQueries: afterWrite })
  const [deleteAuthor] = useMutation(DeleteBlogAuthorDocument, { refetchQueries: lists })
  // Detail query only - the list resolvers return allContent: [], so refetching a list here would
  // blank the locale pills
  const [deleteContent] = useMutation(DeleteBlogAuthorContentDocument, {
    refetchQueries: author ? [{ query: BlogAuthorDocument, variables: { id: author.id } }] : [],
    awaitRefetchQueries: true,
  })

  const baseline = useMemo(() => toAuthorValues(author), [author])
  const record = useRecordForm({
    id: 'author',
    title: 'Author',
    label: `Author · ${baseline.displayName || baseline.slug || 'new'} · ${locale}`,
    isEdit: !isNew,
    baseline,
    schema: authorSchema,
    diff: authorDiff,
    // The bio is per locale, so a draft typed in one locale mustn't come back in another
    persist: { routeKey: `blog-author:${author?.id ?? 'new'}:${locale}` },
    submit: async (v: AuthorValues) => {
      const slug = v.slug.trim()
      const displayName = v.displayName.trim()
      const email = v.email.trim()
      const bio = bioFromText(v.bio.trim())
      if (!author) {
        await createAuthor({
          variables: { input: { slug, displayName, email: email || null, locale, name: displayName, bio } },
        })
        return
      }
      await updateAuthor({
        variables: {
          input: {
            id: author.id,
            slug: slug !== author.slug ? slug : null,
            displayName,
            email: email || null,
            clearEmail: !email,
          },
        },
      })
      await upsertContent({ variables: { input: { authorId: author.id, locale, name: displayName, bio } } })
    },
    onSaved: () => navigate(BACK_TO),
  })
  const { form, draft } = record
  const values = useStore(form.store, (s) => s.values)
  const clash = (field: keyof AuthorValues) => <ClashNote draft={draft} field={field} />

  return (
    <>
      <RecordFormPage
        record={record}
        readOnly={!canManage}
        backTo={BACK_TO}
        backLabel="Back to taxonomy"
        title={isNew ? 'New author' : values.displayName || author.slug || 'Author'}
        meta={!isNew && values.email ? <span>{values.email}</span> : null}
        createLabel="Create author"
        actions={
          <>
            {author && canManage && (
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
            )}
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to={BACK_TO}>Cancel</Link>
            </Button>
          </>
        }
        labels={AUTHOR_LABELS}
      >
        <DetailPageSection title="Basics">
          <div className="grid max-w-2xl gap-4">
            <form.AppField
              name="displayName"
              listeners={{
                // New authors derive the slug from the name until the admin edits it themselves
                onChange: ({ value }) => {
                  if (isNew && !slugTouched) form.setFieldValue('slug', slugify(value))
                },
              }}
            >
              {(f) => (
                <f.TextField
                  id="display-name"
                  label="Display name"
                  placeholder="Jane Doe"
                  autoFocus={isNew}
                  note={clash('displayName')}
                />
              )}
            </form.AppField>
            <form.AppField name="slug">
              {(f) => (
                <f.TextField
                  id="slug"
                  label="Slug"
                  placeholder="jane-doe"
                  onValueChange={() => setSlugTouched(true)}
                  hint={isNew && !slugTouched && values.displayName ? 'Generated from the name - type here to override.' : undefined}
                  note={clash('slug')}
                />
              )}
            </form.AppField>
            <form.AppField name="email">
              {(f) => <f.TextField id="email" label="Email (optional)" placeholder="jane@example.com" note={clash('email')} />}
            </form.AppField>
            <form.AppField name="bio">
              {(f) => <f.TextareaField id="bio" label={`Bio (${locale}, plain text)`} rows={4} note={clash('bio')} />}
            </form.AppField>
            {author && (
              <TranslationLocales
                locale={locale}
                locales={author.allContent.map((c) => c.locale)}
                entityLabel="author"
                canManage={canManage}
                onRemove={async () => {
                  await deleteContent({ variables: { authorId: author.id, locale } })
                }}
              />
            )}
          </div>
        </DetailPageSection>
      </RecordFormPage>

      {author && (
        <ConfirmDialog
          open={confirmDelete}
          onOpenChange={setConfirmDelete}
          title="Delete this author?"
          description="Posts referencing this author lose their byline. This action cannot be undone."
          confirmLabel="Delete"
          busyLabel="Deleting…"
          destructive
          onConfirm={async () => {
            await deleteAuthor({ variables: { id: author.id } })
            navigate(BACK_TO, { replace: true })
          }}
        />
      )}
    </>
  )
}
