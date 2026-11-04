import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { renderDialog } from '@/test/page-harness'
import { SynapseOverviewDocument } from '@/features/synapse/Synapse.gql'
import type { ReportDefinition } from '../report'
import { DEFAULT_DEFINITIONS } from '../report'
import { cleanDefinition, describeReport, isNetSales } from '../report'
import { dropOffFlow, moveStep, removeStep } from './funnel'
import { switchRetentionView } from './retention'
import { ReportControls, suggestTitle } from '.'

const noEvents = { request: { query: SynapseOverviewDocument, variables: () => true }, error: new Error('none'), maxUsageCount: 5 }

function controls(value: ReportDefinition) {
  const onChange = vi.fn<(d: ReportDefinition) => void>()
  renderDialog(<ReportControls channelId="c1" value={value} onChange={onChange} />, [noEvents])
  return onChange
}

describe('funnel steps', () => {
  const d: ReportDefinition = { kind: 'funnel', steps: ['a', 'b', 'c'], filters: [{ step: 0, key: 'device', value: 'mobile' }, { step: 2, key: 'sku', value: 'X' }] }
  it('carry their filters when they move', () => {
    expect(moveStep(d, 0, 1)).toMatchObject({ steps: ['b', 'a', 'c'], filters: [{ step: 1, key: 'device' }, { step: 2, key: 'sku' }] })
  })
  it('take their filters with them and renumber the later ones when removed', () => {
    expect(removeStep(d, 0)).toMatchObject({ steps: ['b', 'c'], filters: [{ step: 1, key: 'sku' }] })
  })
  it("don't move past either end", () => {
    expect(moveStep(d, 2, 3)).toBe(d)
  })
})

describe('the Retention builder', () => {
  it('carries the event across when switching between its two views', () => {
    const rate = switchRetentionView({ kind: 'retention', startEvent: 'order_placed', returnEvent: 'order_placed', unit: 'week', size: 'full' }, 'stickiness')
    expect(rate).toEqual({ kind: 'stickiness', event: 'order_placed', size: 'full' })
    expect(switchRetentionView(rate, 'retention')).toMatchObject({ kind: 'retention', startEvent: 'order_placed', returnEvent: 'order_placed', size: 'full' })
  })
  it('switches view from its toggle, and says which view is on', () => {
    const onChange = controls(DEFAULT_DEFINITIONS.retention)
    expect(screen.getByRole('button', { name: 'By cohort' }).getAttribute('aria-pressed')).toBe('true')
    expect(screen.getByRole('button', { name: 'Return rate' }).getAttribute('aria-pressed')).toBe('false')
    fireEvent.click(screen.getByRole('button', { name: 'Return rate' }))
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ kind: 'stickiness', event: 'order_placed' }))
  })
  it('keeps the return window a return-rate report was built with', () => {
    const onChange = controls({ kind: 'stickiness' })
    fireEvent.change(screen.getByRole('combobox', { name: 'Return window' }), { target: { value: '14' } })
    expect(onChange).toHaveBeenCalledWith({ kind: 'stickiness', windowDays: 14 })
  })
})

describe('each builder hands back the definition its engine reads', () => {
  it('insights: measure, breakdown and unit', () => {
    const onChange = controls(DEFAULT_DEFINITIONS.insights)
    fireEvent.click(screen.getByRole('button', { name: 'Unique' }))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'insights', measure: 'unique' }))
    fireEvent.change(screen.getByRole('combobox', { name: 'Break down by' }), { target: { value: 'device' } })
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ breakdown: 'device' }))
    fireEvent.click(screen.getByRole('button', { name: 'Week' }))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ unit: 'week' }))
  })
  it('funnel: a step filter names the step it holds', () => {
    const onChange = controls(DEFAULT_DEFINITIONS.funnel)
    fireEvent.click(screen.getByRole('button', { name: 'Filter step 3' }))
    fireEvent.change(screen.getByRole('combobox', { name: 'Property for step 3' }), { target: { value: 'sku' } })
    fireEvent.change(screen.getByRole('combobox', { name: 'Operator for step 3' }), { target: { value: 'in' } })
    fireEvent.change(screen.getByRole('textbox', { name: 'Value for step 3' }), { target: { value: 'TSHIRT, MUG' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add filter' }))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ filters: [{ step: 2, key: 'sku', value: 'TSHIRT', op: 'in', values: ['TSHIRT', 'MUG'] }] }))
  })
  it('funnel: a conversion window', () => {
    const onChange = controls(DEFAULT_DEFINITIONS.funnel)
    fireEvent.change(screen.getByRole('combobox', { name: 'Conversion window' }), { target: { value: '24' } })
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ windowHours: 24 }))
  })
  it('retention by cohort: the unit sets how many periods it reads', () => {
    const onChange = controls(DEFAULT_DEFINITIONS.retention)
    fireEvent.click(screen.getByRole('button', { name: 'Day' }))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ unit: 'day', periods: 14 }))
  })
})

describe('suggestTitle', () => {
  it('names a report from what it counts', () => {
    expect(suggestTitle(DEFAULT_DEFINITIONS.funnel)).toBe('Page view → View item → Added to cart → Begin checkout → Order placed')
    expect(suggestTitle({ kind: 'stickiness' })).toBe('Returning shoppers')
    expect(suggestTitle(DEFAULT_DEFINITIONS.retention)).toBe('Order placed retention')
  })
})

describe('conditions', () => {
  it('build a where list from the property picker, the operator and the values', () => {
    const onChange = controls({ kind: 'stickiness' })
    fireEvent.click(screen.getByRole('button', { name: 'Add a condition' }))
    expect(onChange).toHaveBeenLastCalledWith({ kind: 'stickiness', where: [{ key: 'sku', op: 'eq', values: [''] }] })
  })
  it("don't run or save until they have a value", () => {
    const d = cleanDefinition({ kind: 'insights', where: [{ key: 'sku', op: 'eq', values: [''] }, { key: 'device', op: 'in', values: [' mobile ', ''] }] })
    expect(d.where).toEqual([{ key: 'device', op: 'in', values: ['mobile'] }])
  })
  it('read in a report\'s one-line description', () => {
    expect(describeReport({ kind: 'insights', event: 'view_item', where: [{ key: 'sku', op: 'eq', values: ['X'] }] })).toBe('Count of view_item where sku = X')
  })
})

describe('net sales', () => {
  it('is one choice in the event list that sets the events and the sum', () => {
    const onChange = controls(DEFAULT_DEFINITIONS.insights)
    fireEvent.change(screen.getByRole('combobox', { name: 'Event' }), { target: { value: '__net_sales' } })
    const d = onChange.mock.lastCall![0]
    expect(isNetSales(d)).toBe(true)
    expect(describeReport(d)).toBe('Net sales')
  })
})

describe('a funnel drop-off', () => {
  it("opens a flow from the step people dropped off at, without the ones who went on, held to the step's filters", () => {
    const f: ReportDefinition = { kind: 'funnel', steps: ['view_item', 'cart_add', 'order_placed'], windowHours: 24, filters: [{ step: 0, key: 'sku', value: 'X' }] }
    expect(dropOffFlow(f, 0)).toEqual({ kind: 'flow', event: 'view_item', depth: 4, notThen: 'cart_add', notThenWithinHours: 24, where: [{ key: 'sku', op: 'eq', values: ['X'] }] })
    expect(dropOffFlow(f, 1)).toMatchObject({ event: 'cart_add', notThen: 'order_placed', where: undefined })
  })
})
