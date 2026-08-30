import { useState } from 'react'
import { CheckIcon, CopyIcon } from 'lucide-react'
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

export function NewSecretDialog({
  secret,
  webhookName,
  open,
  onClose,
}: {
  secret: string | null
  webhookName: string | null
  open: boolean
  onClose: () => void
}) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    if (!secret) return
    try {
      await navigator.clipboard.writeText(secret)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // ignore
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!v) {
          setCopied(false)
          onClose()
        }
      }}
    >
      <DialogContent
        className="sm:max-w-lg"
        onInteractOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>
            Webhook Secret{webhookName ? ` for ${webhookName}` : ''}
          </DialogTitle>
          <DialogDescription>
            This secret will only be shown once. Copy it now.
          </DialogDescription>
        </DialogHeader>

        <Alert>
          <AlertTitle>Store it somewhere safe</AlertTitle>
          <AlertDescription>
            Use this secret to verify the authenticity of webhook payloads sent to your server.
          </AlertDescription>
        </Alert>

        <div className="flex items-center gap-2 rounded-md border bg-muted px-3 py-2 font-mono text-sm">
          <code className="flex-1 overflow-x-auto whitespace-nowrap">{secret ?? ''}</code>
          <Button variant="ghost" size="icon" className="size-8" onClick={copy} aria-label="Copy secret">
            {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}
          </Button>
        </div>

        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
