// Never render a slug as a display name - it means the entity has no translation, and a nicer slug just hides that.
// Returns MISSING_NAME and warns so an admin can grep Canonical call: displayName(metaTitle ?? name, slug)

export const MISSING_NAME = "(unnamed)";

export function displayName(
  value: string | null | undefined,
  debugId: string,
): string {
  if (value && value.length > 0) return value;
  // Match the leak surface used elsewhere in the storefront: warn with a stable prefix
  // Stable prefix so log aggregators can pattern-match
   
  console.warn(`[displayName] missing translation for "${debugId}"`);
  return MISSING_NAME;
}
