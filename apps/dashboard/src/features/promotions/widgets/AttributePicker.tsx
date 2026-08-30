import { useMemo } from 'react'
import { gql } from '@apollo/client'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'

const ATTRIBUTES_QUERY = gql`
  query SchemaFormAttributes {
    attributes {
      id
      slug
      name
      values {
        slug
        name
      }
    }
  }
`

type AttrMini = {
  id: string
  slug: string
  name: string
  values: Array<{ slug: string; name: string }>
}

// AttributePicker stores the attribute SLUG (matching the rule engine's `core.product_has_attribute.attribute` field) - It does not pick
// values - those are entered separately in the schema (the `values` field is a sibling array on the same condition's params)
export function AttributePicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery<{ attributes: AttrMini[] }>(ATTRIBUTES_QUERY)
  const options = useMemo<Option[]>(
    () =>
      (data?.attributes ?? []).map((a) => ({
        value: a.slug,
        label: a.name || a.slug,
        hint: a.slug,
      })),
    [data],
  )
  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      multi={isMulti}
      placeholder={isMulti ? 'Select attributes…' : 'Select attribute…'}
      emptyMessage="No attributes."
      loading={loading}
    />
  )
}
