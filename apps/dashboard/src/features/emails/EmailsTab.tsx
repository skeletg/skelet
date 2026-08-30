import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { type ColumnDef } from '@/components/data-table/tanstack'
import { MailIcon, PlusIcon, SettingsIcon, Trash2Icon } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DataTable, DataTableColumnHeader } from '@/components/data-table'
import { DetailPageSection } from '@/components/layout/detail-page'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  CreateEmailGroupDocument,
  DeleteEmailGroupDocument,
  DeleteLibraryEmailDocument,
  EmailGroupsDocument,
  LibraryEmailsDocument,
  RenameEmailGroupDocument,
} from './Emails.gql'

function fmtDate(iso?: string | null): string {
  if (!iso) return '-'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '-' : d.toLocaleDateString()
}

type EmailRow = {
  id: string
  name: string
  subject: string
  groupId?: string | null
  updatedAt: string
}

// The "Emails" tab: the reusable-email library rendered with the shared DataTable (search + a Group faceted filter + sortable columns) -
// Group management is a lightweight dialog; the "New email" action lives in the EmailsHub page header
export function EmailsTab({ channelId, isChannel }: { channelId: string; isChannel: boolean }) {
  const { has } = usePermissions()
  const canManage = has('emails.manage')
  const [manageOpen, setManageOpen] = useState(false)

  const { data: groupsData } = useQuery(EmailGroupsDocument, { fetchPolicy: 'cache-and-network' })
  const groups = useMemo(() => groupsData?.emailGroups ?? [], [groupsData])

  const { data, loading, error } = useQuery(LibraryEmailsDocument, {
    variables: { channelId, groupSlug: null },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const emails = (data?.libraryEmails ?? []) as EmailRow[]

  const groupNameById = useMemo(
    () => Object.fromEntries(groups.map((g) => [g.id, g.name])),
    [groups],
  )

  const [deleteEmail] = useMutation(DeleteLibraryEmailDocument, {
    refetchQueries: [
      { query: LibraryEmailsDocument, variables: { channelId, groupSlug: null } },
      { query: EmailGroupsDocument },
    ],
  })

  const columns = useMemo<ColumnDef<EmailRow>[]>(
    () => [
      {
        accessorKey: 'name',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Name" />,
        cell: ({ row }) => (
          <Link to={`/emails/${row.original.id}`} className="font-medium hover:underline">
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'subject',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Subject" />,
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">{row.original.subject || '-'}</span>
        ),
      },
      {
        id: 'group',
        accessorFn: (e) => (e.groupId ? (groupNameById[e.groupId] ?? '') : ''),
        header: ({ column }) => <DataTableColumnHeader column={column} title="Group" />,
        cell: ({ getValue }) => {
          const name = getValue<string>()
          return name ? <Badge variant="outline">{name}</Badge> : <span className="text-sm text-muted-foreground">-</span>
        },
      },
      {
        accessorKey: 'updatedAt',
        header: ({ column }) => <DataTableColumnHeader column={column} title="Updated" />,
        cell: ({ row }) => <span className="text-sm text-muted-foreground">{fmtDate(row.original.updatedAt)}</span>,
      },
      ...(canManage
        ? [
            {
              id: 'actions',
              enableSorting: false,
              enableHiding: false,
              cell: ({ row }) => (
                <div className="text-right">
                  <Button
                    size="icon"
                    variant="ghost"
                    className="size-8 text-muted-foreground hover:text-destructive"
                    aria-label={`Delete ${row.original.name}`}
                    onClick={() => {
                      if (
                        window.confirm(
                          `Delete “${row.original.name}”? This can't be undone. Any flow that sends it will stop until you pick another email.`,
                        )
                      ) {
                        void deleteEmail({ variables: { id: row.original.id } })
                      }
                    }}
                  >
                    <Trash2Icon className="size-4" />
                  </Button>
                </div>
              ),
            } as ColumnDef<EmailRow>,
          ]
        : []),
    ],
    [canManage, groupNameById, deleteEmail],
  )

  const filters =
    groups.length > 0
      ? [{ column: 'group', title: 'Group', options: groups.map((g) => ({ label: g.name, value: g.name })) }]
      : undefined

  if (!isChannel) {
    return (
      <DetailPageSection>
        <Alert>
          <AlertTitle>Pick a channel</AlertTitle>
          <AlertDescription>Emails are per channel - select one in the header scope picker.</AlertDescription>
        </Alert>
      </DetailPageSection>
    )
  }

  return (
    <>
      <DetailPageSection>
        <DataTable
          columns={columns}
          data={emails}
          loading={loading}
          error={error ? { message: error.message } : null}
          getRowId={(e) => e.id}
          searchPlaceholder="Search name or subject…"
          filters={filters}
          toolbarRightSlot={
            canManage ? (
              <Button variant="outline" size="sm" onClick={() => setManageOpen(true)}>
                <SettingsIcon className="size-4" /> Manage groups
              </Button>
            ) : undefined
          }
          emptyState={
            <div className="text-center">
              <MailIcon className="mx-auto mb-2 size-6 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">No emails yet.</p>
              {canManage && (
                <Button asChild variant="outline" size="sm" className="mt-3">
                  <Link to="/emails/new">Build your first email</Link>
                </Button>
              )}
            </div>
          }
        />
      </DetailPageSection>

      {manageOpen && <ManageGroupsDialog open={manageOpen} onClose={() => setManageOpen(false)} />}
    </>
  )
}

function ManageGroupsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { data } = useQuery(EmailGroupsDocument, { fetchPolicy: 'cache-and-network' })
  const groups = data?.emailGroups ?? []
  const [newName, setNewName] = useState('')
  const [error, setError] = useState<string | null>(null)

  const refetch = [{ query: EmailGroupsDocument }]
  const [createGroup, { loading: creating }] = useMutation(CreateEmailGroupDocument, { refetchQueries: refetch })
  const [renameGroup] = useMutation(RenameEmailGroupDocument, { refetchQueries: refetch })
  const [deleteGroup] = useMutation(DeleteEmailGroupDocument, { refetchQueries: refetch })

  async function run(fn: () => Promise<unknown>) {
    setError(null)
    try {
      await fn()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Action failed.')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Email groups</DialogTitle>
          <DialogDescription>Organize the library. Any group can be renamed or deleted - deleting a group leaves its emails in the library, just untagged.</DialogDescription>
        </DialogHeader>

        {error && (
          <Alert variant="destructive">
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <div className="grid gap-2">
          {groups.map((g) => (
            <GroupRow
              key={g.id}
              name={g.name}
              isSystem={g.isSystem}
              emailCount={g.emailCount}
              onRename={(name) => run(() => renameGroup({ variables: { id: g.id, name } }))}
              onDelete={() => run(() => deleteGroup({ variables: { id: g.id } }))}
            />
          ))}
        </div>

        <form
          className="flex items-end gap-2 border-t pt-3"
          onSubmit={(e) => {
            e.preventDefault()
            const name = newName.trim()
            if (!name) return
            run(() => createGroup({ variables: { name } }).then(() => setNewName('')))
          }}
        >
          <div className="grid flex-1 gap-1.5">
            <Input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="New group name" />
          </div>
          <Button type="submit" size="sm" disabled={creating || !newName.trim()}>
            <PlusIcon className="size-4" /> Add
          </Button>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function GroupRow({
  name,
  isSystem,
  emailCount,
  onRename,
  onDelete,
}: {
  name: string
  isSystem: boolean
  emailCount: number
  onRename: (name: string) => void
  onDelete: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(name)

  if (editing) {
    return (
      <form
        className="flex items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          const n = draft.trim()
          if (n && n !== name) onRename(n)
          setEditing(false)
        }}
      >
        <Input value={draft} onChange={(e) => setDraft(e.target.value)} className="h-8" autoFocus />
        <Button type="submit" size="sm" variant="outline" className="h-8">Save</Button>
        <Button type="button" size="sm" variant="ghost" className="h-8" onClick={() => { setDraft(name); setEditing(false) }}>
          Cancel
        </Button>
      </form>
    )
  }

  return (
    <div className="flex items-center gap-2 rounded-md border px-3 py-2">
      <span className="flex-1 truncate text-sm font-medium">
        {name}
        {isSystem && <Badge variant="secondary" className="ml-2 text-[10px]">System</Badge>}
      </span>
      <span className="text-xs tabular-nums text-muted-foreground">{emailCount}</span>
      <Button size="sm" variant="ghost" className="h-7 px-2" onClick={() => { setDraft(name); setEditing(true) }}>
        Rename
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="h-7 px-2 text-muted-foreground hover:text-destructive"
        onClick={() => {
          if (window.confirm(`Delete the “${name}” group? Its ${emailCount} email(s) stay in the library, just untagged.`)) onDelete()
        }}
        aria-label={`Delete ${name}`}
      >
        <Trash2Icon className="size-3.5" />
      </Button>
    </div>
  )
}
