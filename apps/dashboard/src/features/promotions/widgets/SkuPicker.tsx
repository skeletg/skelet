import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'
import { SchemaFormSkuPickerProductsDocument } from './SchemaFormPickers.gql'

export function SkuPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery(SchemaFormSkuPickerProductsDocument)
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
