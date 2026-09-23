import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
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
import { Switch } from '@/components/ui/switch'
import { UpsertShippingBoxDocument } from './Shipping.gql'

export type ShippingBoxRow = {
  id: string
  code: string
  name: string
  innerLengthMm: number
  innerWidthMm: number
  innerHeightMm: number
  maxWeightGrams: number
  boxWeightGrams: number
  isActive: boolean
  sortOrder: number
}

export function ShippingBoxFormDialog({
  open,
  onOpenChange,
  channelId,
  existing,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelId: string
  existing?: ShippingBoxRow | null
  onSaved: () => void
}) {
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [length, setLength] = useState('')
  const [width, setWidth] = useState('')
  const [height, setHeight] = useState('')
  const [maxWeight, setMaxWeight] = useState('')
  const [boxWeight, setBoxWeight] = useState('')
  const [sortOrder, setSortOrder] = useState('0')
  const [isActive, setIsActive] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [upsert, { loading }] = useMutation(UpsertShippingBoxDocument)

  // Hydrate on open - render-time state adjustment, same as the other dialogs
  const [prevKey, setPrevKey] = useState<{ open: boolean; existing?: ShippingBoxRow | null }>({
    open,
    existing,
  })
  if (prevKey.open !== open || prevKey.existing !== existing) {
    setPrevKey({ open, existing })
    if (open) {
      setCode(existing?.code ?? '')
      setName(existing?.name ?? '')
      setLength(existing ? String(existing.innerLengthMm) : '')
      setWidth(existing ? String(existing.innerWidthMm) : '')
      setHeight(existing ? String(existing.innerHeightMm) : '')
      setMaxWeight(existing && existing.maxWeightGrams > 0 ? String(existing.maxWeightGrams) : '')
      setBoxWeight(existing && existing.boxWeightGrams > 0 ? String(existing.boxWeightGrams) : '')
      setSortOrder(existing ? String(existing.sortOrder) : '0')
      setIsActive(existing?.isActive ?? true)
      setError(null)
    }
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await upsert({
        variables: {
          input: {
            channelId,
            code: code.trim(),
            name: name.trim() || undefined,
            innerLengthMm: Number.parseInt(length, 10),
            innerWidthMm: Number.parseInt(width, 10),
            innerHeightMm: Number.parseInt(height, 10),
            maxWeightGrams: maxWeight.trim() ? Number.parseInt(maxWeight, 10) : 0,
            boxWeightGrams: boxWeight.trim() ? Number.parseInt(boxWeight, 10) : 0,
            sortOrder: Number.parseInt(sortOrder, 10) || 0,
            isActive,
          },
        },
      })
      onOpenChange(false)
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  const dim = (id: string, label: string, value: string, set: (v: string) => void) => (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type="number"
        min="1"
        required
        value={value}
        onChange={(e) => set(e.target.value)}
      />
    </div>
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={onSubmit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{existing ? 'Edit box' : 'New box'}</DialogTitle>
            <DialogDescription>
              {existing
                ? 'Update this carton - quotes and labels pick it up immediately.'
                : 'A real carton from your warehouse. Once at least one active box exists, rate quotes and labels pack line items into cartons.'}
            </DialogDescription>
          </DialogHeader>

          <div className="grid grid-cols-2 gap-4 items-start">
            <div className="grid gap-2">
              <Label htmlFor="box-code">Code</Label>
              <Input
                id="box-code"
                placeholder="e.g. small-A"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                required
                disabled={!!existing}
              />
              {existing ? (
                <p className="text-xs text-muted-foreground">Code is the upsert key - fixed after creation</p>
              ) : null}
            </div>
            <div className="grid gap-2">
              <Label htmlFor="box-name">Name</Label>
              <Input
                id="box-name"
                placeholder="Small box A"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
          </div>

          <div className="grid gap-2">
            <Label>Inner dimensions (mm)</Label>
            <div className="grid grid-cols-3 gap-2">
              {dim('box-length', 'Length', length, setLength)}
              {dim('box-width', 'Width', width, setWidth)}
              {dim('box-height', 'Height', height, setHeight)}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="grid gap-2">
              <Label htmlFor="box-max-weight">Max load (g)</Label>
              <Input
                id="box-max-weight"
                type="number"
                min="0"
                placeholder="0 = unlimited"
                value={maxWeight}
                onChange={(e) => setMaxWeight(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="box-weight">Empty box weight (g)</Label>
              <Input
                id="box-weight"
                type="number"
                min="0"
                placeholder="added to each parcel"
                value={boxWeight}
                onChange={(e) => setBoxWeight(e.target.value)}
              />
            </div>
          </div>

          <div className="flex items-end justify-between gap-4">
            <div className="grid gap-2">
              <Label htmlFor="box-sort">Sort order</Label>
              <Input
                id="box-sort"
                type="number"
                className="w-24"
                value={sortOrder}
                onChange={(e) => setSortOrder(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-3 pb-2">
              <Switch id="box-active" checked={isActive} onCheckedChange={setIsActive} />
              <Label htmlFor="box-active" className="font-normal">
                Active
              </Label>
            </div>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={loading}>
              {loading ? 'Saving…' : existing ? 'Save changes' : 'Create box'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
