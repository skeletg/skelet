import type { WidgetRegistry } from '../SchemaForm'
import { AttributePicker } from './AttributePicker'
import { CategoryPicker } from './CategoryPicker'
import { CountryPicker } from './CountryPicker'
import { CustomerGroupPicker } from './CustomerGroupPicker'
import { NestedCondition } from './NestedCondition'
import { SegmentPicker } from './SegmentPicker'
import { SkuPicker } from './SkuPicker'

// The one registry every rendered SchemaForm uses - promotion rules AND tax rules, which render the
// same condition params and were drifting apart (tax rules had no widgets at all, so an operator
// typed the country codes and group slugs that promotions let them pick)
//
// Built-in core.* schemas reference these slugs via x-widget; skelet check contracts fails when a Go
// schema asks for one that isn't here. Plugin authors can reuse the slugs or extend this registry
export const FORM_WIDGETS: WidgetRegistry = {
  'category-picker': CategoryPicker,
  'sku-picker': SkuPicker,
  'customer-group-picker': CustomerGroupPicker,
  'segment-picker': SegmentPicker,
  'country-picker': CountryPicker,
  'attribute-picker': AttributePicker,
  'nested-condition': NestedCondition,
}
