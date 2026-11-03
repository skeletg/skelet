import type { BeaconPath } from "./paths";

// A browser call to a /t/ endpoint. The same-origin proxy passes the shopper's own IP, cookies and anon id through,
// but can't know the channel - the layout stamps it on <html data-channel-id>, or a caller that already has it passes it
export function beaconFetch(path: BeaconPath, { channelId, ...init }: RequestInit & { channelId?: string } = {}) {
  const headers = new Headers(init.headers);
  headers.set("X-Channel-Id", channelId ?? document.documentElement.dataset.channelId ?? "");
  return fetch(path, { ...init, headers });
}
