import { useState } from 'react'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { useQuery } from '@apollo/client/react'
import { XIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { ProductPickerDocument } from '@/features/catalog/Catalog.gql'

// ProductFilter is a compact type-to-search product picker shared by the reviews queue and the questions table to scope to a single product
// Client-side filter over the product list (name / slug / sku) - When a product is picked it renders a removable pill instead of the input
export function ProductFilter({
  product,
  onChange,
  placeholder = 'Filter by product…',
}: {
  product: { id: string; name: string } | null
  onChange: (p: { id: string; name: string } | null) => void
  placeholder?: string
}) {
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const { data, loading } = useQuery(ProductPickerDocument, {
    variables: { search: debounced, limit: 8 },
    skip: debounced === '', // nothing shows on an empty filter anyway
  })
  const matches = debounced === '' ? [] : (data?.productsPaged.items ?? [])

  if (product) {
    return (
      <Badge variant="secondary" className="h-8 gap-1.5 px-2.5 text-xs">
        Product: {product.name}
        <button type="button" aria-label="Clear product filter" onClick={() => onChange(null)}>
          <XIcon className="size-3.5" />
        </button>
      </Badge>
    )
  }

  return (
    <div className="relative">
      <Input
        type="search"
        placeholder={loading ? 'Loading products…' : placeholder}
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="h-8 w-56"
      />
      {search.trim() !== '' && (
        <div className="absolute z-20 mt-1 grid w-72 gap-1 rounded-md border bg-popover p-1 text-sm shadow-md">
          {matches.length === 0 ? (
            <p className="px-2 py-1.5 text-xs text-muted-foreground">No products match.</p>
          ) : (
            matches.map((p) => (
              <button
                key={p.id}
                type="button"
                className="flex flex-col rounded px-2 py-1.5 text-left hover:bg-accent"
                onClick={() => {
                  onChange({ id: p.id, name: p.name ?? p.sku ?? '' })
                  setSearch('')
                }}
              >
                <span className="font-medium">{p.name ?? p.sku ?? ''}</span>
                <span className="font-mono text-[10px] text-muted-foreground">{p.sku ?? ''}</span>
              </button>
            ))
          )}
        </div>
      )}
    </div>
  )
}
