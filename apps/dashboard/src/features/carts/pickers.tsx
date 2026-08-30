import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { useDebounced } from '@/shared/hooks/useDebounced'

import { Input } from '@/components/ui/input'
import { formatCents } from '@/lib/format'
import { ProductPickerDocument } from '@/features/catalog/Catalog.gql'
import { CustomersPagedDocument } from '@/features/customers/Customers.gql'

// CustomerSearch is a compact type-to-search picker over the customer list,
// filtered client-side by email/name - Shared by the new-cart dialog
export function CustomerSearch({
  onPick,
}: {
  onPick: (c: { id: string; email: string; firstName: string; lastName: string }) => void
}) {
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const { data, loading } = useQuery(CustomersPagedDocument, {
    variables: { search: debounced, limit: 8, offset: 0 },
    skip: debounced === '', // nothing renders until the operator types
  })
  const matches = debounced === '' ? [] : (data?.customersPaged.items ?? [])
  return (
    <div className="grid gap-2">
      <Input
        type="search"
        placeholder={loading ? 'Loading customers…' : 'Search customers by name or email…'}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="h-8"
      />
      {search.trim() !== '' && (
        <div className="grid gap-1 rounded-md border bg-popover p-1 text-sm">
          {matches.length === 0 ? (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">No matches.</p>
          ) : (
            matches.map((c) => (
              <button
                key={c.id}
                type="button"
                className="grid gap-0.5 rounded px-2 py-1.5 text-left hover:bg-accent"
                onClick={() => {
                  onPick(c)
                  setSearch('')
                }}
              >
                <span className="font-medium">
                  {c.firstName} {c.lastName}
                </span>
                <span className="text-xs text-muted-foreground">{c.email}</span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}

// ProductSearch is a compact picker over simple products (client-side filter),
// invoking onAdd per pick - Shared by cart editing
export function ProductSearch({
  onAdd,
}: {
  onAdd: (p: { id: string; sku: string; name: string; unitPrice: number; currency: string }) => void
}) {
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const { data, loading } = useQuery(ProductPickerDocument, {
    variables: { search: debounced || null, kinds: ['SIMPLE'], limit: 12 },
  })
  // the server filters kind, the sku guard stays local - a simple product without a sku can't become a cart line
  const matches = useMemo(
    () => (data?.productsPaged.items ?? []).filter((p) => !!p.sku).slice(0, 8),
    [data],
  )
  return (
    <div className="grid gap-2">
      <Input
        type="search"
        placeholder={loading ? 'Loading products…' : 'Search products to add…'}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="h-8"
      />
      {search.trim() !== '' && (
        <div className="grid gap-1 rounded-md border bg-popover p-1 text-sm">
          {matches.length === 0 ? (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">No simple products match.</p>
          ) : (
            matches.map((p) => (
              <button
                key={p.id}
                type="button"
                className="flex items-center justify-between gap-2 rounded px-2 py-1.5 text-left hover:bg-accent"
                onClick={() => {
                  onAdd({
                    id: p.id,
                    sku: p.sku ?? '',
                    name: p.name ?? p.sku ?? '',
                    unitPrice: p.defaultPrice ?? 0,
                    currency: p.defaultCurrencyCode ?? 'USD',
                  })
                  setSearch('')
                }}
              >
                <span className="grid gap-0.5">
                  <span className="font-medium">{p.name ?? p.sku ?? ''}</span>
                  <span className="font-mono text-xs text-muted-foreground">{p.sku ?? ''}</span>
                </span>
                <span className="text-xs tabular-nums text-muted-foreground">
                  {formatCents(p.defaultPrice ?? 0, p.defaultCurrencyCode ?? 'USD')}
                </span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}
