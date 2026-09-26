import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { gql } from '@apollo/client'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  CheckCircle2Icon,
  CircleIcon,
  MoreHorizontalIcon,
  PlusIcon,
  Trash2Icon,
} from 'lucide-react'

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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import { useConfigScope } from '@/lib/config-scope'
import { usePermissions } from '@/shared/auth/usePermissions'

const TAX_RULES_QUERY = gql`
  query TaxRules($channelId: ID) {
    taxRules(channelId: $channelId) {
      id
      channelId
      name
      priority
      exclusivityGroup
      isActive
      conditions {
        type
        params
      }
      effects {
        type
        params
      }
    }
  }
`

const DELETE_TAX_RULE = gql`
  mutation DeleteTaxRule($id: ID!) {
    deleteTaxRule(id: $id)
  }
`

type RuleRow = { type: string; params: Record<string, unknown> | null }

type TaxRule = {
  id: string
  channelId: string
  name: string
  priority: number
  exclusivityGroup: string
  isActive: boolean
  conditions: RuleRow[]
  effects: RuleRow[]
}

function deriveRatePct(rule: TaxRule): string | null {
  const pct = rule.effects.find(
    (e) => e.type === 'core.percentage' || e.type === 'core.compound_percentage',
  )
  const bps = pct?.params?.['bps']
  if (typeof bps !== 'number') return null
  return (bps / 100).toFixed(2)
}

function deriveScope(rule: TaxRule): string {
  const j = rule.conditions.find((c) => c.type === 'core.jurisdiction')
  if (!j?.params) return rule.conditions.length ? `${rule.conditions.length} condition(s)` : 'any'
  const country = (j.params['country_code'] as string) || '*'
  const region = (j.params['region_code'] as string) || ''
  return region ? `${country} / ${region}` : country
}

// TaxRulesTable is the /tax index: the shared DataTable (search, filter, pagination) over the active channel's tax rules - same chrome as
// the channels list - New / row-edit go to dedicated pages - Tax rules are per-channel, so GLOBAL scope shows a prompt instead of the table
export function TaxRulesTable() {
  // Creating and deleting tax rules need settings.manage - without it the buttons only led to an access-restricted page or a refused save
  const canManage = usePermissions().has('settings.manage')
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const navigate = useNavigate()
  const { effectiveChannelId, isChannel, channels } = useConfigScope()

  // Channel scope rides the `X-Channel-Id` header (see lib/apollo.ts), not a query var: GLOBAL scope (no header) →
  // every channel's rules + a Channel column; CHANNEL scope → that channel's rules
  const { data, loading, error, refetch } = useQuery<{ taxRules: TaxRule[] }>(TAX_RULES_QUERY)
  const rows = useMemo(() => data?.taxRules ?? [], [data])

  const channelName = useMemo(() => {
    const m = new Map<string, string>()
    for (const c of channels) m.set(c.id, c.name)
    return (id: string) => m.get(id) ?? id
  }, [channels])

  const [confirmDelete, setConfirmDelete] = useState<TaxRule | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [deleteRule, { loading: deleting }] = useMutation<{ deleteTaxRule: boolean }, { id: string }>(
    DELETE_TAX_RULE,
  )

  async function onConfirmDelete() {
    if (!confirmDelete) return
    setDeleteError(null)
    try {
      await deleteRule({ variables: { id: confirmDelete.id } })
      setConfirmDelete(null)
      void refetch()
    } catch (err) {
      setDeleteError(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  const columns = useMemo<ColumnDef<TaxRule>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/tax/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name || '(unnamed)'}
          </Link>
        ),
      },
      ...(!isChannel
        ? [
            {
              id: 'channel',
              header: 'Channel',
              enableSorting: false,
              cell: ({ row }) => (
                <Badge variant="outline" className="font-normal">
                  {channelName(row.original.channelId)}
                </Badge>
              ),
            } as ColumnDef<TaxRule>,
          ]
        : []),
      {
        id: 'scope',
        header: 'Scope',
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono text-xs text-muted-foreground">{deriveScope(row.original)}</span>
        ),
      },
      {
        id: 'rate',
        header: 'Rate',
        size: 90,
        enableSorting: false,
        cell: ({ row }) => {
          const pct = deriveRatePct(row.original)
          return <span className="font-mono text-xs">{pct != null ? `${pct}%` : '-'}</span>
        },
      },
      {
        accessorKey: 'priority',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Priority" />,
        size: 100,
        cell: ({ row }) => <span className="font-mono text-xs">{row.original.priority}</span>,
      },
      {
        accessorKey: 'exclusivityGroup',
        header: 'Group',
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{row.original.exclusivityGroup || '-'}</span>
        ),
      },
      {
        accessorKey: 'isActive',
        header: 'Status',
        size: 100,
        filterFn: (row, _colId, value: string[]) => {
          if (!value?.length) return true
          return value.includes(row.original.isActive ? 'active' : 'inactive')
        },
        cell: ({ row }) =>
          row.original.isActive ? (
            <Badge variant="secondary">Active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">
              Inactive
            </Badge>
          ),
      },
      {
        id: 'actions',
        size: 48,
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="size-8"
                aria-label={`Actions for ${row.original.name || 'tax rule'}`}
              >
                <MoreHorizontalIcon className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem asChild>
                <Link to={`/tax/${row.original.id}`}>Edit</Link>
              </DropdownMenuItem>
              {canManage && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onSelect={() => {
                      setDeleteError(null)
                      setConfirmDelete(row.original)
                    }}
                  >
                    <Trash2Icon className="size-4" />
                    Delete
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ),
      },
    ],
    [isChannel, channelName, canManage],
  )

  return (
    <>
      <DataTable
        urlState={urlState}
        columns={columns}
        data={rows}
        loading={loading}
        error={error ? { message: error.message } : null}
        getRowId={(row) => row.id}
        emptyState={isChannel ? 'No tax rules for this channel yet.' : 'No tax rules yet.'}
        searchPlaceholder="Search rules…"
        initialSorting={[{ id: 'priority', desc: false }]}
        filters={[
          {
            column: 'isActive',
            title: 'Status',
            options: [
              { label: 'Active', value: 'active', icon: CheckCircle2Icon },
              { label: 'Inactive', value: 'inactive', icon: CircleIcon },
            ],
          },
        ]}
        toolbarRightSlot={
          <div className="flex items-center gap-2">
            {canManage && (
              <Button
                size="sm"
                onClick={() => navigate(`/tax/new?channelId=${effectiveChannelId ?? ''}`)}
                disabled={!isChannel}
                title={isChannel ? undefined : 'Switch the header to a channel to add a rule.'}
              >
                <PlusIcon className="size-4" />
                New tax rule
              </Button>
            )}
          </div>
        }
      />

      <AlertDialog
        open={!!confirmDelete}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmDelete(null)
            setDeleteError(null)
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete tax rule?</AlertDialogTitle>
            <AlertDialogDescription>
              {confirmDelete?.name || '(unnamed rule)'} - this is permanent.
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
              {deleting ? 'Deleting…' : 'Delete'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

