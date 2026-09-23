import type { AnyFieldApi } from '@tanstack/react-form'

// Errors only show once the field is touched - the schema runs on every change across the whole
// form, and flagging "Name is required" while the admin is still typing the city reads as noise.
// Submitting touches every field, so a rejected save still lights up everything that's wrong
export function fieldErrorText(field: AnyFieldApi): string | null {
  if (!field.state.meta.isTouched) return null
  const first: unknown = field.state.meta.errors[0]
  if (!first) return null
  if (typeof first === 'string') return first
  if (typeof first === 'object' && 'message' in first) return String(first.message)
  return null
}
