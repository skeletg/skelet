import { useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { CopyIcon } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { AdminCartOfferEmailPreviewDocument } from '../Carts.gql'

export async function copyText(text: string, done = 'Link copied') {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(done)
  } catch {
    toast.error('Couldn’t copy - select the link and copy it by hand')
  }
}

// The offer email as Send would mail it right now - core's own template, rendered by the backend from the saved
// offer - and its link to copy on its own
export function OfferEmailPreview({ open, onOpenChange, cartId }: { open: boolean; onOpenChange: (open: boolean) => void; cartId: string }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">{open && <PreviewBody cartId={cartId} />}</DialogContent>
    </Dialog>
  )
}

function PreviewBody({ cartId }: { cartId: string }) {
  const { data, loading, error } = useQuery(AdminCartOfferEmailPreviewDocument, { variables: { cartId }, fetchPolicy: 'network-only' })
  const [view, setView] = useState<'html' | 'text'>('html')
  const p = data?.adminCartOfferEmailPreview
  return (
    <>
      <DialogHeader>
        <DialogTitle>Email preview</DialogTitle>
        <DialogDescription>
          What Send mails, from the saved offer - unsaved changes aren’t in it. A Pulse flow on the cart offer trigger, when one is active, sends its own design instead.
        </DialogDescription>
      </DialogHeader>
      {loading && !p ? (
        <p className="text-sm text-muted-foreground">Rendering…</p>
      ) : error || !p ? (
        <p role="alert" className="text-sm text-destructive">
          {error?.message ?? 'The preview isn’t available.'}
        </p>
      ) : (
        <div className="grid gap-3">
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
            <dt className="text-muted-foreground">To</dt>
            <dd>{p.to || <span className="text-destructive">no email - the offer can’t be sent</span>}</dd>
            <dt className="text-muted-foreground">Subject</dt>
            <dd className="font-medium">{p.subject}</dd>
          </dl>

          <div className="grid gap-1.5">
            <div className="flex items-center gap-2">
              <Input readOnly value={p.link} aria-label="Offer link" className="font-mono text-xs" onFocus={(e) => e.currentTarget.select()} />
              <Button type="button" variant="outline" className="h-10" disabled={!p.linkComplete} onClick={() => void copyText(p.link)}>
                <CopyIcon className="size-4" />
                Copy link
              </Button>
            </div>
            {!p.linkComplete && (
              <p className="text-xs text-warning">The channel has no public URL, so this is only the link’s path - set the URL on the channel to send or share it.</p>
            )}
          </div>

          <Tabs value={view} onValueChange={(v) => setView(v as typeof view)}>
            <TabsList>
              <TabsTrigger value="html">HTML</TabsTrigger>
              <TabsTrigger value="text">Plain text</TabsTrigger>
            </TabsList>
          </Tabs>
          {view === 'html' ? (
            // Sandboxed with nothing allowed - the mail's markup renders, its links and scripts can't act in the console
            <iframe title="Email HTML" sandbox="" srcDoc={p.html} className="h-[28rem] w-full rounded-md border bg-white" />
          ) : (
            <pre className="h-[28rem] overflow-auto whitespace-pre-wrap rounded-md border p-3 text-xs">{p.text}</pre>
          )}
        </div>
      )}
    </>
  )
}
