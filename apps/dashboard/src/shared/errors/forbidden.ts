// Split from ChannelScopeError so that file exports only components - fast refresh needs it

// Sniffs the API's `forbidden` guard off a GraphQL error chain, so a detail page can show the
// permissions state instead of the generic load-error panel - auth.ErrForbidden serializes literally
export function isForbiddenError(error: { message?: string } | undefined | null): boolean {
  return !!error?.message && error.message.toLowerCase().includes('forbidden')
}
