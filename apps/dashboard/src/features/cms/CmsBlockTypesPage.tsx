import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import type { ColumnDef } from '@/components/data-table/tanstack'
import { PlusIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { DataTable } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'

import { CmsBlockTypesDocument, DeleteCmsBlockTypeDocument } from './Cms.gql'
import { parseCustomFields } from './page-blocks'

type Row = {
  id: string
  key: string
  name: string
  fieldCount: number
  updatedAt: string
}

// CmsBlockTypesPage lists admin-defined custom block types - the escape hatch
// for new page-builder blocks without touching the builtin trio
export function CmsBlockTypesPage() {
  const navigate = useNavigate()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')
  const { data, loading, refetch } = useQuery(CmsBlockTypesDocument)
  const [deleteType] = useMutation(DeleteCmsBlockTypeDocument)
  const [confirmDelete, setConfirmDelete] = useState<Row | null>(null)

  const rows: Row[] = useMemo(
    () =>
      (data?.cmsBlockTypes ?? []).map((t) => ({
        id: t.id,
        key: t.key,
        name: t.name,
        fieldCount: parseCustomFields(t.fields).length,
        updatedAt: t.updatedAt as string,
      })),
    [data],
  )

  const columns: ColumnDef<Row>[] = useMemo(
    () => [
      {
        accessorKey: 'name',
        header: 'Name',
        cell: ({ row }) => (
          <Link to={`/cms/block-types/${row.original.id}`} className="font-medium underline-offset-2 hover:underline">
            {row.original.name || '(unnamed)'}
          </Link>
        ),
      },
      {
        accessorKey: 'key',
        header: 'Key',
        cell: ({ row }) => <Badge variant="outline" className="font-mono text-xs">{row.original.key}</Badge>,
      },
      {
        accessorKey: 'fieldCount',
        header: 'Fields',
        cell: ({ row }) => <span className="tabular-nums">{row.original.fieldCount}</span>,
      },
      {
        accessorKey: 'updatedAt',
        header: 'Updated',
        cell: ({ row }) => new Date(row.original.updatedAt).toLocaleString(),
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        cell: ({ row }) =>
          canManage ? (
            <Button
              variant="ghost"
              size="icon"
              className="size-8 text-muted-foreground hover:text-destructive"
              aria-label={`Delete ${row.original.name}`}
              onClick={() => setConfirmDelete(row.original)}
            >
              <Trash2Icon className="size-4" />
            </Button>
          ) : null,
      },
    ],
    [canManage],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        backTo="/cms"
        backLabel="CMS"
        title="Block types"
        meta="Admin-defined page-builder blocks: a field schema plus an HTML template. Pages validate and render them without code changes."
        actions={
          canManage && (
            <Button size="sm" onClick={() => navigate('/cms/block-types/new')}>
              <PlusIcon className="size-4" />
              New block type
            </Button>
          )
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable columns={columns} data={rows} loading={loading} />
        </DetailPageSection>
      </DetailPageBody>

      <AlertDialog open={!!confirmDelete} onOpenChange={(o) => !o && setConfirmDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete "{confirmDelete?.name}"?</AlertDialogTitle>
            <AlertDialogDescription>
              Blocks of this type already placed on pages will stop rendering on the storefront.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (!confirmDelete) return
                deleteType({ variables: { id: confirmDelete.id } })
                  .then(() => {
                    toast.success('Block type deleted')
                    void refetch()
                  })
                  .catch((err: unknown) => toast.error(err instanceof Error ? err.message : 'Delete failed'))
                setConfirmDelete(null)
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageLayout>
  )
}
