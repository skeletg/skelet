import { useMemo } from 'react'
import { gql } from '@apollo/client'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'

const CUSTOMER_GROUPS_QUERY = gql`
  query SchemaFormCustomerGroups {
    customerGroups {
      id
      slug
      name
    }
  }
`

type GroupMini = { id: string; slug: string; name: string }

// CustomerGroupPicker stores SLUGS (not ids) because that's what the rule engine's `core.customer_group_in` condition consumes - matching
// against the customer's group slug is more stable than against an id when groups get re-created during migrations
export function CustomerGroupPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery<{ customerGroups: GroupMini[] }>(CUSTOMER_GROUPS_QUERY)
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
