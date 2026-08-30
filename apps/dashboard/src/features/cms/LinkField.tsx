import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@apollo/client/react'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { AutocompletePicker } from '@/features/promotions/widgets/AutocompletePicker'
import { LinkablesDocument } from './Cms.gql'
import { isLinkRef, type CmsLinkRef } from './page-blocks'

// LinkField is a Payload-style link picker for CMS block 'link' props - The value
// is EITHER a plain string (a custom/manual URL) OR a reference object
// { type:'reference', entityType, entityId, label } that the backend resolves to
// the entity's live, channel-aware URL at render - so the link follows slug
// renames. entityType is stored lowercase (the urlrewrite value, e.g. "cms_page")
// - the linkables query returns the uppercase GraphQL enum, so we down-case it
export function LinkField({
  value,
  onChange,
  disabled,
}: {
  value: unknown
  onChange: (v: unknown) => void
  disabled: boolean
}) {
  const ref = isLinkRef(value) ? value : null
  const [mode, setMode] = useState<'internal' | 'custom'>(ref ? 'internal' : 'custom')
  const [search, setSearch] = useState('')
  const [debounced, setDebounced] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setDebounced(search), 250)
    return () => clearTimeout(t)
  }, [search])

  const { data, loading } = useQuery(LinkablesDocument, {
    variables: { search: debounced, limit: 30 },
    skip: mode !== 'internal',
    fetchPolicy: 'cache-and-network',
  })
  const results = useMemo(() => data?.linkables ?? [], [data])

  const options = useMemo(() => {
    const opts = results.map((l) => ({
      value: l.entityId,
      label: l.label,
      hint: `${l.entityType.toLowerCase()} · ${l.path}`,
    }))
    // Keep the current selection visible even when it's not in the latest search
    if (ref && !opts.some((o) => o.value === ref.entityId)) {
      opts.unshift({ value: ref.entityId, label: ref.label || ref.entityId, hint: ref.entityType })
    }
    return opts
  }, [results, ref])

  function switchMode(next: 'internal' | 'custom') {
    if (next === mode) return
    setMode(next)
    onChange(undefined) // clear the other-typed value so it can't linger
  }

  return (
    <div className="grid gap-1.5">
      <div className="flex w-fit overflow-hidden rounded-md border text-xs">
        {(['internal', 'custom'] as const).map((m) => (
          <button
            key={m}
            type="button"
            disabled={disabled}
            onClick={() => switchMode(m)}
            className={cn(
              'px-2.5 py-1 transition-colors',
              mode === m ? 'bg-primary text-primary-foreground' : 'hover:bg-accent',
            )}
          >
            {m === 'internal' ? 'Internal' : 'Custom URL'}
          </button>
        ))}
      </div>

      {mode === 'custom' ? (
        <Input
          type="url"
          value={typeof value === 'string' ? value : ''}
          placeholder="https://… or /path"
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <>
          <AutocompletePicker
            options={options}
            value={ref?.entityId ?? ''}
            onChange={(v) => {
              const id = typeof v === 'string' ? v : v[0]
              const picked = results.find((l) => l.entityId === id)
              if (picked) {
                onChange({
                  type: 'reference',
                  entityType: picked.entityType.toLowerCase(),
                  entityId: picked.entityId,
                  label: picked.label,
                } satisfies CmsLinkRef)
              } else if (ref && ref.entityId === id) {
                // re-selecting the already-stored ref - keep it
                onChange(ref)
              }
            }}
            onSearch={setSearch}
            placeholder="Search pages, products, categories…"
            emptyMessage="No matches."
            loading={loading}
          />
          <p className="text-[11px] leading-tight text-muted-foreground">
            Tracks the entity - the link follows slug changes. Resolves on the published page
            (the live editor preview shows it only after you save).
          </p>
        </>
      )}
    </div>
  )
}
