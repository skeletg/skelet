// The client address an upstream proxy recorded, passed on verbatim. The backend walks the X-Forwarded-For chain
// against TRUSTED_PROXIES itself, so the storefront neither trims nor trusts it - it only makes sure it isn't lost
export function clientIP(inbound: Headers): string {
  return inbound.get("x-forwarded-for") ?? inbound.get("x-real-ip") ?? "";
}
