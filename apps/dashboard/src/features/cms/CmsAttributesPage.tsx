import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { LayersIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { usePermissions } from '@/shared/auth/usePermissions'
import { CmsAttributeGroupsDialog } from './CmsAttributeGroupsDialog'
import {
  CmsAttributesDocument,
  DeleteCmsAttributeDocument,
  type CmsAttributeRowFragment,
} from './Cms.gql'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

const INPUT_TYPE_FILTER_OPTIONS = [
  { label: 'Text', value: 'TEXT' },
  { label: 'Textarea', value: 'TEXTAREA' },
  { label: 'Rich text', value: 'RICHTEXT' },
  { label: 'Number', value: 'NUMBER' },
  { label: 'Boolean', value: 'BOOLEAN' },
  { label: 'Date', value: 'DATE' },
  { label: 'Date + time', value: 'DATETIME' },
  { label: 'URL', value: 'URL' },
  { label: 'Select', value: 'SELECT' },
  { label: 'Multi-select', value: 'MULTISELECT' },
  { label: 'Asset', value: 'ASSET' },
  { label: 'Assets', value: 'ASSETS' },
  { label: 'Relationship', value: 'RELATIONSHIP' },
  { label: 'JSON', value: 'JSON' },
]

function acceptsValues(t: CmsAttributeRowFragment['inputType']): boolean {
  return t === 'SELECT' || t === 'MULTISELECT'
}

function attrName(a: CmsAttributeRowFragment): string {
  return a.content?.name?.trim() || a.allContent[0]?.name?.trim() || a.slug
}

// CmsAttributesPage lists CMS attribute definitions in a catalog-style advanced table (search / sort / paginate /
// filter by input type) - A row links to the standalone attribute editor; "New attribute" opens the create page
export function CmsAttributesPage() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('cms.types_manage')

  const { data, loading, error } = useQuery(CmsAttributesDocument)
  const attributes = useMemo(() => data?.cmsAttributes ?? [], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<CmsAttributeRowFragment | null>(null)
  const [deleteAttr] = useMutation(DeleteCmsAttributeDocument, {
    refetchQueries: [CmsAttributesDocument],
  })

  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteAttr({ variables: { id: confirmingDelete.id } })
  }

  const columns = useMemo<ColumnDef<CmsAttributeRowFragment>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (a) => attrName(a),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="flex flex-wrap items-center gap-2">
            <Link to={`/cms/attributes/${row.original.id}`} className="font-medium hover:underline">
              {attrName(row.original)}
            </Link>
            {row.original.isTranslatable && (
              <Badge variant="secondary" className="text-xs">
                translatable
              </Badge>
            )}
            {row.original.isRequiredDefault && (
              <Badge variant="secondary" className="text-xs">
                required
              </Badge>
            )}
            {row.original.relationshipTarget && (
              <Badge variant="outline" className="text-xs">
                → {row.original.relationshipTarget}
              </Badge>
            )}
          </div>
        ),
      },
      {
        accessorKey: 'slug',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Slug" />,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{row.original.slug}</span>
        ),
      },
      {
        id: 'inputType',
        accessorFn: (a) => a.inputType,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Input type" />,
        filterFn: (row, _id, value: string[]) =>
          !value?.length || value.includes(row.original.inputType),
        cell: ({ row }) => (
          <Badge variant="outline" className="text-xs">
            {row.original.inputType.toLowerCase()}
          </Badge>
        ),
      },
      {
        id: 'values',
        accessorFn: (a) => a.values.length,
        header: ({ column }) => <DataTableColumnHeader column={column} title="Options" />,
        cell: ({ row }) =>
          acceptsValues(row.original.inputType) ? (
            <span className="tabular-nums">{row.original.values.length}</span>
          ) : (
            <span className="text-muted-foreground">-</span>
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
        data={attributes}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(a) => a.id}
        emptyState="No attributes yet."
        searchPlaceholder="Search name or slug…"
        initialSorting={[{ id: 'name', desc: false }]}
        filters={[
          {
            column: 'inputType',
            title: 'Input type',
            options: INPUT_TYPE_FILTER_OPTIONS,
          },
        ]}
        toolbarRightSlot={
          canManage && (
            <div className="flex items-center gap-2">
              <CmsAttributeGroupsDialog
                trigger={
                  <Button size="sm" variant="outline">
                    <LayersIcon className="size-4" />
                    Groups
                  </Button>
                }
              />
              <Button size="sm" asChild>
                <Link to="/cms/attributes/new">
                  <PlusIcon className="size-4" />
                  New attribute
                </Link>
              </Button>
            </div>
          )
        }
      />

      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(open) => !open && setConfirmingDelete(null)}
        title={<>Delete {confirmingDelete ? attrName(confirmingDelete) : 'attribute'}?</>}
        description="This permanently removes the attribute definition. It must be unassigned from all page types first."
        confirmLabel="Delete attribute"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </>
  )
}
