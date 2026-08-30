"use client";

import { Link, useRouter } from "@/app/components/LocaleLink";

import { useEffect, useRef, useState, useTransition } from "react";
import { createPortal } from "react-dom";

import { displayName } from "@/app/lib/display";
import { productHref } from "@/app/lib/entity-href";
import type { ProductSearchHit } from "@/app/lib/search/read";
import { searchProductsAction } from "@/app/lib/search/actions";

const DEBOUNCE_MS = 180;

// Duration must match the CSS transition on the backdrop + dialog so the modal stays
// mounted until the fade-out finishes
const TRANSITION_MS = 200;

export function SearchModal() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<ProductSearchHit[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [pending, startTransition] = useTransition();
  const inputRef = useRef<HTMLInputElement>(null);
  const router = useRouter();
  // Mount guard so createPortal doesn't run during SSR. Mirrors the CartSidebar's pattern - the heuristic flags the
  // effect, but it's the canonical "wait for client takeover" use
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMounted(true);
  }, []);

  // Mount-then-animate: `rendered` keeps the portal alive across the fade-out, `visible`
  // flips a frame later so the CSS transition has a from-state to animate off
  const [rendered, setRendered] = useState(false);
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setRendered(true);
      const id = window.requestAnimationFrame(() => setVisible(true));
      return () => window.cancelAnimationFrame(id);
    }
    setVisible(false);
    const t = window.setTimeout(() => setRendered(false), TRANSITION_MS);
    return () => window.clearTimeout(t);
  }, [open]);

  // Escape closes; Cmd/Ctrl-K opens (or focuses) the modal so power users don't have to
  // mouse over
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen(true);
      } else if (e.key === "Escape" && open) {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const t = setTimeout(() => inputRef.current?.focus(), 0);
    return () => {
      document.body.style.overflow = prev;
      clearTimeout(t);
    };
  }, [open]);

  const trimmedQuery = query.trim();
  // An emptied box shows nothing without the effect having to clear the last response -
  // the hits stay in state, they just stop being the answer to the current query
  const visibleResults = trimmedQuery ? results : [];
  const visibleTotal = trimmedQuery ? totalCount : 0;

  useEffect(() => {
    const q = query.trim();
    if (!q) return;
    const id = setTimeout(() => {
      startTransition(async () => {
        const res = await searchProductsAction(q);
        setResults(res.items);
        setTotalCount(res.totalCount);
      });
    }, DEBOUNCE_MS);
    return () => clearTimeout(id);
  }, [query]);

  const close = () => {
    setOpen(false);
    setQuery("");
    setResults([]);
    setTotalCount(0);
  };

  const submit = () => {
    const q = query.trim();
    if (!q) return;
    router.push(`/search?q=${encodeURIComponent(q)}`);
    close();
  };

  return (
    <>
      {/* Looks like an input so the affordance reads "type to search", but it opens the modal */}
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label="Search products"
        title="Search (⌘K)"
        className="flex h-9 items-center gap-2 rounded-md border border-border bg-background px-2.5 text-sm text-muted-foreground transition hover:border-foreground/40 sm:w-64"
      >
        <SearchIcon className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="hidden min-w-0 flex-1 truncate text-left sm:inline">
          Search products…
        </span>
        <kbd className="hidden rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground lg:inline">
          ⌘K
        </kbd>
      </button>

      {mounted &&
        rendered &&
        createPortal(
          <>
            <div
              onClick={close}
              aria-hidden="true"
              className={[
                // Matches dialog.drawer::backdrop in globals.css - the cart and filter drawers
                // barely tint the page, and a heavier scrim here made search feel like a page
                "fixed inset-0 z-40 bg-black/10 backdrop-blur-[2px] transition-opacity ease-out",
                visible ? "opacity-100" : "opacity-0",
              ].join(" ")}
              style={{ transitionDuration: `${TRANSITION_MS}ms` }}
            />
            <div
              role="dialog"
              aria-modal="true"
              aria-label="Search products"
              className={[
                // Fullscreen sheet below sm, floating panel from sm up
                "glass-strong fixed z-50 shadow-2xl transition-[opacity,transform] ease-out",
                "max-sm:inset-0 max-sm:flex max-sm:flex-col max-sm:rounded-none",
                "sm:left-1/2 sm:top-20 sm:w-[92vw] sm:max-w-xl sm:rounded-2xl",
                visible
                  ? "opacity-100 sm:-translate-x-1/2 translate-y-0"
                  : "opacity-0 sm:-translate-x-1/2 -translate-y-2",
              ].join(" ")}
              style={{ transitionDuration: `${TRANSITION_MS}ms` }}
            >
              <div className="flex items-center gap-3 border-b border-border px-4 py-3">
                <SearchIcon className="h-4 w-4 shrink-0 text-muted-foreground" />
                <input
                  ref={inputRef}
                  type="search"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      submit();
                    }
                  }}
                  placeholder="Search products…"
                  aria-label="Search products"
                  autoComplete="off"
                  className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                />
                {pending && <Spinner className="h-4 w-4 text-muted-foreground" />}
                <kbd className="hidden rounded border border-border px-1.5 py-0.5 text-[10px] text-muted-foreground sm:inline">
                  ESC
                </kbd>
                {/* Fullscreen below sm hides the click-away backdrop and phones have no ESC,
                    so the sheet needs its own close */}
                <button
                  type="button"
                  onClick={close}
                  aria-label="Close search"
                  className="grid size-8 shrink-0 place-items-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground sm:hidden"
                >
                  <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth={2}
                    strokeLinecap="round"
                    className="size-4"
                    aria-hidden="true"
                  >
                    <path d="M18 6 6 18M6 6l12 12" />
                  </svg>
                </button>
              </div>

              <div className="overflow-y-auto p-2 max-sm:min-h-0 max-sm:flex-1 sm:max-h-[60vh]">
                {trimmedQuery === "" && (
                  <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                    Start typing to search products.
                  </p>
                )}
                {trimmedQuery !== "" && !pending && visibleResults.length === 0 && (
                  <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                    No products match “{trimmedQuery}”.
                  </p>
                )}
                {visibleResults.length > 0 && (
                  <ul>
                    {visibleResults.map((p) => (
                      <li key={p.id}>
                        <Link
                          href={productHref(p)}
                          onClick={close}
                          className="flex flex-col rounded-lg px-3 py-2 transition hover:bg-accent"
                        >
                          <span className="text-sm font-medium">
                            {displayName(p.name, p.sku)}
                          </span>
                          <span className="text-xs text-muted-foreground">
                            {p.sku}
                          </span>
                        </Link>
                      </li>
                    ))}
                  </ul>
                )}
                {visibleTotal > visibleResults.length && (
                  <Link
                    href={`/search?q=${encodeURIComponent(trimmedQuery)}`}
                    onClick={close}
                    className="mt-2 flex items-center justify-center rounded-lg px-3 py-2 text-xs text-muted-foreground transition hover:bg-accent"
                  >
                    View all {visibleTotal} results →
                  </Link>
                )}
              </div>
            </div>
          </>,
          document.body,
        )}
    </>
  );
}

function SearchIcon({ className }: { className?: string } = {}) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className ?? "h-5 w-5"}
      aria-hidden="true"
    >
      <circle cx="11" cy="11" r="7" />
      <path d="M21 21l-4.3-4.3" />
    </svg>
  );
}

function Spinner({ className }: { className?: string }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.5}
      strokeLinecap="round"
      className={`animate-spin ${className ?? "h-4 w-4"}`}
      aria-hidden="true"
    >
      <path d="M12 3a9 9 0 1 0 9 9" />
    </svg>
  );
}
