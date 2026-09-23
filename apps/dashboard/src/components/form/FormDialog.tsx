import { useState, type ReactNode } from 'react'
import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog'

// The shell around a useDialogForm body - open state (controlled by the caller, or kept here when
// only a trigger is passed) and the content box. The body renders inside DialogContent, which Radix
// unmounts on close, so every open starts the form fresh
export function FormDialog({
  open,
  onOpenChange,
  trigger,
  className = 'sm:max-w-lg',
  children,
}: {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  trigger?: ReactNode
  className?: string
  /** The form body - `close` shuts the dialog, for Cancel and after a save */
  children: (close: () => void) => ReactNode
}) {
  const [internalOpen, setInternalOpen] = useState(false)
  const isOpen = open ?? internalOpen
  const setOpen = onOpenChange ?? setInternalOpen
  return (
    <Dialog open={isOpen} onOpenChange={setOpen}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent className={className}>{children(() => setOpen(false))}</DialogContent>
    </Dialog>
  )
}
