import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { VenetianMaskIcon } from 'lucide-react'
import { toast } from 'sonner'

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
import { usePermissions } from '@/shared/auth/usePermissions'

import { MasqueradeCustomerDocument } from './Masquerade.gql'

type Session = { path: string; url: string; expiresInSeconds: number }

// Mints a short-lived customer session and hands the operator the storefront link, so support can see
// what the customer sees and a rep can order on their behalf - renders nothing without customers.manage
export function MasqueradeButton({
  customerId,
  variant = 'outline',
  label = 'Shop as customer',
}: {
  customerId: string
  variant?: 'outline' | 'ghost'
  label?: string
}) {
  const { has } = usePermissions()
  const [session, setSession] = useState<Session | null>(null)
  const [mint, { loading }] = useMutation(MasqueradeCustomerDocument)

  if (!has('customers.manage')) return null

  const start = async () => {
    try {
      const res = await mint({ variables: { customerId } })
      const s = res.data?.masqueradeCustomer
      if (!s) throw new Error('No session returned')
      setSession({ path: s.storefrontPath, url: s.storefrontUrl, expiresInSeconds: s.expiresInSeconds })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Could not start masquerade')
    }
  }

  const link = session ? session.url || session.path : ''

  return (
    <>
      <Button type="button" variant={variant} size="sm" className="gap-1.5" disabled={loading} onClick={() => void start()}>
        <VenetianMaskIcon className="size-3.5" />
        {loading ? 'Minting…' : label}
      </Button>
      <Dialog open={!!session} onOpenChange={(v) => !v && setSession(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Masquerade session</DialogTitle>
            <DialogDescription>
              Valid for {Math.round((session?.expiresInSeconds ?? 900) / 60)} minutes. The storefront shows a
              banner while you browse as this customer; every action is theirs (cart, orders, addresses).
            </DialogDescription>
          </DialogHeader>
          <div className="flex gap-2">
            <Input readOnly value={link} className="font-mono text-xs" onFocus={(e) => e.target.select()} />
            <Button
              className="h-10"
              type="button"
              variant="outline"
              onClick={() => {
                void navigator.clipboard.writeText(link)
                toast.success('Link copied')
              }}
            >
              Copy
            </Button>
          </div>
          {!session?.url && (
            <p className="text-xs text-muted-foreground">
              The customer's channel has no public URL configured, so this is a relative path - open it on
              the storefront's domain.
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setSession(null)}>
              Close
            </Button>
            {session?.url && (
              <Button asChild>
                <a href={session.url} target="_blank" rel="noreferrer">
                  Open storefront
                </a>
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
