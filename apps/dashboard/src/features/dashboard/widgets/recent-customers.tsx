import { useQuery } from '@apollo/client/react'
import { Link } from 'react-router-dom'
import { useConfigScope } from '@/lib/config-scope'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'
import { WidgetSection } from './primitives'

export function RecentCustomersWidget() {
  const { effectiveChannelId } = useConfigScope()
  const { data, loading, error } = useQuery(CustomersPagedDocument, {
    variables: { limit: 6, channelId: effectiveChannelId ?? null },
    fetchPolicy: 'cache-and-network',
  })
  const customers = data?.customersPaged.items ?? []

  return (
    <WidgetSection
      title="Recent customers"
      description="The newest accounts to register."
      to="/customers"
      loading={loading && customers.length === 0}
      error={error ?? null}
      empty={!loading && customers.length === 0}
    >
      <div className="overflow-x-auto rounded-md border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-xs uppercase tracking-wider text-muted-foreground">
              <th className="px-3 py-2">Customer</th>
              <th className="px-3 py-2">Group</th>
              <th className="px-3 py-2 text-right">Joined</th>
            </tr>
          </thead>
          <tbody>
            {customers.map((c) => {
              const name = `${c.firstName ?? ''} ${c.lastName ?? ''}`.trim()
              return (
                <tr key={c.id} className="border-b last:border-0 hover:bg-muted/40">
                  <td className="px-3 py-2">
                    <Link to={`/customers/${c.id}`} className="font-medium hover:underline">
                      {name || c.email}
                    </Link>
                    {name && <div className="text-xs text-muted-foreground">{c.email}</div>}
                  </td>
                  <td className="px-3 py-2 text-muted-foreground">{c.group?.name ?? '-'}</td>
                  <td className="px-3 py-2 text-right text-muted-foreground">
                    {new Date(c.createdAt).toLocaleDateString(undefined, { dateStyle: 'medium' })}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </WidgetSection>
  )
}
