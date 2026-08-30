/* eslint-disable react-refresh/only-export-components -- vitest-only module, never hot-reloaded */
import type { ReactNode } from 'react'

// Vitest-only stand-in for @lingui/react(/macro) and @lingui/core/macro - tests assert the
// ENGLISH source strings, so the stub renders messages verbatim: no babel macro transform in the
// vitest pipeline, no I18nProvider required in every render. Wired via resolve.alias in
// vitest.config.ts; the real app never sees this module

function joinTemplate(strings: TemplateStringsArray | string, values: unknown[]): string {
  if (typeof strings === 'string') return strings
  return strings
    .map((s, i) => s + (i < values.length ? String(values[i]) : ''))
    .join('')
}

export function Trans(props: { children?: ReactNode; message?: string }) {
  return <>{props.children ?? props.message}</>
}

export function I18nProvider({ children }: { children: ReactNode }) {
  return <>{children}</>
}

type Descriptorish = { message?: string; id?: string } | string

const identity = {
  t: (strings: TemplateStringsArray | string, ...values: unknown[]) => joinTemplate(strings, values),
  _: (d: Descriptorish) => (typeof d === 'string' ? d : (d.message ?? d.id ?? '')),
  i18n: { locale: 'en' },
}

export function useLingui() {
  return identity
}

export function msg(strings: TemplateStringsArray, ...values: unknown[]) {
  const s = joinTemplate(strings, values)
  return { id: s, message: s }
}

export function plural(n: number, forms: { one: string; other: string }): string {
  return (n === 1 ? forms.one : forms.other).replace('#', String(n))
}

export const t = identity.t
