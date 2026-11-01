// In production Next runs the proxy again on the path its own rewrite pointed at, carrying the request headers the first
// pass set - so a value only this process knows tells that pass from a browser naming the /<channel>/ tree. No imports
const HEADER = "x-storefront-rewrite";
const SECRET = crypto.randomUUID();

export function markRewrite(headers: Headers): void {
  headers.set(HEADER, SECRET);
}

export function clearRewriteMark(headers: Headers): void {
  headers.delete(HEADER);
}

export function isOwnRewrite(headers: Headers): boolean {
  const got = headers.get(HEADER) ?? "";
  if (got.length !== SECRET.length) return false;
  let diff = 0;
  for (let i = 0; i < SECRET.length; i++) diff |= SECRET.charCodeAt(i) ^ got.charCodeAt(i);
  return diff === 0;
}
