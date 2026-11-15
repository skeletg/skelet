// Plugin fields on core detail pages, each one a metadata key of the record. They edit the page's own metadata, so the
// header Save, its before-and-after dialog and the unsaved-work guard cover them with no wiring of their own

export type DetailFieldType = 'text' | 'textarea' | 'number' | 'checkbox' | 'select' | 'date'

export type DetailFieldEntry = {
  id: string // conventionally '<plugin-name>:field' - duplicates throw at registration
  target: string // the page, the same names <PluginSections> uses: 'product', 'customer', 'country'...
  key: string // the metadata key the value lives under - a checkbox stores 'true', and an empty value removes the key
  label: string
  hint?: string
  type?: DetailFieldType // default 'text'
  options?: { value: string; label: string }[]
  order?: number // lower renders earlier, default 100
}

const fields: DetailFieldEntry[] = []

export function registerDetailField(entry: DetailFieldEntry) {
  if (fields.some((f) => f.id === entry.id)) throw new Error(`detail-fields: duplicate id "${entry.id}"`)
  if (!entry.key.trim()) throw new Error(`detail-fields: "${entry.id}" needs a metadata key`)
  if (fields.some((f) => f.target === entry.target && f.key === entry.key)) {
    throw new Error(`detail-fields: "${entry.target}" already has a field on "${entry.key}"`)
  }
  if (entry.type === 'select' && !entry.options?.length) throw new Error(`detail-fields: the select "${entry.id}" needs options`)
  fields.push(entry)
}

export function getDetailFields(target: string | undefined): DetailFieldEntry[] {
  if (!target) return []
  return fields.filter((f) => f.target === target).sort((a, b) => (a.order ?? 100) - (b.order ?? 100))
}

export function detailFieldFor(target: string | undefined, key: string): DetailFieldEntry | undefined {
  return target ? fields.find((f) => f.target === target && f.key === key) : undefined
}

export function detailFieldText(field: DetailFieldEntry, raw: string): string {
  if (field.type === 'checkbox') return raw === 'true' ? 'Yes' : 'No'
  if (field.type === 'select') return field.options?.find((o) => o.value === raw)?.label ?? raw
  return raw
}

export function __resetDetailFieldsForTests() {
  fields.length = 0
}
