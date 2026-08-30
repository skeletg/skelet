import { Badge } from '@/components/ui/badge'
import type { AttributeInputType } from '@/graphql/schema.types'
import { INPUT_TYPE_LABEL } from './attributeInputTypes'

export function AttributeInputTypeBadge({ type }: { type: AttributeInputType }) {
  return (
    <Badge variant="outline" className="font-mono text-[10px] uppercase tracking-wider">
      {INPUT_TYPE_LABEL[type]}
    </Badge>
  )
}
