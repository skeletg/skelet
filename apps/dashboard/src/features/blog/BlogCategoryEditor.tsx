import { useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { LoadingState } from '@/components/ui/loading-state'
import { LoadError } from '@/shared/errors/LoadError'
import { BlogTermForm, type TermKind } from './BlogTermForm'
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

const CATEGORY: TermKind = {
  noun: 'category',
  title: (name, slug) => name || slug || 'Category',
  backTo: '/blog/taxonomy?tab=categories',
  nameRequired: true,
  defaultPublished: false,
  namePlaceholder: 'News',
  slugPlaceholder: 'news',
  deleteDescription: 'This category must have no children. This action cannot be undone.',
}

// The standalone create + edit page for one blog category - `/blog/categories/new` creates,
// `/blog/categories/:id` loads and edits. The form is key-remounted per category and per stored
// locale list, so removing a translation re-seeds it from the refetched record
export function BlogCategoryEditor() {
  const { id } = useParams()
  const isNew = !id || id === 'new'
  const { data, loading, error, refetch } = useQuery(BlogCategoryDocument, { variables: { id: id ?? '' }, skip: isNew })
  const category = data?.blogCategory

  if (isNew) return <CategoryForm key="new" />
  if (loading) return <LoadingState label="Loading category…" />
  if (error) {
    return <LoadError title="Couldn't load category" message={error.message} onRetry={() => void refetch()} retrying={loading} />
  }
  if (!category) return <p className="text-sm text-muted-foreground">Category not found.</p>
  return <CategoryForm key={`${category.id}:${category.allContent.map((c) => c.locale).join(',')}`} category={category} />
}

function CategoryForm({ category }: { category?: BlogCategoryNodeFragment }) {
  // Unlike tags and authors the detail is fetched again after a write - updates answer with
  // `content: null`, and the category tree stops three levels down, so for a deeper category the
  // list refetch wouldn't put it back
  const lists = [{ query: BlogCategoriesDocument }]
  const afterWrite = category ? [...lists, { query: BlogCategoryDocument, variables: { id: category.id } }] : lists
  const [createCategory] = useMutation(CreateBlogCategoryDocument, { refetchQueries: lists })
  const [updateCategory] = useMutation(UpdateBlogCategoryDocument, { refetchQueries: afterWrite })
  const [upsertContent] = useMutation(UpsertBlogCategoryContentDocument, { refetchQueries: afterWrite })
  const [deleteCategory] = useMutation(DeleteBlogCategoryDocument, { refetchQueries: lists })
  // Answers with a Boolean, so the detail is fetched again - the category is normalised, so the
  // tree loses the removed locale too
  const [deleteContent] = useMutation(DeleteBlogCategoryContentDocument, {
    refetchQueries: category ? [{ query: BlogCategoryDocument, variables: { id: category.id } }] : [],
    awaitRefetchQueries: true,
  })

  return (
    <BlogTermForm
      kind={CATEGORY}
      term={category}
      writes={{
        create: (v) => createCategory({ variables: { input: v } }),
        update: (id, v) => updateCategory({ variables: { input: { id, ...v } } }),
        upsertName: (categoryId, locale, name) =>
          upsertContent({ variables: { input: { categoryId, locale, name } } }),
        remove: (id) => deleteCategory({ variables: { id } }),
        removeTranslation: (categoryId, locale) => deleteContent({ variables: { categoryId, locale } }),
      }}
    />
  )
}
