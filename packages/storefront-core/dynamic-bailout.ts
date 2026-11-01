// Next says "this route can't be static" by throwing from its cookie and header reads and no-store fetches. Swallow that
// and the route stays static, then 500s on every request - a framework that throws nothing of the kind never trips it
export function rethrowIfDynamicBailout(err: unknown): void {
  const digest = (err as { digest?: unknown } | null | undefined)?.digest;
  if (typeof digest === "string" && digest.startsWith("DYNAMIC_SERVER_USAGE")) throw err;
}
