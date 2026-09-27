import type { CategorySummaryFieldsFragment } from './Categories.gql'

// The tree query nests a fixed number of levels, each carrying the same fields, and the deepest
// leaves `children` out - so a walker can take any level as this node, children optional
type Deepest = CategorySummaryFieldsFragment['children'][number]['children'][number]['children'][number]['children'][number]

export type CategorySummary = Deepest & { children?: CategorySummary[] }
