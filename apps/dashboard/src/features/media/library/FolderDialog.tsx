import { useState, type FormEvent } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function FolderDialog({
  open,
  title,
  description,
  initial = '',
  submitLabel,
  busy,
  onOpenChange,
  onSubmit,
}: {
  open: boolean
  title: string
  description: string
  initial?: string
  submitLabel: string
  busy: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (name: string) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">{open ? <NameForm title={title} description={description} initial={initial} submitLabel={submitLabel} busy={busy} onCancel={() => onOpenChange(false)} onSubmit={onSubmit} /> : null}</DialogContent>
    </Dialog>
  )
}

function NameForm({ title, description, initial, submitLabel, busy, onCancel, onSubmit }: { title: string; description: string; initial: string; submitLabel: string; busy: boolean; onCancel: () => void; onSubmit: (name: string) => void }) {
  const [name, setName] = useState(initial)
  const usable = name.trim() !== '' && name.trim() !== initial
  function submit(e: FormEvent) {
    e.preventDefault()
    if (usable) onSubmit(name.trim())
  }
  return (
    <form className="grid gap-4" onSubmit={submit}>
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <div className="grid gap-1.5">
        <Label htmlFor="media-folder-name">Folder name</Label>
        <Input id="media-folder-name" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="submit" disabled={!usable || busy}>
          {submitLabel}
        </Button>
      </DialogFooter>
    </form>
  )
}
