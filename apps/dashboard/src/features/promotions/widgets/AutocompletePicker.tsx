import { useState } from 'react'
import { CheckIcon, ChevronsUpDownIcon, XIcon } from 'lucide-react'

import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { cn } from '@/lib/utils'

export type Option = {
  value: string
  label: string
  hint?: string
}

// AutocompletePicker is the shared combobox used by all schema-form
// pickers (categories, SKUs, attributes, groups) - Caller supplies
// the option list (already fetched / search-narrowed) and chooses
// between single / multi mode
//
// Multi mode renders selected values as removable badges above the
// trigger; single mode renders the selected label inside the trigger
// (or the placeholder when empty)
export function AutocompletePicker({
  options,
  value,
  onChange,
  multi = false,
  placeholder = 'Select…',
  emptyMessage = 'No results.',
  loading = false,
  onSearch,
}: {
  options: Option[]
  value: string | string[] | null | undefined
  onChange: (next: string | string[]) => void
  multi?: boolean
  placeholder?: string
  emptyMessage?: string
  loading?: boolean
  // Optional callback for typeahead - let the parent narrow the
  // option list as the user types - Omit for static option lists
  onSearch?: (query: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')

  const selected = multi ? (Array.isArray(value) ? value : []) : value ?? ''
  const selectedArray = Array.isArray(selected) ? selected : selected ? [selected] : []
  const selectedSet = new Set(selectedArray)

  const handleSelect = (val: string) => {
    if (multi) {
      const next = new Set(Array.isArray(value) ? value : [])
      if (next.has(val)) next.delete(val)
      else next.add(val)
      onChange(Array.from(next))
    } else {
      onChange(val)
      setOpen(false)
    }
  }

  const removeBadge = (val: string) => {
    if (multi && Array.isArray(value)) {
      onChange(value.filter((v) => v !== val))
    }
  }

  // For single-select, surface the picked option's label inside the
  // trigger so the user sees what they chose without opening the menu
  const singleLabel = !multi
    ? options.find((o) => o.value === selected)?.label ?? (selected ? String(selected) : '')
    : ''

  return (
    <div className="grid gap-2">
      {multi && selectedArray.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {selectedArray.map((val) => {
            const opt = options.find((o) => o.value === val)
            return (
              <Badge key={val} variant="secondary" className="gap-1 pr-1">
                <span className="text-xs">{opt?.label ?? val}</span>
                <button
                  type="button"
                  onClick={() => removeBadge(val)}
                  className="rounded p-0.5 hover:bg-muted"
                  aria-label={`Remove ${opt?.label ?? val}`}
                >
                  <XIcon className="size-3" />
                </button>
              </Badge>
            )
          })}
        </div>
      ) : null}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            className="w-full justify-between font-normal"
          >
            <span className={cn('min-w-0 truncate', !singleLabel && 'text-muted-foreground')}>
              {multi ? placeholder : singleLabel || placeholder}
            </span>
            <ChevronsUpDownIcon className="size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[--radix-popover-trigger-width] p-0" align="start">
          <Command shouldFilter={!onSearch}>
            <CommandInput
              placeholder="Search…"
              value={search}
              onValueChange={(v) => {
                setSearch(v)
                onSearch?.(v)
              }}
            />
            <CommandList>
              <CommandEmpty>{loading ? 'Loading…' : emptyMessage}</CommandEmpty>
              <CommandGroup>
                {options.map((opt) => {
                  const isSelected = selectedSet.has(opt.value)
                  return (
                    <CommandItem
                      key={opt.value}
                      value={opt.label}
                      onSelect={() => handleSelect(opt.value)}
                    >
                      <CheckIcon
                        className={cn('mr-2 size-4', isSelected ? 'opacity-100' : 'opacity-0')}
                      />
                      <div className="flex flex-col">
                        <span>{opt.label}</span>
                        {opt.hint ? (
                          <span className="text-xs text-muted-foreground">{opt.hint}</span>
                        ) : null}
                      </div>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    </div>
  )
}
