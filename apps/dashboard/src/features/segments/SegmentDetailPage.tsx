import { useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { useStore } from '@tanstack/react-form'
import { RefreshCwIcon, Trash2Icon, UserPlusIcon } from 'lucide-react'
import { toast } from 'sonner'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { RecordFormPage } from '@/shared/saving/RecordFormPage'
import { useRecordForm } from '@/shared/saving/useRecordForm'
import { CustomersDocument } from '@/features/customers/Customers.gql'
import { AssignCustomersToSegmentDocument, ClearSegmentMembershipDocument, RefreshSegmentMembershipDocument, RemoveCustomerFromSegmentDocument, SegmentDetailDocument, SetSegmentRulesDocument, UpdateSegmentDocument } from './Segments.gql'
import { useCustomerNames } from '@/shared/signals/useCustomerNames'
import { RulesBuilder } from './RulesBuilder'
import type { Rules } from './rules'
import { PluginSections } from '@/plugins/detail-sections'
import {
  SEGMENT_LABELS,
  generalChanged,
  parseRules,
  rulesChanged,
  segmentDiff,
  segmentRules,
  segmentSchema,
  toSegmentInput,
  toSegmentRules,
  toSegmentValues,
} from './schemas/segment'

type Segment = {
  id: string
  slug: string
  name: string
  description: string
  isActive: boolean
  memberCount: number
  rules: unknown
  updatedAt: string
  members: { id: string; joinedAt: string }[]
}

export function SegmentDetailPage() {
  const { id = '' } = useParams()
  const { data, loading, error, refetch } = useQuery(SegmentDetailDocument, {
    variables: { id },
    skip: !id,
  })
  const segment = data?.customerSegment as Segment | null | undefined

  if (loading && !segment) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/segments" title="Segment" meta="Loading…" />
      </DetailPageLayout>
    )
  }
  if (error || !segment) {
    return (
      <DetailPageLayout>
        <DetailPageHeader backTo="/segments" title="Segment" meta={error?.message ?? 'Not found'} />
      </DetailPageLayout>
    )
  }
  return <SegmentForm segment={segment} refetch={() => void refetch()} />
}

// General settings and rules are one form - the header Save reviews both; each goes through its
// own mutation, and only when it changed. Membership acts on its own below the form
function SegmentForm({ segment, refetch }: { segment: Segment; refetch: () => void }) {
  const [update] = useMutation(UpdateSegmentDocument)
  const [setRules] = useMutation(SetSegmentRulesDocument)

  const baseline = useMemo(() => toSegmentValues(segment), [segment])
  const record = useRecordForm({
    id: 'segment',
    title: 'Segment',
    label: `Segment · ${segment.name}`,
    isEdit: true,
    baseline,
    schema: segmentSchema,
    diff: segmentDiff,
    persist: { routeKey: `segment:${segment.id}` },
    submit: async (v) => {
      if (generalChanged(baseline, v)) await update({ variables: { input: toSegmentInput(v, segment.id) } })
      if (rulesChanged(baseline, v)) await setRules({ variables: { segmentId: segment.id, rules: toSegmentRules(v) } })
      refetch()
    },
  })
  const { form } = record
  const rules = useStore(form.store, (s) => segmentRules(s.values))
  const dynamic = parseRules(segment.rules) !== null

  return (
    <RecordFormPage
      record={record}
      backTo="/segments"
      backLabel="Back to segments"
      title={segment.name}
      badges={
        <>
          {segment.isActive ? (
            <Badge variant="secondary">Active</Badge>
          ) : (
            <Badge variant="outline" className="text-muted-foreground">Inactive</Badge>
          )}
          <Badge variant="outline">{dynamic ? 'Dynamic' : 'Manual'}</Badge>
        </>
      }
      meta={`${segment.memberCount} members · updated ${new Date(segment.updatedAt).toLocaleString()}`}
      createLabel="Save changes"
      labels={SEGMENT_LABELS}
      after={
        <>
          <MembersSection segment={segment} refetch={refetch} />
          <PluginSections target="segment" entity={segment} mode="edit" />
        </>
      }
    >
      <DetailPageSection title="General">
        <div className="grid max-w-2xl gap-4">
          <div className="grid grid-cols-2 items-start gap-4">
            <form.AppField name="name">{(f) => <f.TextField id="seg-name" label="Name" />}</form.AppField>
            <form.AppField name="slug">
              {(f) => (
                <f.TextField id="seg-slug" label="Slug" hint="Referenced by promotions, campaigns and flows - renaming can break them" />
              )}
            </form.AppField>
          </div>
          <form.AppField name="description">{(f) => <f.TextareaField id="seg-desc" label="Description" rows={2} />}</form.AppField>
          <form.Field name="isActive">
            {(f) => (
              <div className="flex items-center gap-3">
                <Switch id="seg-active" checked={f.state.value} onCheckedChange={f.handleChange} />
                <Label htmlFor="seg-active" className="font-normal">
                  Active - inactive segments are skipped by the rules refresh and hidden from pickers
                </Label>
              </div>
            )}
          </form.Field>
        </div>
      </DetailPageSection>

      <DetailPageSection
        title="Rules"
        description="Dynamic segments recompute membership from these conditions - on the 6-hour refresh job or a manual refresh. Without rules, membership is manual (assigned below or by flows)."
      >
        <div className="grid gap-3">
          <div className="flex items-center gap-3">
            <Switch
              id="seg-dynamic"
              checked={rules !== null}
              onCheckedChange={(on) => form.setFieldValue('rules', on ? ({ logic: 'and', conditions: [] } satisfies Rules) : null)}
            />
            <Label htmlFor="seg-dynamic" className="font-normal">
              Dynamic membership (rule-based)
            </Label>
          </div>
          {rules !== null && <RulesBuilder value={rules} onChange={(next) => form.setFieldValue('rules', next)} />}
          {rules !== null && rules.conditions.length === 0 && (
            <p className="text-xs text-muted-foreground">No conditions yet - saving like this turns the segment back to manual</p>
          )}
        </div>
      </DetailPageSection>
    </RecordFormPage>
  )
}

function MembersSection({ segment, refetch }: { segment: Segment; refetch: () => void }) {
  const memberIDs = useMemo(() => segment.members.map((m) => m.id), [segment])
  const nameById = useCustomerNames(memberIDs)
  const dynamic = parseRules(segment.rules) !== null
  const [refresh, { loading: refreshing }] = useMutation(RefreshSegmentMembershipDocument)
  const [removeMember] = useMutation(RemoveCustomerFromSegmentDocument)
  const [clear, { loading: clearing }] = useMutation(ClearSegmentMembershipDocument)
  const [assignOpen, setAssignOpen] = useState(false)
  const [confirmingClear, setConfirmingClear] = useState(false)

  async function onRefresh() {
    try {
      const { data } = await refresh({ variables: { segmentId: segment.id } })
      toast.success(`Refreshed - ${data?.refreshSegment ?? 0} members match the rules`)
      refetch()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Refresh failed')
    }
  }

  async function onRemove(customerId: string, email: string) {
    try {
      await removeMember({ variables: { segmentId: segment.id, customerId } })
      toast.success(`Removed ${email}`)
      refetch()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Remove failed')
    }
  }

  async function onClear() {
    try {
      await clear({ variables: { segmentId: segment.id } })
      setConfirmingClear(false)
      toast.success('Membership cleared')
      refetch()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Clear failed')
    }
  }

  return (
    <DetailPageSection
      title="Members"
      description={
        dynamic
          ? 'Rule-computed membership. A manual refresh recomputes right now instead of waiting for the 6-hour job; manual assignments get replaced on the next refresh.'
          : 'Manually assigned membership.'
      }
      trailing={
        <div className="flex items-center gap-2">
          {dynamic && (
            <Button variant="outline" size="sm" disabled={refreshing} onClick={() => void onRefresh()}>
              <RefreshCwIcon className="size-4" />
              {refreshing ? 'Refreshing…' : 'Refresh now'}
            </Button>
          )}
          <Button variant="outline" size="sm" onClick={() => setAssignOpen(true)}>
            <UserPlusIcon className="size-4" />
            Assign customers
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={segment.memberCount === 0 || clearing}
            onClick={() => setConfirmingClear(true)}
          >
            <Trash2Icon className="size-4" />
            Clear all
          </Button>
        </div>
      }
    >
      {segment.members.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No members yet{dynamic ? ' - hit Refresh now to compute from the rules' : ''}
        </p>
      ) : (
        <div className="overflow-hidden rounded-md border">
          <ul className="divide-y">
            {segment.members.map((m) => (
              <li key={m.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                <span className="min-w-0 flex-1 truncate">
                  {nameById.get(m.id)?.name || nameById.get(m.id)?.email || m.id.slice(0, 8)}
                  {nameById.get(m.id)?.name && (
                    <span className="ml-2 text-xs text-muted-foreground">{nameById.get(m.id)?.email}</span>
                  )}
                </span>
                <span className="text-xs text-muted-foreground">
                  joined {new Date(m.joinedAt).toLocaleDateString()}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() => void onRemove(m.id, nameById.get(m.id)?.email ?? m.id)}
                >
                  Remove
                </Button>
              </li>
            ))}
          </ul>
          {segment.memberCount > segment.members.length && (
            <p className="border-t px-3 py-2 text-xs text-muted-foreground">
              Showing the newest {segment.members.length} of {segment.memberCount} members
            </p>
          )}
        </div>
      )}

      <AssignCustomersDialog
        open={assignOpen}
        onOpenChange={setAssignOpen}
        segmentId={segment.id}
        onAssigned={refetch}
      />

      <AlertDialog open={confirmingClear} onOpenChange={setConfirmingClear}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Clear all {segment.memberCount} members?</AlertDialogTitle>
            <AlertDialogDescription>
              Empties the segment immediately. {dynamic ? 'The next rules refresh will repopulate it.' : 'Manual segments stay empty until you assign customers again.'}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={clearing}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={clearing}
              onClick={(e) => {
                e.preventDefault()
                void onClear()
              }}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {clearing ? 'Clearing…' : 'Clear membership'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </DetailPageSection>
  )
}

function AssignCustomersDialog({
  open,
  onOpenChange,
  segmentId,
  onAssigned,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  segmentId: string
  onAssigned: () => void
}) {
  const { data, loading } = useQuery(CustomersDocument, { skip: !open })
  const [assign, { loading: assigning }] = useMutation(AssignCustomersToSegmentDocument)
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [error, setError] = useState<string | null>(null)

  // Reset per open
  const [prevOpen, setPrevOpen] = useState(open)
  if (prevOpen !== open) {
    setPrevOpen(open)
    if (open) {
      setFilter('')
      setSelected(new Set())
      setError(null)
    }
  }

  const customers = useMemo(() => {
    const all = data?.customers ?? []
    const q = filter.trim().toLowerCase()
    if (!q) return all.slice(0, 50)
    return all
      .filter(
        (c) =>
          c.email.toLowerCase().includes(q) ||
          `${c.firstName} ${c.lastName}`.toLowerCase().includes(q),
      )
      .slice(0, 50)
  }, [data, filter])

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  async function onAssign() {
    setError(null)
    try {
      await assign({ variables: { segmentId, customerIds: [...selected] } })
      onOpenChange(false)
      onAssigned()
      toast.success(`Assigned ${selected.size} customer${selected.size === 1 ? '' : 's'}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Assign failed')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[80vh] flex-col sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Assign customers</DialogTitle>
          <DialogDescription>
            Adds the selected customers to this segment. On a dynamic segment they last until the
            next rules refresh replaces membership.
          </DialogDescription>
        </DialogHeader>
        <Input
          placeholder="Search email or name…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <div className="min-h-0 flex-1 overflow-y-auto rounded-md border">
          {loading ? (
            <p className="p-3 text-sm text-muted-foreground">Loading…</p>
          ) : customers.length === 0 ? (
            <p className="p-3 text-sm text-muted-foreground">No matches</p>
          ) : (
            <ul className="divide-y">
              {customers.map((c) => (
                <li key={c.id}>
                  <label className="flex cursor-pointer items-center gap-3 px-3 py-2 text-sm hover:bg-muted/40">
                    <Checkbox checked={selected.has(c.id)} onCheckedChange={() => toggle(c.id)} />
                    <span className="min-w-0 flex-1 truncate">
                      {`${c.firstName} ${c.lastName}`.trim() || c.email}
                      <span className="ml-2 text-xs text-muted-foreground">{c.email}</span>
                    </span>
                  </label>
                </li>
              ))}
            </ul>
          )}
        </div>
        {error && (
          <Alert variant="destructive">
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button disabled={selected.size === 0 || assigning} onClick={() => void onAssign()}>
            {assigning ? 'Assigning…' : `Assign ${selected.size || ''}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
