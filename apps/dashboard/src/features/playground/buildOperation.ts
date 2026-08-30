import {
  isEnumType,
  isListType,
  isNonNullType,
  isObjectType,
  isScalarType,
  isUnionType,
  type GraphQLField,
  type GraphQLNamedType,
  type GraphQLObjectType,
  type GraphQLOutputType,
} from 'graphql'

export type OperationKind = 'query' | 'mutation'

export type OperationEntry = {
  kind: OperationKind
  name: string
  description: string
  field: GraphQLField<unknown, unknown>
}

const MAX_DEPTH = 1 // the recursive catalog edges (linkedProducts, partOf) never terminate on their own

const MAX_FIELDS = 120 // FixedComplexityLimit rejects over 500 fields on admin and 300 on front, so an unbudgeted expansion opens a query that can't run

function namedType(type: GraphQLOutputType): GraphQLNamedType {
  let t: unknown = type
  while (isNonNullType(t) || isListType(t)) {
    t = (t as { ofType: unknown }).ofType
  }
  return t as GraphQLNamedType
}

function isLeaf(type: GraphQLOutputType): boolean {
  const named = namedType(type)
  return isScalarType(named) || isEnumType(named)
}

// Unions and interfaces need inline fragments to be valid, so they get __typename rather than a guess
function selectionSet(
  type: GraphQLOutputType,
  depth: number,
  indent: string,
  budget: { left: number },
): string {
  const named = namedType(type)
  if (isUnionType(named)) {
    return ` {\n${indent}  __typename\n${indent}}`
  }
  if (!isObjectType(named)) {
    return ''
  }
  const fields = Object.values((named as GraphQLObjectType).getFields())
  const lines: string[] = []
  for (const f of fields) {
    if (budget.left <= 0) break
    const leaf = isLeaf(f.type)
    if (!leaf && depth >= MAX_DEPTH) continue
    budget.left--
    const nested = leaf ? '' : selectionSet(f.type, depth + 1, `${indent}  `, budget)
    // Interfaces come back empty, and a field with no selection set is a syntax error - skip it
    if (!leaf && nested === '') {
      continue
    }
    lines.push(`${indent}  ${f.name}${nested}`)
  }
  if (lines.length === 0) {
    return ` {\n${indent}  __typename\n${indent}}`
  }
  return ` {\n${lines.join('\n')}\n${indent}}`
}

// Required args become variables so the operation runs the moment it opens - a defaulted arg counts as optional
export function buildOperation(entry: OperationEntry): { query: string; variables: string } {
  const required = entry.field.args.filter(
    (a) => isNonNullType(a.type) && a.defaultValue === undefined,
  )
  const varDefs = required.map((a) => `$${a.name}: ${a.type.toString()}`).join(', ')
  const argList = required.map((a) => `${a.name}: $${a.name}`).join(', ')
  const opName = entry.name.charAt(0).toUpperCase() + entry.name.slice(1)
  const head = `${entry.kind} ${opName}${varDefs ? `(${varDefs})` : ''} {`
  const budget = { left: MAX_FIELDS }
  const body = `  ${entry.name}${argList ? `(${argList})` : ''}${selectionSet(entry.field.type, 0, '  ', budget)}`
  const variables = required.length
    ? JSON.stringify(Object.fromEntries(required.map((a) => [a.name, null])), null, 2)
    : ''
  return { query: `${head}\n${body}\n}\n`, variables }
}

export function matchesTerm(entry: OperationEntry, term: string): boolean {
  const q = term.trim().toLowerCase()
  if (!q) return true
  return entry.name.toLowerCase().includes(q) || entry.description.toLowerCase().includes(q)
}
