import { useState, type ReactNode } from 'react'
import { ChevronsUpDownIcon, XIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { formatPath, useCategoryIndex } from './category-options'
import { CategoryTree } from './CategoryTree'

// Form fields over the category tree - a trigger showing the chosen category's path, and a popover
// holding the same tree and search as the picker dialog

type TriggerProps = {
  id?: string
  // A combobox takes no name from its text - give one wherever no <label htmlFor> points at `id`
  'aria-label'?: string
  disabled?: boolean
  invalid?: boolean
  excludeIds?: ReadonlySet<string>
  placeholder?: string
}

const POPOVER_CLASS = 'w-(--radix-popover-trigger-width) min-w-80 p-2'
const LIST_CLASS = 'max-h-[min(22rem,50vh)]'

// One category, or none. `noneLabel` offers '' as a first row; `fallbackLabel` names a value the
// list doesn't hold (still loading, or outside the admin's scope)
export function CategorySelect({
  value,
  onChange,
  noneLabel,
  fallbackLabel,
  placeholder = 'Select category…',
  id,
  'aria-label': ariaLabel,
  disabled,
  invalid,
  excludeIds,
}: TriggerProps & {
  value: string
  onChange: (id: string) => void
  noneLabel?: ReactNode
  fallbackLabel?: string
}) {
  const [open, setOpen] = useState(false)
  const { index, loading } = useCategoryIndex()
  const label = value ? formatPath(index, value, fallbackLabel) : null

  const pick = (id: string) => {
    onChange(id)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-label={ariaLabel}
          aria-expanded={open}
          aria-invalid={invalid || undefined}
          disabled={disabled}
          className="w-full justify-between font-normal"
          title={label ?? undefined}
        >
          <span className={cn('min-w-0 truncate', !label && 'text-muted-foreground')}>
            {label ?? (noneLabel !== undefined ? noneLabel : placeholder)}
          </span>
          <ChevronsUpDownIcon className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className={POPOVER_CLASS} align="start">
        <CategoryTree
          index={index}
          loading={loading}
          isChecked={(id) => id === value}
          excludeIds={excludeIds}
          none={noneLabel !== undefined ? { label: noneLabel, checked: !value, onSelect: () => pick('') } : undefined}
          onActivate={pick}
          autoFocus
          listClassName={LIST_CLASS}
        />
      </PopoverContent>
    </Popover>
  )
}

// Any number of categories - chips with a remove button above a trigger whose popover ticks and
// unticks in place
export function CategoryMultiSelect({
  value,
  onChange,
  placeholder = 'Select categories…',
  id,
  'aria-label': ariaLabel,
  disabled,
  invalid,
  excludeIds,
}: TriggerProps & {
  value: string[]
  onChange: (ids: string[]) => void
}) {
  const [open, setOpen] = useState(false)
  const { index, loading } = useCategoryIndex()
  const toggle = (cat: string) => onChange(value.includes(cat) ? value.filter((v) => v !== cat) : [...value, cat])

  return (
    <div className="grid gap-2">
      {value.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {value.map((cat) => {
            const label = formatPath(index, cat)
            return (
              <span key={cat} className="inline-flex max-w-full items-center gap-1 rounded-md border bg-muted/50 py-0.5 pr-1 pl-2 text-xs">
                <span className="truncate" title={label}>{label}</span>
                <button
                  type="button"
                  onClick={() => toggle(cat)}
                  disabled={disabled}
                  className="rounded p-0.5 hover:bg-muted"
                  aria-label={`Remove ${label}`}
                >
                  <XIcon className="size-3" />
                </button>
              </span>
            )
          })}
        </div>
      ) : null}
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            id={id}
            type="button"
            variant="outline"
            role="combobox"
            aria-label={ariaLabel}
            aria-expanded={open}
            aria-invalid={invalid || undefined}
            disabled={disabled}
            className="w-full justify-between font-normal"
          >
            <span className="min-w-0 truncate text-muted-foreground">
              {value.length ? `${value.length} selected` : placeholder}
            </span>
            <ChevronsUpDownIcon className="size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className={POPOVER_CLASS} align="start">
          <CategoryTree
            index={index}
            loading={loading}
            multi
            isChecked={(cat) => value.includes(cat)}
            excludeIds={excludeIds}
            onActivate={toggle}
            autoFocus
            listClassName={LIST_CLASS}
          />
        </PopoverContent>
      </Popover>
    </div>
  )
}
