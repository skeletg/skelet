import { useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { BlogTermForm, type TermKind } from './BlogTermForm'
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

const TAG: TermKind = {
  noun: 'tag',
  title: (name, slug) => `#${name || slug || 'tag'}`,
  backTo: '/blog/taxonomy?tab=tags',
  nameRequired: false,
  defaultPublished: true,
  namePlaceholder: 'Release notes',
  slugPlaceholder: 'release-notes',
  deleteDescription: "This removes the tag from every post it's attached to. This action cannot be undone.",
}

// The standalone create + edit page for one blog tag - `/blog/tags/new` creates, `/blog/tags/:id`
// loads and edits. The form is key-remounted per tag and per stored locale list, so removing a
// translation re-seeds it from the refetched record
export function BlogTagEditor() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const { data, loading, error, refetch } = useQuery(BlogTagDocument, { variables: { id: id ?? '' }, skip: isNew })
  const tag = data?.blogTag

  if (isNew) return <TagForm key="new" />
  if (loading) return <LoadingState label="Loading tag…" />
  if (error) return <LoadError title="Couldn't load tag" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  if (!tag) return <p className="text-sm text-muted-foreground">Tag not found.</p>
  return <TagForm key={`${tag.id}:${tag.allContent.map((c) => c.locale).join(',')}`} tag={tag} />
}

function TagForm({ tag }: { tag?: BlogTagRowFragment }) {
  const lists = [{ query: BlogTagsDocument }]
  const afterWrite = tag ? [...lists, { query: BlogTagDocument, variables: { id: tag.id } }] : lists
  const [createTag] = useMutation(CreateBlogTagDocument, { refetchQueries: lists })
  const [updateTag] = useMutation(UpdateBlogTagDocument, { refetchQueries: afterWrite })
  const [upsertContent] = useMutation(UpsertBlogTagContentDocument, { refetchQueries: afterWrite })
  const [deleteTag] = useMutation(DeleteBlogTagDocument, { refetchQueries: lists })
  // Detail query only - the list resolvers return allContent: [], so refetching a list here would
  // blank the locale pills
  const [deleteContent] = useMutation(DeleteBlogTagContentDocument, {
    refetchQueries: tag ? [{ query: BlogTagDocument, variables: { id: tag.id } }] : [],
    awaitRefetchQueries: true,
  })

  return (
    <BlogTermForm
      kind={TAG}
      term={tag}
      writes={{
        create: (v) => createTag({ variables: { input: v } }),
        update: (id, v) => updateTag({ variables: { input: { id, ...v } } }),
        upsertName: (tagId, locale, name) => upsertContent({ variables: { input: { tagId, locale, name } } }),
        remove: (id) => deleteTag({ variables: { id } }),
        removeTranslation: (tagId, locale) => deleteContent({ variables: { tagId, locale } }),
      }}
    />
  )
}
