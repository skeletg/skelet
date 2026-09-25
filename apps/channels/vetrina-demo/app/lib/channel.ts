// The request's channel, decided from its host - no imports, since proxy.ts runs this and can't pull in server-only modules

// The request header the proxy stamps with the resolved channel, overwriting whatever the client sent
export const CHANNEL_HEADER = "x-storefront-channel";

// Lets the revalidate route warm a page as a given channel - honoured only beside the revalidate secret
export const WARM_CHANNEL_HEADER = "x-storefront-warm-channel";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The one [channel] value the build renders, so Next sees which routes read the request; no host or warm can reach it
export const BUILD_PROBE_CHANNEL = "00000000-0000-0000-0000-000000000000";

export function isChannelId(v: string | null | undefined): v is string {
  return !!v && UUID.test(v) && v !== BUILD_PROBE_CHANNEL;
}

export function parseChannelHosts(raw: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const part of raw.split(",")) {
    const [host, id] = part.split("=").map((s) => s?.trim() ?? "");
    const h = hostOf(host);
    if (h && isChannelId(id)) out.set(h, id.toLowerCase());
  }
  return out;
}

export function hostOf(hostHeader: string | null | undefined): string {
  let h = (hostHeader ?? "").trim().toLowerCase();
  if (h.startsWith("[")) {
    const end = h.indexOf("]");
    return end > 0 ? h.slice(0, end + 1) : "";
  }
  const colon = h.indexOf(":");
  if (colon >= 0) h = h.slice(0, colon);
  return h;
}

const PINNED = (process.env.STOREFRONT_CHANNEL_ID ?? "").trim().toLowerCase();
export const PINNED_CHANNEL_ID = isChannelId(PINNED) ? PINNED : "";
const HOSTS = parseChannelHosts(process.env.STOREFRONT_CHANNEL_HOSTS ?? "");

// A mapped host wins, the pin answers every other host, and with neither the host serves no channel
export function channelForHost(
  host: string,
  hosts: Map<string, string> = HOSTS,
  pinned: string = PINNED_CHANNEL_ID,
): string {
  return hosts.get(hostOf(host)) ?? pinned;
}
