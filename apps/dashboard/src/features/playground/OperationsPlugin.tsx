import { useDeferredValue, useMemo, useState } from 'react'
import { useGraphiQL, useGraphiQLActions } from '@graphiql/react'
import type { GraphQLObjectType } from 'graphql'
import { ListIcon, SearchIcon } from 'lucide-react'
import {
  buildOperation,
  matchesTerm,
  type OperationEntry,
  type OperationKind,
} from './buildOperation'

export function OperationsPluginContent() {
  const schema = useGraphiQL((state) => state.schema)
  // updateActiveTabValues only writes tab STATE - the editors are separate Monaco models, so
  // without setting them too the new tab opens blank
  const queryEditor = useGraphiQL((state) => state.queryEditor)
  const variableEditor = useGraphiQL((state) => state.variableEditor)
  const { addTab, updateActiveTabValues } = useGraphiQLActions()
  const [term, setTerm] = useState('')
  // The list is every root field on the schema - filtering it on each keystroke is the slow part
  const deferred = useDeferredValue(term)

  const entries = useMemo<OperationEntry[]>(() => {
    if (!schema) return []
    const collect = (type: GraphQLObjectType | null | undefined, kind: OperationKind): OperationEntry[] =>
      type
        ? Object.values(type.getFields()).map((field) => ({
            kind,
            name: field.name,
            description: field.description ?? '',
            field,
          }))
        : []
    return [
      ...collect(schema.getQueryType(), 'query'),
      ...collect(schema.getMutationType(), 'mutation'),
    ]
  }, [schema])

  const matches = useMemo(
    () => entries.filter((e) => matchesTerm(e, deferred)),
    [entries, deferred],
  )

  function open(entry: OperationEntry) {
    const { query, variables } = buildOperation(entry)
    // A new tab, so clicking through several operations doesn't overwrite the one being edited
    addTab()
    // addTab swaps the editors to the (empty) new tab asynchronously - writing in the same tick
    // gets overwritten by that swap, so land after it
    requestAnimationFrame(() => {
      queryEditor?.setValue(query)
      variableEditor?.setValue(variables)
      updateActiveTabValues({ query, variables })
    })
  }

  if (!schema) {
    return (
      <div className="p-3 text-sm text-muted-foreground">
        No schema loaded. Introspection needs the <code>developer.playground</code> permission.
      </div>
    )
  }

  const queries = matches.filter((e) => e.kind === 'query')
  const mutations = matches.filter((e) => e.kind === 'mutation')

  return (
    <div className="flex h-full flex-col gap-3 overflow-hidden">
      <div className="relative">
        <SearchIcon className="absolute left-2 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <input
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          placeholder="Search operations"
          aria-label="Search operations"
          className="w-full rounded-md border bg-transparent py-1.5 pl-8 pr-2 text-sm outline-none focus:ring-1 focus:ring-ring"
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {matches.length} of {entries.length} operations
      </p>
      <div className="flex-1 space-y-4 overflow-y-auto">
        {[
          { title: 'Queries', items: queries },
          { title: 'Mutations', items: mutations },
        ].map(({ title, items }) =>
          items.length === 0 ? null : (
            <section key={title}>
              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {title} ({items.length})
              </h3>
              <ul className="divide-y rounded-md border">
                {items.map((entry) => (
                  <li key={`${entry.kind}.${entry.name}`}>
                    <button
                      type="button"
                      onClick={() => open(entry)}
                      className="w-full px-2 py-1.5 text-left hover:bg-accent"
                    >
                      <span className="block font-mono text-xs">{entry.name}</span>
                      {entry.description && (
                        <span className="mt-0.5 block line-clamp-2 text-xs text-muted-foreground">
                          {entry.description}
                        </span>
                      )}
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          ),
        )}
        {matches.length === 0 && (
          <p className="text-sm text-muted-foreground">No operation matches “{deferred}”.</p>
        )}
      </div>
    </div>
  )
}

export function OperationsPluginIcon() {
  return <ListIcon className="size-4" />
}
