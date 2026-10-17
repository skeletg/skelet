"use client";

import { AccountBadge } from "./AccountBadge";
import { Link, usePathname } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";
import type { ComponentType } from "react";
import { getAccountNavItems, type AccountNavItem } from "@/app/plugins/registry";

import {
  CreditCardIcon,
  DownloadIcon,
  HeartIcon,
  LayoutGridIcon,
  LockIcon,
  MapPinIcon,
  PackageIcon,
  RefreshCwIcon,
  ShieldIcon,
  TagIcon,
  UserIcon,
} from "lucide-react";

// Icon is structural rather than lucide's own type: core's entries are all lucide, but a plugin
// contributing a nav item should not be forced to depend on the same icon library
type NavIcon = ComponentType<{ className?: string }>;
type Item = { href: string; label: string; Icon: NavIcon; badge?: number; badgeLabel?: string; newLabel?: string };
// Built-in entries name an account.nav message; plugin entries bring their own label
type NavKey =
  | "overview"
  | "orders"
  | "wishlist"
  | "downloads"
  | "offers"
  | "subscriptions"
  | "profile"
  | "addresses"
  | "paymentMethods"
  | "password"
  | "privacy";
type BuiltIn = { href: string; key: NavKey; Icon: NavIcon };

const SHOPPING: BuiltIn[] = [
  { href: "/account/orders", key: "orders", Icon: PackageIcon },
  { href: "/account/wishlist", key: "wishlist", Icon: HeartIcon },
  { href: "/account/downloads", key: "downloads", Icon: DownloadIcon },
  { href: "/account/offers", key: "offers", Icon: TagIcon },
  { href: "/account/subscriptions", key: "subscriptions", Icon: RefreshCwIcon },
];

const DETAILS: BuiltIn[] = [
  { href: "/account/profile", key: "profile", Icon: UserIcon },
  { href: "/account/addresses", key: "addresses", Icon: MapPinIcon },
  { href: "/account/payment-methods", key: "paymentMethods", Icon: CreditCardIcon },
  { href: "/account/password", key: "password", Icon: LockIcon },
  { href: "/account/privacy", key: "privacy", Icon: ShieldIcon },
];

const OVERVIEW_ITEM: BuiltIn = {
  href: "/account",
  key: "overview",
  Icon: LayoutGridIcon,
};

export function AccountNav({
  variant = "sidebar",
  counts = { wishlist: 0, offers: 0, newOffers: 0 },
}: {
  variant?: "sidebar" | "strip";
  counts?: { wishlist: number; offers: number; newOffers: number };
}) {
  const pathname = usePathname();
  const t = useTranslations("account.nav");
  const label = ({ href, key, Icon }: BuiltIn): Item => ({
    href, label: t(key), Icon,
    ...(key === "offers" ? {
      badge: counts.offers,
      badgeLabel: t("offerCount", { count: counts.offers }),
      newLabel: counts.newOffers > 0 ? t("newOffers", { count: counts.newOffers }) : undefined,
    } : key === "wishlist" ? {
      badge: counts.wishlist,
      badgeLabel: t("wishlistCount", { count: counts.wishlist }),
    } : {}),
  });
  const OVERVIEW = label(OVERVIEW_ITEM);
  const shopping = SHOPPING.map(label);
  const details = DETAILS.map(label);
  // Plugin-contributed links sit after the built-in shopping ones - see app/plugins/registry.ts
  const pluginItems = getAccountNavItems();

  if (variant === "strip") {
    return (
      <nav
        aria-label={t("label")}
        className="-mx-6 overflow-x-auto px-6 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        <ul className="flex w-max gap-2 text-sm">
          {[OVERVIEW, ...shopping].map((it) => (
            <li key={it.href}>
              <NavLink
                {...it}
                pathname={pathname}
                exact={it.href === OVERVIEW.href}
                className="whitespace-nowrap border border-border"
              />
            </li>
          ))}
          {pluginItems.map((it) => (
            <PluginNavItem
              key={it.id}
              item={it}
              pathname={pathname}
              className="whitespace-nowrap border border-border"
            />
          ))}
          {details.map((it) => (
            <li key={it.href}>
              <NavLink
                {...it}
                pathname={pathname}
                className="whitespace-nowrap border border-border"
              />
            </li>
          ))}
        </ul>
      </nav>
    );
  }

  return (
    <nav aria-label={t("label")} className="text-sm">
      <NavLink {...OVERVIEW} pathname={pathname} exact />
      <Group title={t("shopping")} items={shopping} pathname={pathname} extra={pluginItems} />
      <Group title={t("yourDetails")} items={details} pathname={pathname} />
    </nav>
  );
}

function Group({
  title,
  items,
  pathname,
  extra = [],
}: {
  title: string;
  items: Item[];
  pathname: string;
  extra?: AccountNavItem[];
}) {
  return (
    <div className="mt-6">
      <p className="px-3 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {title}
      </p>
      <ul className="mt-1.5 space-y-0.5">
        {items.map((it) => (
          <li key={it.href}>
            <NavLink {...it} pathname={pathname} />
          </li>
        ))}
        {extra.map((it) => (
          <PluginNavItem key={it.id} item={it} pathname={pathname} />
        ))}
      </ul>
    </div>
  );
}

// One component per plugin item, so its useVisible hook is called unconditionally within its own
// render rather than inside a loop - a plugin that answers "not for this account" renders nothing
function PluginNavItem({
  item,
  pathname,
  className = "",
}: {
  item: AccountNavItem;
  pathname: string;
  className?: string;
}) {
  const visible = item.useVisible ? item.useVisible() : true;
  if (!visible) return null;
  return (
    <li>
      <NavLink
        href={item.href}
        label={item.label}
        Icon={item.Icon}
        pathname={pathname}
        className={className}
      />
    </li>
  );
}

function NavLink({
  href,
  label,
  Icon,
  badge,
  badgeLabel,
  newLabel,
  pathname,
  exact = false,
  className = "",
}: Item & { pathname: string; exact?: boolean; className?: string }) {
  const active = exact ? pathname === href : pathname.startsWith(href);
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={`flex items-center gap-2.5 rounded-lg px-3 py-2 transition-colors ${
        active
          ? "bg-foreground font-semibold text-background"
          : "text-muted-foreground hover:bg-accent hover:text-foreground"
      } ${className}`}
    >
      <Icon className="size-4 shrink-0" aria-hidden="true" />
      {label}
      {badge && badgeLabel ? <AccountBadge count={badge} label={badgeLabel} newLabel={newLabel} /> : null}
    </Link>
  );
}
