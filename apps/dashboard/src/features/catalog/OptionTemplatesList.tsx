import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@apollo/client/react'
import { PlusIcon } from 'lucide-react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { usePermissions } from '@/shared/auth/usePermissions'
import { OptionTemplatesDocument, type OptionTemplatesQuery } from './Catalog.gql'
import { OptionTemplateDetail } from './OptionTemplateDetail'
import { NewTemplateForm } from './product-options/NewTemplateForm'
import { templatePath, usedByLabel } from './product-options/template-path'

type Row = OptionTemplatesQuery['optionTemplates'][number]

const count = (n: number) => <span className="text-xs text-muted-foreground">{n === 0 ? '-' : n}</span>

export function OptionTemplatesList() {
  const urlState = useListUrlState()
  const params = useParams<{ id?: string }>()
  const { allowedChannelScopes } = usePermissions()
  const canCreate = allowedChannelScopes('catalog.manage').allChannels
  const { data, loading, error } = useQuery(OptionTemplatesDocument)
  const rows = useMemo(() => data?.optionTemplates ?? [], [data])

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="grid min-w-0">
            <Link to={templatePath(row.original.id)} className="truncate font-medium hover:underline">
              {row.original.name}
            </Link>
            {row.original.description ? <span className="truncate text-xs text-muted-foreground">{row.original.description}</span> : null}
          </div>
        ),
      },
      { id: 'options', header: 'Options', size: 100, cell: ({ row }) => count(row.original.optionCount) },
      { id: 'rules', header: 'Rules', size: 90, cell: ({ row }) => count(row.original.ruleCount) },
      {
        id: 'products',
        header: 'Used by',
        size: 170,
        cell: ({ row }) => <span className="text-xs text-muted-foreground">{usedByLabel(row.original) || '-'}</span>,
      },
      {
        accessorKey: 'currencyCode',
        header: 'Currency',
        size: 100,
        cell: ({ row }) => <span className="font-mono text-xs text-muted-foreground">{row.original.currencyCode}</span>,
      },
    ],
    [],
  )

  if (params.id) return <OptionTemplateDetail templateId={params.id} />

  return (
    <DataTable
      urlState={urlState}
      columns={columns}
      data={rows}
      loading={loading}
      error={error ? { message: error.message } : null}
      getRowId={(row) => row.id}
      emptyState="No configurator templates yet. A template is a set of options and rules you keep once and use on many products."
      searchPlaceholder="Search templates…"
      toolbarRightSlot={canCreate ? <NewTemplateButton /> : null}
    />
  )
}

function NewTemplateButton() {
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  return (
    <>
      <Button size="sm" onClick={() => setOpen(true)}>
        <PlusIcon className="size-4" />
        New template
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>New configurator template</DialogTitle>
            <DialogDescription>Add its options and rules next, then pick the products or product types that use it.</DialogDescription>
          </DialogHeader>
          {open ? (
            <NewTemplateForm
              onCancel={() => setOpen(false)}
              onCreated={(id) => {
                setOpen(false)
                navigate(templatePath(id))
              }}
            />
          ) : null}
        </DialogContent>
      </Dialog>
    </>
  )
}
