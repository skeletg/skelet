// The client address an upstream proxy recorded, passed on verbatim. The backend walks the X-Forwarded-For chain
// against TRUSTED_PROXIES itself, so the storefront neither trims nor trusts it - it only makes sure it isn't lost
export function clientIP(inbound: { get(name: string): string | null }): string {
  return inbound.get("x-forwarded-for") ?? inbound.get("x-real-ip") ?? "";
}

export const CLIENT_IP_HEADER = "X-Skelet-Client-IP";
export const PROXY_KEY_HEADER = "X-Skelet-Proxy-Key";

export function forwardedClient(inbound: { get(name: string): string | null }): Record<string, string> {
  const chain = clientIP(inbound);
  if (!chain) return {};
  const out: Record<string, string> = { "X-Forwarded-For": chain };
  const key = (process.env.TRUSTED_PROXY_KEY ?? "").trim();
  const recordedByTheProxyInFront = chain.split(",").at(-1)?.trim() ?? "";
  if (key && recordedByTheProxyInFront) {
    out[CLIENT_IP_HEADER] = recordedByTheProxyInFront;
    out[PROXY_KEY_HEADER] = key;
  }
  return out;
}
