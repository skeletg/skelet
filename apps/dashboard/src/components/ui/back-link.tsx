import { ArrowLeftIcon } from 'lucide-react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

type BackLinkProps = {
  to: string
  label: string
  className?: string
}

export function BackLink({ to, label, className }: BackLinkProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      className={cn(
        'h-8 gap-1.5 rounded-full border-border/70 bg-background px-3 text-muted-foreground shadow-xs hover:border-foreground/15 hover:bg-muted hover:text-foreground',
        className,
      )}
      asChild
    >
      <Link to={to}>
        <ArrowLeftIcon className="size-3.5" />
        {label}
      </Link>
    </Button>
  )
}
