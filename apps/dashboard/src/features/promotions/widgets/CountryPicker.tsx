import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'

import { CountriesPickerDocument } from '@/features/countries/Countries.gql'
import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'

// Stores ISO-2 CODES, which is what the tax jurisdiction condition matches the order's shipping
// country against - the label carries the flag and name so the operator picks a country, not a code
export function CountryPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery(CountriesPickerDocument)
  const options = useMemo<Option[]>(
    () =>
      (data?.countries ?? []).map((c) => ({
        value: c.code,
        label: `${c.emojiFlag ? c.emojiFlag + ' ' : ''}${c.name}`,
        hint: c.code,
      })),
    [data],
  )
  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      multi={isMulti}
      placeholder={isMulti ? 'Select countries…' : 'Select country…'}
      emptyMessage="No countries."
      loading={loading}
    />
  )
}
