import { GripVerticalIcon } from 'lucide-react'
import { createContext, useContext, useMemo, type ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

// CatalogAttrInfo is the slice of an Attribute the editor needs to auto-merge catalog attributes that are flagged Searchable=true into the
// displayed rows - Slug becomes the OpenSearch field name (prefixed `attr_`); name + inputType drive the displayed row labels
export type CatalogAttrInfo = {
  slug: string
  name: string
  inputType: string
}

type CatalogAttrContextValue = {
  attributes: CatalogAttrInfo[]
  // path of the field this context applies to - SearchAttributesEditor only auto-merges catalog attributes when its own path matches; the
  // blog attributes field gets the same context but the path filter keeps catalog attrs out of blog search
  forPath: string
}

const CatalogAttrContext = createContext<CatalogAttrContextValue | null>(null)

export function CatalogAttrProvider({
  attributes,
  forPath,
  children,
}: {
  attributes: CatalogAttrInfo[]
  forPath: string
  children: ReactNode
}) {
  const value = useMemo(() => ({ attributes, forPath }), [attributes, forPath])
  return <CatalogAttrContext.Provider value={value}>{children}</CatalogAttrContext.Provider>
}

// Per-attribute search settings table behind the SEARCH_ATTRIBUTES config type - one row per indexed field
// (name, sku, description) exposing the four knobs the query builder honors: searchable, weight, matchType, fuzziness

// Uncontrolled at the row level: every edit re-serializes the whole list through onChange, so it joins the same
// dirty-tracking + save-confirm flow as plain fields

type Attribute = {
  name: string
  searchable: boolean
  weight: number
  matchType: string
  fuzziness: string
}

const MATCH_TYPE_OPTIONS: { value: string; label: string; help: string }[] = [
  {
    value: 'multi_match',
    label: 'Analyzed match',
    help: 'Standard full-text. Respects fuzziness.',
  },
  {
    value: 'bool_prefix',
    label: 'Typeahead (bool_prefix)',
    help: 'Last word matches as prefix. Fuzziness ignored.',
  },
  {
    value: 'match_phrase_prefix',
    label: 'Phrase prefix',
    help: 'Full phrase, last token is a prefix. Strict.',
  },
  {
    value: 'exact',
    label: 'Exact (term)',
    help: 'Whole value must match. Use for SKU / IDs.',
  },
]

// Radix Select forbids an empty-string value (it reserves "" for the placeholder), so we use 'off' as a sentinel here
// and map it to the backend's empty-string convention only when serializing
const FUZZ_OFF = 'off'
const FUZZINESS_OPTIONS: { value: string; label: string }[] = [
  { value: 'AUTO', label: 'AUTO (length-based)' },
  { value: '0', label: '0 (exact)' },
  { value: '1', label: '1 (one edit)' },
  { value: '2', label: '2 (two edits)' },
  { value: FUZZ_OFF, label: 'Off' },
]

function parseAttributes(raw: string): Attribute[] {
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map((entry: unknown) => normalize(entry as Partial<Attribute>))
  } catch {
    return []
  }
}

function normalize(entry: Partial<Attribute>): Attribute {
  return {
    name: String(entry.name ?? ''),
    searchable: entry.searchable !== false,
    weight: Number.isFinite(entry.weight) ? Number(entry.weight) : 1,
    matchType: typeof entry.matchType === 'string' ? entry.matchType : 'multi_match',
    fuzziness: typeof entry.fuzziness === 'string' ? entry.fuzziness : 'AUTO',
  }
}

export function SearchAttributesEditor({
  value,
  onChange,
  disabled,
  path,
}: {
  value: string
  onChange: (v: string) => void
  disabled?: boolean
  path?: string
}) {
  const catalogCtx = useContext(CatalogAttrContext)
  const stored = useMemo(() => parseAttributes(value), [value])

  // Merge catalog attributes flagged Searchable=true into the displayed
  // rows only for the matching field path - Newly-merged rows aren't
  // persisted until the operator actually edits one; the next save
  // request includes them implicitly because the editor's onChange
  // serializes the visible list - Pre-edit state shows them as "catalog"
  // rows so the operator knows where they came from
  const catalogSlugByField = useMemo(() => {
    const map = new Map<string, CatalogAttrInfo>()
    if (catalogCtx && catalogCtx.forPath === path) {
      for (const ca of catalogCtx.attributes) {
        map.set(`attr_${ca.slug}`, ca)
      }
    }
    return map
  }, [catalogCtx, path])

  const attributes = useMemo(() => {
    if (catalogSlugByField.size === 0) return stored
    const present = new Set(stored.map((a) => a.name))
    const merged = [...stored]
    for (const [field, ca] of catalogSlugByField) {
      if (present.has(field)) continue
      merged.push({
        name: field,
        searchable: true,
        weight: 1,
        matchType: 'multi_match',
        fuzziness: 'AUTO',
        // _meta is internal; stripped on save below
        ...({ _meta: { catalog: true, label: ca.name } } as object),
      } as Attribute)
    }
    return merged
  }, [stored, catalogSlugByField])

  function update(index: number, patch: Partial<Attribute>) {
    const next = attributes.map((a, i) => (i === index ? { ...a, ...patch } : a))
    // Strip the internal _meta key so the JSON we persist matches the
    // backend's Attribute shape
    const serialized = next.map((row) => {
      const r = row as Attribute & { _meta?: unknown }
      const { _meta, ...rest } = r
      void _meta
      return rest
    })
    onChange(JSON.stringify(serialized))
  }

  if (attributes.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-4 text-xs text-muted-foreground italic">
        No searchable attributes configured. The backend registers defaults at boot;
        reset this field to restore them.
      </div>
    )
  }

  return (
    <div className="overflow-hidden rounded-lg border bg-muted/10">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-[10px] uppercase tracking-wider text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left font-medium">Attribute</th>
            <th className="px-3 py-2 text-left font-medium w-24">Searchable</th>
            <th className="px-3 py-2 text-left font-medium w-20">Weight</th>
            <th className="px-3 py-2 text-left font-medium w-52">Match type</th>
            <th className="px-3 py-2 text-left font-medium w-44">Fuzziness</th>
          </tr>
        </thead>
        <tbody>
          {attributes.map((attr, i) => {
            const meta = (attr as Attribute & { _meta?: { catalog: boolean; label: string } })
              ._meta
            const fuzzDisabled =
              disabled ||
              !attr.searchable ||
              attr.matchType === 'bool_prefix' ||
              attr.matchType === 'match_phrase_prefix' ||
              attr.matchType === 'exact'
            return (
              <tr
                key={`${attr.name}-${i}`}
                className={cn(
                  'border-t transition-colors',
                  !attr.searchable && 'opacity-50',
                )}
              >
                <td className="px-3 py-2">
                  <div className="flex min-h-8 items-center gap-2">
                    <GripVerticalIcon className="size-3.5 text-muted-foreground/40" />
                    <div className="flex flex-col">
                      <span className="font-mono text-xs">{meta?.label ?? attr.name}</span>
                      {meta ? (
                        <div className="flex items-center gap-1">
                          <Badge
                            variant="outline"
                            className="h-4 px-1 text-[9px] uppercase tracking-tight font-medium"
                          >
                            catalog
                          </Badge>
                          <code className="text-[10px] text-muted-foreground/60">{attr.name}</code>
                        </div>
                      ) : null}
                    </div>
                  </div>
                </td>
                <td className="px-3 py-2">
                  <div className="flex h-8 items-center">
                    <Switch
                      checked={attr.searchable}
                      onCheckedChange={(v) => update(i, { searchable: v })}
                      disabled={disabled}
                    />
                  </div>
                </td>
                <td className="px-3 py-2">
                  <Input
                    type="number"
                    inputMode="numeric"
                    min={1}
                    max={100}
                    value={attr.weight}
                    onChange={(e) => {
                      const n = Number(e.target.value)
                      update(i, { weight: Number.isFinite(n) ? n : 1 })
                    }}
                    disabled={disabled || !attr.searchable}
                    className="h-8 w-16"
                  />
                </td>
                <td className="px-3 py-2">
                  <Select
                    value={attr.matchType}
                    onValueChange={(v) => update(i, { matchType: v })}
                    disabled={disabled || !attr.searchable}
                  >
                    {/* The value mirrors the item's label + help, so the trigger grows to two lines
                        instead of clipping to one, and reads left-aligned like the open list */}
                    <SelectTrigger
                      size="sm"
                      className="h-auto min-h-8 w-full py-1 text-left *:data-[slot=select-value]:line-clamp-none"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {MATCH_TYPE_OPTIONS.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          <div className="flex flex-col">
                            <span>{opt.label}</span>
                            <span className="text-[10px] text-muted-foreground">{opt.help}</span>
                          </div>
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </td>
                <td className="px-3 py-2">
                  <Select
                    value={attr.fuzziness === '' ? FUZZ_OFF : attr.fuzziness}
                    onValueChange={(v) => update(i, { fuzziness: v === FUZZ_OFF ? '' : v })}
                    disabled={fuzzDisabled}
                  >
                    <SelectTrigger size="sm" className="w-full">
                      <SelectValue placeholder="-" />
                    </SelectTrigger>
                    <SelectContent>
                      {FUZZINESS_OPTIONS.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          {opt.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <p className="border-t bg-muted/20 px-3 py-2 text-[10px] text-muted-foreground">
        Algolia-style per-attribute tuning. Mixed match types fan out into a bool.should
        query at search time; uniform settings stay on the faster multi_match path.
      </p>
    </div>
  )
}
