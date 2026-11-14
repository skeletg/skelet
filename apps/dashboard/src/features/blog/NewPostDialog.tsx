import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { useEditingLocale } from '@/lib/editing-locale'
import { slugify } from '@/lib/slug'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { BlogPostsDocument, CreateBlogPostDocument } from './Blog.gql'
import { newPostSchema, toNewPostInput } from './schemas/newPost'

export function NewPostDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange} className="sm:max-w-xl">
      {(close) => <NewPostForm onClose={close} />}
    </FormDialog>
  )
}

function NewPostForm({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const editingLocale = useEditingLocale()
  const locales = useQuery(LocalesPickerDocument).data?.locales ?? []
  // The slug follows the title until it is typed into by hand
  const [slugTouched, setSlugTouched] = useState(false)
  const [createPost] = useMutation(CreateBlogPostDocument, { refetchQueries: [BlogPostsDocument] })

  const dialog = useDialogForm({
    baseline: { title: '', slug: '', locale: editingLocale, excerpt: '' },
    schema: newPostSchema,
    submit: async (v) => {
      const result = await createPost({ variables: { input: toNewPostInput(v) } })
      const id = result.data?.createBlogPost.id
      if (!id) throw new Error("The post wasn't created.")
      navigate(`/blog/posts/${id}`)
    },
  })
  const { form } = dialog
  const title = useStore(form.store, (s) => s.values.title)
  const locale = useStore(form.store, (s) => s.values.locale)

  return (
    <FormDialogBody
      dialog={dialog}
      title="New post"
      description="A title is enough to start. The writing, its categories and where it's published all come on the next page."
      submitLabel="Create post"
      onCancel={onClose}
    >
      <form.AppField
        name="title"
        listeners={{
          onChange: ({ value }) => {
            if (!slugTouched) form.setFieldValue('slug', slugify(value))
          },
        }}
      >
        {(f) => <f.TextField id="title" label="Title" placeholder="What is the post called?" autoFocus className="[&_input]:h-12 [&_input]:text-lg [&_input]:font-medium" />}
      </form.AppField>
      <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-[minmax(0,1fr)_12rem]">
        <form.AppField name="slug">
          {(f) => (
            <f.TextField
              id="slug"
              label="Slug"
              placeholder="hello-world"
              onValueChange={() => setSlugTouched(true)}
              hint={!slugTouched && title ? 'Follows the title until you change it.' : "The end of the post's address."}
            />
          )}
        </form.AppField>
        <form.AppField name="locale">
          {(f) => (
            <f.SelectField
              id="locale"
              label="Written in"
              placeholder="Pick a language…"
              options={
                locales.length === 0
                  ? [{ value: locale, label: locale }]
                  : locales.map((l) => ({ value: l.code, label: `${l.flagEmoji ? `${l.flagEmoji} ` : ''}${l.name} (${l.code})` }))
              }
            />
          )}
        </form.AppField>
      </div>
      <form.AppField name="excerpt">
        {(f) => <f.TextareaField id="excerpt" label="Excerpt (optional)" rows={3} placeholder="A line or two for post lists and feeds." />}
      </form.AppField>
    </FormDialogBody>
  )
}
