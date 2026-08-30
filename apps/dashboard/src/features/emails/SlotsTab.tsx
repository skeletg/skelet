import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DetailPageSection } from '@/components/layout/detail-page'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { usePermissions } from '@/shared/auth/usePermissions'
import {
  AssignEmailSlotDocument,
  LibraryEmailsDocument,
  TransactionalSlotsDocument,
} from './Emails.gql'

// The "Slots" tab: binds a library email to each transactional slot
// (order confirmation, shipment, …) - Without this the library is
// decorative - internal/emailtemplate resolve() prefers a slot-assigned
// email and otherwise falls through to the built-in, so an unassigned
// slot silently keeps sending the built-in copy
//
// assignEmailSlot is email-centric (emailId + slot, slot "" clears), so
// clearing a slot means calling it with the email currently sitting in it

const BUILT_IN = '__builtin__'

export function SlotsTab({ channelId, isChannel }: { channelId: string; isChannel: boolean }) {
  const { has } = usePermissions()
  const canManage = has('emails.manage')
  const [pending, setPending] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const slotsQuery = useQuery(TransactionalSlotsDocument, {
    variables: { channelId },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const emailsQuery = useQuery(LibraryEmailsDocument, {
    variables: { channelId },
    skip: !channelId,
    fetchPolicy: 'cache-and-network',
  })
  const [assignSlot] = useMutation(AssignEmailSlotDocument)

  const slots = useMemo(() => slotsQuery.data?.transactionalSlots ?? [], [slotsQuery.data])
  const emails = useMemo(() => emailsQuery.data?.libraryEmails ?? [], [emailsQuery.data])

  async function onChange(slot: string, currentEmailId: string | null, next: string) {
    setError(null)
    setPending(slot)
    try {
      if (next === BUILT_IN) {
        // Clearing is expressed as "unbind the email that holds this slot"
        if (currentEmailId) {
          await assignSlot({ variables: { emailId: currentEmailId, slot: '' } })
        }
      } else {
        await assignSlot({ variables: { emailId: next, slot } })
      }
      await slotsQuery.refetch()
      // An email carries its slot, so the library list is stale too - a
      // reassignment moves the slot off whichever email held it before
      await emailsQuery.refetch()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not update the slot.')
    } finally {
      setPending(null)
    }
  }

  if (!isChannel) {
    return (
      <DetailPageSection>
        <Alert>
          <AlertTitle>Pick a channel</AlertTitle>
          <AlertDescription>
            Slot assignments are per channel - select one in the header scope picker.
          </AlertDescription>
        </Alert>
      </DetailPageSection>
    )
  }

  const loadError = slotsQuery.error ?? emailsQuery.error

  return (
    <DetailPageSection
      title="Transactional slots"
      description="Pick which email the store sends for each event. Slots left on the built-in default keep sending the stock template."
    >
      {loadError && (
        <Alert variant="destructive" className="mb-4">
          <AlertTitle>Could not load slots</AlertTitle>
          <AlertDescription>{loadError.message}</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertTitle>Assignment failed</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <div className="divide-y rounded-md border">
        {slots.length === 0 && !slotsQuery.loading ? (
          <p className="p-5 text-sm text-muted-foreground">No transactional slots are defined.</p>
        ) : (
          slots.map((s) => (
            <div key={s.slot} className="flex flex-wrap items-center justify-between gap-3 p-4">
              <div className="min-w-0">
                <p className="text-sm font-medium">{s.label}</p>
                <code className="font-mono text-xs text-muted-foreground">{s.slot}</code>
              </div>
              <div className="flex items-center gap-3">
                {s.emailId ? (
                  <Button asChild variant="ghost" size="sm">
                    <Link to={`/emails/${s.emailId}`}>Open</Link>
                  </Button>
                ) : (
                  <Badge variant="secondary">Built-in</Badge>
                )}
                <Select
                  value={s.emailId ?? BUILT_IN}
                  disabled={!canManage || pending === s.slot}
                  onValueChange={(v) => void onChange(s.slot, s.emailId ?? null, v)}
                >
                  <SelectTrigger className="w-64">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={BUILT_IN}>Built-in default</SelectItem>
                    {emails.map((e) => (
                      <SelectItem key={e.id} value={e.id}>
                        {e.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          ))
        )}
      </div>
    </DetailPageSection>
  )
}
