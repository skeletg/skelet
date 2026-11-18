import { renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useRecordExtensions } from './extensions'

describe('a plugin part of a record save', () => {
  it('rides the record mutation, clears only once that went through, and goes alone when the record had nothing', async () => {
    const send = vi.fn(async () => {})
    const { result } = renderHook(() => useRecordExtensions(send))
    const ext = result.current
    const saved = vi.fn()

    ext.stage('acme:1', { plugin: 'acme', value: { months: 24 }, key: '24', onSaved: saved })
    const taken = ext.take()
    expect(taken?.parts).toEqual({ acme: { months: 24 } })
    // The record's mutation failed - nothing called saved, so the part goes with the next try
    expect(ext.take()?.parts).toEqual({ acme: { months: 24 } })

    // It went through - the plugin's own section, saving after the record's, has nothing left to send
    taken?.saved()
    expect(saved).toHaveBeenCalledOnce()
    expect(ext.take()).toBeNull()
    await ext.flush()
    expect(send).not.toHaveBeenCalled()

    // Only a plugin part changed - it goes out alone through the same mutation
    ext.stage('acme:1', { plugin: 'acme', value: { months: 36 }, key: '36' })
    await ext.flush()
    expect(send).toHaveBeenCalledWith({ acme: { months: 36 } })
    expect(ext.take()).toBeNull()

    // Changed back to what was saved before - a change against the last save, so it goes again
    ext.stage('acme:1', { plugin: 'acme', value: { months: 24 }, key: '24' })
    expect(ext.take()?.parts).toEqual({ acme: { months: 24 } })
  })

  it('joins the parts of two sections of one plugin, and drops only the one whose section went away', () => {
    const { result } = renderHook(() => useRecordExtensions(async () => {}))
    const ext = result.current
    ext.stage('acme:warranty', { plugin: 'acme', value: { months: 24 }, key: 'a' })
    ext.stage('acme:notes', { plugin: 'acme', value: { note: 'fragile' }, key: 'b' })
    ext.stage('erp:sync', { plugin: 'erp', value: true, key: 'c' })
    expect(ext.take()?.parts).toEqual({ acme: { months: 24, note: 'fragile' }, erp: true })

    ext.stage('acme:notes', null)
    expect(ext.take()?.parts).toEqual({ acme: { months: 24 }, erp: true })
  })
})
