import { describe, expect, it } from 'vitest'
import { emptyNewProduct, newProductSchema } from './newProduct'

// What these pin: a create can't slip past the required attributes by being clicked before the
// chosen type's attributes have loaded

const valid = { ...emptyNewProduct, name: 'Tee', productTypeId: 'pt-tee' }

describe('newProductSchema', () => {
  it('waits for the chosen type\'s attributes instead of validating against none', () => {
    const schema = newProductSchema({ fallbackLocale: 'en', attributes: () => null })
    const result = schema.safeParse(valid)
    expect(result.success).toBe(false)
    expect(result.error?.issues[0]).toMatchObject({ path: ['productTypeId'], message: expect.stringMatching(/Still loading/) })
  })

  it('passes once the type is loaded and has nothing required', () => {
    const schema = newProductSchema({ fallbackLocale: 'en', attributes: () => [] })
    expect(schema.safeParse(valid).success).toBe(true)
  })
})
