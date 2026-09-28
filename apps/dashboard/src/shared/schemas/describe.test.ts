import { expect, it } from 'vitest'
import { z } from 'zod'
import { postSchema, type PostValues } from '@/features/blog/schemas/post'
import { termSchema } from '@/features/blog/schemas/term'
import { attributeSettingsSchema } from '@/features/catalog/schemas/attributeSettings'
import { collectionSchema, type CollectionValues } from '@/features/catalog/schemas/collection'
import { newCollectionSchema } from '@/features/catalog/schemas/newCollection'
import { newProductSchema } from '@/features/catalog/schemas/newProduct'
import { cmsAttributeDefaultsSchema } from '@/features/cms/schemas/cmsAttribute'
import { destinationSchema } from '@/features/destinations/schemas/destination'
import { orderAddressSchema, type OrderAddressValues } from '@/features/orders/schemas/editOrderAddress'

// Every form schema describes itself and each of its fields, so it can double as an AI tool schema
// (z.toJSONSchema carries the descriptions). A refine leaves nothing in the JSON Schema, which makes
// the description the only place a field's format and meaning survive

const modules = import.meta.glob<Record<string, unknown>>(['../../features/**/schemas/*.ts', './*.ts', '!**/*.test.ts'], {
  eager: true,
})

// Factories take what the form knows at runtime - the baselines are only read inside refinements,
// so an empty stand-in builds the same shape
const FACTORIES: Record<string, () => z.ZodType> = {
  postSchema: () => postSchema({} as PostValues),
  termSchema: () => termSchema({ nameRequired: false }),
  attributeSettingsSchema: () => attributeSettingsSchema('PLAIN_TEXT'),
  collectionSchema: () => collectionSchema({} as CollectionValues),
  newCollectionSchema: () => newCollectionSchema('en'),
  newProductSchema: () => newProductSchema({ fallbackLocale: 'en', attributes: () => [] }),
  cmsAttributeDefaultsSchema: () => cmsAttributeDefaultsSchema('TEXT'),
  destinationSchema: () => destinationSchema({ isEdit: false }),
  orderAddressSchema: () => orderAddressSchema({} as OrderAddressValues),
}

// Object fields without a description, looking through arrays, records and nullable/optional wrappers
function undescribedFields(schema: z.ZodType, path: string): string[] {
  const def = schema._zod.def
  switch (def.type) {
    case 'object':
      return Object.entries((schema as z.ZodObject).shape).flatMap(([key, field]) => [
        ...(field.description ? [] : [`${path}.${key}`]),
        ...undescribedFields(field, `${path}.${key}`),
      ])
    case 'array':
      return undescribedFields((def as z.core.$ZodArrayDef).element as z.ZodType, `${path}[]`)
    case 'record':
      return undescribedFields((def as z.core.$ZodRecordDef).valueType as z.ZodType, `${path}{}`)
    case 'nullable':
    case 'optional':
      return undescribedFields((def as z.core.$ZodOptionalDef).innerType as z.ZodType, path)
    default:
      return []
  }
}

function exportedSchemas(): [string, z.ZodType][] {
  return Object.entries(modules).flatMap(([file, mod]) =>
    Object.entries(mod).flatMap(([name, value]): [string, z.ZodType][] => {
      if (value instanceof z.ZodType) return [[`${file} ${name}`, value]]
      if (typeof value === 'function' && name.endsWith('Schema')) {
        const build = FACTORIES[name]
        if (!build) throw new Error(`${file} ${name} is a schema factory - add it to FACTORIES so its descriptions are checked`)
        return [[`${file} ${name}()`, build()]]
      }
      return []
    }),
  )
}

it('finds the form schemas', () => {
  expect(exportedSchemas().length).toBeGreaterThan(50)
})

it('describes every form schema and each of its fields', () => {
  const missing = exportedSchemas().flatMap(([name, schema]) => [
    ...(schema.description ? [] : [`${name} (the schema itself)`]),
    ...undescribedFields(schema, name),
  ])
  expect(missing).toEqual([])
})
