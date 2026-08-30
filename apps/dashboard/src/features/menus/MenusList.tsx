import { useMemo, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useDraft } from '@/shared/saving/useDraft'
import { duringCommit } from '@/lib/nav-guard'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon, Trash2Icon } from 'lucide-react'

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
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import { slugify } from '@/lib/slug'

import {
  CreateMenuDocument,
  DeleteMenuDocument,
  MenusListDocument,
} from './Menus.gql'

type MenuRow = {
  id: string
  slug: string
  name: string
  itemCount: number
  channelIds: string[]
}

// MenusList lists navigation menus in a catalog-style advanced table (search / sort / paginate) - A row links to the
// standalone menu editor; "New menu" opens the create page
export function MenusList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('cms.manage')

  const { data, loading, error } = useQuery(MenusListDocument)
  const rows = useMemo(
    () => ((data?.menus ?? []) as MenuRow[]).map((m) => ({ ...m })),
    [data],
  )

  const [confirmingDelete, setConfirmingDelete] = useState<MenuRow | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteMenu, { loading: deleting }] = useMutation(DeleteMenuDocument, {
    refetchQueries: [{ query: MenusListDocument }],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteMenu({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<MenuRow>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (m) => m.name,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/menus/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        id: 'items',
        accessorFn: (m) => m.itemCount,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Items" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.itemCount}</span>,
      },
      {
        id: 'channels',
        accessorFn: (m) => m.channelIds.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Channels" />,
        cell: ({ row }) => {
          const n = row.original.channelIds.length
          return <span className="text-sm text-muted-foreground">{n === 0 ? 'Unassigned' : `${n} channel${n === 1 ? '' : 's'}`}</span>
        },
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
                setDeleteError(null)
                setConfirmingDelete(row.original)
              }}
            >
              <Trash2Icon className="size-3.5" />
            </Button>
          ) : null,
      },
    ],
    [canManage],
  )

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(m) => m.id}
        emptyState="No menus yet."
        searchPlaceholder="Search name or slug…"
        initialSorting={[{ id: 'name', desc: false }]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/menus/new">
                <PlusIcon className="size-4" />
                New menu
              </Link>
            </Button>
          )
        }
      />

      <AlertDialog
        open={!!confirmingDelete}
        onOpenChange={(v) => {
          if (!v) {
            setConfirmingDelete(null)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {confirmingDelete?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the menu and every item inside it. Channels referencing
              the slug fall back to their empty state.
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
                onConfirmDelete()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleting ? 'Deleting…' : 'Delete menu'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

// MenuCreatePane is the standalone /menus/new create page (reached from the
// table's "New menu" button)
export function MenuCreatePane() {
  const navigate = useNavigate()
  const draft = useDraft({
    slug: '',
    name: '',
  }, 'New menu')
  const { slug, name } = draft.values
  const { setSlug, setName } = draft.setters
  const [slugTouched, setSlugTouched] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [createMenu, { loading: creating }] = useMutation(CreateMenuDocument, {
    refetchQueries: [{ query: MenusListDocument }],
  })

  function onNameChange(next: string) {
    setName(next)
    if (!slugTouched) setSlug(slugify(next))
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setSubmitError(null)
    try {
      const res = await createMenu({
        variables: { input: { slug: slug.trim(), name: name.trim() } },
      })
      const id = res.data?.createMenu.id
      await duringCommit(() => (id ? navigate(`/menus/${id}`) : navigate('/menus')))
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  return (
    <form id="menu-create-form" onSubmit={onSubmit} className="flex flex-col">
      <div className="sticky top-0 z-10 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="flex flex-wrap items-center justify-between gap-3 px-6 py-3">
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-lg font-semibold tracking-tight">
              New menu
            </h1>
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              Admin-curated navigation menus for the channel header, footer, and
              any other slot.
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button type="button" variant="outline" size="sm" asChild>
              <Link to="/menus">Cancel</Link>
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={creating || !slug.trim() || !name.trim()}
            >
              {creating ? 'Creating…' : 'Create menu'}
            </Button>
          </div>
        </div>
      </div>

      <DetailPageBody>
        {submitError && (
          <div className="px-6 py-4">
            <Alert variant="destructive">
              <AlertTitle>Create failed</AlertTitle>
              <AlertDescription>{submitError}</AlertDescription>
            </Alert>
          </div>
        )}

        <DetailPageSection
          title="Basics"
          description="Pick a slug the channel will reference (e.g. main-nav, footer-shop) and a human-readable name. Items can be added on the next screen."
        >
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="menu-name">Name</Label>
              <Input
                id="menu-name"
                required
                placeholder="Main Navigation"
                value={name}
                onChange={(e) => onNameChange(e.target.value)}
                autoFocus
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="menu-slug">Slug</Label>
              <Input
                id="menu-slug"
                required
                placeholder="main-nav"
                value={slug}
                onChange={(e) => {
                  setSlug(e.target.value)
                  setSlugTouched(true)
                }}
              />
              <p className="text-xs text-muted-foreground">
                Auto-filled from name - edit to override.
              </p>
            </div>
          </div>
        </DetailPageSection>
      </DetailPageBody>
    </form>
  )
}
