import { useQuery } from '@apollo/client/react'
import { useConfigScope } from '@/lib/config-scope'
import { OrderQueueCountsDocument } from '@/features/orders/Orders.gql'
import { QueueRow, RowList, WidgetSection } from './primitives'

// The same shape the orders list encodes its column filters in, so a queue lands you in exactly
// that filtered list rather than on an unfiltered page you have to narrow again
function ordersLink(field: 'status' | 'paymentStatus', values: readonly string[]): string {
  return `/orders?f=${encodeURIComponent(JSON.stringify([{ id: field, value: values }]))}`
}

// The link targets have to say the same thing the query's aliases do, which is why they sit
// together - a queue whose count and whose list disagree is worse than no queue
const QUEUES = [
  { key: 'pending', label: 'Awaiting confirmation', hint: 'Placed, not yet confirmed', field: 'status', values: ['PENDING'], tone: 'default' },
  { key: 'toFulfil', label: 'To fulfil', hint: 'Confirmed or partly shipped', field: 'status', values: ['CONFIRMED', 'PROCESSING', 'PARTIALLY_FULFILLED'], tone: 'default' },
  { key: 'onHold', label: 'On hold', hint: 'Parked before fulfillment', field: 'status', values: ['ON_HOLD'], tone: 'warn' },
  { key: 'unpaid', label: 'Unpaid', hint: 'No payment captured yet', field: 'paymentStatus', values: ['UNPAID'], tone: 'bad' },
] as const

export function FulfillmentWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(OrderQueueCountsDocument, {
    variables: { channelId: effectiveChannelId ?? null },
    fetchPolicy: 'cache-and-network',
  })

  return (
    <WidgetSection
      title="Order queues"
      description="What is waiting on someone."
      to="/orders"
      loading={loading && !data}
      error={error ?? null}
    >
      <RowList>
        {QUEUES.map((q) => (
          <QueueRow
            key={q.key}
            label={q.label}
            hint={q.hint}
            count={data?.[q.key].totalCount ?? 0}
            to={ordersLink(q.field, q.values)}
            tone={q.tone}
          />
        ))}
      </RowList>
    </WidgetSection>
  )
}
