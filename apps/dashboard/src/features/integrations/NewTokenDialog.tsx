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
import { ADMIN_GRAPHQL_ENDPOINT } from '@/lib/api-endpoint'

// Surfaced once after create or regenerate - Backend never returns the raw token again, so this dialog is the single
// chance the user has to copy it Dismiss-on-backdrop is disabled to make accidental loss less likely
// Reads the same var the Apollo client does - a hardcoded path here would hand integrators a dead
// URL the moment the admin endpoint moves behind a slug
const ADMIN_ENDPOINT = ADMIN_GRAPHQL_ENDPOINT

export function NewTokenDialog({
  token,
  integrationName,
  open,
  onClose,
}: {
  token: string | null
  integrationName: string | null
  open: boolean
  onClose: () => void
}) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    if (!token) return
    try {
      await navigator.clipboard.writeText(token)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // ignore - clipboard APIs can be blocked by permissions
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
            New token{integrationName ? ` for ${integrationName}` : ''}
          </DialogTitle>
          <DialogDescription>
            Copy this token now. It will not be shown again.
          </DialogDescription>
        </DialogHeader>

        <Alert>
          <AlertTitle>Store it somewhere safe</AlertTitle>
          <AlertDescription>
            Anyone with this token can call the API with the integration's permissions.
          </AlertDescription>
        </Alert>

        <div className="flex items-center gap-2 rounded-md border bg-muted px-3 py-2 font-mono text-sm">
          <code className="flex-1 overflow-x-auto whitespace-nowrap">{token ?? ''}</code>
          <Button variant="ghost" size="icon" className="size-8" onClick={copy} aria-label="Copy token">
            {copied ? <CheckIcon className="size-4" /> : <CopyIcon className="size-4" />}
          </Button>
        </div>

        <p className="text-sm text-muted-foreground">
          Send it as the <code className="font-mono">X-Api-Token</code> header on requests to{' '}
          <code className="font-mono">{ADMIN_ENDPOINT}</code>.
        </p>

        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
