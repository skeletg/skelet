import { createContext, useContext, useDeferredValue, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useGraphiQL, useGraphiQLActions } from '@graphiql/react'
import { DatabaseIcon } from 'lucide-react'
import { GQ } from './graphiqlTheme'
import {
  buildListQuery,
  findValueSlots,
  listerIndex,
  matchesPick,
  pickOperation,
  readListResult,
  readVariable,
  writeVariable,
  type Lister,
  type PickOption,
  type PickRunner,
  type ValueSlot,
} from './valueTargets'

const CHANNEL_HEADER = 'X-Channel-Id'
const CHANNELS_QUERY = 'query PlaygroundChannels { channels { id name slug } }'

function useChannels(run: PickRunner) {
  const [state, setState] = useState<{ options: PickOption[]; error: string | null; loading: boolean }>({ options: [], error: null, loading: true })
  useEffect(() => {
    let live = true
    run({ query: CHANNELS_QUERY, variables: {}, admin: true })
      .then((res) => {
        if (!live) return
        const rows = ((res.data as { channels?: { id: string; name: string; slug: string }[] } | undefined)?.channels ?? [])
        setState({ options: rows.map((c) => ({ id: c.id, value: c.id, label: c.name, detail: c.slug })), error: res.errors?.[0]?.message ?? null, loading: false })
      })
      .catch((e: unknown) => live && setState({ options: [], error: e instanceof Error ? e.message : 'Could not load channels', loading: false }))
    return () => {
      live = false
    }
  }, [run])
  return state
}

// GraphiQL renders plugin content with no props, so the lookup function and surface reach it through context
const PickerContext = createContext<{ run: PickRunner; surface: 'admin' | 'front' } | null>(null)

export function PickerProvider({ run, surface, children }: { run: PickRunner; surface: 'admin' | 'front'; children: ReactNode }) {
  const value = useMemo(() => ({ run, surface }), [run, surface])
  return <PickerContext.Provider value={value}>{children}</PickerContext.Provider>
}

export function ValuesPluginIcon() {
  return <DatabaseIcon className="size-4" />
}

export function ValuesPluginContent() {
  const ctx = useContext(PickerContext)
  if (!ctx) return null
  return <ValuesPanel run={ctx.run} surface={ctx.surface} />
}

function ValuesPanel({ run, surface }: { run: PickRunner; surface: 'admin' | 'front' }) {
  const schema = useGraphiQL((s) => s.schema)
  const documentAST = useGraphiQL((s) => s.documentAST)
  const operationName = useGraphiQL((s) => s.operationName)
  const variablesText = useGraphiQL((s) => s.tabs[s.activeTabIndex]?.variables ?? '')
  const headersText = useGraphiQL((s) => s.tabs[s.activeTabIndex]?.headers ?? '')
  const variableEditor = useGraphiQL((s) => s.variableEditor)
  const headerEditor = useGraphiQL((s) => s.headerEditor)
  const { updateActiveTabValues } = useGraphiQLActions()
  const channels = useChannels(run)
  const [adminChannel, setAdminChannel] = useState<string | null>(null)
  const [open, setOpen] = useState<string | null>(null)
  const [writeError, setWriteError] = useState<string | null>(null)
  const [labels, setLabels] = useState<Record<string, string>>({}) // what each picked id was called, so the row reads "Europe" rather than a uuid

  const operation = useMemo(() => pickOperation(documentAST, operationName), [documentAST, operationName])
  const slots = useMemo(() => (schema ? findValueSlots(schema, operation) : []), [schema, operation])
  const listers = useMemo(() => (schema ? listerIndex(schema) : new Map<string, Lister>()), [schema])

  const headerChannel = readVariable(headersText, [CHANNEL_HEADER])
  const channelId = surface === 'front' ? (typeof headerChannel === 'string' ? headerChannel : null) : adminChannel

  function pickChannel(id: string) {
    if (surface === 'admin') {
      setAdminChannel(id)
      return
    }
    try {
      const next = writeVariable(headersText, { path: [CHANNEL_HEADER], isList: false }, id)
      headerEditor?.setValue(next)
      updateActiveTabValues({ headers: next })
      setWriteError(null)
    } catch {
      setWriteError('The headers are not valid JSON - fix them before picking a channel.')
    }
  }

  function pick(slot: ValueSlot, option: PickOption) {
    setLabels((prev) => ({ ...prev, [option.value]: option.label }))
    try {
      const next = writeVariable(variablesText, slot, option.value)
      variableEditor?.setValue(next)
      updateActiveTabValues({ variables: next })
      setWriteError(null)
      if (!slot.isList) setOpen(null)
    } catch {
      setWriteError('The variables are not valid JSON - fix them before picking a value.')
    }
  }

  if (!schema) return <p className={`p-3 text-sm ${GQ.muted}`}>No schema loaded yet.</p>

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto text-sm">
      <section className="space-y-1.5">
        <h3 className={`text-xs font-semibold uppercase tracking-wide ${GQ.muted}`}>Channel</h3>
        <p className={`text-xs ${GQ.faint}`}>
          {surface === 'front'
            ? `Sets the ${CHANNEL_HEADER} header most storefront operations need.`
            : 'Scopes the lists below for queries that take a channel.'}
        </p>
        <ChannelSelect options={channels.options} loading={channels.loading} error={channels.error} value={channelId} onChange={pickChannel} />
      </section>

      <section className="space-y-1.5">
        <h3 className={`text-xs font-semibold uppercase tracking-wide ${GQ.muted}`}>Ids in this operation</h3>
        {writeError && <p className={`text-xs ${GQ.error}`}>{writeError}</p>}
        {slots.length === 0 ? (
          <p className={`text-xs ${GQ.faint}`}>{operation ? 'This operation takes no ids.' : 'Write or open an operation to pick its ids.'}</p>
        ) : (
          <ul className={`divide-y rounded-md border ${GQ.border} [&>li]:border-[inherit]`}>
            {slots.map((slot) => {
              const key = slot.path.join('.')
              const current = readVariable(variablesText, slot.path)
              return (
                <li key={key} className="px-2 py-1.5">
                  <button type="button" onClick={() => setOpen(open === key ? null : key)} className="flex w-full items-baseline justify-between gap-2 text-left">
                    <span className={`font-mono text-xs ${GQ.strong}`}>${key}</span>
                    <span className={`text-xs ${GQ.faint}`}>
                      {slot.targetType ?? 'unknown type'}
                      {slot.valueField !== 'id' ? ` ${slot.valueField}` : ''}
                    </span>
                  </button>
                  <p className={`truncate text-xs ${GQ.muted}`}>{describeValue(current, labels)}</p>
                  {open === key && (
                    <SlotPicker
                      slot={slot}
                      lister={slot.targetType ? listers.get(slot.targetType) : undefined}
                      channels={slot.targetType === 'Channel' ? channels.options : null}
                      channelId={channelId}
                      run={run}
                      selected={current}
                      onPick={(option) => pick(slot, option)}
                    />
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </section>
    </div>
  )
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i // shortened in the row - a sku or slug is short and meaningful, so it stays whole

function describeValue(value: unknown, labels: Record<string, string>): string {
  if (value === undefined || value === null || value === '') return 'not set'
  const one = (v: unknown) => (typeof v === 'string' && labels[v] ? `${labels[v]} · ${UUID.test(v) ? v.slice(0, 8) : v}` : JSON.stringify(v))
  return Array.isArray(value) ? value.map(one).join(', ') || 'none' : one(value)
}

function ChannelSelect({ options, loading, error, value, onChange }: { options: PickOption[]; loading: boolean; error: string | null; value: string | null; onChange: (id: string) => void }) {
  if (error) return <p className={`text-xs ${GQ.error}`}>{error}</p>
  return (
    <select
      aria-label="Channel"
      value={value ?? ''}
      disabled={loading}
      onChange={(e) => e.target.value && onChange(e.target.value)}
      className={`w-full rounded-md border bg-transparent px-2 py-1 text-sm ${GQ.border} ${GQ.strong}`}
    >
      <option value="">{loading ? 'Loading channels…' : 'No channel'}</option>
      {options.map((o) => (
        <option key={o.id} value={o.id}>
          {o.label}
          {o.detail ? ` (${o.detail})` : ''}
        </option>
      ))}
      {value && !options.some((o) => o.id === value) && <option value={value}>{value}</option>}
    </select>
  )
}

function SlotPicker({
  slot,
  lister,
  channels,
  channelId,
  run,
  selected,
  onPick,
}: {
  slot: ValueSlot
  lister: Lister | undefined
  channels: PickOption[] | null
  channelId: string | null
  run: PickRunner
  selected: unknown
  onPick: (option: PickOption) => void
}) {
  const [term, setTerm] = useState('')
  const search = useDeferredValue(term)
  const serverSearch = lister?.searchArg ? search : ''
  const [state, setState] = useState<{ options: PickOption[]; error: string | null; loading: boolean }>({ options: [], error: null, loading: true })
  const blocked = !channels && lister?.channelArg?.required && !channelId

  useEffect(() => {
    if (channels || !lister || blocked) return
    let live = true
    const { query, variables } = buildListQuery(lister, { search: serverSearch, channelId, valueField: slot.valueField })
    run({ query, variables, channelId })
      .then((res) => live && setState({ options: readListResult(lister, res.data, slot.valueField), error: res.errors?.[0]?.message ?? null, loading: false }))
      .catch((e: unknown) => live && setState({ options: [], error: e instanceof Error ? e.message : 'Request failed', loading: false }))
    return () => {
      live = false
    }
  }, [channels, lister, blocked, serverSearch, channelId, run, slot.valueField])

  if (!slot.targetType) return <p className={`mt-1 text-xs ${GQ.faint}`}>Couldn't tell which type this id belongs to.</p>
  if (!channels && !lister) return <p className={`mt-1 text-xs ${GQ.faint}`}>No query lists {slot.targetType} records here, so it can't be picked.</p>
  if (blocked) return <p className={`mt-1 text-xs ${GQ.faint}`}>Listing {slot.targetType} needs a channel - pick one above.</p>

  const options = (channels ?? state.options).filter((o) => matchesPick(o, lister?.searchArg && !channels ? '' : search))
  const chosen = new Set(Array.isArray(selected) ? selected.map(String) : typeof selected === 'string' ? [selected] : [])
  const isChosen = (o: PickOption) => chosen.has(o.value)
  return (
    <div className="mt-1.5 space-y-1">
      <input
        value={term}
        onChange={(e) => setTerm(e.target.value)}
        placeholder={`Search ${slot.targetType}`}
        aria-label={`Search ${slot.targetType}`}
        className={`w-full rounded-md border bg-transparent px-2 py-1 text-xs outline-none ${GQ.border} ${GQ.strong}`}
      />
      {state.error && !channels && <p className={`text-xs ${GQ.error}`}>{state.error}</p>}
      {!channels && state.loading ? (
        <p className={`text-xs ${GQ.faint}`}>Loading…</p>
      ) : options.length === 0 ? (
        <p className={`text-xs ${GQ.faint}`}>Nothing found.</p>
      ) : (
        <ul className="max-h-56 overflow-y-auto" role="listbox" aria-label={`${slot.targetType} records`} aria-multiselectable={slot.isList}>
          {options.map((o) => (
            <li key={o.id} role="option" aria-selected={isChosen(o)}>
              <button
                type="button"
                onClick={() => onPick(o)}
                className={`flex w-full items-baseline justify-between gap-2 rounded-sm px-1.5 py-1 text-left text-xs ${GQ.hover} ${isChosen(o) ? GQ.selected : ''}`}
              >
                <span className={`truncate ${GQ.strong}`}>{o.label}</span>
                <span className={`shrink-0 font-mono ${GQ.faint}`}>{o.value !== o.id ? o.value : (o.detail ?? o.id.slice(0, 8))}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
