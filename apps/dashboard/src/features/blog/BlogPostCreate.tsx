import { useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { DetailPageSection } from '@/components/layout/detail-page'
import { Button } from '@/components/ui/button'
import { useEditingLocale } from '@/lib/editing-locale'
import { slugify } from '@/lib/slug'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { BlogPostsDocument, CreateBlogPostDocument } from './Blog.gql'
import { newPostSchema, toNewPostInput } from './schemas/newPost'

export function BlogPostCreate() {
  const navigate = useNavigate()
  const editingLocale = useEditingLocale()
  const locales = useQuery(LocalesPickerDocument).data?.locales ?? []
  // Until the title is typed into the slug by hand, the slug follows the title
  const [slugTouched, setSlugTouched] = useState(false)
  const [createPost] = useMutation(CreateBlogPostDocument, {
    refetchQueries: [{ query: BlogPostsDocument, variables: { filter: { includeUnpublished: true }, limit: 200, offset: 0 } }],
  })
  const createdId = useRef<string | null>(null)

  const baseline = useMemo(() => ({ title: '', slug: '', locale: editingLocale, excerpt: '' }), [editingLocale])
  const record = useRecordForm({
    id: 'blog-post',
    title: 'Post',
    label: 'New post',
    isEdit: false,
    baseline,
    schema: newPostSchema,
    diff: () => [],
    persist: { routeKey: 'blog-post:new' },
    submit: async (v) => {
      const result = await createPost({ variables: { input: toNewPostInput(v) } })
      createdId.current = result.data?.createBlogPost.id ?? null
    },
    onSaved: () => navigate(createdId.current ? `/blog/posts/${createdId.current}` : '/blog/posts'),
  })
  const { form } = record
  const title = useStore(form.store, (s) => s.values.title)
  const locale = useStore(form.store, (s) => s.values.locale)

  return (
    <RecordFormPage
      record={record}
      backTo="/blog/posts"
      backLabel="Back to posts"
      title="New blog post"
      meta="Start with a title - slug auto-fills, but you can override it. Body, status, categories, and tags come later."
      createLabel="Create post"
      actions={
        <Button type="button" variant="outline" size="sm" asChild>
          <Link to="/blog/posts">Cancel</Link>
        </Button>
      }
    >
      <DetailPageSection title="Basics" description="The slug is permanent in URLs but a 301 redirect is created on change.">
        <div className="grid max-w-2xl gap-4">
          <form.AppField
            name="title"
            listeners={{
              onChange: ({ value }) => {
                if (!slugTouched) form.setFieldValue('slug', slugify(value))
              },
            }}
          >
            {(f) => <f.TextField id="title" label="Title" placeholder="A clear, concise title" />}
          </form.AppField>
          <form.AppField name="slug">
            {(f) => (
              <f.TextField
                id="slug"
                label="Slug"
                placeholder="hello-world"
                onValueChange={() => setSlugTouched(true)}
                hint={!slugTouched && title ? 'Generated from the title - type here to override.' : undefined}
              />
            )}
          </form.AppField>
          <form.AppField name="locale">
            {(f) => (
              <f.SelectField
                id="locale"
                label="Initial locale"
                placeholder="Pick a locale…"
                options={
                  locales.length === 0
                    ? [{ value: locale, label: locale }]
                    : locales.map((l) => ({
                        value: l.code,
                        label: `${l.flagEmoji ? `${l.flagEmoji} ` : ''}${l.name} (${l.code})`,
                      }))
                }
              />
            )}
          </form.AppField>
          <form.AppField name="excerpt">{(f) => <f.TextareaField id="excerpt" label="Excerpt (optional)" rows={3} />}</form.AppField>
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}
