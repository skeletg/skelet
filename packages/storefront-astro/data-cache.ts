// The core's cached reads - a channel's settings, its config - kept in this process under the hint's tags, for the hint's
// seconds or until a tag is invalidated, the way Next's data cache keeps them. A hit hands back a fresh Response

import type { CacheHint } from "@skelet/storefront-core/request";

type Entry = { body: string; status: number; contentType: string; expires: number; tags: string[] };

const entries = new Map<string, Entry>();
const MAX_ENTRIES = 500;

// What makes two reads the same answer: the query and the request headers the backend varies it on. A shopper's own
// headers (their address, their visitor id) aren't among them, so every shopper shares the entry
const VARY = ["x-channel-id", "locale", "authorization", "x-skelet-cache-surface"];

function keyOf(url: string, init: RequestInit): string {
  const headers = new Headers(init.headers);
  return [url, typeof init.body === "string" ? init.body : "", ...VARY.map((h) => headers.get(h) ?? "")].join("\n");
}

export async function cachedFetch(url: string, init: RequestInit, hint: CacheHint): Promise<Response> {
  const key = keyOf(url, init);
  const hit = entries.get(key);
  if (hit && hit.expires > Date.now()) {
    return new Response(hit.body, { status: hit.status, headers: { "Content-Type": hit.contentType } });
  }
  const res = await fetch(url, init);
  if (!res.ok) return res;
  const body = await res.text();
  const contentType = res.headers.get("Content-Type") ?? "application/json";
  if (entries.size >= MAX_ENTRIES) entries.delete(entries.keys().next().value!);
  entries.delete(key);
  entries.set(key, {
    body,
    status: res.status,
    contentType,
    expires: hint.revalidate ? Date.now() + hint.revalidate * 1000 : Number.POSITIVE_INFINITY,
    tags: hint.tags ?? [],
  });
  return new Response(body, { status: res.status, headers: { "Content-Type": contentType } });
}

// invalidateData drops every cached read under any of these tags - a revalidate route calls it with the backend's
export function invalidateData(tags: string[]): number {
  let dropped = 0;
  for (const [key, entry] of entries) {
    if (entry.tags.some((t) => tags.includes(t))) {
      entries.delete(key);
      dropped++;
    }
  }
  return dropped;
}
