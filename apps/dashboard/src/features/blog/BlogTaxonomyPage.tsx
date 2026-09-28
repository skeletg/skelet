import { useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { CheckCircle2Icon, CircleOffIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  BlogAuthorsDocument,
  BlogCategoriesDocument,
  BlogTagsDocument,
  DeleteBlogAuthorDocument,
  DeleteBlogCategoryDocument,
  DeleteBlogTagDocument,
  type BlogAuthorRowFragment,
  type BlogCategoryNodeFragment,
  type BlogTagRowFragment,
} from './Blog.gql'

const TABS = ['categories', 'tags', 'authors'] as const
type TabValue = (typeof TABS)[number]

function isTab(v: string | null): v is TabValue {
  return v != null && (TABS as readonly string[]).includes(v)
}

// BlogTaxonomyPage hosts the three blog taxonomies (categories, tags, authors)
// as catalog-style advanced tables - one per tab - Each row links to its
// standalone editor and each tab has its own "New …" button - The active tab is
// driven by a `?tab=` search param so deep-links and the editors' "back" both
// land on the right tab
export function BlogTaxonomyPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tab = isTab(searchParams.get('tab')) ? (searchParams.get('tab') as TabValue) : 'categories'

  function onTabChange(next: string) {
    const params = new URLSearchParams(searchParams)
    params.set('tab', next)
    setSearchParams(params, { replace: true })
  }

  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Blog taxonomy</h1>
        <p className="text-sm text-muted-foreground">Categories, tags, and authors.</p>
      </div>
      <Tabs value={tab} onValueChange={onTabChange}>
        <TabsList>
          <TabsTrigger value="categories">Categories</TabsTrigger>
          <TabsTrigger value="tags">Tags</TabsTrigger>
          <TabsTrigger value="authors">Authors</TabsTrigger>
        </TabsList>
        <TabsContent value="categories" className="mt-4">
          <CategoriesTable />
        </TabsContent>
        <TabsContent value="tags" className="mt-4">
          <TagsTable />
        </TabsContent>
        <TabsContent value="authors" className="mt-4">
          <AuthorsTable />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ---------- shared helpers ----------

function localizedName(
  entity: { content?: { name: string } | null; allContent: Array<{ name: string }> },
  fallback: string,
): string {
  return entity.content?.name ?? entity.allContent[0]?.name ?? fallback
}

function PublishedBadge({ published }: { published: boolean }) {
  return published ? (
    <span className="inline-flex items-center gap-1.5 text-sm text-success">
      <CheckCircle2Icon className="size-3.5" /> Published
    </span>
  ) : (
    <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
      <CircleOffIcon className="size-3.5" /> Draft
    </span>
  )
}

// Per-tab delete confirmation wired to a mutation; refetches the matching list
function useTaxonomyDelete<T extends { id: string }>(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  document: any,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  refetch: any,
) {
  const [target, setTarget] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [run, { loading }] = useMutation(document, {
    refetchQueries: [{ query: refetch }],
  })
  async function confirm() {
    if (!target) return
    setError(null)
    try {
      await run({ variables: { id: target.id } })
      setTarget(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }
  return { target, setTarget, error, setError, loading, confirm }
}

// ---------- Categories ----------

type CategoryFlatRow = BlogCategoryNodeFragment & { depth: number }

function flattenCategories(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  roots: any[],
  depth = 0,
  out: CategoryFlatRow[] = [],
): CategoryFlatRow[] {
  for (const n of roots) {
    out.push({ ...(n as BlogCategoryNodeFragment), depth })
    if (n.children?.length) flattenCategories(n.children, depth + 1, out)
  }
  return out
}

function CategoriesTable() {
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { data, loading, error } = useQuery(BlogCategoriesDocument)
  const del = useTaxonomyDelete<CategoryFlatRow>(DeleteBlogCategoryDocument, BlogCategoriesDocument)

  const rows = useMemo(() => flattenCategories(data?.blogCategories ?? []), [data])

  const columns = useMemo<ColumnDef<CategoryFlatRow>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (c) => localizedName(c, c.slug),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link
            to={`/blog/categories/${row.original.id}`}
            className="font-medium hover:underline"
            style={{ paddingLeft: `${row.original.depth * 1}rem` }}
          >
            {row.original.depth > 0 && <span className="text-muted-foreground">- </span>}
            {localizedName(row.original, row.original.slug)}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        accessorKey: 'urlPath',
        header: ({ column }) => <DataTableColumnHeader column={column} title="URL path" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.urlPath}</span>,
      },
      {
        id: 'status',
        accessorFn: (c) => (c.isPublished ? 'published' : 'draft'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        filterFn: (r, _id, value: string[]) =>
          !value?.length || value.includes(r.original.isPublished ? 'published' : 'draft'),
        cell: ({ row }) => <PublishedBadge published={row.original.isPublished} />,
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) =>
          canManage ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-muted-foreground hover:text-destructive"
              onClick={() => {
                del.setError(null)
                del.setTarget(row.original)
              }}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          ) : null,
      },
    ],
    [canManage, del],
  )

  return (
    <>
      <DataTable
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(c) => c.id}
        emptyState="No categories yet."
        searchPlaceholder="Search name, slug, or URL…"
        initialSorting={[{ id: 'name', desc: false }]}
        filters={[
          {
            column: 'status',
            title: 'Status',
            options: [
              { label: 'Published', value: 'published' },
              { label: 'Draft', value: 'draft' },
            ],
          },
        ]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/blog/categories/new">
                <PlusIcon className="size-4" />
                New category
              </Link>
            </Button>
          )
        }
      />
      <DeleteDialog
        target={del.target}
        title={del.target ? `Delete ${localizedName(del.target, del.target.slug)}?` : ''}
        description="This category must have no children. This action cannot be undone."
        error={del.error}
        loading={del.loading}
        onCancel={() => {
          del.setTarget(null)
          del.setError(null)
        }}
        onConfirm={del.confirm}
      />
    </>
  )
}

// ---------- Tags ----------

function TagsTable() {
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { data, loading, error } = useQuery(BlogTagsDocument)
  const del = useTaxonomyDelete<BlogTagRowFragment>(DeleteBlogTagDocument, BlogTagsDocument)

  const rows = useMemo(() => data?.blogTags ?? [], [data])

  const columns = useMemo<ColumnDef<BlogTagRowFragment>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (t) => localizedName(t, t.slug),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/blog/tags/${row.original.id}`} className="font-medium hover:underline">
            #{localizedName(row.original, row.original.slug)}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        id: 'status',
        accessorFn: (t) => (t.isPublished ? 'published' : 'draft'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Status" />,
        filterFn: (r, _id, value: string[]) =>
          !value?.length || value.includes(r.original.isPublished ? 'published' : 'draft'),
        cell: ({ row }) => <PublishedBadge published={row.original.isPublished} />,
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) =>
          canManage ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-muted-foreground hover:text-destructive"
              onClick={() => {
                del.setError(null)
                del.setTarget(row.original)
              }}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          ) : null,
      },
    ],
    [canManage, del],
  )

  return (
    <>
      <DataTable
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(t) => t.id}
        emptyState="No tags yet."
        searchPlaceholder="Search name or slug…"
        initialSorting={[{ id: 'name', desc: false }]}
        filters={[
          {
            column: 'status',
            title: 'Status',
            options: [
              { label: 'Published', value: 'published' },
              { label: 'Draft', value: 'draft' },
            ],
          },
        ]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/blog/tags/new">
                <PlusIcon className="size-4" />
                New tag
              </Link>
            </Button>
          )
        }
      />
      <DeleteDialog
        target={del.target}
        title={del.target ? `Delete #${localizedName(del.target, del.target.slug)}?` : ''}
        description="This removes the tag from every post it's attached to. This action cannot be undone."
        error={del.error}
        loading={del.loading}
        onCancel={() => {
          del.setTarget(null)
          del.setError(null)
        }}
        onConfirm={del.confirm}
      />
    </>
  )
}

// ---------- Authors ----------

function AuthorsTable() {
  const { has } = usePermissions()
  const canManage = has('blog.manage')
  const { data, loading, error } = useQuery(BlogAuthorsDocument)
  const del = useTaxonomyDelete<BlogAuthorRowFragment>(DeleteBlogAuthorDocument, BlogAuthorsDocument)

  const rows = useMemo(() => data?.blogAuthors ?? [], [data])

  const columns = useMemo<ColumnDef<BlogAuthorRowFragment>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (a) => a.displayName,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/blog/authors/${row.original.id}`} className="font-medium hover:underline">
            {row.original.displayName}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        id: 'email',
        accessorFn: (a) => a.email ?? '',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Email" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{row.original.email || '-'}</span>
        ),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) =>
          canManage ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-7 px-2 text-muted-foreground hover:text-destructive"
              onClick={() => {
                del.setError(null)
                del.setTarget(row.original)
              }}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          ) : null,
      },
    ],
    [canManage, del],
  )

  return (
    <>
      <DataTable
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(a) => a.id}
        emptyState="No authors yet."
        searchPlaceholder="Search name, slug, or email…"
        initialSorting={[{ id: 'name', desc: false }]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/blog/authors/new">
                <PlusIcon className="size-4" />
                New author
              </Link>
            </Button>
          )
        }
      />
      <DeleteDialog
        target={del.target}
        title={del.target ? `Delete ${del.target.displayName}?` : ''}
        description="This removes the author. Posts referencing it lose their byline. This action cannot be undone."
        error={del.error}
        loading={del.loading}
        onCancel={() => {
          del.setTarget(null)
          del.setError(null)
        }}
        onConfirm={del.confirm}
      />
    </>
  )
}

// ---------- shared delete dialog ----------

function DeleteDialog({
  target,
  title,
  description,
  error,
  loading,
  onCancel,
  onConfirm,
}: {
  target: unknown
  title: string
  description: string
  error: string | null
  loading: boolean
  onCancel: () => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog open={!!target} onOpenChange={(o) => !o && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Delete failed</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={loading}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={loading}
            onClick={(e) => {
              e.preventDefault()
              onConfirm()
            }}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {loading ? 'Deleting…' : 'Delete'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
