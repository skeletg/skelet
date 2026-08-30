import { useMutation, useQuery } from '@apollo/client/react'
import { useMemo, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import type { ColumnDef } from '@/components/data-table/tanstack'

import { DataTable } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { formatCents } from '@/lib/format'
import { useConfigScope } from '@/lib/config-scope'

import { CompaniesDocument, CreateCompanyDocument } from './B2B.gql'

type Row = {
  id: string
  name: string
  status: string
  creditLimitCents: number
  creditUsedCents: number
  paymentTermsDays: number
  defaultCustomerGroupSlug: string
}

export function CompaniesPage() {
  const { data, loading, refetch } = useQuery(CompaniesDocument)
  const [creating, setCreating] = useState(false)

  const rows: Row[] = useMemo(
    () =>
      (data?.companies ?? []).map((c) => ({
        id: c.id,
        name: c.name,
        status: c.status,
        creditLimitCents: c.creditLimitCents,
        creditUsedCents: c.creditUsedCents,
        paymentTermsDays: c.paymentTermsDays,
        defaultCustomerGroupSlug: c.defaultCustomerGroupSlug,
      })),
    [data],
  )

  const columns: ColumnDef<Row>[] = useMemo(
    () => [
      {
        accessorKey: 'name',
        header: 'Company',
        cell: ({ row }) => (
          <Link
            to={`/b2b/companies/${row.original.id}`}
            className="font-medium underline-offset-2 hover:underline"
          >
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => (
          <Badge variant={row.original.status === 'active' ? 'default' : 'destructive'}>
            {row.original.status}
          </Badge>
        ),
      },
      {
        id: 'credit',
        header: 'Credit',
        cell: ({ row }) => (
          <span className="tabular-nums">
            {formatCents(row.original.creditUsedCents, 'USD')} /{' '}
            {formatCents(row.original.creditLimitCents, 'USD')}
          </span>
        ),
      },
      {
        accessorKey: 'paymentTermsDays',
        header: 'Terms',
        cell: ({ row }) => `Net ${row.original.paymentTermsDays}`,
      },
      {
        accessorKey: 'defaultCustomerGroupSlug',
        header: 'Price group',
        cell: ({ row }) => row.original.defaultCustomerGroupSlug || '-',
      },
    ],
    [],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Companies"
        meta="Wholesale accounts: shared credit lines, payment terms, and a price group applied to every member."
        actions={<Button onClick={() => setCreating(true)}>New company</Button>}
      />
      <DetailPageBody>
        <DetailPageSection>
          <DataTable columns={columns} data={rows} loading={loading} />
        </DetailPageSection>
      </DetailPageBody>

      <CreateCompanyDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={() => void refetch()}
      />
    </DetailPageLayout>
  )
}

function CreateCompanyDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated: () => void
}) {
  const { effectiveChannelId } = useConfigScope()
  const [name, setName] = useState('')
  const [limit, setLimit] = useState('0')
  const [terms, setTerms] = useState('30')
  const [error, setError] = useState<string | null>(null)
  const [create, { loading }] = useMutation(CreateCompanyDocument)

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    if (!effectiveChannelId) {
      setError('Pick a channel first (header switcher).')
      return
    }
    try {
      await create({
        variables: {
          channelId: effectiveChannelId,
          name: name.trim(),
          creditLimitCents: Math.round(Number.parseFloat(limit || '0') * 100),
          paymentTermsDays: Number.parseInt(terms || '30', 10),
        },
      })
      setName('')
      onCreated()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not create company.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>New company</DialogTitle>
          <DialogDescription>
            Created on the active channel. Attach members from the company
            detail afterwards.
          </DialogDescription>
        </DialogHeader>
        <form id="b2b-create" onSubmit={onSubmit} className="grid gap-3">
          <div className="grid gap-1.5">
            <Label htmlFor="b2b-name">Company name</Label>
            <Input id="b2b-name" required value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="b2b-limit">Credit limit</Label>
              <Input id="b2b-limit" type="number" step="0.01" min="0" value={limit} onChange={(e) => setLimit(e.target.value)} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="b2b-terms">Terms (days)</Label>
              <Input id="b2b-terms" type="number" min="0" value={terms} onChange={(e) => setTerms(e.target.value)} />
            </div>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </form>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" form="b2b-create" disabled={loading}>
            {loading ? 'Creating…' : 'Create company'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
