import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SLOW, captureInput, renderDialog } from '@/test/page-harness'
import { AttributeFormDialog } from './AttributeFormDialog'
import { AttributesDocument, CreateAttributeDocument } from './Catalog.gql'
import { createAttributeSchema, toAttributeCreateInput, toAttributeValues } from './schemas/attribute'

const issuesOf = (values: Parameters<typeof createAttributeSchema.safeParse>[0]) =>
  createAttributeSchema.safeParse(values).error?.issues.map((i) => i.message) ?? []

describe('attribute schema', () => {
  const base = { ...toAttributeValues(), slug: 'size', name: 'Size' }

  it('refuses a required selectable attribute at create', () => {
    expect(issuesOf({ ...base, inputType: 'DROPDOWN', valueRequired: true })[0]).toMatch(/Required selectable attributes need a default/)
  })

  it('wants a usable default for a required scalar', () => {
    expect(issuesOf({ ...base, valueRequired: true })).toEqual(['Required attributes must have a default value.'])
    expect(issuesOf({ ...base, inputType: 'NUMERIC', hasDefault: true, defaultRaw: 'ten' })).toEqual(['Default value must be a number.'])
    expect(issuesOf({ ...base, hasDefault: true, defaultRaw: '  ' })).toEqual(['Default value cannot be empty.'])
  })

  it('sends a typed default', () => {
    expect(toAttributeCreateInput({ ...base, inputType: 'NUMERIC', hasDefault: true, defaultRaw: '42' }).defaultValue).toBe(42)
    expect(toAttributeCreateInput({ ...base, inputType: 'BOOLEAN', hasDefault: true, defaultBool: true }).defaultValue).toBe(true)
    expect(toAttributeCreateInput(base).defaultValue).toBeUndefined()
  })
})

describe('AttributeFormDialog', () => {
  it('holds the default on for a required attribute and creates with it', async () => {
    const onCreate = vi.fn()
    const onOpenChange = vi.fn()
    renderDialog(<AttributeFormDialog open onOpenChange={onOpenChange} />, [
      {
        request: { query: CreateAttributeDocument, variables: captureInput(onCreate) },
        result: { data: { createAttribute: { __typename: 'Attribute', id: 'a1' } } },
      },
      { request: { query: AttributesDocument }, result: { data: { attributes: [] } }, maxUsageCount: 3 },
    ])
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Material' } })
    fireEvent.click(screen.getByLabelText('Value required'))
    const toggle = screen.getByLabelText('Default value (required)')
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    expect((toggle as HTMLButtonElement).disabled).toBe(true)

    fireEvent.click(screen.getByRole('button', { name: 'Create attribute' }))
    expect(await screen.findByText('Default value cannot be empty.')).toBeDefined()
    expect(onCreate).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('Default value'), { target: { value: 'Cotton' } })
    fireEvent.click(screen.getByRole('button', { name: 'Create attribute' }))
    await vi.waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false), SLOW)
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ slug: 'material', inputType: 'PLAIN_TEXT', valueRequired: true, defaultValue: 'Cotton' }),
    )
  })
})
