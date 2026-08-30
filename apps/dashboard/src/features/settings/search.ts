import type { ConfigFieldData } from '@/features/settings/ConfigField'

// One matcher for the tree and the pane, so the sidebar can never disagree with what's listed
// A query matches anything visible about a field - label, the hint in its trailing paren, section,
// group, path, current value, option labels - since searching labels alone missed "remote" when that
// was a field's VALUE - Terms are ANDed, so "sales guest" narrows rather than widens

export function searchTerms(query: string): string[] {
  return query.trim().toLowerCase().split(/\s+/).filter(Boolean)
}

function fieldHaystack(field: ConfigFieldData, sectionLabel: string, groupLabel: string): string {
  const parts = [field.label, field.path, sectionLabel, groupLabel]
  // Secrets never leave the server, so there is nothing to search but the label
  if (field.type !== 'SECRET') parts.push(field.value)
  for (const option of field.options ?? []) parts.push(option.label)
  return parts.join(' ').toLowerCase()
}

export function fieldMatches(
  field: ConfigFieldData,
  terms: string[],
  labels: { section: string; group: string },
): boolean {
  if (terms.length === 0) return true
  const haystack = fieldHaystack(field, labels.section, labels.group)
  return terms.every((t) => haystack.includes(t))
}

/** A whole group can answer a query like "checkout" through its own name */
export function labelMatches(label: string, terms: string[]): boolean {
  if (terms.length === 0) return true
  const lower = label.toLowerCase()
  return terms.every((t) => lower.includes(t))
}
