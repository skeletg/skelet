"use client";

import { Link, usePathname } from "@/app/components/LocaleLink";
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
  UserIcon,
} from "lucide-react";

// Icon is structural rather than lucide's own type: core's entries are all lucide, but a plugin
// contributing a nav item should not be forced to depend on the same icon library
type NavIcon = ComponentType<{ className?: string }>;
type Item = { href: string; label: string; Icon: NavIcon };

const SHOPPING: Item[] = [
  { href: "/account/orders", label: "Orders", Icon: PackageIcon },
  { href: "/account/wishlist", label: "Wishlist", Icon: HeartIcon },
  { href: "/account/downloads", label: "Downloads", Icon: DownloadIcon },
  {
    href: "/account/subscriptions",
    label: "Subscriptions",
    Icon: RefreshCwIcon,
  },
];

const DETAILS: Item[] = [
  { href: "/account/profile", label: "Profile", Icon: UserIcon },
  { href: "/account/addresses", label: "Addresses", Icon: MapPinIcon },
  {
    href: "/account/payment-methods",
    label: "Payment methods",
    Icon: CreditCardIcon,
  },
  { href: "/account/password", label: "Password", Icon: LockIcon },
  { href: "/account/privacy", label: "Data & privacy", Icon: ShieldIcon },
];

const OVERVIEW: Item = {
  href: "/account",
  label: "Overview",
  Icon: LayoutGridIcon,
};

export function AccountNav({
  variant = "sidebar",
}: {
  variant?: "sidebar" | "strip";
}) {
  const pathname = usePathname();
  const shopping = SHOPPING;
  // Plugin-contributed links sit after the built-in shopping ones - see app/plugins/registry.ts
  const pluginItems = getAccountNavItems();

  if (variant === "strip") {
    return (
      <nav
        aria-label="Account"
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
          {DETAILS.map((it) => (
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
    <nav aria-label="Account" className="text-sm">
      <NavLink {...OVERVIEW} pathname={pathname} exact />
      <Group title="Shopping" items={shopping} pathname={pathname} extra={pluginItems} />
      <Group title="Your details" items={DETAILS} pathname={pathname} />
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
    </Link>
  );
}
