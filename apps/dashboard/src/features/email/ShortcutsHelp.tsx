import { KeyboardIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { SHORTCUTS } from './shortcuts'

export function ShortcutsHelp() {
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="icon" className="size-8" aria-label="Keyboard shortcuts" title="Keyboard shortcuts">
          <KeyboardIcon className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-3">
        <p className="mb-2 text-xs font-medium">Keyboard shortcuts</p>
        <ul className="grid gap-1.5">
          {SHORTCUTS.map((s) => (
            <li key={s.what} className="flex items-center justify-between gap-3 text-xs">
              <span className="text-muted-foreground">{s.what}</span>
              <span className="flex shrink-0 gap-0.5">
                {s.keys.map((k) => (
                  <kbd key={k} className="rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px]">
                    {k}
                  </kbd>
                ))}
              </span>
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
