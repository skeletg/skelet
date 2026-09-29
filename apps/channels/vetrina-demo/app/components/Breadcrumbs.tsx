// Semantic breadcrumb trail

import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";

export type Crumb = {
  name: string;
  href: string;
};

export function Breadcrumbs({
  items,
  className,
}: {
  items: (Crumb | null | undefined)[];
  className?: string;
}) {
  const t = useTranslations("listing");
  const crumbs = items.filter((c): c is Crumb => Boolean(c));
  if (crumbs.length === 0) return null;
  return (
    <div className={`flex items-center ${className ?? "text-sm"}`}>
      {/* One row always; contain-paint keeps the track out of the layout viewport */}
      <nav aria-label={t("breadcrumb")} className="min-w-0 flex-1">
        <ol className="scrollbar-none flex flex-nowrap items-center gap-1 overflow-x-auto text-muted-foreground contain-paint">
        {crumbs.map((c, i) => {
          const last = i === crumbs.length - 1;
          return (
            <li key={`${c.href}-${i}`} className="flex shrink-0 items-center gap-1 whitespace-nowrap">
              {last ? (
                <span
                  aria-current="page"
                  className="text-foreground"
                >
                  {c.name}
                </span>
              ) : (
                <Link
                  href={c.href}
                  className="hover:text-foreground hover:underline"
                >
                  {c.name}
                </Link>
              )}
              {!last && (
                <span aria-hidden="true" className="text-muted-foreground">
                  /
                </span>
              )}
            </li>
          );
        })}
        </ol>
      </nav>
    </div>
  );
}
