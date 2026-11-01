// The visitor identity Lens tracking, session replay and Prism assignment all key on. They must agree on
// the cookie and on the hash - FNV-1a, the same as the Go backend - or replay and experiments drift
// from the visitor Lens counted

import { readCookie } from "../cookies";
import { randomId } from "../random-id";

export const AID_COOKIE = "lens_aid";

// The Lens anon id, minted and stored on first call - "" on the server
export function ensureAnonId(): string {
  if (typeof document === "undefined") return "";
  let id = readCookie(AID_COOKIE);
  if (!id) {
    id = randomId();
    document.cookie = `${AID_COOKIE}=${id}; max-age=${60 * 60 * 24 * 365}; path=/; samesite=lax`;
  }
  return id;
}

export function fnv1a(s: string): number {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

// Deterministic per-visitor 0-99 bucket, compared against the server's sample %
export function sampleBucket(id: string): number {
  return fnv1a(id) % 100;
}

export function viewportBucket(): string {
  const w = window.innerWidth;
  if (w < 768) return "mobile";
  if (w < 1024) return "tablet";
  return "desktop";
}
