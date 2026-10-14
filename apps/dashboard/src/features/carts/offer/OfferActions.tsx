import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { BanIcon, CopyIcon, EyeIcon, MailIcon, MoreHorizontalIcon, RefreshCwIcon } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { ConfirmDialog } from '@/shared/dialogs/ConfirmDialog'
import {
  AdminRegenerateCartOfferLinkDocument,
  AdminRevokeCartOfferDocument,
  AdminSendCartOfferDocument,
  type CartOfferFieldsFragment,
} from '../Carts.gql'
import { copyText, OfferEmailPreview } from './OfferEmailPreview'

// The offer's lifecycle actions - sending, sharing and withdrawing it are actions rather than field
// edits, so they apply on their own like an order's resend (docs/conventions.md). Staged edits go
// out first: sending while the page holds unsaved changes would mail the old terms
export function OfferActions({
  cartId,
  offer,
  canEdit,
  hasLines,
  isDirty,
}: {
  cartId: string
  offer: CartOfferFieldsFragment
  canEdit: boolean
  hasLines: boolean
  isDirty: boolean
}) {
  const [send, sendState] = useMutation(AdminSendCartOfferDocument)
  const [regenerate] = useMutation(AdminRegenerateCartOfferLinkDocument)
  const [revoke] = useMutation(AdminRevokeCartOfferDocument)
  const [confirm, setConfirm] = useState<'send' | 'regenerate' | 'revoke' | null>(null)
  const [preview, setPreview] = useState(false)

  if (offer.state === 'COMPLETED') {
    return offer.orderId ? (
      <Button size="sm" variant="outline" asChild>
        <Link to={`/orders/${offer.orderId}`}>View order</Link>
      </Button>
    ) : null
  }
  const open = offer.state !== 'REVOKED' && offer.state !== 'EXPIRED'
  const sent = offer.sendCount > 0

  return (
    <>
      {canEdit && open && (
        <Button
          size="sm"
          disabled={isDirty || !hasLines || !offer.link || sendState.loading}
          title={isDirty ? 'Save your changes first' : !offer.link ? 'The channel needs a public URL to link to' : undefined}
          onClick={() => setConfirm('send')}
        >
          <MailIcon className="size-4" />
          {sent ? 'Resend' : 'Send'}
        </Button>
      )}
      {open && (
        <Button size="sm" variant="outline" onClick={() => setPreview(true)}>
          <EyeIcon className="size-4" />
          Preview email
        </Button>
      )}
      {offer.link && open && (
        <Button size="sm" variant="outline" onClick={() => void copyText(offer.link)}>
          <CopyIcon className="size-4" />
          Copy link
        </Button>
      )}
      {canEdit && offer.state !== 'REVOKED' && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="icon-sm" variant="outline" aria-label="More offer actions">
              <MoreHorizontalIcon className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={() => setConfirm('regenerate')}>
              <RefreshCwIcon className="size-4" />
              Replace link
            </DropdownMenuItem>
            <DropdownMenuItem className="text-destructive" onSelect={() => setConfirm('revoke')}>
              <BanIcon className="size-4" />
              Withdraw offer
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      <OfferEmailPreview open={preview} onOpenChange={setPreview} cartId={cartId} />
      <ConfirmDialog
        open={confirm === 'send'}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={sent ? 'Send the offer again?' : 'Send the offer?'}
        description="The recipient gets an email with the link to this cart, as it is now."
        confirmLabel={sent ? 'Resend' : 'Send'}
        busyLabel="Sending…"
        onConfirm={async () => {
          await send({ variables: { cartId } })
          toast.success('Offer sent')
        }}
      />
      <ConfirmDialog
        open={confirm === 'regenerate'}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Replace the link?"
        description="Every link sent or shared so far stops working. Send the offer again so the recipient has the new one."
        confirmLabel="Replace link"
        onConfirm={async () => {
          await regenerate({ variables: { cartId } })
          toast.success('Link replaced')
        }}
      />
      <ConfirmDialog
        open={confirm === 'revoke'}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Withdraw the offer?"
        description="The link stops working for good and the offer can’t be completed. This can’t be undone."
        confirmLabel="Withdraw"
        destructive
        onConfirm={async () => {
          await revoke({ variables: { cartId } })
          toast.success('Offer withdrawn')
        }}
      />
    </>
  )
}
