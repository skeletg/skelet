"use client";

// Drop-in for next/link that keeps the current language on every internal click
//
// Hand-rolled rather than next-intl's navigation helpers because those need the whole locale list
// declared statically at module scope, and this app's locale set is channel data from the backend.
// All this actually needs is the current locale (from context) and which one is default (passed
// down by the server layout, which fetched it) - no list required

import NextLink from "next/link";

import { NavPendingProbe } from "./NavProgress";
import { beginNavigation, endNavigation } from "./nav-progress";
import { usePathname as useNextPathname, useRouter as useNextRouter } from "next/navigation";
import { useLocale } from "next-intl";
import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useTransition,
  type ComponentProps,
} from "react";

const DefaultLocaleContext = createContext<string>("");

export function DefaultLocaleProvider({
  value,
  children,
}: {
  value: string;
  children: React.ReactNode;
}) {
  return <DefaultLocaleContext.Provider value={value}>{children}</DefaultLocaleContext.Provider>;
}

export function useDefaultLocale(): string {
  return useContext(DefaultLocaleContext);
}

export function localeHref(href: string, locale: string, defaultLocale: string): string {
  // Only internal absolute paths get a prefix - external urls, anchors, mailto: and query-only
  // links must pass through untouched
  if (!href.startsWith("/") || href.startsWith("//")) return href;
  if (!locale || !defaultLocale || locale === defaultLocale) return href;
  const first = href.split("/")[1] ?? "";
  if (first === locale) return href; // already prefixed
  return `/${locale}${href === "/" ? "" : href}`;
}

export function Link({ children, href, ...rest }: ComponentProps<typeof NextLink>) {
  const locale = useLocale();
  const defaultLocale = useDefaultLocale();
  const resolved = typeof href === "string" ? localeHref(href, locale, defaultLocale) : href;
  return (
    <NextLink href={resolved} {...rest}>
      {children}
      {/* Renders nothing - it's how the top progress bar learns this link is waiting */}
      <NavPendingProbe />
    </NextLink>
  );
}

// Locale-aware useRouter. The filter and sort controls navigate with router.push() rather than the
// Link they sit next to, and a raw push drops the prefix - which is what threw you back to the
// default language the moment you touched a filter
export function useRouter() {
  const router = useNextRouter();
  const locale = useLocale();
  const defaultLocale = useDefaultLocale();
  // A push never passes through a Link, so useLinkStatus can't see it - the transition's own
  // pending flag is what tells the progress bar a filter or sort change is still in flight
  const [pending, startTransition] = useTransition();
  useEffect(() => {
    if (!pending) return;
    beginNavigation();
    return endNavigation;
  }, [pending]);
  return useMemo(
    () => ({
      ...router,
      push: (href: string, opts?: Parameters<typeof router.push>[1]) =>
        startTransition(() => router.push(localeHref(href, locale, defaultLocale), opts)),
      replace: (href: string, opts?: Parameters<typeof router.replace>[1]) =>
        startTransition(() => router.replace(localeHref(href, locale, defaultLocale), opts)),
      prefetch: (href: string, opts?: Parameters<typeof router.prefetch>[1]) =>
        router.prefetch(localeHref(href, locale, defaultLocale), opts),
    }),
    [router, locale, defaultLocale, startTransition],
  );
}

// Locale-aware usePathname: gives back the path WITHOUT the locale prefix, so client code compares
// and builds hrefs in one consistent space and the prefix only reappears at the navigation
// boundary. Without this, `pathname === "/compare"` and the account nav's active-state checks all
// silently stop matching the moment you are on a prefixed url
export function usePathname(): string {
  const pathname = useNextPathname();
  const locale = useLocale();
  const first = pathname.split("/")[1] ?? "";
  if (locale && first === locale) return pathname.slice(locale.length + 1) || "/";
  return pathname;
}
