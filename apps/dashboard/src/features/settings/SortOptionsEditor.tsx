import { ArrowDownIcon, ArrowUpIcon, PlusIcon, TrashIcon } from 'lucide-react'
import { useMemo } from 'react'
import { useQuery } from '@apollo/client/react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { AttributesDocument } from '@/features/catalog/Catalog.gql'
import { slugify } from '@/lib/slug'

// SortOptionsEditor backs the SORT_OPTIONS config type - Each row maps a
// URL-safe slug onto an OpenSearch field + direction; the front
// resolver looks up the slug at query time and emits the sort clause
//
// The field picker mixes a small set of built-ins (relevance / price /
// name / newest) with attr_<slug>.keyword fields drawn from every
// filterable catalog attribute - Filterable is the gate because the
// indexer guarantees a .keyword sub-field for those - numeric
// attributes currently sort lexicographically, which the help text
// flags

type SortOption = {
  slug: string
  label: string
  field: string
  direction: 'asc' | 'desc'
  isIndexable: boolean
}

type FieldOption = {
  value: string
  label: string
  help?: string
  // hideDirection collapses the direction selector when the field doesn't have a meaningful asc/desc pairing (e.g.
  // _score is always descending in practice)
  hideDirection?: boolean
}

const BUILTIN_FIELDS: FieldOption[] = [
  {
    value: '_score',
    label: 'Relevance (_score)',
    help: 'Full-text score. Only meaningful when a search term is present; behaves like newest otherwise.',
    hideDirection: true,
  },
  { value: 'price_cents', label: 'Price (price_cents)' },
  { value: 'name.keyword', label: 'Name (name.keyword)' },
  { value: 'indexed_at', label: 'Indexed at (indexed_at)' },
]

function parseSortOptions(raw: string): SortOption[] {
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map(normalize)
  } catch {
    return []
  }
}

function normalize(entry: unknown): SortOption {
  const e = (entry ?? {}) as Partial<SortOption>
  return {
    slug: String(e.slug ?? ''),
    label: String(e.label ?? ''),
    field: String(e.field ?? '_score'),
    direction: e.direction === 'asc' ? 'asc' : 'desc',
    isIndexable: e.isIndexable === true,
  }
}

export function SortOptionsEditor({
  value,
  onChange,
  disabled,
}: {
  value: string
  onChange: (v: string) => void
  disabled?: boolean
}) {
  const options = useMemo(() => parseSortOptions(value), [value])

  // Filterable catalog attributes are the universe of attribute-driven sort fields - Filterable is the gate because the
  // indexer guarantees a .keyword sub-field on attr_<slug> only for filterable attributes
  const attributesQuery = useQuery(AttributesDocument)
  const attributeFields = useMemo<FieldOption[]>(() => {
    const list = attributesQuery.data?.attributes ?? []
    return list
      .filter((a) => a.filterable)
      .map((a) => ({
        value: `attr_${a.slug}.keyword`,
        label: `${a.name} (attr_${a.slug})`,
        help:
          String(a.inputType) === 'NUMERIC'
            ? 'Stored as keyword - sorts lexicographically, not numerically.'
            : undefined,
      }))
  }, [attributesQuery.data])

  const fieldOptions = useMemo(() => {
    return [...BUILTIN_FIELDS, ...attributeFields]
  }, [attributeFields])

  function commit(next: SortOption[]) {
    onChange(JSON.stringify(next))
  }

  function update(index: number, patch: Partial<SortOption>) {
    commit(options.map((o, i) => (i === index ? { ...o, ...patch } : o)))
  }

  function remove(index: number) {
    commit(options.filter((_, i) => i !== index))
  }

  function move(index: number, delta: -1 | 1) {
    const target = index + delta
    if (target < 0 || target >= options.length) return
    const next = [...options]
    ;[next[index], next[target]] = [next[target], next[index]]
    commit(next)
  }

  function add() {
    const fresh: SortOption = {
      slug: `option-${options.length + 1}`,
      label: 'New sort',
      field: '_score',
      direction: 'desc',
      isIndexable: false,
    }
    commit([...options, fresh])
  }

  function onLabelChange(index: number, label: string) {
    // Auto-derive the slug from the label until the operator touches the slug field directly - Mirrors
    // AttributeFormDialog's slug behavior so the editor feels consistent
    const current = options[index]
    const derivedSlug = slugify(label)
    const prevDerived = slugify(current.label)
    const patch: Partial<SortOption> =
      current.slug === '' || current.slug === prevDerived
        ? { label, slug: derivedSlug }
        : { label }
    update(index, patch)
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-muted/10">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-[10px] uppercase tracking-wider text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left font-medium w-12"></th>
            <th className="px-3 py-2 text-left font-medium w-52">Label</th>
            <th className="px-3 py-2 text-left font-medium w-40">Slug</th>
            <th className="px-3 py-2 text-left font-medium">Field</th>
            <th className="px-3 py-2 text-left font-medium w-32">Direction</th>
            <th className="px-3 py-2 text-left font-medium w-24">Indexable</th>
            <th className="px-3 py-2 text-left font-medium w-12"></th>
          </tr>
        </thead>
        <tbody>
          {options.map((opt, i) => {
            const fieldMeta = fieldOptions.find((f) => f.value === opt.field)
            const directionDisabled = disabled || fieldMeta?.hideDirection === true
            return (
              <tr key={i} className="border-t">
                <td className="px-3 py-2">
                  <div className="flex flex-col">
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => move(i, -1)}
                      disabled={disabled || i === 0}
                      className="size-5"
                      title="Move up"
                    >
                      <ArrowUpIcon className="size-3" />
                    </Button>
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      onClick={() => move(i, 1)}
                      disabled={disabled || i === options.length - 1}
                      className="size-5"
                      title="Move down"
                    >
                      <ArrowDownIcon className="size-3" />
                    </Button>
                  </div>
                </td>
                <td className="px-3 py-2">
                  <Input
                    value={opt.label}
                    onChange={(e) => onLabelChange(i, e.target.value)}
                    disabled={disabled}
                    className="h-8"
                  />
                </td>
                <td className="px-3 py-2">
                  <Input
                    value={opt.slug}
                    onChange={(e) => update(i, { slug: e.target.value })}
                    disabled={disabled}
                    className="h-8 font-mono text-xs"
                  />
                </td>
                <td className="px-3 py-2">
                  <Select
                    value={opt.field}
                    onValueChange={(v) => update(i, { field: v })}
                    disabled={disabled}
                  >
                    {/* Children, so the trigger shows the label alone - a bare SelectValue mirrors
                        the whole item, repeating the help text and widening the column until the
                        auto-layout table squeezes Label and Slug down to a character */}
                    <SelectTrigger className="h-8">
                      <SelectValue>{fieldMeta?.label ?? opt.field}</SelectValue>
                    </SelectTrigger>
                    <SelectContent>
                      {BUILTIN_FIELDS.map((f) => (
                        <SelectItem key={f.value} value={f.value}>
                          <div className="flex flex-col">
                            <span>{f.label}</span>
                            {f.help ? (
                              <span className="text-[10px] text-muted-foreground">{f.help}</span>
                            ) : null}
                          </div>
                        </SelectItem>
                      ))}
                      {attributeFields.length > 0 ? (
                        <div className="my-1 border-t" />
                      ) : null}
                      {attributeFields.map((f) => (
                        <SelectItem key={f.value} value={f.value}>
                          <div className="flex flex-col">
                            <span>{f.label}</span>
                            {f.help ? (
                              <span className="text-[10px] text-muted-foreground">{f.help}</span>
                            ) : null}
                          </div>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {fieldMeta?.help ? (
                    <p className="mt-1 text-[10px] text-muted-foreground">{fieldMeta.help}</p>
                  ) : null}
                </td>
                <td className="px-3 py-2">
                  <Select
                    value={opt.direction}
                    onValueChange={(v) => update(i, { direction: v as 'asc' | 'desc' })}
                    disabled={directionDisabled}
                  >
                    <SelectTrigger className="h-8">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="asc">Ascending</SelectItem>
                      <SelectItem value="desc">Descending</SelectItem>
                    </SelectContent>
                  </Select>
                </td>
                <td className="px-3 py-2">
                  {/* The first option's canonical URL drops the
                      /s/<slug> segment entirely (it IS the canonical
                      page), so its indexability is governed by the
                      filter chain alone - flag is moot - Disable the
                      switch + label as "n/a" for that row to avoid
                      operator confusion */}
                  {i === 0 ? (
                    <span
                      className="text-[10px] text-muted-foreground"
                      title="Default option. Its URL has no /s/ segment, so indexability is decided by the filter chain alone."
                    >
                      n/a
                    </span>
                  ) : (
                    <Switch
                      checked={opt.isIndexable}
                      onCheckedChange={(v) => update(i, { isIndexable: v })}
                      disabled={disabled}
                    />
                  )}
                </td>
                <td className="px-3 py-2">
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    onClick={() => remove(i)}
                    disabled={disabled}
                    className="size-7 text-muted-foreground hover:text-destructive"
                    title="Remove"
                  >
                    <TrashIcon className="size-3.5" />
                  </Button>
                </td>
              </tr>
            )
          })}
          {options.length === 0 ? (
            <tr>
              <td colSpan={7} className="px-3 py-6 text-center text-xs text-muted-foreground italic">
                No sort options. Add one to give shoppers a way to reorder browse results.
              </td>
            </tr>
          ) : null}
        </tbody>
      </table>
      <div className="flex items-center justify-between gap-3 border-t bg-muted/20 px-3 py-2">
        <p className="text-[10px] text-muted-foreground">
          First entry is the default - its URL has no /s/ segment, so it&apos;s always
          canonical. Non-default options need the{' '}
          <Badge variant="outline" className="h-4 px-1 text-[9px] uppercase tracking-tight font-medium">
            Indexable
          </Badge>{' '}
          switch flipped on for the resulting URL to be crawlable + indexed.
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={add}
          disabled={disabled}
          className="h-7"
        >
          <PlusIcon className="size-3.5" />
          Add option
        </Button>
      </div>
    </div>
  )
}
