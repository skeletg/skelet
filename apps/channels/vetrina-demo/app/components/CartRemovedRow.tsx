"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  UNDO_WINDOW_MS,
  useCartStore,
  type PendingRemoval,
} from "@/app/lib/cart/store";
import { displayName } from "@/app/lib/display";

const EXIT_MS = 220;

export function CartRemovedRow({
  pending,
  size = "md",
}: {
  pending: PendingRemoval;
  size?: "sm" | "md";
}) {
  const { undoRemove, dismissRemoval } = useCartStore();

  // Honour reduced-motion: keep the behaviour, drop the animation
  const [reduce] = useState(
    () =>
      typeof window !== "undefined" &&
      !!window.matchMedia?.("(prefers-reduced-motion: reduce)").matches,
  );

  const [entered, setEntered] = useState(false); // enter transition
  const [exiting, setExiting] = useState(false); // exit (collapse + fade)
  const [barPrimed, setBarPrimed] = useState(false); // countdown bar start
  const finalized = useRef(false); // one terminal action only

  const [remainingMs] = useState(() =>
    Math.max(0, pending.expiresAt - Date.now()),
  );
  const startPct = Math.min(100, (remainingMs / UNDO_WINDOW_MS) * 100);

  useEffect(() => {
    const raf = requestAnimationFrame(() => {
      setEntered(true);
      setBarPrimed(true);
    });
    return () => cancelAnimationFrame(raf);
  }, []);

  const finish = useCallback((action: () => void) => {
    if (finalized.current) return;
    finalized.current = true;
    setExiting(true);
    setTimeout(action, EXIT_MS);
  }, []);

  useEffect(() => {
    if (remainingMs <= 0) {
      dismissRemoval(pending.token);
      return;
    }
    const t = setTimeout(
      () => finish(() => dismissRemoval(pending.token)),
      remainingMs,
    );
    return () => clearTimeout(t);
  }, [pending.token, dismissRemoval, finish, remainingMs]);

  const name = displayName(pending.line.name, pending.line.sku);
  const imgSize = size === "sm" ? "size-12" : "size-14";
  const dur = reduce ? 0 : EXIT_MS;
  // Padding lives inside the collapsing wrapper (not on the parent <li>) so the row's
  // vertical space animates away too - neighbours slide up cleanly
  const padCls = size === "sm" ? "p-4" : "py-4";

  // Expired already (mounted after the deadline - e.g. the cart drawer was reopened too late): the effect above drops
  // it from the store; render nothing so no stale row flashes in
  if (remainingMs <= 0) return null;

  return (
    <div
      role="status"
      className="overflow-hidden"
      style={{
        maxHeight: exiting ? 0 : 240,
        opacity: exiting ? 0 : entered ? 1 : 0,
        transform:
          !reduce && entered && !exiting ? "translateY(0)" : "translateY(-6px)",
        transition: `max-height ${dur}ms ease, opacity ${dur}ms ease, transform ${dur}ms ease`,
      }}
    >
      <div className={padCls}>
      <div className="flex items-center gap-3 rounded-lg border border-dashed border-border bg-muted p-3">
        {pending.line.imageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={pending.line.imageUrl}
            alt={name}
            className={`${imgSize} shrink-0 rounded-lg border border-border object-cover opacity-50 grayscale`}
          />
        ) : (
          <div
            className={`${imgSize} flex shrink-0 items-center justify-center rounded-lg border border-border bg-muted text-[9px] text-muted-foreground`}
          >
            No image
          </div>
        )}

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm text-muted-foreground">
            Removed{" "}
            <span className="font-medium text-foreground">
              {name}
            </span>
          </p>
          {/* Countdown bar: depletes over the undo window. */}
          <div className="mt-2 h-1 w-full overflow-hidden rounded-full bg-border">
            <div
              className="h-full rounded-full bg-muted-foreground/60"
              style={{
                width: reduce ? "100%" : barPrimed ? "0%" : `${startPct}%`,
                transition: reduce ? "none" : `width ${remainingMs}ms linear`,
              }}
            />
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <button
            type="button"
            onClick={() => finish(() => undoRemove(pending.token))}
            className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
          >
            Undo
          </button>
          <button
            type="button"
            aria-label="Dismiss"
            onClick={() => finish(() => dismissRemoval(pending.token))}
            className="rounded-md p-1.5 text-muted-foreground transition-colors hover:text-foreground"
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              strokeLinecap="round"
              className="h-4 w-4"
              aria-hidden="true"
            >
              <path d="M6 6l12 12M18 6L6 18" />
            </svg>
          </button>
        </div>
      </div>
      </div>
    </div>
  );
}
