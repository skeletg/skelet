export function isForbiddenError(error: { message?: string } | undefined | null): boolean {
  return !!error?.message && error.message.toLowerCase().includes('forbidden')
}
