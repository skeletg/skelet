// Next says "this route can't be static" by throwing from cookies(), headers() and
// no-store fetches. Swallow that and the route stays static, then 500s on every request
export function rethrowIfDynamicBailout(err: unknown): void {
  const digest = (err as { digest?: unknown } | null | undefined)?.digest;
  if (typeof digest === "string" && digest.startsWith("DYNAMIC_SERVER_USAGE")) throw err;
}
