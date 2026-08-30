// Semantic breadcrumb trail

import { Link } from "@/app/components/LocaleLink";

import { BackButton } from "./BackButton";

export type Crumb = {
  name: string;
  href: string;
};

export function Breadcrumbs({
  items,
  className,
  showBack = true,
}: {
  items: (Crumb | null | undefined)[];
  className?: string;
  showBack?: boolean;
}) {
  const crumbs = items.filter((c): c is Crumb => Boolean(c));
  if (crumbs.length === 0) return null;
  // The parent crumb is the natural fallback when there's no history to pop (link
  // opened in a fresh tab)
  const parentHref = crumbs.length > 1 ? crumbs[crumbs.length - 2].href : "/";
  return (
    <div className={`flex flex-wrap items-center gap-x-3 gap-y-2 ${className ?? "text-sm"}`}>
      {showBack && (
        <>
          <BackButton fallbackHref={parentHref} />
          <span aria-hidden="true" className="h-4 w-px bg-border" />
        </>
      )}
      <nav aria-label="Breadcrumb">
        <ol className="flex flex-wrap items-center gap-1 text-muted-foreground">
        {crumbs.map((c, i) => {
          const last = i === crumbs.length - 1;
          return (
            <li key={`${c.href}-${i}`} className="flex items-center gap-1">
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
