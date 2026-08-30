import { useMemo } from 'react'
import { gql } from '@apollo/client'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'

// Lightweight Products query: only the fields the picker actually renders - Distinct from the main Catalog Products query (which pulls
// images, attribute values, etc.) so dropping into a long promotion editor doesn't drag those payloads into the network tab
const SKU_PICKER_PRODUCTS = gql`
  query SchemaFormSkuPickerProducts {
    products {
      id
      sku
      name
    }
  }
`

type ProductMini = { id: string; sku: string; name: string }

export function SkuPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery<{ products: ProductMini[] }>(SKU_PICKER_PRODUCTS)
  const options = useMemo<Option[]>(
    () =>
      (data?.products ?? []).map((p) => ({
        value: p.sku,
        label: p.name || p.sku,
        hint: p.sku,
      })),
    [data],
  )
  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      multi={isMulti}
      placeholder={isMulti ? 'Select SKUs…' : 'Select SKU…'}
      emptyMessage="No products."
      loading={loading}
    />
  )
}
