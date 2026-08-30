import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation } from '@apollo/client/react'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useConfigScope } from '@/lib/config-scope'
import { useSaveSection, type SectionDiffEntry } from '@/shared/saving/saveRegistry'
import { changed } from '@/shared/saving/diff'

import {
  AttributeDocument,
  SetAttributeDescriptionDocument,
  SetAttributeValueDescriptionDocument,
} from './Catalog.gql'

// Locale-aware translation editor for both the attribute level (facet panel header) and the
// value level (filter landing page), each locale carrying an optional display name (empty
// falls back to the global one) plus description copy
// Edits are STAGED, never inline: the attribute level goes through the page save registry, the
// value level reports {dirty, flush} to the enclosing row form - Both fields empty deletes the row

type ContentRow = { locale: string; name?: string | null; description: string }
type Draft = { name: string; description: string }

// Pending handle an embedded (deferred) editor hands its host form
export type EmbeddedPending = { dirty: boolean; flush: () => Promise<void> }

function serverDraft(rows: Map<string, ContentRow>, locale: string): Draft {
  const row = rows.get(locale)
  return { name: row?.name ?? '', description: row?.description ?? '' }
}

function sameDraft(a: Draft, b: Draft): boolean {
  return a.name === b.name && a.description === b.description
}

/** Shared staged-draft state: one Draft per locale, seeded from the
 *  server rows, edits accumulate across locale switches */
function useTranslationDrafts(rows: ContentRow[]) {
  const { effectiveLocale, availableLocales } = useConfigScope()
  const [locale, setLocale] = useState(effectiveLocale)
  const byLocale = useMemo(() => {
    const m = new Map<string, ContentRow>()
    for (const r of rows) m.set(r.locale, r)
    return m
  }, [rows])

  const localeOptions = useMemo(() => {
    // Surface every locale already translated, even ones retired from availableLocales since -
    // availableLocales front-loaded so common picks rise to the top
    const seen = new Set<string>()
    const out: string[] = []
    for (const l of availableLocales) {
      if (!seen.has(l)) { seen.add(l); out.push(l) }
    }
    for (const r of rows) {
      if (!seen.has(r.locale)) { seen.add(r.locale); out.push(r.locale) }
    }
    if (!seen.has(effectiveLocale)) out.unshift(effectiveLocale)
    return out
  }, [availableLocales, rows, effectiveLocale])

  const [drafts, setDrafts] = useState<Record<string, Draft>>({})
  useEffect(() => {
    // Server rows changed, so staged edits are applied or stale - reset to the fresh source
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setDrafts({})
  }, [byLocale])

  const current = drafts[locale] ?? serverDraft(byLocale, locale)
  function setField(patch: Partial<Draft>) {
    setDrafts((prev) => ({
      ...prev,
      [locale]: { ...(prev[locale] ?? serverDraft(byLocale, locale)), ...patch },
    }))
  }

  const dirtyLocales = useMemo(
    () =>
      Object.entries(drafts)
        .filter(([l, d]) => !sameDraft(d, serverDraft(byLocale, l)))
        .map(([l]) => l)
        .sort(),
    [drafts, byLocale],
  )

  const diff = useMemo<SectionDiffEntry[]>(
    () =>
      dirtyLocales.flatMap((l) => {
        const before = serverDraft(byLocale, l)
        const after = drafts[l]
        return [
          changed(`${l} · name`, before.name, after.name),
          changed(`${l} · description`, before.description, after.description),
        ].filter((e): e is NonNullable<typeof e> => e !== null)
      }),
    [dirtyLocales, drafts, byLocale],
  )

  return { locale, setLocale, localeOptions, byLocale, current, setField, drafts, dirtyLocales, diff }
}

function localeHasContent(byLocale: Map<string, ContentRow>, drafts: Record<string, Draft>, l: string): boolean {
  const d = drafts[l] ?? serverDraft(byLocale, l)
  return !!(d.name || d.description)
}

function TranslationFields({
  state,
  canManage,
  namePlaceholder,
  placeholder,
}: {
  state: ReturnType<typeof useTranslationDrafts>
  canManage: boolean
  namePlaceholder: string
  placeholder: string
}) {
  const { locale, setLocale, localeOptions, byLocale, current, setField, drafts, dirtyLocales } = state
  if (localeOptions.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No locales configured for this store.
      </p>
    )
  }
  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between gap-2">
        <Label className="text-xs">Translation (per locale)</Label>
        <select
          className="h-7 rounded-md border bg-background px-2 text-xs"
          value={locale}
          disabled={!canManage}
          onChange={(e) => setLocale(e.target.value)}
        >
          {localeOptions.map((l) => (
            <option key={l} value={l}>
              {l}
              {dirtyLocales.includes(l) ? ' *' : localeHasContent(byLocale, drafts, l) ? ' ✓' : ''}
            </option>
          ))}
        </select>
      </div>
      <Input
        placeholder={namePlaceholder}
        value={current.name}
        disabled={!canManage}
        onChange={(e) => setField({ name: e.target.value })}
      />
      <Textarea
        rows={2}
        placeholder={placeholder}
        value={current.description}
        disabled={!canManage}
        onChange={(e) => setField({ description: e.target.value })}
      />
      {dirtyLocales.length > 0 && (
        <p className="text-[11px] text-muted-foreground">
          Pending translation changes: {dirtyLocales.join(', ')} - applied on Save.
        </p>
      )}
    </div>
  )
}

/** Attribute-level translations - stages edits and registers them with the page save registry */
export function AttributeDescriptionEditor({
  attributeId,
  descriptions,
  canManage,
}: {
  attributeId: string
  descriptions: ContentRow[]
  canManage: boolean
}) {
  const state = useTranslationDrafts(descriptions)
  const [save] = useMutation(SetAttributeDescriptionDocument, {
    refetchQueries: [{ query: AttributeDocument, variables: { id: attributeId } }],
  })

  const { drafts, dirtyLocales, diff, byLocale } = state
  useSaveSection({
    id: 'attribute-translations',
    title: 'Translations',
    diff,
    save: async () => {
      for (const l of dirtyLocales) {
        const d = drafts[l] ?? serverDraft(byLocale, l)
        await save({ variables: { attributeId, locale: l, description: d.description, name: d.name } })
      }
    },
  })

  return (
    <TranslationFields
      state={state}
      canManage={canManage}
      namePlaceholder="Translated attribute label (empty = use global name)"
      placeholder="Short copy rendered next to the facet panel header on the channel."
    />
  )
}

/** Value-level translations, embedded in the value row form - stages edits and reports a
 *  {dirty, flush} pending upward, which the row's own Save flushes with the row fields */
export function AttributeValueDescriptionEditor({
  valueId,
  attributeId,
  descriptions,
  canManage,
  onPendingChange,
}: {
  valueId: string
  attributeId: string
  descriptions: ContentRow[]
  canManage: boolean
  onPendingChange: (pending: EmbeddedPending) => void
}) {
  const state = useTranslationDrafts(descriptions)
  const [save] = useMutation(SetAttributeValueDescriptionDocument, {
    refetchQueries: [{ query: AttributeDocument, variables: { id: attributeId } }],
  })

  const { drafts, dirtyLocales, byLocale } = state
  const stateRef = useRef({ drafts, dirtyLocales, byLocale })
  useEffect(() => {
    // Declared before the onPendingChange effect so the flush closure's snapshot stays fresh
    stateRef.current = { drafts, dirtyLocales, byLocale }
  })

  useEffect(() => {
    onPendingChange({
      dirty: dirtyLocales.length > 0,
      flush: async () => {
        const cur = stateRef.current
        for (const l of cur.dirtyLocales) {
          const d = cur.drafts[l] ?? serverDraft(cur.byLocale, l)
          await save({ variables: { valueId, locale: l, description: d.description, name: d.name } })
        }
      },
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dirtyLocales.join(','), valueId])

  return (
    <TranslationFields
      state={state}
      canManage={canManage}
      namePlaceholder="Translated value label (empty = use global name)"
      placeholder="Landing-page copy under the H1 on /products/f/<attr>/v/<value>."
    />
  )
}
