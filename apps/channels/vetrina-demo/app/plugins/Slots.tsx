"use client";

import { getSlots } from "./registry";

// Renders whatever is registered for one zone, nothing at all when no plugin claims it. ctx is
// handed to each slot's render - see the zone conventions in registry.ts
export function Slots<TCtx = unknown>({ zone, ctx }: { zone: string; ctx?: TCtx }) {
  const entries = getSlots<TCtx>(zone);
  if (entries.length === 0) return null;
  return (
    <>
      {entries.map((s) => (
        <span key={`${s.zone}:${s.id}`} className="contents">
          {s.render(ctx as TCtx)}
        </span>
      ))}
    </>
  );
}
