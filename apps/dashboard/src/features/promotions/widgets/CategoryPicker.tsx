import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'
import { SchemaFormRootCategoriesDocument, type SchemaFormRootCategoriesQuery } from './SchemaFormPickers.gql'

// The query nests three levels; the walker takes any depth
type CategoryNode = Pick<SchemaFormRootCategoriesQuery['rootCategories'][number], 'id' | 'slug' | 'name'> & {
  children?: CategoryNode[]
}

// Prefixes nested names with parent slugs, so the user sees "Apparel > Shirts > T-Shirts",
// matching how the catalog UI surfaces categories elsewhere
function flattenTree(nodes: CategoryNode[], prefix: string[] = []): Option[] {
  const out: Option[] = []
  for (const n of nodes) {
    const label = [...prefix, n.name || n.slug].join(' › ')
    out.push({ value: n.id, label, hint: n.slug })
    if (n.children && n.children.length > 0) {
      out.push(...flattenTree(n.children, [...prefix, n.name || n.slug]))
    }
  }
  return out
}

// Handles single-select (string) and multi-select (array) - the mode is decided by whether the
// schema declares `type: array`
export function CategoryPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery(SchemaFormRootCategoriesDocument)
  const options = useMemo(() => flattenTree(data?.rootCategories ?? []), [data])

  return (
    <AutocompletePicker
      options={options}
      value={value as string | string[] | null | undefined}
      onChange={onChange}
      multi={isMulti}
      placeholder={isMulti ? 'Select categories…' : 'Select category…'}
      emptyMessage="No categories."
      loading={loading}
    />
  )
}
