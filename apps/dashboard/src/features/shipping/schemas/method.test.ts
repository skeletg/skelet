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

  it('stores a weight table in grams and cents, read back in kilograms and dollars', () => {
    const table: ExistingMethod = {
      ...express,
      calcType: 'TABLE',
      tableBasis: 'WEIGHT',
      rateTiers: [
        { from: 0, priceCents: 490, notOffered: false },
        { from: 2500, priceCents: 890, notOffered: false },
        { from: 30000, priceCents: 0, notOffered: true },
      ],
    }
    const values = toMethodValues(table)
    expect(values.rateTiers).toEqual([
      { from: '0', priceDollars: '4.90', notOffered: false },
      { from: '2.5', priceDollars: '8.90', notOffered: false },
      { from: '30', priceDollars: '', notOffered: true },
    ])
    expect(toMethodInput(values).rateTiers).toEqual([
      { from: 0, priceCents: 490, notOffered: false },
      { from: 2500, priceCents: 890, notOffered: false },
      { from: 30000, priceCents: 0, notOffered: true },
    ])
  })

  it('reads subtotal tiers in dollars and quantity tiers in items', () => {
    const subtotal = toMethodInput({ ...toMethodValues(express), calcType: 'TABLE', tableBasis: 'SUBTOTAL', rateTiers: [{ from: '50', priceDollars: '0', notOffered: false }] })
    expect(subtotal.rateTiers).toEqual([{ from: 5000, priceCents: 0, notOffered: false }])
    const quantity = toMethodInput({ ...toMethodValues(express), calcType: 'TABLE', tableBasis: 'QUANTITY', rateTiers: [{ from: '3', priceDollars: '6', notOffered: false }] })
    expect(quantity.rateTiers).toEqual([{ from: 3, priceCents: 600, notOffered: false }])
  })

  it('needs a tier for a table and refuses two starting at the same place', () => {
    const values = { ...toMethodValues(express), calcType: 'TABLE' as const, tableBasis: 'WEIGHT' as const }
    expect(methodSchema.safeParse({ ...values, rateTiers: [] }).success).toBe(false)
    const tier = (from: string, priceDollars: string, notOffered = false) => ({ from, priceDollars, notOffered })
    expect(methodSchema.safeParse({ ...values, rateTiers: [tier('1', '5'), tier('1.000', '6')] }).success).toBe(false)
    expect(methodSchema.safeParse({ ...values, rateTiers: [tier('0', '5')] }).success).toBe(true)
    // a method that isn't a table sends no tiers, whatever the form still holds
    expect(toMethodInput({ ...values, calcType: 'FLAT', rateTiers: [tier('0', '5')] }).rateTiers).toEqual([])
  })

  it('takes a not-offered ceiling but never a table that offers nothing', () => {
    const values = { ...toMethodValues(express), calcType: 'TABLE' as const, tableBasis: 'WEIGHT' as const }
    const tier = (from: string, priceDollars: string, notOffered = false) => ({ from, priceDollars, notOffered })
    expect(methodSchema.safeParse({ ...values, rateTiers: [tier('0', '5'), tier('30', '', true)] }).success).toBe(true)
    expect(methodSchema.safeParse({ ...values, rateTiers: [tier('0', '', true)] }).success).toBe(false)
    // an offered tier still needs its price, 0 for free
    expect(methodSchema.safeParse({ ...values, rateTiers: [tier('0', '')] }).success).toBe(false)
  })

  it('counts items in whole numbers', () => {
    const values = { ...toMethodValues(express), calcType: 'TABLE' as const, tableBasis: 'QUANTITY' as const }
    expect(methodSchema.safeParse({ ...values, rateTiers: [{ from: '2.5', priceDollars: '5', notOffered: false }] }).success).toBe(false)
    expect(methodSchema.safeParse({ ...values, rateTiers: [{ from: '2', priceDollars: '5', notOffered: false }] }).success).toBe(true)
  })
})
