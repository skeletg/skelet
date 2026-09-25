import { describe, expect, it } from 'vitest'

import { resendableMessages } from './resendable-messages'

// The backend refuses a shipment notice with no shipment and a refund notice with no refund, so an entry
// offered in either state is a guaranteed error handed to the operator
describe('resendableMessages', () => {
  it('offers only the confirmation on a plain unshipped order', () => {
    expect(resendableMessages({ hasFulfillments: false, isCancelled: false, isRefunded: false }))
      .toEqual(['CONFIRMATION'])
  })

  it('offers the shipment notice once the order has a fulfillment', () => {
    expect(resendableMessages({ hasFulfillments: true, isCancelled: false, isRefunded: false }))
      .toEqual(['CONFIRMATION', 'SHIPMENT'])
  })

  it('offers the cancellation only on a cancelled order', () => {
    expect(resendableMessages({ hasFulfillments: false, isCancelled: true, isRefunded: false }))
      .toEqual(['CONFIRMATION', 'CANCELLATION'])
  })

  it('offers the refund only once money went back', () => {
    expect(resendableMessages({ hasFulfillments: false, isCancelled: false, isRefunded: true }))
      .toEqual(['CONFIRMATION', 'REFUND'])
  })

  it('offers everything on a cancelled, shipped and refunded order', () => {
    expect(resendableMessages({ hasFulfillments: true, isCancelled: true, isRefunded: true }))
      .toEqual(['CONFIRMATION', 'SHIPMENT', 'CANCELLATION', 'REFUND'])
  })
})
