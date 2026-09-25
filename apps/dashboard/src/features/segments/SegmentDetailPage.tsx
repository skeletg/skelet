import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
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
import { Textarea } from '@/components/ui/textarea'
import {
  DetailPageBody,
  DetailPageHeader,
  DetailPageLayout,
  DetailPageSection,
} from '@/components/layout/detail-page'
import { SaveConfirmDialog } from '@/shared/saving/SaveConfirmDialog'
import { SaveRegistryContext, useSaveRegistry, useSaveSection } from '@/shared/saving/saveRegistry'
import { changed, fieldDiff } from '@/shared/saving/diff'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { AssignCustomersToSegmentDocument, ClearSegmentMembershipDocument, RefreshSegmentMembershipDocument, RemoveCustomerFromSegmentDocument, SegmentDetailDocument, SetSegmentRulesDocument, UpdateSegmentDocument } from './Segments.gql'
import { useCustomerNames } from '@/shared/signals/useCustomerNames'
import { RulesBuilder } from './RulesBuilder'
import { summarizeRules, type Rules } from './rules'
import { PluginSections } from '@/plugins/detail-sections'

type Segment = {
  id: string
  slug: string
  name: string
  description: string
  isActive: boolean
  memberCount: number
  rules: unknown
  rulesProblem?: string | null
  updatedAt: string
  members: { id: string; joinedAt: string }[]
}

const parseRules = (raw: unknown): Rules | null => {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Rules
  return { logic: r.logic === 'or' ? 'or' : 'and', conditions: r.conditions ?? [] }
}

export function SegmentDetailPage() {
  const { id = '' } = useParams()
  const { data, loading, error, refetch } = useQuery(SegmentDetailDocument, {
    variables: { id },
    skip: !id,
  })
  const segment = data?.customerSegment as Segment | null | undefined


  const { contextValue, dirtySections, isDirty, saveAll } = useSaveRegistry()
  const [confirmingSave, setConfirmingSave] = useState(false)
  const [savingAll, setSavingAll] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  async function onConfirmSave() {
    setSaveError(null)
    setSavingAll(true)
    try {
      await saveAll()
      setConfirmingSave(false)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Save failed')
    } finally {
      setSavingAll(false)
    }
  }

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
        <DetailPageHeader
          backTo="/segments"
          title="Segment"
          meta={error?.message ?? 'Not found'}
        />
      </DetailPageLayout>
    )
  }

  const dynamic = parseRules(segment.rules) !== null

  return (
    <SaveRegistryContext.Provider value={contextValue}>
      <DetailPageLayout>
        <DetailPageHeader
          backTo="/segments"
          backLabel="Back to segments"
          title={segment.name}
          badges={
            <>
              <Badge variant={segment.isActive ? 'default' : 'secondary'}>
                {segment.isActive ? 'Active' : 'Inactive'}
              </Badge>
              <Badge variant="outline">{dynamic ? 'Dynamic' : 'Manual'}</Badge>
            </>
          }
          meta={`${segment.memberCount} members · updated ${new Date(segment.updatedAt).toLocaleString()}`}
          actions={
            <Button
              disabled={!isDirty || savingAll}
              onClick={() => {
                setSaveError(null)
                setConfirmingSave(true)
              }}
            >
              {savingAll ? 'Saving…' : isDirty ? 'Save changes' : 'No changes'}
            </Button>
          }
        />
        <DetailPageBody>
          <GeneralSection segment={segment} refetch={refetch} />
          <RulesSection segment={segment} refetch={refetch} />
          <MembersSection segment={segment} refetch={refetch} />
          <PluginSections target="segment" entity={segment} mode="edit" />
        </DetailPageBody>

        <SaveConfirmDialog
          open={confirmingSave}
          scopeLabel={`Segment · ${segment.name}`}
          sections={dirtySections}
          saving={savingAll}
          error={saveError}
          onCancel={() => {
            if (!savingAll) {
              setConfirmingSave(false)
              setSaveError(null)
            }
          }}
          onConfirm={onConfirmSave}
        />
      </DetailPageLayout>
    </SaveRegistryContext.Provider>
  )
}

function GeneralSection({ segment, refetch }: { segment: Segment; refetch: () => void }) {
  const [update] = useMutation(UpdateSegmentDocument)
  const [name, setName] = useState(segment.name)
  const [slug, setSlug] = useState(segment.slug)
  const [description, setDescription] = useState(segment.description)
  const [isActive, setIsActive] = useState(segment.isActive)

  // Re-seed after saves so the section doesn't go phantom-dirty
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setName(segment.name)
    setSlug(segment.slug)
    setDescription(segment.description)
    setIsActive(segment.isActive)
  }, [segment])

  const draft = { name, slug, description, isActive }
  const server = {
    name: segment.name,
    slug: segment.slug,
    description: segment.description,
    isActive: segment.isActive,
  }

  useSaveSection({
    id: 'segment-general',
    title: 'General',
    diff: fieldDiff(server, draft, [
      { label: 'Name', value: (o) => o.name },
      { label: 'Slug', value: (o) => o.slug },
      { label: 'Description', value: (o) => o.description },
      { label: 'Active', value: (o) => o.isActive },
    ]),
    save: async () => {
      await update({
        variables: {
          input: { id: segment.id, name, slug, description, isActive },
        },
      })
      refetch()
    },
  })

  return (
    <DetailPageSection title="General">
      <div className="grid max-w-2xl gap-4">
        <div className="grid grid-cols-2 gap-4">
          <div className="grid gap-2">
            <Label htmlFor="seg-name">Name</Label>
            <Input id="seg-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="seg-slug">Slug</Label>
            <Input id="seg-slug" value={slug} onChange={(e) => setSlug(e.target.value)} />
            <p className="text-xs text-muted-foreground">
              Referenced by promotions, campaigns and flows - renaming can break them
            </p>
          </div>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="seg-desc">Description</Label>
          <Textarea
            id="seg-desc"
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-3">
          <Switch id="seg-active" checked={isActive} onCheckedChange={setIsActive} />
          <Label htmlFor="seg-active" className="font-normal">
            Active - inactive segments are skipped by the rules refresh and hidden from pickers
          </Label>
        </div>
      </div>
    </DetailPageSection>
  )
}

function RulesSection({ segment, refetch }: { segment: Segment; refetch: () => void }) {
  const [setRules] = useMutation(SetSegmentRulesDocument)
  const server = useMemo(() => parseRules(segment.rules), [segment.rules])
  const [draft, setDraft] = useState<Rules | null>(server)

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDraft(parseRules(segment.rules))
  }, [segment.rules])

  const dirty = JSON.stringify(server) !== JSON.stringify(draft)
  const entry = dirty ? changed('Rules', summarizeRules(server), summarizeRules(draft)) : null

  useSaveSection({
    id: 'segment-rules',
    title: 'Rules',
    diff: entry ? [entry] : [],
    save: async () => {
      await setRules({
        variables: { segmentId: segment.id, rules: draft && draft.conditions.length > 0 ? draft : null },
      })
      refetch()
    },
  })

  return (
    <DetailPageSection
      title="Rules"
      description="Dynamic segments recompute membership from these conditions - on the 6-hour refresh job or a manual refresh. Without rules, membership is manual (assigned below or by flows)."
    >
      <div className="grid gap-3">
        <div className="flex items-center gap-3">
          <Switch
            id="seg-dynamic"
            checked={draft !== null}
            onCheckedChange={(on) =>
              setDraft(on ? { logic: 'and', conditions: [] } : null)
            }
          />
          <Label htmlFor="seg-dynamic" className="font-normal">
            Dynamic membership (rule-based)
          </Label>
        </div>
        {segment.rulesProblem && (
          <p className="rounded-md border border-destructive bg-destructive/10 px-3 py-2 text-sm text-destructive">
            These rules can't run, so membership isn't refreshing: {segment.rulesProblem}
          </p>
        )}
        {draft !== null && <RulesBuilder value={draft} onChange={setDraft} />}
        {draft !== null && draft.conditions.length === 0 && (
          <p className="text-xs text-muted-foreground">
            No conditions yet - saving like this turns the segment back to manual
          </p>
        )}
      </div>
    </DetailPageSection>
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
  const [filter, setFilter] = useState('')
  const search = useDebounced(filter.trim(), 250)
  const { data, loading } = useQuery(CustomersPagedDocument, { variables: { search, limit: 50, offset: 0 }, skip: !open })
  const [assign, { loading: assigning }] = useMutation(AssignCustomersToSegmentDocument)
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

  const customers = data?.customersPaged.items ?? []

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
