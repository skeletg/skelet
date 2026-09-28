import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'
import { SchemaFormCustomerGroupsDocument } from './SchemaFormPickers.gql'

// CustomerGroupPicker stores SLUGS (not ids) because that's what the rule engine's `core.customer_group_in` condition consumes - matching
// against the customer's group slug is more stable than against an id when groups get re-created during migrations
export function CustomerGroupPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery(SchemaFormCustomerGroupsDocument)
  const options = useMemo<Option[]>(
    () =>
      (data?.customerGroups ?? []).map((g) => ({
        value: g.slug,
        label: g.name || g.slug,
        hint: g.slug,
      })),
    [data],
  )
  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      multi={isMulti}
      placeholder={isMulti ? 'Select customer groups…' : 'Select customer group…'}
      emptyMessage="No groups."
      loading={loading}
    />
  )
}
