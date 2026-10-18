import type { WidgetProps } from '../SchemaForm'
import { CategoryMultiSelect, CategorySelect } from '@/features/catalog/category-picker/CategorySelect'

// Handles single-select (string) and multi-select (array) - the mode is decided by whether the
// schema declares `type: array`. Both label a category by its path through the tree, at any depth
export function CategoryPicker({ schema, value, onChange, label }: WidgetProps) {
  if (schema.type === 'array') {
    const ids = Array.isArray(value) ? value.map(String) : []
    return <CategoryMultiSelect value={ids} onChange={onChange} aria-label={label ?? 'Categories'} />
  }
  return <CategorySelect value={typeof value === 'string' ? value : ''} onChange={onChange} aria-label={label ?? 'Category'} />
}
