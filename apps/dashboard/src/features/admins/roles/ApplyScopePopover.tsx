import { useState } from 'react'
import { LayersIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'

const ANY = '__any__'

export type ApplyScopePopoverChannel = {
  id: string
  name: string
  defaultLocale: string
  locales: { code: string }[]
}

// ApplyScopePopover renders the "apply scope to all granted in this
// group" bulk affordance - The operator picks a channel + locale (each
// defaulting to "any") and the parent appends that scope row to every
// already-granted permission in the active group
//
// Disabled when the group has zero granted permissions - the affordance
// is meaningless until something exists to scope
export function ApplyScopePopover({
  channels,
  disabled,
  onApply,
}: {
  channels: ApplyScopePopoverChannel[]
  disabled?: boolean
  onApply: (channelId: string | null, locale: string | null) => void
}) {
  const [open, setOpen] = useState(false)
  const [channelId, setChannelId] = useState<string>('')
  const [locale, setLocale] = useState<string>('')

  // Reset selections each time the popover opens so the operator
  // doesn't accidentally re-apply yesterday's stale picks
  function onOpenChange(next: boolean) {
    if (next) {
      setChannelId('')
      setLocale('')
    }
    setOpen(next)
  }

  const channel = channels.find((c) => c.id === channelId)
  // When no channel is picked, the locale picker offers every locale across every channel (deduped) - When a channel IS picked, it narrows
  // to that channel's supported locales - matches how the per-row pickers in GroupPanel scope locales
  const localeOptions = channel
    ? Array.from(new Set([channel.defaultLocale, ...channel.locales.map((l) => l.code)]))
    : Array.from(
        new Set(
          channels.flatMap((c) => [c.defaultLocale, ...c.locales.map((l) => l.code)]),
        ),
      )

  function apply() {
    onApply(channelId || null, locale || null)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <LayersIcon className="size-4" />
          Apply scope to all granted
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-80" align="end">
        <div className="grid gap-3">
          <p className="text-xs text-muted-foreground">
            Appends one scope to every permission already granted in this group. Doesn't replace existing scopes.
          </p>
          <div className="grid gap-1.5">
            <Label className="text-xs">Channel</Label>
            <Select
              value={channelId === '' ? ANY : channelId}
              onValueChange={(v) => {
                setChannelId(v === ANY ? '' : v)
                setLocale('')
              }}
            >
              <SelectTrigger className="h-8">
                <SelectValue placeholder="Any channel" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>Any channel</SelectItem>
                {channels.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label className="text-xs">Locale</Label>
            <Select
              value={locale === '' ? ANY : locale}
              onValueChange={(v) => setLocale(v === ANY ? '' : v)}
            >
              <SelectTrigger className="h-8">
                <SelectValue placeholder="Any locale" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>Any locale</SelectItem>
                {localeOptions.map((l) => (
                  <SelectItem key={l} value={l}>
                    <span className="font-mono">{l}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button type="button" size="sm" onClick={apply}>
              Apply
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
