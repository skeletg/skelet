"use client";

import { Link, useRouter } from "@/app/components/LocaleLink";

import { useEffect, useRef } from "react";
import { useTranslations } from "next-intl";

import type { ProductSortOption } from "@skelet/storefront-next/catalog/read";
import { joinBasePathWithFilters } from "@skelet/storefront-core/filter-path";

export function SortSelect({
  options,
  currentSlug,
  basePath,
  pathFilters,
  currentQuery = "",
}: {
  options: ProductSortOption[];
  currentSlug: string;
  // Required for path-based callers (every listing page today)
  basePath?: string;
  pathFilters?: { slug: string; values: string[] }[];
  // The page's query string (the search term, query-string filters) - carried over so a sort refines the same results
  currentQuery?: string;
}) {
  const router = useRouter();
  const t = useTranslations("listing");
  const detailsRef = useRef<HTMLDetailsElement>(null);

  useEffect(() => {
    function onPointerDown(e: PointerEvent) {
      const el = detailsRef.current;
      if (!el || !el.open) return;
      if (e.target instanceof Node && el.contains(e.target)) return;
      el.open = false;
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key !== "Escape") return;
      const el = detailsRef.current;
      if (!el || !el.open) return;
      el.open = false;
      el.querySelector("summary")?.focus();
    }
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, []);

  if (options.length === 0 || !basePath) return null;

  const defaultSlug = options[0].slug;
  const effective =
    options.find((o) => o.slug === currentSlug) ?? options[0];

  // A new order starts from the first page
  const carried = new URLSearchParams(currentQuery);
  carried.delete("page");

  function hrefFor(slug: string): string {
    const next = slug === defaultSlug ? "" : slug;
    return joinBasePathWithFilters(basePath!, pathFilters ?? [], next, carried.toString());
  }

  return (
    <details
      ref={detailsRef}
      className="group relative [&_summary::-webkit-details-marker]:hidden"
    >
      <summary className="flex cursor-pointer list-none items-center gap-2 text-xs font-semibold uppercase tracking-wider text-foreground">
        <span className="text-muted-foreground normal-case tracking-normal font-normal">
          {t("sort")}
        </span>
        <span>{effective.label}</span>
        <svg
          aria-hidden="true"
          viewBox="0 0 12 12"
          className="size-3 shrink-0 transition-transform group-open:rotate-180"
        >
          <path
            d="M3 4.5l3 3 3-3"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </summary>

      {/* right-0 anchors to the trigger's right edge so the panel stays on-screen when narrow */}
      <div className="absolute right-0 z-10 mt-2 min-w-[14rem] overflow-hidden rounded-md border border-border bg-popover py-1 shadow-md">
        <ul className="grid">
          {options.map((o) => {
            const active = o.slug === effective.slug;
            // The default option is always crawlable: its URL has no /s/<slug> segment,
            // so it IS the canonical browse page
            const indexable = o.slug === defaultSlug || o.isIndexable;
            const href = hrefFor(o.slug);
            const className =
              "block w-full text-left px-3 py-1.5 text-xs " +
              (active
                ? "bg-accent font-medium text-accent-foreground"
                : "text-muted-foreground hover:bg-accent hover:text-accent-foreground");
            return (
              <li key={o.slug}>
                {indexable ? (
                  <Link
                    href={href}
                    scroll={false}
                    prefetch={false}
                    aria-current={active ? "true" : undefined}
                    className={className}
                  >
                    {o.label}
                  </Link>
                ) : (
                  <button
                    type="button"
                    onClick={() => router.push(href, { scroll: false })}
                    aria-current={active ? "true" : undefined}
                    className={className}
                  >
                    {o.label}
                  </button>
                )}
              </li>
            );
          })}
        </ul>
      </div>
    </details>
  );
}
