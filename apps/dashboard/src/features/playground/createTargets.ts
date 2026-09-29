import {
  getNamedType,
  isEnumType,
  isInputObjectType,
  isListType,
  isNonNullType,
  isObjectType,
  isScalarType,
  type GraphQLField,
  type GraphQLInputType,
  type GraphQLObjectType,
  type GraphQLSchema,
} from 'graphql'
import { slotsForInput } from './valueTargets'

export type Creator = {
  typeName: string
  field: string
  viaField: string | null // the payload field holding the new record, when the mutation returns a payload
  requiredArgs: { name: string; type: string }[]
  labelField: string | null
}

const CREATE_VERB = /^(create|add|start|new|open|register)(?=[A-Z])/
const LABEL_FIELDS = ['name', 'title', 'label', 'email', 'number', 'code', 'sku', 'slug']

function hasId(t: unknown): t is GraphQLObjectType {
  return isObjectType(t) && 'id' in t.getFields()
}

function createdType(field: GraphQLField<unknown, unknown>): { type: GraphQLObjectType; via: string | null } | null {
  const ret = getNamedType(field.type)
  if (hasId(ret)) return { type: ret, via: null }
  if (!isObjectType(ret)) return null
  for (const f of Object.values(ret.getFields())) {
    const inner = getNamedType(f.type)
    if (hasId(inner) && !isListType(isNonNullType(f.type) ? f.type.ofType : f.type)) return { type: inner, via: f.name }
  }
  return null
}

// addToCart returns a Cart but needs one - a mutation that requires an id of the very type it returns adds to a record, it doesn't make one
function needsItsOwnType(schema: GraphQLSchema, field: GraphQLField<unknown, unknown>, typeName: string) {
  return field.args.some(
    (a) =>
      isNonNullType(a.type) &&
      slotsForInput(schema, field, a.name, a.type, [a.name], 0).some((s) => s.required && s.valueField === 'id' && s.targetType === typeName),
  )
}

const creatorCache = new WeakMap<GraphQLSchema, Map<string, Creator[]>>()

// Mutations that make a new record of a type, best first: create-named, then the fewest required arguments
export function creatorIndex(schema: GraphQLSchema): Map<string, Creator[]> {
  const cached = creatorCache.get(schema)
  if (cached) return cached
  const index = new Map<string, Creator[]>()
  creatorCache.set(schema, index)
  for (const field of Object.values(schema.getMutationType()?.getFields() ?? {})) {
    if (!CREATE_VERB.test(field.name)) continue
    const made = createdType(field)
    if (!made || needsItsOwnType(schema, field, made.type.name)) continue
    const fields = made.type.getFields()
    const creator: Creator = {
      typeName: made.type.name,
      field: field.name,
      viaField: made.via,
      requiredArgs: field.args.filter((a) => isNonNullType(a.type)).map((a) => ({ name: a.name, type: String(a.type) })),
      labelField: LABEL_FIELDS.find((f) => f in fields && isScalarType(getNamedType(fields[f].type))) ?? null,
    }
    index.set(made.type.name, [...(index.get(made.type.name) ?? []), creator])
  }
  for (const list of index.values()) {
    list.sort((a, b) => Number(!a.field.startsWith('create')) - Number(!b.field.startsWith('create')) || a.requiredArgs.length - b.requiredArgs.length)
  }
  return index
}

export function buildCreateOperation(c: Creator): string {
  const defs = c.requiredArgs.map((a) => `$${a.name}: ${a.type}`).join(', ')
  const args = c.requiredArgs.map((a) => `${a.name}: $${a.name}`).join(', ')
  const selection = ['id', c.labelField].filter(Boolean).join(' ')
  const body = c.viaField ? `${c.viaField} { ${selection} }` : selection
  return `mutation PlaygroundCreate${defs ? `(${defs})` : ''} { ${c.field}${args ? `(${args})` : ''} { ${body} } }`
}

function placeholder(type: GraphQLInputType, depth: number): unknown {
  const inner = isNonNullType(type) ? type.ofType : type
  if (isListType(inner)) return []
  const named = getNamedType(type)
  if (isEnumType(named)) return named.getValues()[0]?.value ?? null
  if (isInputObjectType(named)) {
    if (depth >= 3) return {}
    const out: Record<string, unknown> = {}
    for (const f of Object.values(named.getFields())) {
      if (isNonNullType(f.type) && f.defaultValue === undefined) out[f.name] = placeholder(f.type, depth + 1)
    }
    return out
  }
  switch (named.name) {
    case 'Int':
    case 'Float':
      return 0
    case 'Boolean':
      return false
    default:
      return ''
  }
}

// Every required argument, and every required field inside input objects, stubbed by type - what's left to fill in is visible at a glance
export function skeletonVariables(schema: GraphQLSchema, c: Creator): Record<string, unknown> {
  const field = schema.getMutationType()?.getFields()[c.field]
  const out: Record<string, unknown> = {}
  for (const a of field?.args ?? []) {
    if (isNonNullType(a.type)) out[a.name] = placeholder(a.type, 0)
  }
  return out
}

export function readCreated(c: Creator, data: unknown): { id: string; label: string | null } | null {
  const root = (data as Record<string, unknown> | null)?.[c.field] as Record<string, unknown> | null | undefined
  const rec = (c.viaField ? root?.[c.viaField] : root) as Record<string, unknown> | null | undefined
  if (!rec || rec.id == null) return null
  return { id: String(rec.id), label: c.labelField && rec[c.labelField] != null ? String(rec[c.labelField]) : null }
}
