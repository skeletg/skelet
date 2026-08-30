// Guards a returnTo / redirect param down to a same-origin PATH. One shared copy so the
// rule can't drift between the login/register pages and the auth server actions
export function safeReturnTo(raw: unknown): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw;
  if (typeof value !== "string" || !value) return null;
  if (!value.startsWith("/")) return null;
  if (value.startsWith("//")) return null;
  if (value.includes("\\")) return null;
  return value;
}
