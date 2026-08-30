import type { AttributeInputType } from '@/graphql/schema.types'

// Split from AttributeInputTypeBadge so that file exports only its component - mixing the two
// breaks Vite fast refresh (react-refresh/only-export-components)

// The backend enum is SCREAMING_SNAKE, the UI should read like English
export const INPUT_TYPE_LABEL: Record<AttributeInputType, string> = {
  PLAIN_TEXT: 'Plain text',
  NUMERIC: 'Numeric',
  BOOLEAN: 'Boolean',
  DATE: 'Date',
  DATE_TIME: 'Date/time',
  DROPDOWN: 'Dropdown',
  MULTISELECT: 'Multi-select',
  SWATCH: 'Swatch',
  REFERENCE: 'Reference',
}

// Input types carrying pre-defined AttributeValue rows - the rest are free-form per product
export const INPUT_TYPES_WITH_VALUES: Set<AttributeInputType> = new Set([
  'DROPDOWN',
  'MULTISELECT',
  'SWATCH',
])
