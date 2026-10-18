import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon, Trash2Icon } from 'lucide-react'
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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CmsPageTypesDocument,
  DeleteCmsPageTypeDocument,
  type CmsPageTypeRowFragment,
} from './Cms.gql'

function typeName(t: CmsPageTypeRowFragment) {
  return t.content?.name ?? t.slug
}

// CmsPageTypesPage lists CMS page types in a catalog-style advanced table (search / sort / paginate / system filter) -
// A row links to the standalone page-type editor; "New page type" opens the create page
export function CmsPageTypesPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')

  const { data, loading, error } = useQuery(CmsPageTypesDocument)
  const types = useMemo(() => data?.cmsPageTypes ?? [], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<CmsPageTypeRowFragment | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteType, { loading: deleting }] = useMutation(DeleteCmsPageTypeDocument, {
    refetchQueries: [CmsPageTypesDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    setDeleteError(null)
    try {
      await deleteType({ variables: { id: confirmingDelete.id } })
      setConfirmingDelete(null)
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed.')
    }
  }

  const columns = useMemo<ColumnDef<CmsPageTypeRowFragment>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (t) => typeName(t),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/cms/page-types/${row.original.id}`} className="font-medium hover:underline">
            {typeName(row.original)}
          </Link>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>,
      },
      {
        id: 'attributes',
        accessorFn: (t) => t.attributes.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Attributes" />,
        cell: ({ row }) => <span className="tabular-nums">{row.original.attributes.length}</span>,
      },
      {
        id: 'system',
        accessorFn: (t) => (t.isSystem ? 'system' : 'custom'),
        header: ({ column }) => <DataTableColumnHeader column={column} title="System" />,
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.isSystem ? 'system' : 'custom'),
        cell: ({ row }) =>
          row.original.isSystem ? (
            <Badge variant="secondary">system</Badge>
          ) : (
            <span className="text-sm text-muted-foreground">-</span>
          ),
      },
      {
        id: 'actions',
        header: '',
        enableSorting: false,
        cell: ({ row }) =>
          canManage && !row.original.isSystem ? (
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
        data={types}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(t) => t.id}
        emptyState="No page types yet."
        searchPlaceholder="Search name or slug…"
        initialSorting={[{ id: 'name', desc: false }]}
        filters={[
          {
            column: 'system',
            title: 'System',
            options: [
              { label: 'System', value: 'system' },
              { label: 'Custom', value: 'custom' },
            ],
          },
        ]}
        toolbarRightSlot={
          canManage && (
            <Button size="sm" asChild>
              <Link to="/cms/page-types/new">
                <PlusIcon className="size-4" />
                New page type
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
            <AlertDialogTitle>Delete {confirmingDelete ? typeName(confirmingDelete) : 'page type'}?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes the page type and its attribute assignments. It must have no pages.
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
              {deleting ? 'Deleting…' : 'Delete page type'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
