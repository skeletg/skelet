import { useMemo } from 'react'
import { gql } from '@apollo/client'
import { useQuery } from '@apollo/client/react'

import type { WidgetProps } from '../SchemaForm'
import { AutocompletePicker, type Option } from './AutocompletePicker'

// Lazy - many conditions don't use categories, and Apollo dedupes so several pickers share one request
const ROOT_CATEGORIES_QUERY = gql`
  query SchemaFormRootCategories {
    rootCategories {
      id
      slug
      effectiveName
      children {
        id
        slug
        effectiveName
        children {
          id
          slug
          effectiveName
        }
      }
    }
  }
`

type CategoryNode = {
  id: string
  slug: string
  effectiveName: string
  children?: CategoryNode[]
}

// Prefixes nested names with parent slugs, so the user sees "Apparel > Shirts > T-Shirts",
// matching how the catalog UI surfaces categories elsewhere
function flattenTree(nodes: CategoryNode[], prefix: string[] = []): Option[] {
  const out: Option[] = []
  for (const n of nodes) {
    const label = [...prefix, n.effectiveName || n.slug].join(' › ')
    out.push({ value: n.id, label, hint: n.slug })
    if (n.children && n.children.length > 0) {
      out.push(...flattenTree(n.children, [...prefix, n.effectiveName || n.slug]))
    }
  }
  return out
}

// Handles single-select (string) and multi-select (array) - the mode is decided by whether the
// schema declares `type: array`
export function CategoryPicker({ schema, value, onChange }: WidgetProps) {
  const isMulti = schema.type === 'array'
  const { data, loading } = useQuery<{ rootCategories: CategoryNode[] }>(ROOT_CATEGORIES_QUERY)
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
