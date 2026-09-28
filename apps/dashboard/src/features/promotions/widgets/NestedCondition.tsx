import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { SchemaForm, type WidgetProps, type JSONSchema, type WidgetRegistry } from '../SchemaForm'
import { CategoryPicker } from './CategoryPicker'
import { SkuPicker } from './SkuPicker'
import { CustomerGroupPicker } from './CustomerGroupPicker'
import { AttributePicker } from './AttributePicker'
import { PromotionConditionTypesDocument } from '../Promotions.gql'

// A recursive condition picker for composites (core.all_of / any_of / not) and matching-line actions, over a `{ type, params }` value -
// recursion works because the widget ships its own `nested-condition` slug to the inner SchemaForm, so nesting has no depth limit
export function NestedCondition({ value, onChange }: WidgetProps) {
  const { data } = useQuery(PromotionConditionTypesDocument)
  const types = useMemo(() => data?.promotionConditionTypes ?? [], [data])

  const v =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as { type?: string; params?: unknown })
      : { type: '', params: {} }

  const currentType = types.find((t) => t.slug === v.type)
  const currentSchema = currentType?.paramsSchema as JSONSchema | undefined

  // Registers the nested-condition slug pointing at THIS component, so children of composites
  // nest further - the other pickers are here so a nested condition gets the same autocompletes
  const registry: WidgetRegistry = {
    'category-picker': CategoryPicker,
    'sku-picker': SkuPicker,
    'customer-group-picker': CustomerGroupPicker,
    'attribute-picker': AttributePicker,
    'nested-condition': NestedCondition,
  }

  return (
    <div className="grid gap-3 rounded border border-dashed p-3">
      <div className="grid gap-1.5">
        <span className="text-xs font-medium text-muted-foreground">Condition type</span>
        <Select
          value={v.type || undefined}
          onValueChange={(slug) => {
            // The new type's schema almost certainly won't accept the old type's fields
            onChange({ type: slug, params: {} })
          }}
        >
          <SelectTrigger>
            <SelectValue placeholder="Select condition type…" />
          </SelectTrigger>
          <SelectContent>
            {types.map((t) => (
              <SelectItem key={t.slug} value={t.slug}>
                {t.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {currentType?.description ? (
          <p className="text-xs text-muted-foreground">{currentType.description}</p>
        ) : null}
      </div>
      {currentSchema ? (
        <SchemaForm
          schema={currentSchema}
          value={v.params ?? {}}
          onChange={(next) => onChange({ type: v.type, params: next })}
          widgets={registry}
        />
      ) : null}
    </div>
  )
}
