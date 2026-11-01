// The core's cached reads - a channel's settings, its config - kept in this process under the hint's tags, for the hint's
// seconds or until a tag is invalidated, the way Next's data cache keeps them. A hit hands back a fresh Response

import type { CacheHint } from "@skelet/storefront-core/request";

type Answer = { body: string; status: number; contentType: string; cacheable: boolean };
type Entry = Answer & { expires: number; tags: string[] };

const entries = new Map<string, Entry>();
const pending = new Map<string, Promise<Answer>>();
const MAX_ENTRIES = 500;
// Bumped by every purge, so a fetch that started before one doesn't store what it got
let generation = 0;

// What makes two reads the same answer: the query and the request headers the backend varies it on. A shopper's own
// headers (their address, their visitor id) aren't among them, so every shopper shares the entry
const VARY = ["x-channel-id", "locale", "authorization", "x-skelet-cache-surface"];

function keyOf(url: string, init: RequestInit): string {
  const headers = new Headers(init.headers);
  return [url, typeof init.body === "string" ? init.body : "", ...VARY.map((h) => headers.get(h) ?? "")].join("\n");
}

const respond = (a: Answer) => new Response(a.body, { status: a.status, headers: { "Content-Type": a.contentType } });

export async function cachedFetch(url: string, init: RequestInit, hint: CacheHint): Promise<Response> {
  if (hint.revalidate === 0) return fetch(url, init);
  const key = keyOf(url, init);
  const hit = entries.get(key);
  if (hit && hit.expires > Date.now()) {
    entries.delete(key);
    entries.set(key, hit);
    return respond(hit);
  }
  let answer = pending.get(key);
  if (!answer) {
    const started = generation;
    answer = load(url, init)
      .then((a) => {
        if (a.cacheable && started === generation) store(key, a, hint);
        return a;
      })
      .finally(() => pending.delete(key));
    pending.set(key, answer);
  }
  return respond(await answer);
}

async function load(url: string, init: RequestInit): Promise<Answer> {
  const res = await fetch(url, init);
  const body = await res.text();
  let cacheable = res.ok;
  if (cacheable) {
    try {
      const parsed = JSON.parse(body) as { data?: unknown; errors?: unknown[] };
      cacheable = parsed.data != null && !parsed.errors?.length;
    } catch {
      cacheable = false;
    }
  }
  return { body, status: res.status, contentType: res.headers.get("Content-Type") ?? "application/json", cacheable };
}

function store(key: string, a: Answer, hint: CacheHint): void {
  entries.delete(key);
  if (entries.size >= MAX_ENTRIES) entries.delete(entries.keys().next().value!);
  entries.set(key, {
    ...a,
    expires: hint.revalidate ? Date.now() + hint.revalidate * 1000 : Number.POSITIVE_INFINITY,
    tags: hint.tags ?? [],
  });
}

// invalidateData drops every cached read under any of these tags - a revalidate route calls it with the backend's
export function invalidateData(tags: string[]): number {
  generation++;
  let dropped = 0;
  for (const [key, entry] of entries) {
    if (entry.tags.some((t) => tags.includes(t))) {
      entries.delete(key);
      dropped++;
    }
  }
  return dropped;
}
