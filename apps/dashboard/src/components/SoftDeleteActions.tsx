import { useState, type ReactNode } from 'react'
import { RotateCcwIcon, Trash2Icon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'

// Shared controls for the soft-deleted entities (flows, email campaigns,
// web forms) - Deleting these keeps the row - a sent campaign's recipients
// and stats, a flow's runs, a form's submissions all still point at
// something real - so the list needs a way to see and undo deletions

/** Header toggle that flips a list between live-only and including deleted */
export function ShowDeletedToggle({
  id,
  checked,
  onCheckedChange,
}: {
  id: string
  checked: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center gap-2">
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
      <Label htmlFor={id} className="text-sm font-normal text-muted-foreground">
        Show deleted
      </Label>
    </div>
  )
}

/** "Deleted" marker for a row whose deletedAt is set */
export function DeletedBadge({ deletedAt }: { deletedAt?: string | null }) {
  if (!deletedAt) return null
  return (
    <Badge variant="outline" className="text-destructive">
      Deleted
    </Badge>
  )
}

/**
 * Row action: Delete (with confirm) for a live row, Restore for a deleted
 * one. `onRun` should resolve only once the list refetch has landed, so the
 * row's state is settled by the time the dialog closes
 */
export function SoftDeleteRowAction({
  deletedAt,
  entityLabel,
  name,
  consequence,
  onDelete,
  onRestore,
  disabled = false,
}: {
  deletedAt?: string | null
  /** e.g. "flow" - used in the confirm copy */
  entityLabel: string
  /** the row's display name, quoted in the confirm copy */
  name: string
  /** one line on what survives the delete, shown in the dialog */
  consequence: ReactNode
  onDelete: () => Promise<void>
  onRestore: () => Promise<void>
  disabled?: boolean
}) {
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)

  async function restore() {
    setBusy(true)
    try {
      await onRestore()
    } catch {
      // shortcut: a failed restore has never been shown anywhere, give it an inline error when restores need one
    } finally {
      setBusy(false)
    }
  }

  if (deletedAt) {
    return (
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={disabled || busy}
        onClick={() => void restore()}
      >
        <RotateCcwIcon className="size-3.5" />
        Restore
      </Button>
    )
  }

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7 text-destructive"
        title={`Delete ${entityLabel}`}
        disabled={disabled || busy}
        onClick={() => setConfirming(true)}
      >
        <Trash2Icon className="size-3.5" />
      </Button>

      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={<>Delete “{name}”?</>}
        description={consequence}
        confirmLabel="Delete"
        busyLabel="Deleting…"
        destructive
        onConfirm={onDelete}
      />
    </>
  )
}
