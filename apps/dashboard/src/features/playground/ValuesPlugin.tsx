import { createContext, useContext, useDeferredValue, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useGraphiQL, useGraphiQLActions } from '@graphiql/react'
import { parse, type GraphQLSchema } from 'graphql'
import { DatabaseIcon, PlusIcon } from 'lucide-react'
import { buildCreateOperation, creatorIndex, readCreated, skeletonVariables, type Creator } from './createTargets'
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
                      schema={schema}
                      surface={surface}
                      allowCreate
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
  schema,
  surface,
  allowCreate,
  slot,
  lister,
  channels,
  channelId,
  run,
  selected,
  onPick,
}: {
  schema: GraphQLSchema
  surface: 'admin' | 'front'
  allowCreate: boolean // one level only - a record made to satisfy another record's create doesn't get a create of its own
  slot: ValueSlot
  lister: Lister | undefined
  channels: PickOption[] | null
  channelId: string | null
  run: PickRunner
  selected: unknown
  onPick: (option: PickOption) => void
}) {
  if (!slot.targetType) return <p className={`mt-1 text-xs ${GQ.faint}`}>Couldn't tell which type this id belongs to.</p>
  const creators = allowCreate && slot.valueField === 'id' ? (creatorIndex(schema).get(slot.targetType) ?? []) : []
  const pickable = !!channels || !!lister
  return (
    <div className="mt-1.5 space-y-2">
      {pickable ? (
        <RecordList slot={slot} lister={lister} channels={channels} channelId={channelId} run={run} selected={selected} onPick={onPick} />
      ) : (
        <p className={`text-xs ${GQ.faint}`}>
          No query lists {slot.targetType} records here{creators.length > 0 ? ' - create one instead.' : ", so it can't be picked."}
        </p>
      )}
      {creators.length > 0 && (
        <CreateRecord schema={schema} surface={surface} creators={creators} typeName={slot.targetType} channelId={channelId} run={run} onCreated={onPick} />
      )}
    </div>
  )
}

function RecordList({
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

  if (blocked) return <p className={`text-xs ${GQ.faint}`}>Listing {slot.targetType} needs a channel - pick one above.</p>

  const options = (channels ?? state.options).filter((o) => matchesPick(o, lister?.searchArg && !channels ? '' : search))
  const chosen = new Set(Array.isArray(selected) ? selected.map(String) : typeof selected === 'string' ? [selected] : [])
  const isChosen = (o: PickOption) => chosen.has(o.value)
  return (
    <div className="space-y-1">
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

function CreateRecord({
  schema,
  surface,
  creators,
  typeName,
  channelId,
  run,
  onCreated,
}: {
  schema: GraphQLSchema
  surface: 'admin' | 'front'
  creators: Creator[]
  typeName: string
  channelId: string | null
  run: PickRunner
  onCreated: (option: PickOption) => void
}) {
  const [open, setOpen] = useState(false)
  const [index, setIndex] = useState(0)
  const creator = creators[Math.min(index, creators.length - 1)]
  const [varsText, setVarsText] = useState(() => JSON.stringify(skeletonVariables(schema, creator), null, 2))
  const [slotOpen, setSlotOpen] = useState<string | null>(null)
  const [status, setStatus] = useState<{ busy: boolean; error: string | null }>({ busy: false, error: null })
  const operation = useMemo(() => buildCreateOperation(creator), [creator])
  const slots = useMemo(() => findValueSlots(schema, pickOperation(parse(operation))), [schema, operation])
  const listers = useMemo(() => listerIndex(schema), [schema])
  const needsChannel = surface === 'front' && !channelId // storefront records belong to the channel in the header

  function choose(i: number) {
    setIndex(i)
    setVarsText(JSON.stringify(skeletonVariables(schema, creators[i]), null, 2))
    setStatus({ busy: false, error: null })
  }

  function fill(slot: ValueSlot, option: PickOption) {
    try {
      setVarsText(writeVariable(varsText, slot, option.value))
      if (!slot.isList) setSlotOpen(null)
    } catch {
      setStatus({ busy: false, error: 'The arguments are not valid JSON - fix them before picking a value.' })
    }
  }

  async function create() {
    let variables: Record<string, unknown>
    try {
      variables = varsText.trim() ? (JSON.parse(varsText) as Record<string, unknown>) : {}
    } catch {
      setStatus({ busy: false, error: 'The arguments are not valid JSON.' })
      return
    }
    setStatus({ busy: true, error: null })
    try {
      const res = await run({ query: operation, variables, channelId })
      const made = readCreated(creator, res.data)
      if (!made) {
        setStatus({ busy: false, error: res.errors?.[0]?.message ?? `${creator.field} returned no id.` })
        return
      }
      setStatus({ busy: false, error: null })
      setOpen(false)
      onCreated({ id: made.id, value: made.id, label: made.label ?? `New ${typeName}`, detail: null })
    } catch (e: unknown) {
      setStatus({ busy: false, error: e instanceof Error ? e.message : 'Request failed' })
    }
  }

  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)} className={`inline-flex items-center gap-1 text-xs ${GQ.strong} hover:underline`}>
        <PlusIcon className="size-3" aria-hidden /> Create new {typeName}
      </button>
    )
  }

  return (
    <div className={`space-y-1.5 rounded-md border p-2 ${GQ.border}`} role="group" aria-label={`Create ${typeName}`}>
      <div className="flex items-center justify-between gap-2">
        <span className={`text-xs font-medium ${GQ.strong}`}>Create {typeName}</span>
        {creators.length > 1 ? (
          <select
            aria-label="Create with"
            value={index}
            onChange={(e) => choose(Number(e.target.value))}
            className={`rounded-md border bg-transparent px-1 py-0.5 font-mono text-xs ${GQ.border} ${GQ.strong}`}
          >
            {creators.map((c, i) => (
              <option key={c.field} value={i}>
                {c.field}
              </option>
            ))}
          </select>
        ) : (
          <span className={`font-mono text-xs ${GQ.faint}`}>{creator.field}</span>
        )}
      </div>
      <p className={`text-xs ${GQ.warningText}`}>This writes a real {typeName} to the database behind this API.</p>
      {needsChannel && <p className={`text-xs ${GQ.faint}`}>Pick a channel above first - the storefront creates it on that channel.</p>}
      {slots.length > 0 && (
        <ul className={`divide-y rounded-md border ${GQ.border} [&>li]:border-[inherit]`}>
          {slots.map((slot) => {
            const key = slot.path.join('.')
            return (
              <li key={key} className="px-2 py-1">
                <button type="button" onClick={() => setSlotOpen(slotOpen === key ? null : key)} className="flex w-full items-baseline justify-between gap-2 text-left">
                  <span className={`font-mono text-xs ${GQ.strong}`}>{key}</span>
                  <span className={`text-xs ${GQ.faint}`}>{slot.targetType ?? 'unknown type'}</span>
                </button>
                {slotOpen === key && (
                  <SlotPicker
                    schema={schema}
                    surface={surface}
                    allowCreate={false}
                    slot={slot}
                    lister={slot.targetType ? listers.get(slot.targetType) : undefined}
                    channels={null}
                    channelId={channelId}
                    run={run}
                    selected={readVariable(varsText, slot.path)}
                    onPick={(o) => fill(slot, o)}
                  />
                )}
              </li>
            )
          })}
        </ul>
      )}
      {creator.requiredArgs.length > 0 && (
        <textarea
          aria-label="Arguments"
          value={varsText}
          onChange={(e) => setVarsText(e.target.value)}
          rows={Math.min(12, varsText.split('\n').length + 1)}
          spellCheck={false}
          className={`w-full rounded-md border bg-transparent px-2 py-1 font-mono text-xs outline-none ${GQ.border} ${GQ.strong}`}
        />
      )}
      {status.error && <p className={`text-xs ${GQ.error}`}>{status.error}</p>}
      <div className="flex gap-2">
        <button
          type="button"
          onClick={() => void create()}
          disabled={status.busy || needsChannel}
          className={`rounded-md border px-2 py-0.5 text-xs font-medium ${GQ.border} ${GQ.strong} ${GQ.hover} disabled:opacity-50`}
        >
          {status.busy ? 'Creating…' : `Create ${typeName}`}
        </button>
        <button type="button" onClick={() => setOpen(false)} className={`px-2 py-0.5 text-xs ${GQ.muted} hover:underline`}>
          Cancel
        </button>
      </div>
    </div>
  )
}
