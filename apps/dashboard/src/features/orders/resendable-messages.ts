import type { OrderEmailMessage } from '@/graphql/schema.types'

// The backend refuses a shipment notice with no shipment and a refund notice with no refund, so neither is offered
export function resendableMessages({
  hasFulfillments,
  isCancelled,
  isRefunded,
}: {
  hasFulfillments: boolean
  isCancelled: boolean
  isRefunded: boolean
}): OrderEmailMessage[] {
  const available: OrderEmailMessage[] = ['CONFIRMATION']
  if (hasFulfillments) available.push('SHIPMENT')
  if (isCancelled) available.push('CANCELLATION')
  if (isRefunded) available.push('REFUND')
  return available
}
