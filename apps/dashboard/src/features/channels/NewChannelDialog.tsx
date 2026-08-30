import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { LocalesPickerDocument } from '@/features/locales/Locales.gql'
import { ChannelsDocument, CreateChannelDocument } from './Channels.gql'

// NewChannelDialog is the minimal create flow for the Channels list
// The four required fields are slug, name, currency, and default
// locale - anything else (identity tweaks, branding, locales picker,
// inventory) is filled in via the Settings page after the redirect
// `onCreated` receives the new channel id so the caller can navigate
export function NewChannelDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (channelId: string) => void
}) {
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [currencyCode, setCurrencyCode] = useState('')
  const [defaultLocale, setDefaultLocale] = useState('')
  const [error, setError] = useState<string | null>(null)

  const currenciesQuery = useQuery(CurrenciesPickerDocument, { skip: !open })
  const localesQuery = useQuery(LocalesPickerDocument, { skip: !open })

  const [createChannel, { loading }] = useMutation(CreateChannelDocument, {
    refetchQueries: [{ query: ChannelsDocument }],
  })

  function reset() {
    setSlug('')
    setName('')
    setCurrencyCode('')
    setDefaultLocale('')
    setError(null)
  }

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    if (!slug.trim() || !name.trim() || !currencyCode || !defaultLocale) {
      setError('All four fields are required.')
      return
    }
    try {
      const res = await createChannel({
        variables: {
          input: {
            slug: slug.trim(),
            name: name.trim(),
            currencyCode: currencyCode.toUpperCase(),
            defaultLocale,
          },
        },
      })
      const newId = res.data?.createChannel.id
      if (!newId) {
        setError('Create succeeded but no id returned. Reload to find the new channel.')
        return
      }
      reset()
      onOpenChange(false)
      onCreated(newId)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed.')
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) reset()
        onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <form onSubmit={onSubmit} className="contents">
          <DialogHeader>
            <DialogTitle>New channel</DialogTitle>
            <DialogDescription>
              Start with the identity. Everything else - branding, contact,
              legal, locales, inventory - is configured under Settings after
              the channel exists.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="channel-new-slug">Slug</Label>
              <Input
                id="channel-new-slug"
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                placeholder="us"
                pattern="[a-z0-9_-]+"
                title="Lowercase letters, digits, dashes, underscores only"
                required
              />
              <p className="text-xs text-muted-foreground">
                Stable identifier for this channel, used by tooling and the
                X-Store-Slug header. Storefronts resolve by id or host, not by
                slug. Cannot be changed after create.
              </p>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="channel-new-name">Name</Label>
              <Input
                id="channel-new-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="United States"
                required
              />
            </div>

            <div className="grid gap-2">
              <Label htmlFor="channel-new-currency">Currency</Label>
              <Select value={currencyCode} onValueChange={setCurrencyCode}>
                <SelectTrigger id="channel-new-currency">
                  <SelectValue placeholder="Pick a currency" />
                </SelectTrigger>
                <SelectContent>
                  {(currenciesQuery.data?.currencies ?? []).map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      <span className="font-mono text-xs">{c.code}</span> · {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-2">
              <Label htmlFor="channel-new-locale">Default locale</Label>
              <Select value={defaultLocale} onValueChange={setDefaultLocale}>
                <SelectTrigger id="channel-new-locale">
                  <SelectValue placeholder="Pick a default locale" />
                </SelectTrigger>
                <SelectContent>
                  {(localesQuery.data?.locales ?? []).map((l) => (
                    <SelectItem key={l.code} value={l.code}>
                      {l.flagEmoji ? `${l.flagEmoji} ` : ''}
                      <span className="font-mono text-xs">{l.code}</span> · {l.nativeName}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Supported-locales list can be expanded later in Settings.
              </p>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Couldn't create</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading}>
              {loading ? 'Creating…' : 'Create channel'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
