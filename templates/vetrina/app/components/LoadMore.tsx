"use client";

// The load-more control: a real anchor to ?page=N+1, so it works with no JS and crawlers can
// walk the whole chain even when the numbered pager is hidden - the server renders ?page=N
// cumulatively (items 1..N), so the click appends and scroll stays put via scroll={false}

import { useLinkStatus } from "next/link";
import { useTranslations } from "next-intl";

import { Link } from "@/app/components/LocaleLink";
import { hrefForPage } from "@/app/lib/pagination";

function LoadMoreLabel({ remaining }: { remaining: number }) {
  const { pending } = useLinkStatus();
  const t = useTranslations("listing");
  const tc = useTranslations("common");
  return (
    <span className={pending ? "opacity-60" : undefined}>
      {pending ? tc("loading") : t("loadMore", { count: remaining })}
    </span>
  );
}

export function LoadMore({
  basePath,
  preservedParams,
  currentPage,
  pageSize,
  shownCount,
  totalCount,
}: {
  basePath: string;
  preservedParams: Record<string, string>;
  currentPage: number;
  pageSize: number;
  shownCount: number;
  totalCount: number;
}) {
  const t = useTranslations("listing");
  if (shownCount >= totalCount) {
    return totalCount > 0 ? (
      <p className="mt-10 text-center text-xs text-muted-foreground" aria-live="polite">
        {t("showingAll", { count: totalCount })}
      </p>
    ) : null;
  }
  const remaining = Math.min(pageSize, totalCount - shownCount);
  return (
    <div className="mt-10 flex flex-col items-center gap-2">
      <Link
        href={hrefForPage(basePath, preservedParams, currentPage + 1)}
        rel="next"
        scroll={false}
        className="inline-flex h-11 items-center justify-center border border-foreground bg-background px-8 font-mono text-[13px] uppercase tracking-[0.14em] text-foreground transition-colors hover:bg-foreground hover:text-background"
      >
        <LoadMoreLabel remaining={remaining} />
      </Link>
      <p className="text-xs text-muted-foreground" aria-live="polite">
        {t("showingOf", { shown: shownCount, total: totalCount })}
      </p>
    </div>
  );
}
