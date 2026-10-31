// The structured-data builder spec and its JSON-LD generator, used for the "edit as raw
// JSON" escape hatch - the operator picks a type + field toggles, and this produces the
// same shape the storefront builds from the spec at render
// SYNC: mirrors cmsSchemaFromSpec in packages/storefront-next/json-ld.ts

export type SchemaType = 'WebPage' | 'Article'

export type SchemaSpec = {
  type?: SchemaType
  image?: boolean
  datePublished?: boolean
  dateModified?: boolean
  author?: string
}

// The resolved page fields the spec draws from - passed in so this stays free of any
// page-shape or env coupling
export type SchemaPageFields = {
  name: string
  description?: string
  url: string
  image?: string
  publishedAt?: string | null
}

export function buildSchemaFromSpec(spec: SchemaSpec, f: SchemaPageFields): Record<string, unknown> {
  const image = spec.image ? f.image || undefined : undefined
  // publishedAt is the only date the page carries, so it stands in for both when toggled
  const datePublished = spec.datePublished ? f.publishedAt ?? undefined : undefined
  const dateModified = spec.dateModified ? f.publishedAt ?? undefined : undefined
  if (spec.type === 'Article') {
    const author = spec.author?.trim()
    return {
      '@context': 'https://schema.org',
      '@type': 'Article',
      '@id': f.url,
      headline: f.name,
      url: f.url,
      description: f.description || undefined,
      image,
      datePublished,
      dateModified,
      author: author ? { '@type': 'Person', name: author } : undefined,
    }
  }
  return {
    '@context': 'https://schema.org',
    '@type': 'WebPage',
    '@id': f.url,
    name: f.name,
    url: f.url,
    description: f.description || undefined,
    image,
    datePublished,
    dateModified,
  }
}
