// Where "contact us" mail goes. The backend has no support-address field - not on the
// Channel type, not in publicConfig - so the store's own public URL is the closest thing
// to a real one, with an env override for stores whose inbox lives elsewhere
// Pure on purpose: no server imports, so it stays unit-testable

export function supportEmailFrom(
  publicUrl: string | null | undefined,
  override?: string | null,
): string | null {
  const explicit = (override ?? "").trim();
  if (explicit) return explicit;
  if (!publicUrl) return null;
  try {
    const host = new URL(publicUrl).hostname.replace(/^www\./i, "");
    return host ? `support@${host}` : null;
  } catch {
    return null;
  }
}
