import { BracesIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { mergeTagGroup, mergeTagLabel } from './blocks'

export function MergeTagMenu({
  tags,
  onPick,
  className,
  label = 'Variable',
}: {
  tags: string[]
  onPick: (tag: string) => void
  className?: string
  label?: string
}) {
  if (tags.length === 0) return null
  const groups = new Map<string, string[]>()
  for (const tag of tags) {
    const g = mergeTagGroup(tag)
    groups.set(g, [...(groups.get(g) ?? []), tag])
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          // The mouse-down would otherwise move focus off the field the tag goes into
          onMouseDown={(e) => e.preventDefault()}
          aria-label={label ? undefined : 'Insert variable'}
          title={label ? undefined : 'Insert variable'}
          className={cn('h-6 gap-1 px-1.5 text-xs text-muted-foreground', className)}
        >
          <BracesIcon className="size-3.5" />
          {label}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-h-80 w-60 overflow-y-auto" onCloseAutoFocus={(e) => e.preventDefault()}>
        {[...groups.entries()].map(([group, list], i) => (
          <DropdownMenuGroup key={group}>
            {i > 0 ? <DropdownMenuSeparator /> : null}
            <DropdownMenuLabel className="text-xs text-muted-foreground">{group}</DropdownMenuLabel>
            {list.map((tag) => (
              <DropdownMenuItem key={tag} onSelect={() => onPick(tag)} className="flex items-center justify-between gap-3">
                <span>{mergeTagLabel(tag)}</span>
                <span className="truncate font-mono text-[10px] text-muted-foreground">{tag}</span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
