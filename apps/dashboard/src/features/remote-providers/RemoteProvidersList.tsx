import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import {
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  RefreshCwIcon,
  Trash2Icon,
} from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTable, DataTableColumnHeader, useListUrlState } from '@/components/data-table'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import { usePermissions } from '@/shared/auth/usePermissions'
import { NewSecretDialog } from '@/features/webhooks/NewSecretDialog'
import { RemoteProviderFormDialog, type ExistingProvider } from './RemoteProviderFormDialog'
import { KIND_LABELS } from './kinds'
import {
  DeleteRemoteProviderDocument,
  RemoteProvidersDocument,
  RotateRemoteProviderSecretDocument,
  SetRemoteProviderActiveDocument,
} from './RemoteProviders.gql'

type ProviderRow = ExistingProvider & {
  isActive: boolean
  circuit?: { state: string; consecutiveFailures: number } | null
}

// RemoteProvidersList - the /remote-providers admin page - Providers are endpoints that IMPLEMENT a role (rates, tax, validation, promo
// logic, flow actions); this page is registration + health, the consumers live in their own subsystems
export function RemoteProvidersList() {
  // URL-backed, so a filtered view survives navigation and can be pasted to someone else
  const urlState = useListUrlState()
  const { has } = usePermissions()
  const canManage = has('remote_providers.manage')

  const { data, loading, error, refetch } = useQuery(RemoteProvidersDocument)
  const rows = useMemo(() => (data?.remoteProviders ?? []) as ProviderRow[], [data])

  const [confirmingDelete, setConfirmingDelete] = useState<ProviderRow | null>(null)
  const [confirmingRotate, setConfirmingRotate] = useState<ProviderRow | null>(null)
  const [newSecret, setNewSecret] = useState<{ secret: string; name: string } | null>(null)
  const [opError, setOpError] = useState<string | null>(null)

  const [deleteProvider] = useMutation(DeleteRemoteProviderDocument, {
    refetchQueries: [RemoteProvidersDocument],
  })
  const [setActive] = useMutation(SetRemoteProviderActiveDocument)
  const [rotateSecret] = useMutation(RotateRemoteProviderSecretDocument)

  async function onToggleActive(row: ProviderRow, active: boolean) {
    setOpError(null)
    try {
      await setActive({ variables: { id: row.id, active } })
      refetch()
    } catch (err) {
      setOpError(err instanceof Error ? err.message : 'Toggle failed.')
    }
  }

  // The old secret stops verifying the moment this runs, so it sits behind a confirm - which also shows a failure
  async function onConfirmRotate() {
    if (!confirmingRotate) return
    const { data } = await rotateSecret({ variables: { id: confirmingRotate.id } })
    if (data) {
      setNewSecret({
        secret: data.rotateRemoteProviderSecret.rawSecret,
        name: data.rotateRemoteProviderSecret.provider.name,
      })
    }
  }

  // Throws on failure - ConfirmDialog shows the message and stays open, rather than closing
  // of closing the dialog and putting the error in the page-level banner
  async function onConfirmDelete() {
    if (!confirmingDelete) return
    await deleteProvider({ variables: { id: confirmingDelete.id } })
    setConfirmingDelete(null)
  }

  const columns = useMemo<ColumnDef<ProviderRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <div className="grid gap-0.5">
            <span className="font-medium">{row.original.name}</span>
            <code className="font-mono text-[11px] text-muted-foreground">
              {row.original.slug}
            </code>
          </div>
        ),
      },
      {
        accessorKey: 'kind',
        header: 'Role',
        cell: ({ row }) => (
          <Badge variant="secondary">{KIND_LABELS[row.original.kind]}</Badge>
        ),
      },
      {
        accessorKey: 'targetUrl',
        header: 'Endpoint',
        cell: ({ row }) => (
          <code className="block max-w-md truncate font-mono text-xs text-muted-foreground">
            {row.original.targetUrl}
          </code>
        ),
      },
      {
        accessorKey: 'failurePolicy',
        header: 'On failure',
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {row.original.failurePolicy === 'FAIL_CLOSED' ? 'Abort' : 'Fall back'}
          </span>
        ),
      },
      {
        id: 'health',
        header: 'Health',
        cell: ({ row }) => {
          const circuit = row.original.circuit
          if (!row.original.isActive) {
            return <Badge variant="outline">Inactive</Badge>
          }
          if (circuit && circuit.state !== 'CLOSED') {
            return (
              <Badge variant="destructive">
                Circuit {circuit.state.toLowerCase().replace('_', '-')}
              </Badge>
            )
          }
          return <Badge variant="outline">OK</Badge>
        },
      },
      {
        accessorKey: 'channel',
        header: 'Scope',
        cell: ({ row }) =>
          row.original.channel ? (
            <Badge variant="secondary">{row.original.channel.name}</Badge>
          ) : (
            <span className="text-xs text-muted-foreground">Global</span>
          ),
      },
      ...(canManage
        ? ([
            {
              id: 'actions',
              cell: ({ row }) => (
                <div className="flex items-center justify-end gap-2">
                  <Switch
                    checked={row.original.isActive}
                    onCheckedChange={(v) => onToggleActive(row.original, v)}
                    aria-label="Active"
                  />
                  <RemoteProviderFormDialog
                    trigger={
                      <Button aria-label="Edit provider" variant="ghost" size="icon" className="size-8">
                        <PencilIcon className="size-4" />
                      </Button>
                    }
                    existing={row.original}
                    onUpdated={() => refetch()}
                  />
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="size-8" aria-label={`Actions for ${row.original.name}`}>
                        <MoreHorizontalIcon className="size-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem onClick={() => setConfirmingRotate(row.original)}>
                        <RefreshCwIcon className="size-4" />
                        Rotate secret
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => setConfirmingDelete(row.original)}
                      >
                        <Trash2Icon className="size-4" />
                        Delete
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              ),
            },
          ] as ColumnDef<ProviderRow>[])
        : []),
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [canManage],
  )

  return (
    <DetailPageLayout>
      <DetailPageHeader
        title="Remote providers"
        meta="HTTP endpoints that implement shipping rates, tax, address validation, promotion logic or flow actions - extend Skelet without writing a plugin."
        actions={
          canManage && (
            <RemoteProviderFormDialog
              trigger={
                <Button size="sm">
                  <PlusIcon className="size-4" />
                  New provider
                </Button>
              }
              onCreated={setNewSecret}
            />
          )
        }
      />
      <DetailPageBody>
        <DetailPageSection>
          {opError && (
            <Alert variant="destructive">
              <AlertTitle>Operation failed</AlertTitle>
              <AlertDescription>{opError}</AlertDescription>
            </Alert>
          )}

          <DataTable
            urlState={urlState}
            columns={columns}
            data={rows}
            loading={loading}
            error={error ? { message: error.message } : null}
            getRowId={(row) => row.id}
            emptyState="No remote providers yet. Register an endpoint to extend a pluggable role over HTTP."
            searchPlaceholder="Filter providers…"
          />
        </DetailPageSection>
      </DetailPageBody>

      <NewSecretDialog
        secret={newSecret?.secret ?? null}
        webhookName={newSecret?.name ?? null}
        open={!!newSecret}
        onClose={() => setNewSecret(null)}
      />

      <ConfirmDialog
        open={!!confirmingRotate}
        onOpenChange={(open) => !open && setConfirmingRotate(null)}
        title={`Rotate the secret for ${confirmingRotate?.name ?? 'this provider'}?`}
        description="The current secret stops working immediately. Calls signed with it fail until the provider has the new one."
        confirmLabel="Rotate secret"
        destructive
        onConfirm={onConfirmRotate}
      />
      <ConfirmDialog
        open={!!confirmingDelete}
        onOpenChange={(o) => !o && setConfirmingDelete(null)}
        title={`Delete ${confirmingDelete?.name}?`}
        description="Anything referencing this provider (promotion rules, flow nodes) will stop working. This cannot be undone."
        confirmLabel="Delete provider"
        busyLabel="Deleting…"
        destructive
        onConfirm={onConfirmDelete}
      />
    </DetailPageLayout>
  )
}
