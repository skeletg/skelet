import { SignpostIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

export function OrderStageBadge({ label, className }: { label: string; className?: string }) {
  return (
    <Badge variant="outline" className={cn('gap-1 font-normal text-muted-foreground', className)}>
      <SignpostIcon className="size-3" />
      {label}
    </Badge>
  )
}
