import { Link } from "@/app/components/LocaleLink";
import { getTranslations } from "next-intl/server";

import { getFreeShippingThreshold } from "@/app/lib/cart/read";
import { loadHeader } from "@/app/lib/header/read";
import { AccountSlot } from "@/app/components/AccountSlot";
import { getCurrentLocale } from "@/app/lib/locale/read";
import { loadMenu } from "@/app/lib/menus/read";
import { getChannel, getStoreName } from "@/app/lib/channel/read";
import { getBranding } from "@/app/lib/config/read";
import { formatPrice } from "@/app/lib/format";
import { AuthSidebar } from "./AuthSidebar";
import { CartSidebar } from "./CartSidebar";
import { HeaderMenu } from "./HeaderMenu";
import { HeaderShell } from "./HeaderShell";
import { SearchModal } from "./SearchModal";

export async function Header() {
  // getChannel is cached at the request level so this doesn't
  // re-fetch what the root layout already loaded
  const [
    { categories, collections, locales },
    currentLocale,
    channel,
    mainNav,
    freeShipThreshold,
  ] = await Promise.all([
    loadHeader(),
    getCurrentLocale(),
    getChannel(),
    loadMenu("main-nav"),
    getFreeShippingThreshold(),
  ]);
  const [brand, branding, t] = await Promise.all([
    getStoreName(),
    getBranding(),
    getTranslations("header"),
  ]);

  return (
    <>
      {/* Announcement bar - not sticky, it scrolls away and the white nav pins alone */}
      <div className="site-banner bg-midnight-deep text-white">
        <p className="mx-auto w-full max-w-8xl px-4 py-1.5 text-center font-mono text-[11px] uppercase tracking-[0.14em] text-white/80 md:px-8">
          {freeShipThreshold > 0 ? (
            t.rich("freeShippingOver", {
              amount: () => (
                <span className="text-white">
                  {formatPrice(freeShipThreshold, channel?.currencyCode ?? "USD")}
                </span>
              ),
            })
          ) : (
            t("tagline")
          )}
        </p>
      </div>

      <HeaderShell>
        <div className="mx-auto flex h-16 w-full max-w-8xl items-center gap-2 px-4 text-foreground sm:gap-3 md:px-8">
          <div className="flex shrink-0 items-center gap-2">
            <HeaderMenu
              brand={brand}
              categories={categories}
              collections={collections}
              locales={locales}
              currentLocale={currentLocale}
            />
            {/* An uploaded logo replaces the wordmark rather than sitting beside it - two brand marks
                in one header reads as a mistake. alt carries the store name either way, so the
                heading is announced identically to screen readers and the text fallback is intact */}
            <Link
              href="/"
              aria-label={brand}
              className="flex items-baseline font-heading text-lg font-extrabold uppercase leading-none tracking-[0.08em] text-foreground"
            >
              {branding.logoUrl ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={branding.logoUrl}
                  alt={brand}
                  // Bounded both ways: the header row is h-16, and a wide logo would otherwise push
                  // the nav off a narrow viewport
                  className="max-h-8 w-auto max-w-[11rem] object-contain"
                />
              ) : (
                <>
                  <span>{brand}</span>
                  <span aria-hidden="true" className="text-signal">
                    .
                  </span>
                </>
              )}
            </Link>
          </div>

          {/* Admin-curated main-nav, catalog fallbacks otherwise - unresolved targets drop silently */}
          <nav
            aria-label={mainNav?.name ?? t("browse")}
            className="hidden min-w-0 md:block md:pl-4"
          >
            <ul className="flex items-center gap-5 text-sm font-medium text-muted-foreground">
              {(mainNav && mainNav.items.length > 0
                ? mainNav.items.flatMap((it) =>
                    it.url ? [{ key: it.id, url: it.url, label: it.label }] : [],
                  )
                : FALLBACK_NAV.map((it) => ({ ...it, label: t(it.label) }))
              ).map((it) => (
                <li key={it.key} className="shrink-0">
                  <Link
                    href={it.url}
                    className="whitespace-nowrap transition hover:text-foreground"
                  >
                    {it.label}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>

          <div className="ml-auto flex shrink-0 items-center gap-1">
            <SearchModal />
            {/* The currency switcher (approx-price presentment) is deliberately not rendered - the
                provider, cookie plumbing and backend rates all still work, so restoring it is just
                mounting components/CurrencySwitcher here again */}

            {/* Guests get the drawer rather than a navigation, so they stay on the page they were on */}
            {/* Resolved after hydration - reading the auth cookie here would force every route dynamic */}
            <AccountSlot />

            <CartSidebar freeShipThreshold={freeShipThreshold} />
            {/* Mounted for signed-in users too, so a re-auth prompt has a seam to open */}
            <AuthSidebar />
          </div>
        </div>
      </HeaderShell>
    </>
  );
}

// label is a header.* message key
const FALLBACK_NAV = [
  { key: "products", url: "/products", label: "allProducts" },
  { key: "collections", url: "/collections", label: "collections" },
  { key: "blog", url: "/blog", label: "blog" },
] as const;
