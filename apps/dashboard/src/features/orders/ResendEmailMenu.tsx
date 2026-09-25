import { useState } from 'react'
import { useMutation } from '@apollo/client/react'
import { MailIcon } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ResendOrderEmailDocument } from './Orders.gql'
import type { OrderEmailMessage } from '@/graphql/schema.types'
import { resendableMessages } from './resendable-messages'

type Props = {
  orderId: string
  disabled?: boolean
  hasFulfillments: boolean
  isCancelled: boolean
  isRefunded: boolean
}

const LABELS: Record<OrderEmailMessage, string> = {
  CONFIRMATION: 'Order confirmation',
  SHIPMENT: 'Shipment notice',
  CANCELLATION: 'Cancellation notice',
  REFUND: 'Refund notice',
}

// Sends one of the order's lifecycle emails again, for a receipt a paused flow swallowed or a customer never got
export function ResendEmailMenu({ orderId, disabled, hasFulfillments, isCancelled, isRefunded }: Props) {
  const [resend, { loading }] = useMutation(ResendOrderEmailDocument)
  const [sending, setSending] = useState<OrderEmailMessage | null>(null)

  const available = resendableMessages({ hasFulfillments, isCancelled, isRefunded })

  async function send(message: OrderEmailMessage) {
    setSending(message)
    try {
      await resend({ variables: { orderId, message, fulfillmentId: null } })
      toast.success(`${LABELS[message]} resent`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Resend failed')
    } finally {
      setSending(null)
    }
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" disabled={disabled || loading}>
          <MailIcon className="size-4" />
          {sending ? `Sending ${LABELS[sending].toLowerCase()}…` : 'Resend email'}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {available.map((message) => (
          <DropdownMenuItem key={message} disabled={loading} onSelect={() => void send(message)}>
            {LABELS[message]}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
