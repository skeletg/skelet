import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@apollo/client/react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ConfigSectionsDocument,
  SetConfigValueDocument,
} from '@/features/settings/Settings.gql'

// Per-attribute search tuning edited inline on the Attribute detail page - the data still lives
// in the central `search/products/attributes` JSON config, so this loads the whole array, edits
// the `attr_<slug>` entry in place and saves it all back, staying in sync with /search-engine
// The row only applies when Attribute.searchable is true - when false the search runtime ignores
// it regardless of tuning, so the card shows a short explanation instead

type AttrEntry = {
  name: string
  searchable: boolean
  weight: number
  matchType: string
  fuzziness: string
}

const SEARCH_ATTRIBUTES_PATH = 'search/products/attributes'

const MATCH_TYPE_OPTIONS: { value: string; label: string; help: string }[] = [
  { value: 'multi_match', label: 'Analyzed match', help: 'Standard full-text. Respects fuzziness.' },
  { value: 'bool_prefix', label: 'Typeahead (bool_prefix)', help: 'Last word as prefix. Fuzziness ignored.' },
  { value: 'match_phrase_prefix', label: 'Phrase prefix', help: 'Full phrase, last token is a prefix.' },
  { value: 'exact', label: 'Exact (term)', help: 'Whole value must match. SKU / IDs.' },
]

// Radix Select reserves "" for the placeholder, so we map the
// backend's empty-string convention onto a sentinel here
const FUZZ_OFF = 'off'
const FUZZINESS_OPTIONS: { value: string; label: string }[] = [
  { value: 'AUTO', label: 'AUTO (length-based)' },
  { value: '0', label: '0 (exact)' },
  { value: '1', label: '1 (one edit)' },
  { value: '2', label: '2 (two edits)' },
  { value: FUZZ_OFF, label: 'Off' },
]

function defaultEntry(field: string): AttrEntry {
  return {
    name: field,
    searchable: true,
    weight: 1,
    matchType: 'multi_match',
    fuzziness: 'AUTO',
  }
}

function parseEntries(raw: string): AttrEntry[] {
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map((entry: unknown) => {
      const e = (entry ?? {}) as Partial<AttrEntry>
      return {
        name: String(e.name ?? ''),
        searchable: e.searchable !== false,
        weight: Number.isFinite(e.weight) ? Number(e.weight) : 1,
        matchType: typeof e.matchType === 'string' ? e.matchType : 'multi_match',
        fuzziness: typeof e.fuzziness === 'string' ? e.fuzziness : 'AUTO',
      }
    })
  } catch {
    return []
  }
}

export function AttributeSearchTuningCard({
  attributeSlug,
  searchable,
}: {
  attributeSlug: string
  searchable: boolean
}) {
  // Scope is DEFAULT - search config is global, never per-store
  const scopeInput = useMemo(() => ({ kind: 'DEFAULT' as const }), [])
  const { data, loading, error } = useQuery(ConfigSectionsDocument, {
    variables: { scope: scopeInput },
  })

  const [setConfigValue, { loading: saving }] = useMutation(SetConfigValueDocument, {
    refetchQueries: [{ query: ConfigSectionsDocument, variables: { scope: scopeInput } }],
  })

  const field = useMemo(() => {
    if (!data) return null
    for (const section of data.configSections ?? []) {
      for (const group of section.groups ?? []) {
        const found = group.fields.find((f) => f.path === SEARCH_ATTRIBUTES_PATH)
        if (found) return found
      }
    }
    return null
  }, [data])

  const fieldKey = `attr_${attributeSlug}`
  const entries = useMemo(() => parseEntries(field?.value ?? '[]'), [field])
  const myEntry = useMemo(
    () => entries.find((e) => e.name === fieldKey) ?? defaultEntry(fieldKey),
    [entries, fieldKey],
  )

  const [weight, setWeight] = useState(myEntry.weight)
  const [matchType, setMatchType] = useState(myEntry.matchType)
  const [fuzziness, setFuzziness] = useState(myEntry.fuzziness)

  // Re-seed the drafts when the stored entry changes (config loaded or save refetch landed) - Adjusted during render
  // (React's "adjust state on prop change" pattern)
  const [prevEntry, setPrevEntry] = useState(myEntry)
  if (
    prevEntry.name !== myEntry.name ||
    prevEntry.weight !== myEntry.weight ||
    prevEntry.matchType !== myEntry.matchType ||
    prevEntry.fuzziness !== myEntry.fuzziness
  ) {
    setPrevEntry(myEntry)
    setWeight(myEntry.weight)
    setMatchType(myEntry.matchType)
    setFuzziness(myEntry.fuzziness)
  }

  // Fuzziness is meaningless for prefix-style and exact matches - the OpenSearch builder ignores it - Lock the dropdown
  // so the operator can't stage no-op edits
  const fuzzDisabled =
    !searchable ||
    matchType === 'bool_prefix' ||
    matchType === 'match_phrase_prefix' ||
    matchType === 'exact'

  // Staged save: edits register with the page save registry and apply
  // through the header Save + confirm modal, like every other section
  const diff = useMemo<SectionDiffEntry[]>(() => {
    const out: SectionDiffEntry[] = []
    if (weight !== myEntry.weight) {
      out.push({ label: 'weight', before: String(myEntry.weight), after: String(weight) })
    }
    if (matchType !== myEntry.matchType) {
      const human = (v: string) => MATCH_TYPE_OPTIONS.find((o) => o.value === v)?.label ?? v
      out.push({ label: 'match type', before: human(myEntry.matchType), after: human(matchType) })
    }
    if (fuzziness !== myEntry.fuzziness) {
      out.push({ label: 'fuzziness', before: myEntry.fuzziness || 'off', after: fuzziness || 'off' })
    }
    return out
  }, [weight, matchType, fuzziness, myEntry])

  useSaveSection({
    id: 'search-tuning',
    title: 'Search tuning',
    diff,
    save: async () => {
      if (!field) throw new Error('Search config not loaded yet.')
      const next: AttrEntry = {
        name: fieldKey,
        // Mirror catalog Attribute.searchable so the runtime signature stays consistent - If the operator toggles the
        // catalog flag and forgets to update the JSON, the search runtime now sees them in sync
        searchable,
        weight,
        matchType,
        fuzziness,
      }
      const idx = entries.findIndex((e) => e.name === fieldKey)
      const nextEntries =
        idx === -1 ? [...entries, next] : entries.map((e, i) => (i === idx ? next : e))
      await setConfigValue({
        variables: {
          path: SEARCH_ATTRIBUTES_PATH,
          value: JSON.stringify(nextEntries),
          scope: scopeInput,
        },
      })
    },
  })

  if (loading && !data) {
    return <p className="text-sm text-muted-foreground">Loading search settings…</p>
  }
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn't load search settings</AlertTitle>
        <AlertDescription>{error.message}</AlertDescription>
      </Alert>
    )
  }
  if (!field) {
    return (
      <Alert>
        <AlertTitle>Search config not registered</AlertTitle>
        <AlertDescription>
          The backend isn't reporting <code>{SEARCH_ATTRIBUTES_PATH}</code>.
          Check the search domain's <code>init()</code> at boot.
        </AlertDescription>
      </Alert>
    )
  }
  if (!searchable) {
    return (
      <p className="rounded-md border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
        Flip <strong>Searchable</strong> on the Settings card above to include this attribute in product search. The tuning fields appear once it's on.
      </p>
    )
  }

  return (
    <div className="grid gap-4">
      {/* Equal-height row: items-start so the conditional fuzziness
          helper hangs off its own column without pushing the siblings
          down - Inputs/selects share the default h-9 so they line up
          visually */}
      <div className="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-[120px_minmax(0,2fr)_minmax(0,1fr)] sm:items-start">
        <div className="grid gap-1.5">
          <Label htmlFor="search-weight" className="text-xs">Weight</Label>
          <Input
            id="search-weight"
            type="number"
            inputMode="numeric"
            min={1}
            max={100}
            value={weight}
            onChange={(e) => {
              const n = Number(e.target.value)
              setWeight(Number.isFinite(n) ? n : 1)
            }}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="search-match-type" className="text-xs">Match type</Label>
          <Select value={matchType} onValueChange={setMatchType}>
            <SelectTrigger className="w-full" id="search-match-type">
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
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="search-fuzziness" className="text-xs">Fuzziness</Label>
          <Select
            value={fuzziness === '' ? FUZZ_OFF : fuzziness}
            onValueChange={(v) => setFuzziness(v === FUZZ_OFF ? '' : v)}
            disabled={fuzzDisabled}
          >
            <SelectTrigger className="w-full" id="search-fuzziness">
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
          {fuzzDisabled && (
            <p className="text-[11px] leading-tight text-muted-foreground">
              Ignored by this match type.
            </p>
          )}
        </div>
      </div>

      <div className="flex items-center justify-between gap-3 border-t pt-3">
        <p className="text-[11px] text-muted-foreground">
          Index field <code className="font-mono">{fieldKey}</code> ·
          stored in <code className="font-mono">{SEARCH_ATTRIBUTES_PATH}</code>
          {saving ? ' · saving…' : diff.length > 0 ? ' · pending - applied on Save' : ''}
        </p>
      </div>
    </div>
  )
}
