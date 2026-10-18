import { describe, expect, it } from 'vitest'
import { methodSchema, toMethodInput, toMethodValues, type ExistingMethod } from './method'

const express: ExistingMethod = {
  id: 'm1',
  code: 'express',
  name: 'Express',
  calcType: 'FLAT',
  flatCents: 1200,
  freeAboveCents: 0,
  sortOrder: 1,
  isActive: true,
  deliveryKind: 'ship_to_address',
  unitsPerCharge: 0,
  estimatedDays: 1,
  deliversOnWeekends: true,
}

describe('method schema', () => {
  it('round-trips delivery days and the weekend rule', () => {
    const input = toMethodInput(toMethodValues(express))
    expect(input.estimatedDays).toBe(1)
    expect(input.deliversOnWeekends).toBe(true)
  })

  it('reads a method saved before estimates as no estimate on weekdays', () => {
    const older: ExistingMethod = { ...express, estimatedDays: undefined, deliversOnWeekends: undefined }
    const values = toMethodValues(older)
    expect(values.estimatedDays).toBe(0)
    expect(values.deliversOnWeekends).toBe(false)
    expect(toMethodValues().estimatedDays).toBe(0)
  })

  it('refuses negative or fractional days', () => {
    const values = toMethodValues(express)
    expect(methodSchema.safeParse({ ...values, estimatedDays: -1 }).success).toBe(false)
    expect(methodSchema.safeParse({ ...values, estimatedDays: 1.5 }).success).toBe(false)
    expect(methodSchema.safeParse({ ...values, estimatedDays: 0 }).success).toBe(true)
  })
})
