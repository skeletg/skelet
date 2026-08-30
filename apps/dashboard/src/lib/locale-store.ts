let current = '' // module-scoped so apollo.ts can attach the `Locale` header without importing config-scope.tsx, which would cycle back through apollo.ts for resetStore()

export function setEffectiveLocale(locale: string): void {
  current = locale
}

export function getEffectiveLocale(): string {
  return current // '' means "let the server pick" - internal/locale.Middleware reads a missing or empty header as the system default
}
