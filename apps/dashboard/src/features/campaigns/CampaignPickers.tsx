import { useId, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { CheckIcon } from 'lucide-react'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Choice } from './SendSheet'
import { localInput, nextMorning, timeZone, type When } from './sentence'

export type EmailOption = { id: string; name: string; subject: string }
export type AudienceOption = { id: string; name: string; count: number }

const link = 'text-foreground underline underline-offset-2 hover:no-underline'
const SEARCH_FROM = 6 // a shorter list is quicker to read than to search

// cmdk matches an item's value too, and the value here is an id
const byWords = (_value: string, search: string, keywords?: string[]) => ((keywords ?? []).join(' ').toLowerCase().includes(search.trim().toLowerCase()) ? 1 : 0)

export function EmailPicker({ emails, value, canCreate, onPick }: { emails: EmailOption[]; value: string | null; canCreate: boolean; onPick: (id: string) => void }) {
  return (
    <>
      {emails.length === 0 ? (
        <p className="px-3 py-4 text-sm text-muted-foreground">No emails in the library yet. A campaign sends one you've built there.</p>
      ) : (
        <Command filter={byWords}>
          {emails.length > SEARCH_FROM ? <CommandInput placeholder="Find an email" /> : null}
          <CommandList className="max-h-80">
            <CommandEmpty>No email by that name.</CommandEmpty>
            <CommandGroup>
              {emails.map((e) => (
                <CommandItem key={e.id} value={e.id} keywords={[e.name, e.subject]} data-checked={e.id === value} onSelect={() => onPick(e.id)}>
                  <span className="grid min-w-0 gap-0.5">
                    <span className="truncate font-medium">{e.name}</span>
                    <span className="truncate text-xs text-muted-foreground">{e.subject || '(no subject)'}</span>
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      )}
      {value || canCreate ? (
        <div className="flex gap-4 border-t px-3 py-2.5 text-xs">
          {value ? (
            <Link to={`/emails/${value}`} target="_blank" className={link}>
              Edit this email
            </Link>
          ) : null}
          {canCreate ? (
            <Link to="/emails/new" target="_blank" className={link}>
              Build a new email
            </Link>
          ) : null}
        </div>
      ) : null}
    </>
  )
}

function Options({ items, value, empty, onPick }: { items: AudienceOption[]; value: string | null; empty: ReactNode; onPick: (id: string) => void }) {
  if (items.length === 0) return <p className="px-1 pb-1 text-xs text-muted-foreground">{empty}</p>
  return (
    <ul className="grid max-h-64 gap-0.5 overflow-y-auto">
      {items.map((o) => (
        <li key={o.id}>
          <button
            type="button"
            aria-pressed={o.id === value}
            onClick={() => onPick(o.id)}
            className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none hover:bg-muted focus-visible:bg-muted"
          >
            <span className="min-w-0 flex-1 truncate">{o.name}</span>
            <span className="text-xs tabular-nums text-muted-foreground">{formatNumber(o.count)}</span>
            <CheckIcon className={cn('size-4', o.id === value ? 'opacity-100' : 'opacity-0')} />
          </button>
        </li>
      ))}
    </ul>
  )
}

export function AudiencePicker({
  kind,
  id,
  channel,
  groups,
  segments,
  onChange,
  close,
}: {
  kind: string
  id: string | null
  channel: string
  groups: AudienceOption[]
  segments: AudienceOption[]
  onChange: (kind: string, id: string | null) => void
  close: () => void
}) {
  const pick = (k: string) => (v: string) => {
    onChange(k, v)
    close()
  }
  return (
    <div className="grid gap-2.5 p-2.5">
      <Choice
        label="Audience"
        value={kind}
        onChange={(k) => {
          if (k !== kind) onChange(k, null)
          if (k === 'all') close()
        }}
        options={[
          ['all', 'Everyone'],
          ['group', 'A group'],
          ['segment', 'A segment'],
        ]}
      />
      {kind === 'all' ? <p className="px-1 pb-1 text-xs text-muted-foreground">Every active customer on {channel || 'this channel'} with an email address.</p> : null}
      {kind === 'group' ? <Options items={groups} value={id} onPick={pick('group')} empty="No customer groups yet." /> : null}
      {kind === 'segment' ? (
        <Options
          items={segments}
          value={id}
          onPick={pick('segment')}
          empty={
            <>
              No segments yet.{' '}
              <Link to="/segments" target="_blank" className={link}>
                Make one
              </Link>
            </>
          }
        />
      ) : null}
    </div>
  )
}

const preset = 'rounded-md border px-2 py-1 text-xs text-muted-foreground transition-colors hover:border-foreground/40 hover:text-foreground'

export function WhenPicker({ when, min, past, onChange }: { when: When; min: string; past: boolean; onChange: (w: When) => void }) {
  const id = useId()
  const zone = timeZone()
  return (
    <div className="grid gap-3 p-3">
      <Choice
        label="When to send"
        value={when.mode}
        onChange={(m) => onChange(m === 'now' ? { mode: 'now' } : { mode: 'later', at: when.mode === 'later' ? when.at : localInput(nextMorning()) })}
        options={[
          ['now', 'Now'],
          ['later', 'At a set time'],
        ]}
      />
      {when.mode === 'now' ? (
        <p className="text-xs text-muted-foreground">It starts going out within about a minute of you sending it.</p>
      ) : (
        <>
          <div className="grid gap-1.5">
            <Label htmlFor={id}>Date and time</Label>
            <Input id={id} type="datetime-local" value={when.at} min={min} onChange={(e) => onChange({ mode: 'later', at: e.target.value })} />
            {past ? <p className="text-xs text-destructive">That time has passed.</p> : <p className="text-xs text-muted-foreground">In your time zone{zone ? `, ${zone}` : ''}.</p>}
          </div>
          <div className="flex flex-wrap gap-1.5">
            <button type="button" className={preset} onClick={() => onChange({ mode: 'later', at: localInput(nextMorning()) })}>
              Tomorrow at 9:00
            </button>
            <button type="button" className={preset} onClick={() => onChange({ mode: 'later', at: localInput(nextMorning(new Date(), 1)) })}>
              Monday at 9:00
            </button>
          </div>
        </>
      )}
    </div>
  )
}

export function SubjectPicker({ value, own, onChange, close }: { value: string; own: string; onChange: (v: string) => void; close: () => void }) {
  const id = useId()
  return (
    <div className="grid gap-2 p-3">
      <Label htmlFor={id}>Subject B</Label>
      <Input id={id} value={value} placeholder="Another way to say it" onChange={(e) => onChange(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && close()} />
      <p className="text-xs text-muted-foreground">Subject A is the email's own{own ? `: "${own}"` : ''}. The email itself is the same for both.</p>
    </div>
  )
}

// The bounds are the server's, so the slider waits for them
export function SharePicker({ value, limits, onChange }: { value: number; limits?: { abTestPercentMin: number; abTestPercentMax: number }; onChange: (v: number) => void }) {
  const id = useId()
  return (
    <div className="grid gap-2 p-3">
      <Label htmlFor={id}>How much of the audience is in the test</Label>
      <div className="flex items-center gap-3">
        <input
          id={id}
          type="range"
          min={limits?.abTestPercentMin ?? value}
          max={limits?.abTestPercentMax ?? value}
          disabled={!limits}
          value={value}
          onChange={(e) => onChange(Number(e.target.value))}
          className="min-w-0 flex-1 accent-foreground"
        />
        <span className="w-10 text-right text-sm tabular-nums">{value}%</span>
      </div>
      <p className="text-xs text-muted-foreground">Half of them get each subject, and everyone else waits for the winner.</p>
    </div>
  )
}

const WINDOWS: Array<[string, string]> = [
  ['60', '1 hour'],
  ['240', '4 hours'],
  ['480', '8 hours'],
  ['1440', '1 day'],
]

export function WindowPicker({ minutes, onChange }: { minutes: number; onChange: (v: number) => void }) {
  const id = useId()
  return (
    <div className="grid gap-3 p-3">
      <Choice label="How long the test runs" value={String(minutes)} onChange={(v) => onChange(Number(v))} options={WINDOWS} />
      <div className="grid gap-1.5">
        <Label htmlFor={id}>Or in minutes</Label>
        <Input id={id} type="number" min={1} value={minutes} onChange={(e) => onChange(Math.max(1, Math.round(Number(e.target.value)) || 1))} />
      </div>
    </div>
  )
}

export function MeasurePicker({ value, onChange, close }: { value: string; onChange: (v: string) => void; close: () => void }) {
  return (
    <div className="grid gap-2 p-3">
      <Choice
        label="What decides the winner"
        value={value}
        onChange={(v) => {
          onChange(v)
          close()
        }}
        options={[
          ['open', 'Opens'],
          ['click', 'Clicks'],
        ]}
      />
      <p className="text-xs text-muted-foreground">Opens say more about a subject line. Clicks say more about who went on to read.</p>
    </div>
  )
}
