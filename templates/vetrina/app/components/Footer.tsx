import { Link } from "@/app/components/LocaleLink";

import { loadMenu } from "@/app/lib/menus/read";
import type { MenuItem } from "@/app/lib/menus/read";
import { getStoreName } from "@/app/lib/channel/read";
import { getBranding } from "@/app/lib/config/read";

// Driven by the "footer" menu - columns when any top-level item has children, a single row when none do. Copyright +
// support row always renders, so a store with no menu still gets a usable footer
export async function Footer() {
  const [menu, brand, branding] = await Promise.all([
    loadMenu("footer"),
    getStoreName(),
    getBranding(),
  ]);
  const supportEmail: string | null = null;

  const hasGroups = !!menu?.items.some((it) => it.children.length > 0);

  return (
    <footer className="mt-16 border-t border-border bg-muted text-foreground">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14 space-y-10">
        {/* Wordmark row - the footer restates the brand where the header's has scrolled away, so it
            takes the uploaded logo too rather than leaving one slot text and the other an image */}
        {branding.logoUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={branding.logoUrl}
            alt={brand}
            className="max-h-10 w-auto max-w-[13rem] object-contain"
          />
        ) : (
          <p className="flex items-baseline font-heading text-lg font-extrabold uppercase leading-none tracking-[0.08em]">
            <span>{brand}</span>
            <span aria-hidden="true" className="text-signal">
              .
            </span>
          </p>
        )}
        {menu && menu.items.length > 0 && (
          hasGroups ? (
            <div className="grid grid-cols-2 gap-x-8 gap-y-8 sm:grid-cols-3 lg:grid-cols-4">
              {menu.items.map((group) => (
                <FooterColumn key={group.id} item={group} />
              ))}
            </div>
          ) : (
            <div>
              <h2 className="font-mono text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
                {menu.name}
              </h2>
              <ul className="mt-3 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-foreground/70">
                {menu.items.map((it) =>
                  it.url ? (
                    <li key={it.id}>
                      <Link
                        href={it.url}
                        className="transition hover:text-foreground"
                      >
                        {it.label}
                      </Link>
                    </li>
                  ) : (
                    <li key={it.id} className="text-muted-foreground/70">
                      {it.label}
                    </li>
                  ),
                )}
              </ul>
            </div>
          )
        )}

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-6 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground">
          <span>
            © {new Date().getFullYear()} {brand}. Demo storefront.
          </span>
          {supportEmail && (
            <a
              href={`mailto:${supportEmail}`}
              className="transition hover:text-foreground"
            >
              {supportEmail}
            </a>
          )}
        </div>
      </div>
    </footer>
  );
}
function FooterColumn({ item }: { item: MenuItem }) {
  return (
    <div>
      <h3 className="font-mono text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
        {item.url ? (
          <Link href={item.url} className="transition hover:opacity-80">
            {item.label}
          </Link>
        ) : (
          item.label
        )}
      </h3>
      {item.children.length > 0 && (
        <ul className="mt-3 space-y-2 text-sm text-foreground/70">
          {item.children.map((child) =>
            child.url ? (
              <li key={child.id}>
                <Link
                  href={child.url}
                  className="transition hover:text-foreground"
                >
                  {child.label}
                </Link>
              </li>
            ) : (
              <li key={child.id} className="text-muted-foreground/70">
                {child.label}
              </li>
            ),
          )}
        </ul>
      )}
    </div>
  );
}
