import { useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { formatPath, useCategoryIndex } from './category-options'
import { CategoryTree } from './CategoryTree'

// The "pick categories" dialog - browse or search the tree, tick any number, confirm once. Ticks
// survive searching and folding; whether the confirm stages or writes is the caller's

export type CategoryPickerProps = {
  takenIds?: ReadonlySet<string>
  takenLabel?: string
  excludeIds?: ReadonlySet<string>
  confirmLabel?: (count: number) => string
  onCancel: () => void
  onConfirm: (ids: string[]) => void
}

const defaultConfirmLabel = (n: number) =>
  n === 0 ? 'Add categories' : `Add ${n} categor${n === 1 ? 'y' : 'ies'}`

export function CategoryPickerDialog({
  open,
  onOpenChange,
  title,
  description,
  ...picker
}: Omit<CategoryPickerProps, 'onCancel'> & {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex h-[min(40rem,85vh)] flex-col gap-4 sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {/* Content unmounts on close, so the ticks and the search start fresh each time */}
        {open ? (
          <CategoryPicker
            {...picker}
            onCancel={() => onOpenChange(false)}
            onConfirm={(ids) => {
              picker.onConfirm(ids)
              onOpenChange(false)
            }}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

export function CategoryPicker({
  takenIds,
  takenLabel,
  excludeIds,
  confirmLabel = defaultConfirmLabel,
  onCancel,
  onConfirm,
}: CategoryPickerProps) {
  const { index, loading } = useCategoryIndex()
  // In tick order, so the confirm hands them back the way the admin chose them
  const [picked, setPicked] = useState<string[]>([])

  const toggle = (id: string) =>
    setPicked((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <CategoryTree
        index={index}
        loading={loading}
        multi
        isChecked={(id) => picked.includes(id)}
        isTaken={(id) => takenIds?.has(id) ?? false}
        takenLabel={takenLabel}
        excludeIds={excludeIds}
        onActivate={toggle}
        autoFocus
      />

      {picked.length > 0 ? (
        <div className="flex flex-wrap gap-1" aria-label="Selected categories">
          {picked.map((id) => (
            <span key={id} className="max-w-full truncate rounded-md border bg-muted/50 px-2 py-0.5 text-xs">
              {formatPath(index, id)}
            </span>
          ))}
        </div>
      ) : null}

      <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-3">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>{picked.length} selected</span>
          {picked.length > 0 ? (
            <Button type="button" variant="link" size="sm" className="h-auto p-0" onClick={() => setPicked([])}>
              Clear
            </Button>
          ) : null}
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button type="button" disabled={picked.length === 0} onClick={() => onConfirm(picked)}>
            {confirmLabel(picked.length)}
          </Button>
        </div>
      </div>
    </div>
  )
}
