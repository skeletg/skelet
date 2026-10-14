import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AnyFormApi } from '@tanstack/react-form'
import { useAppForm } from './form'

let api: AnyFormApi

const valueOf = (el: HTMLElement) => (el as HTMLInputElement).value

function Harness({ phone = '', country }: { phone?: string; country?: string }) {
  const form = useAppForm({ defaultValues: { phone } })
  api = form
  return <form.AppField name="phone">{(f) => <f.PhoneField label="Phone" country={country} />}</form.AppField>
}

// Each keystroke is an input event with the caret at the end - how typing at the end of the box reaches onChange
function typeInto(input: HTMLElement, text: string) {
  for (const ch of text) {
    const el = input as HTMLInputElement
    fireEvent.input(el, { target: { value: el.value + ch }, inputType: 'insertText' })
  }
}

describe('PhoneField', () => {
  it('formats as it is typed, in the country it is given, and stages what it shows', () => {
    render(<Harness country="US" />)
    typeInto(screen.getByLabelText('Phone'), '2015550123')
    expect(valueOf(screen.getByLabelText('Phone'))).toBe('(201) 555-0123')
    expect(api.state.values.phone).toBe('(201) 555-0123')
  })

  it('shows a stored number internationally without staging a change', () => {
    render(<Harness phone="+381641234567" />)
    expect(valueOf(screen.getByLabelText('Phone'))).toBe('+381 64 1234567')
    expect(api.state.values.phone).toBe('+381641234567')
    expect(api.state.isDirty).toBe(false)
  })

  it('leaves a deletion as the admin made it', () => {
    render(<Harness country="US" />)
    const input = screen.getByLabelText('Phone')
    typeInto(input, '201')
    expect(valueOf(input)).toBe('(201)')
    fireEvent.input(input, { target: { value: '(201' }, inputType: 'deleteContentBackward' })
    expect(valueOf(input)).toBe('(201')
  })

  it('is a tel input with the phone autofill hint', () => {
    render(<Harness />)
    const input = screen.getByLabelText('Phone')
    expect(input.getAttribute('type')).toBe('tel')
    expect(input.getAttribute('autocomplete')).toBe('tel')
  })
})
