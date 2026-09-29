import {
  getNamedType,
  isInputObjectType,
  isListType,
  isNonNullType,
  isObjectType,
  isScalarType,
  Kind,
  TypeInfo,
  visit,
  visitWithTypeInfo,
  type ASTNode,
  type DocumentNode,
  type GraphQLField,
  type GraphQLInputType,
  type GraphQLNamedType,
  type GraphQLObjectType,
  type GraphQLSchema,
  type OperationDefinitionNode,
} from 'graphql'

// A place in the variables JSON that takes a record id, and which type that id belongs to
export type ValueSlot = {
  path: string[] // ['input', 'cartId'] for $input.cartId
  targetType: string | null
  valueField: string // what gets written - the id, or the sku a storefront addToCart takes
  isList: boolean
  required: boolean
}

export type Lister = {
  typeName: string
  field: string
  viaItems: boolean // the list sits under a page type's `items`
  channelArg: { type: string; required: boolean } | null
  searchArg: 'search' | null
  limitArg: boolean
  labelField: string | null
  detailField: string | null
}

export type PickOption = { id: string; value: string; label: string; detail: string | null }

export type PickRequest = {
  query: string
  variables: Record<string, unknown>
  admin?: boolean // the storefront API has no channel list, so channels always come from the admin one
  channelId?: string | null
}
export type PickRunner = (req: PickRequest) => Promise<{ data?: unknown; errors?: { message: string }[] }>

const VERB =
  /^(create|update|delete|remove|add|set|get|archive|restore|publish|unpublish|duplicate|cancel|approve|reject|toggle|move|reorder|assign|unassign|clear|reset|apply|run|send|mark|close|reopen|merge|upsert|attach|detach)(?=[A-Z])/
const LABEL_FIELDS = ['name', 'title', 'label', 'displayName', 'email', 'number', 'orderNumber', 'code', 'sku', 'slug', 'key', 'question', 'subject']
const DETAIL_FIELDS = ['sku', 'slug', 'email', 'code', 'status']
const MAX_INPUT_DEPTH = 3

function isEntity(t: GraphQLNamedType | null | undefined): t is GraphQLObjectType {
  return !!t && isObjectType(t) && 'id' in t.getFields()
}

function hasList(schema: GraphQLSchema, typeName: string) {
  return listerIndex(schema).has(typeName)
}

function entityByStem(schema: GraphQLSchema, stem: string): string | null {
  if (!stem) return null
  const lower = stem.toLowerCase()
  const types = Object.values(schema.getTypeMap()).filter(isEntity)
  const exact = types.find((t) => t.name.toLowerCase() === lower)
  if (exact) return exact.name
  // pageId is a CMSPage and postId a BlogPost - take a suffix match only when exactly one listable type fits
  const suffix = types.filter((t) => t.name.toLowerCase().endsWith(lower) && hasList(schema, t.name))
  return suffix.length === 1 ? suffix[0].name : null
}

function fieldEntity(schema: GraphQLSchema, field: GraphQLField<unknown, unknown>): string | null {
  const ret = getNamedType(field.type)
  if (isEntity(ret) && !ret.name.endsWith('Payload')) return ret.name
  return entityByStem(schema, field.name.replace(VERB, ''))
}

export function resolveTarget(schema: GraphQLSchema, field: GraphQLField<unknown, unknown>, name: string): string | null {
  if (name === 'id' || name === 'parentId') return fieldEntity(schema, field)
  const m = /^(.*?)Ids?$/.exec(name)
  return m ? entityByStem(schema, m[1]) : null
}

const KEY_SUFFIX = /^(.*?)(Sku|Slug|Code)s?$/

// A string argument that names a record by one of its keys - productSku, categorySlug - and only when that type really has the key
function resolveKeyTarget(schema: GraphQLSchema, field: GraphQLField<unknown, unknown>, name: string): { type: string; key: string } | null {
  const m = KEY_SUFFIX.exec(name) ?? (name === 'sku' ? ['', 'product', 'Sku'] : name === 'slug' || name === 'code' ? ['', '', name] : null)
  if (!m) return null
  if (!m[1] && name !== 'sku' && VERB.test(field.name)) return null // a bare slug on createCategory is the new record's own, not a lookup of an existing one
  const key = m[2].toLowerCase()
  const type = m[1] ? entityByStem(schema, m[1]) : fieldEntity(schema, field)
  const t = type ? schema.getType(type) : null
  return type && isObjectType(t) && key in t.getFields() ? { type, key } : null
}

export function slotsForInput(
  schema: GraphQLSchema,
  field: GraphQLField<unknown, unknown>,
  name: string,
  type: GraphQLInputType,
  path: string[],
  depth: number,
): ValueSlot[] {
  const named = getNamedType(type)
  const inner = isNonNullType(type) ? type.ofType : type
  if (isScalarType(named) && named.name === 'ID') {
    return [{ path, targetType: resolveTarget(schema, field, name), valueField: 'id', isList: isListType(inner), required: isNonNullType(type) }]
  }
  if (isScalarType(named) && named.name === 'String') {
    const keyed = resolveKeyTarget(schema, field, name)
    return keyed && listerIndex(schema).has(keyed.type)
      ? [{ path, targetType: keyed.type, valueField: keyed.key, isList: isListType(inner), required: isNonNullType(type) }]
      : []
  }
  if (!isInputObjectType(named) || depth >= MAX_INPUT_DEPTH || isListType(isNonNullType(type) ? type.ofType : type)) return []
  return Object.values(named.getFields()).flatMap((f) => slotsForInput(schema, field, f.name, f.type, [...path, f.name], depth + 1))
}

export function pickOperation(document: DocumentNode | null | undefined, operationName?: string | null): OperationDefinitionNode | null {
  const ops = (document?.definitions ?? []).filter((d): d is OperationDefinitionNode => d.kind === Kind.OPERATION_DEFINITION)
  return ops.find((o) => o.name?.value === operationName) ?? ops[0] ?? null
}

// Every variable, or field of an input-object variable, that carries an id - found by where the operation passes it, since the variable's own type only says ID
export function findValueSlots(schema: GraphQLSchema, operation: OperationDefinitionNode | null): ValueSlot[] {
  if (!operation) return []
  const typeInfo = new TypeInfo(schema)
  const slots = new Map<string, ValueSlot>()
  visit(
    operation,
    visitWithTypeInfo(typeInfo, {
      Variable(node, _key, parent) {
        const field = typeInfo.getFieldDef()
        const type = typeInfo.getInputType()
        const owner = parent as ASTNode | undefined
        if (!field || !type || !owner || (owner.kind !== Kind.ARGUMENT && owner.kind !== Kind.OBJECT_FIELD)) return
        for (const slot of slotsForInput(schema, field, owner.name.value, type, [node.name.value], 0)) {
          slots.set(slot.path.join('.'), slot)
        }
      },
    }),
  )
  return [...slots.values()]
}

const listerCache = new WeakMap<GraphQLSchema, Map<string, Lister>>()

function scalarField(t: GraphQLObjectType, candidates: string[], except?: string | null): string | null {
  const fields = t.getFields()
  return candidates.find((c) => c !== except && c in fields && isScalarType(getNamedType(fields[c].type))) ?? null
}

// Root queries that list one entity type with nothing required but an optional channel - built once per schema
export function listerIndex(schema: GraphQLSchema): Map<string, Lister> {
  const cached = listerCache.get(schema)
  if (cached) return cached
  const index = new Map<string, Lister>()
  listerCache.set(schema, index)
  for (const field of Object.values(schema.getQueryType()?.getFields() ?? {})) {
    if (field.args.some((a) => isNonNullType(a.type) && a.name !== 'channelId')) continue
    const ret = isNonNullType(field.type) ? field.type.ofType : field.type
    let entity: GraphQLObjectType | null = null
    let viaItems = false
    if (isListType(ret) && isEntity(getNamedType(ret))) entity = getNamedType(ret) as GraphQLObjectType
    else {
      const named = getNamedType(ret)
      const items = isObjectType(named) ? named.getFields().items : undefined
      if (items && isListType(isNonNullType(items.type) ? items.type.ofType : items.type) && isEntity(getNamedType(items.type))) {
        entity = getNamedType(items.type) as GraphQLObjectType
        viaItems = true
      }
    }
    if (!entity) continue
    const existing = index.get(entity.name)
    const plural = `${entity.name[0].toLowerCase()}${entity.name.slice(1)}s`
    if (existing && existing.field.toLowerCase() === plural.toLowerCase()) continue
    if (existing && field.name.toLowerCase() !== plural.toLowerCase()) continue
    const channel = field.args.find((a) => a.name === 'channelId')
    const labelField = scalarField(entity, LABEL_FIELDS)
    index.set(entity.name, {
      typeName: entity.name,
      field: field.name,
      viaItems,
      channelArg: channel ? { type: String(channel.type), required: isNonNullType(channel.type) } : null,
      searchArg: field.args.some((a) => a.name === 'search' && getNamedType(a.type).name === 'String') ? 'search' : null,
      limitArg: field.args.some((a) => a.name === 'limit'),
      labelField,
      detailField: scalarField(entity, DETAIL_FIELDS, labelField),
    })
  }
  return index
}

export const PICK_LIMIT = 50

export function buildListQuery(l: Lister, opts: { search?: string; channelId?: string | null; valueField?: string }) {
  const defs: string[] = []
  const args: string[] = []
  const variables: Record<string, unknown> = {}
  if (l.channelArg && opts.channelId) {
    defs.push(`$channelId: ${l.channelArg.type}`)
    args.push('channelId: $channelId')
    variables.channelId = opts.channelId
  }
  if (l.searchArg && opts.search) {
    defs.push('$search: String')
    args.push('search: $search')
    variables.search = opts.search
  }
  if (l.limitArg) args.push(`limit: ${PICK_LIMIT}`)
  const selection = [...new Set(['id', opts.valueField ?? 'id', l.labelField, l.detailField].filter(Boolean))].join(' ')
  const inner = l.viaItems ? `items { ${selection} }` : selection
  const query = `query PlaygroundPick${defs.length ? `(${defs.join(', ')})` : ''} { ${l.field}${args.length ? `(${args.join(', ')})` : ''} { ${inner} } }`
  return { query, variables }
}

export function readListResult(l: Lister, data: unknown, valueField = 'id'): PickOption[] {
  const root = (data as Record<string, unknown> | null)?.[l.field]
  const rows = (l.viaItems ? (root as { items?: unknown } | null)?.items : root) ?? []
  if (!Array.isArray(rows)) return []
  return rows.map((r: Record<string, unknown>) => ({
    id: String(r.id),
    value: String(r[valueField] ?? r.id),
    label: l.labelField && r[l.labelField] != null ? String(r[l.labelField]) : String(r.id),
    detail: l.detailField && r[l.detailField] != null ? String(r[l.detailField]) : null,
  }))
}

// Client-side filter for listers without a search argument
export function matchesPick(o: PickOption, term: string) {
  const t = term.trim().toLowerCase()
  return !t || [o.label, o.id, o.value, o.detail].some((v) => !!v && v.toLowerCase().includes(t))
}

// Sets one path in the variables JSON, keeping everything else - list slots toggle membership
export function writeVariable(text: string, slot: Pick<ValueSlot, 'path' | 'isList'>, value: string): string {
  const parsed: unknown = text.trim() ? JSON.parse(text) : {}
  const root = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? (parsed as Record<string, unknown>) : {}
  let node = root
  for (const key of slot.path.slice(0, -1)) {
    const next = node[key]
    node[key] = next && typeof next === 'object' && !Array.isArray(next) ? next : {}
    node = node[key] as Record<string, unknown>
  }
  const last = slot.path[slot.path.length - 1]
  if (slot.isList) {
    const current = Array.isArray(node[last]) ? (node[last] as unknown[]) : []
    node[last] = current.includes(value) ? current.filter((v) => v !== value) : [...current, value]
  } else {
    node[last] = value
  }
  return JSON.stringify(root, null, 2)
}

export function readVariable(text: string | null | undefined, path: string[]): unknown {
  try {
    let node: unknown = text?.trim() ? JSON.parse(text) : {}
    for (const key of path) node = (node as Record<string, unknown> | null)?.[key]
    return node
  } catch {
    return undefined
  }
}
