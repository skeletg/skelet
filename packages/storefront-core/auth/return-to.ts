// A browser drops a tab or newline inside a URL and reads a backslash as a slash, so the check resolves it the way a browser would
const UNSAFE = /[\u0000- \u007f\\]/;
const HERE = "http://storefront.invalid";

export function safeReturnTo(raw: unknown): string | null {
  const value = Array.isArray(raw) ? raw[0] : raw;
  if (typeof value !== "string" || !value.startsWith("/") || UNSAFE.test(value)) return null;
  try {
    return new URL(value, HERE).origin === HERE ? value : null;
  } catch {
    return null;
  }
}
