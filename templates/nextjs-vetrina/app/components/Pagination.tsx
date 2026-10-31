// Listing pagination

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";
import { hrefForPage } from "@skelet/storefront-next/pagination";

type Props = {
  totalCount: number;
  pageSize: number;
  currentPage: number;
  basePath: string;
  preservedParams: Record<string, string>;
};

const PAGE_WINDOW = 2; // pages on each side of the current page

export function Pagination({
  totalCount,
  pageSize,
  currentPage,
  basePath,
  preservedParams,
}: Props) {
  const t = useTranslations("listing");
  const totalPages = Math.max(1, Math.ceil(totalCount / pageSize));
  if (totalPages <= 1) return null;

  const pages: (number | "gap")[] = [];
  const push = (n: number | "gap") => {
    if (n === "gap") {
      if (pages[pages.length - 1] !== "gap") pages.push("gap");
    } else if (!pages.includes(n)) {
      pages.push(n);
    }
  };

  push(1);
  if (currentPage - PAGE_WINDOW > 2) push("gap");
  for (
    let p = Math.max(2, currentPage - PAGE_WINDOW);
    p <= Math.min(totalPages - 1, currentPage + PAGE_WINDOW);
    p++
  ) {
    push(p);
  }
  if (currentPage + PAGE_WINDOW < totalPages - 1) push("gap");
  if (totalPages > 1) push(totalPages);

  const prevPage = currentPage > 1 ? currentPage - 1 : null;
  const nextPage = currentPage < totalPages ? currentPage + 1 : null;

  const baseBtn =
    "inline-flex h-9 min-w-9 items-center justify-center rounded-md border border-border bg-card px-3 text-sm";
  const hoverBtn = "hover:bg-accent";
  const activeBtn =
    "border-foreground bg-foreground text-background";

  return (
    <nav
      aria-label={t("pagination")}
      className="mt-10 flex flex-wrap items-center justify-center gap-1"
    >
      {prevPage ? (
        <Link
          href={hrefForPage(basePath, preservedParams, prevPage)}
          rel="prev"
          className={`${baseBtn} ${hoverBtn}`}
        >
          {t("prev")}
        </Link>
      ) : (
        <span
          aria-disabled="true"
          className={`${baseBtn} opacity-40`}
        >
          {t("prev")}
        </span>
      )}

      {pages.map((p, i) =>
        p === "gap" ? (
          <span
            key={`gap-${i}`}
            className="px-2 text-sm text-muted-foreground"
          >
            …
          </span>
        ) : p === currentPage ? (
          <span
            key={p}
            aria-current="page"
            className={`${baseBtn} ${activeBtn}`}
          >
            {p}
          </span>
        ) : (
          <Link
            key={p}
            href={hrefForPage(basePath, preservedParams, p)}
            className={`${baseBtn} ${hoverBtn}`}
          >
            {p}
          </Link>
        ),
      )}

      {nextPage ? (
        <Link
          href={hrefForPage(basePath, preservedParams, nextPage)}
          rel="next"
          className={`${baseBtn} ${hoverBtn}`}
        >
          {t("next")}
        </Link>
      ) : (
        <span
          aria-disabled="true"
          className={`${baseBtn} opacity-40`}
        >
          {t("next")}
        </span>
      )}
    </nav>
  );
}
