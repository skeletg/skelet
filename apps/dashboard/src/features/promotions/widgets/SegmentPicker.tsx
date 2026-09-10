import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'

import { CustomerSegmentsListDocument } from '@/features/promotions/Promotions.signals.gql'
import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'

// SegmentPicker stores SLUGS, which is what `core.customer_in_segment` consumes - same reasoning as
// CustomerGroupPicker, a slug survives a re-created segment where an id doesn't
// The backend schema has asked for this widget since the condition shipped; without it registered the
// param silently fell back to a free-text list of slugs
export function SegmentPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery(CustomerSegmentsListDocument, {
    variables: { limit: 200, offset: 0 },
  })
  const options = useMemo<Option[]>(
    () =>
      (data?.customerSegments.items ?? []).map((s) => ({
        value: s.slug,
        label: s.name || s.slug,
        hint: s.isActive ? s.slug : `${s.slug} · inactive`,
      })),
    [data],
  )
  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      multi={isMulti}
      placeholder={isMulti ? 'Select segments…' : 'Select segment…'}
      emptyMessage="No segments."
      loading={loading}
    />
  )
}
