// Overrides rather than appends - appending leaves a duplicate the render might read
// stale-first
export function buildForwardedCookie(
  existing: string | null,
  overrides: Record<string, string>,
): string {
  const kept = (existing ?? "")
    .split(";")
    .map((s) => s.trim())
    .filter(Boolean)
    .filter((pair) => {
      const eq = pair.indexOf("=");
      const name = (eq === -1 ? pair : pair.slice(0, eq)).trim();
      return !(name in overrides);
    });
  for (const [name, value] of Object.entries(overrides)) kept.push(`${name}=${value}`);
  return kept.join("; ");
}
