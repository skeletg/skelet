import { useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { CurrenciesPickerDocument } from '@/features/currencies/Currencies.gql'
import { CreateOptionTemplateDocument, OptionTemplatesDocument } from '../Catalog.gql'

// Name, description and currency - the options and rules are added on the template's own page after
export function NewTemplateForm({
  defaultCurrency = 'USD',
  submitLabel = 'Create template',
  onCreated,
  onCancel,
}: {
  defaultCurrency?: string
  submitLabel?: string
  onCreated: (id: string) => void | Promise<void>
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [currency, setCurrency] = useState(defaultCurrency || 'USD')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const currencies = useQuery(CurrenciesPickerDocument).data?.currencies ?? []
  const [create] = useMutation(CreateOptionTemplateDocument, { refetchQueries: [{ query: OptionTemplatesDocument }] })

  return (
    <form
      className="grid gap-4"
      onSubmit={async (e) => {
        e.preventDefault()
        setError(null)
        if (!name.trim()) {
          setError('Give it a name.')
          return
        }
        setBusy(true)
        try {
          const res = await create({ variables: { input: { name, description, currencyCode: currency } } })
          const id = res.data?.createOptionTemplate.id
          if (id) await onCreated(id)
        } catch (err) {
          setError(err instanceof Error ? err.message : 'Could not create the template.')
        } finally {
          setBusy(false)
        }
      }}
    >
      <div className="grid gap-1.5">
        <Label htmlFor="new-tpl-name">Name</Label>
        <Input id="new-tpl-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Gift wrapping" autoFocus />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="new-tpl-description">Description</Label>
        <Textarea id="new-tpl-description" rows={2} value={description} onChange={(e) => setDescription(e.target.value)} />
      </div>
      <div className="grid gap-1.5">
        <Label htmlFor="new-tpl-currency">Currency</Label>
        <Select value={currency} onValueChange={setCurrency}>
          <SelectTrigger id="new-tpl-currency">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(currencies.length ? currencies : [{ code: currency, name: '' }]).map((c) => (
              <SelectItem key={c.code} value={c.code}>
                <span className="font-mono text-xs">{c.code}</span>
                {c.name ? ` · ${c.name}` : ''}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-muted-foreground">Flat charges on its options are in this currency.</p>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={busy}>
          {busy ? 'Creating…' : submitLabel}
        </Button>
      </div>
    </form>
  )
}
