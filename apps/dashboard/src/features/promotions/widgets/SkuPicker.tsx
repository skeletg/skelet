import { useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import type { WidgetProps } from '../SchemaForm'
import { useDebounced } from '@/shared/hooks/useDebounced'
import { AutocompletePicker, type Option } from './AutocompletePicker'
import { SchemaFormSkuPickerProductsDocument } from './SchemaFormPickers.gql'

export function SkuPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const [search, setSearch] = useState('')
  const debounced = useDebounced(search.trim(), 250)
  const { data, loading } = useQuery(SchemaFormSkuPickerProductsDocument, {
    variables: { search: debounced || null },
  })

  // Names for SKUs already picked - the current search may not include them, so every option seen
  // is remembered, and one never seen falls back to the SKU itself
  const [seen, setSeen] = useState<Map<string, Option>>(() => new Map())
  const [prevData, setPrevData] = useState(data)
  if (prevData !== data) {
    setPrevData(data)
    const next = new Map(seen)
    for (const p of data?.productsPaged.items ?? []) {
      if (p.sku) next.set(p.sku, { value: p.sku, label: p.name || p.sku, hint: p.sku })
    }
    setSeen(next)
  }

  const options = useMemo<Option[]>(() => {
    const current = (data?.productsPaged.items ?? [])
      .filter((p) => !!p.sku)
      .map((p) => ({ value: p.sku!, label: p.name || p.sku!, hint: p.sku! }))
    const picked = (Array.isArray(value) ? value : value ? [value] : []) as string[]
    const extra = picked
      .filter((sku) => !current.some((o) => o.value === sku))
      .map((sku) => seen.get(sku) ?? { value: sku, label: sku, hint: sku })
    return [...current, ...extra]
  }, [data, value, seen])

  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      onSearch={setSearch}
      multi={isMulti}
      placeholder={isMulti ? 'Select SKUs…' : 'Select SKU…'}
      emptyMessage={loading ? 'Searching…' : 'No products match.'}
      loading={loading}
    />
  )
}
