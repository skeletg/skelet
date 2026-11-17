import { createContext, useContext, useEffect, useMemo, useRef } from 'react'
import { useSaveSection, type SectionDiffEntry } from './saveRegistry'

// A plugin's part of a record's save - its own data, sent as the record mutation's `extensions` so the backend writes
// it in the record's own transaction, and a plugin refusing it refuses the whole save
//
// A plugin section stages its part with useExtensionSection; the page's record section takes everything staged into
// its own mutation when it saves, and when only plugin parts changed, they go out alone through the same mutation

type Staged = { value: unknown; key: string; onSaved?: () => void }

export type RecordExtensions = {
  stage: (plugin: string, staged: Staged | null) => void
  // The record section's half - the parts to send with its mutation, and what to call once it went through
  take: () => { parts: Record<string, unknown>; saved: () => void } | null
  // An extension section's half, for when the record section had nothing to send - sends what's still staged alone
  flush: () => Promise<void>
}

export const RecordExtensionsContext = createContext<RecordExtensions | null>(null)

// The page's half - send carries extensions alone through the record's own save mutation
export function useRecordExtensions(send: (parts: Record<string, unknown>) => Promise<unknown>): RecordExtensions {
  const staged = useRef(new Map<string, Staged>())
  // What last went through per plugin, so one Save never sends a part twice - the record section and the plugin's own
  // section both reach for it - and a part stays staged until the plugin's section stops reporting a change
  const sent = useRef(new Map<string, string>())
  const sendRef = useRef(send)
  useEffect(() => {
    sendRef.current = send
  })
  return useMemo<RecordExtensions>(() => {
    const take = () => {
      const entries = [...staged.current.entries()].filter(([plugin, s]) => sent.current.get(plugin) !== s.key)
      if (entries.length === 0) return null
      return {
        parts: Object.fromEntries(entries.map(([plugin, s]) => [plugin, s.value])),
        // Only after the mutation went through - a failed save leaves every part to go with the next try
        saved: () => {
          for (const [plugin, s] of entries) {
            sent.current.set(plugin, s.key)
            s.onSaved?.()
          }
        },
      }
    }
    return {
      stage: (plugin, s) => {
        if (s) {
          staged.current.set(plugin, s)
          return
        }
        staged.current.delete(plugin)
        sent.current.delete(plugin)
      },
      take,
      flush: async () => {
        const t = take()
        if (!t) return
        await sendRef.current(t.parts)
        t.saved()
      },
    }
  }, [])
}

// A plugin section's half: value is its part of the save, diff what the confirm dialog lists for it, empty when nothing
// changed. onSaved runs once the part went through - refetch your data there, so the diff empties against what's saved
export function useExtensionSection(
  plugin: string,
  opts: { title: string; diff: SectionDiffEntry[]; value: unknown; onSaved?: () => void },
) {
  const ext = useContext(RecordExtensionsContext)
  const dirty = opts.diff.length > 0
  const valueKey = JSON.stringify(opts.value)
  const onSaved = useRef(opts.onSaved)
  useEffect(() => {
    onSaved.current = opts.onSaved
  })
  useEffect(() => {
    ext?.stage(plugin, dirty ? { value: JSON.parse(valueKey) as unknown, key: valueKey, onSaved: () => onSaved.current?.() } : null)
  }, [ext, plugin, dirty, valueKey])
  useEffect(() => () => ext?.stage(plugin, null), [ext, plugin])
  useSaveSection({
    id: `extension:${plugin}`,
    title: opts.title,
    diff: opts.diff,
    // After the record's own sections, which take the staged parts into their mutation when they have one to send
    priority: 100,
    save: () => (ext ? ext.flush() : Promise.reject(new Error(`this page doesn't save ${plugin}'s part with its record`))),
  })
}
