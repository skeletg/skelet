"use client";

import { Link, usePathname } from "@/app/components/LocaleLink";
import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";

import {
  ArrowLeftIcon,
  ChevronRightIcon,
  GlobeIcon,
  HomeIcon,
  LogInIcon,
  type LucideIcon,
  MenuIcon,
  NewspaperIcon,
  PackageIcon,
  SparklesIcon,
  TagIcon,
  UserIcon,
} from "lucide-react";

import type {
  HeaderCategory,
  HeaderCollection,
  HeaderLocale,
} from "@/app/lib/header/read";
import { displayName } from "@/app/lib/display";
import { categoryHref, collectionHref } from "@/app/lib/entity-href";
import { parseFilterSegments } from "@/app/lib/filter-path";
import { setLocale } from "@/app/lib/locale/actions";
import { useCustomerIdentity } from "@/app/lib/customer/client";
import { Drawer } from "./Drawer";

type Props = {
  brand: string;
  categories: HeaderCategory[];
  collections: HeaderCollection[];
  locales: HeaderLocale[];
  currentLocale: string;
};

type DrillNode =
  | { kind: "root" }
  | { kind: "categories" }
  | { kind: "collections" }
  | { kind: "category"; category: HeaderCategory };

// Duration must match the CSS transition below so the stack-pruning timeout doesn't
// snap the panel back into view mid-animation
const PANEL_TRANSITION_MS = 300;

function useActiveFilters(): { slug: string; values: string[] }[] {
  const pathname = usePathname();
  return useMemo(() => {
    if (!pathname) return [];
    const segments = pathname.split("/").filter(Boolean);
    const start = segments.indexOf("f");
    if (start === -1) return [];
    return parseFilterSegments(segments.slice(start));
  }, [pathname]);
}

export function HeaderMenu({
  brand,
  categories,
  collections,
  locales,
  currentLocale,
}: Props) {
  // Resolved after hydration - a server-side auth read would force every route dynamic
  const { identity: customer, resolved } = useCustomerIdentity();
  const tm = useTranslations("header");
  const tc = useTranslations("common");
  const [open, setOpen] = useState(false);
  const close = () => setOpen(false);
  const activeFilters = useActiveFilters();

  // The stack always starts with `root`
  const [stack, setStack] = useState<DrillNode[]>([{ kind: "root" }]);
  const [depth, setDepth] = useState(0);

  // Reset to root after the drawer closes - wait for the close animation so the user
  // doesn't see the panels flicker back to root on the way out
  useEffect(() => {
    if (!open) {
      const t = window.setTimeout(() => {
        setStack([{ kind: "root" }]);
        setDepth(0);
      }, PANEL_TRANSITION_MS);
      return () => window.clearTimeout(t);
    }
  }, [open]);

  const push = (node: DrillNode) => {
    setStack((s) => [...s, node]);
    setDepth((d) => d + 1);
  };

  const pop = () => {
    setDepth((d) => Math.max(0, d - 1));
    window.setTimeout(() => {
      setStack((s) => (s.length > 1 ? s.slice(0, -1) : s));
    }, PANEL_TRANSITION_MS);
  };

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={tm("openMenu")}
        className="flex h-9 w-9 items-center justify-center rounded-md text-foreground transition hover:bg-muted"
      >
        <MenuIcon className="h-5 w-5" aria-hidden="true" />
      </button>

      <Drawer
        open={open}
        onOpenChange={setOpen}
        side="left"
        title={brand}
        hideTitle
      >
        <header className="gap-1 border-b border-border px-5 py-4">
          <h2 className="text-xl font-semibold tracking-tight">{brand}</h2>
          <p className="lens-mask text-sm text-muted-foreground">
            {customer
              ? tm("signedInAs", { email: customer.email })
              : tm("browseAndShop")}
          </p>
        </header>

        {/* Identity block - stays fixed across drill-downs. */}
        <div className="px-3 py-2">
          {!resolved ? (
            <div aria-hidden="true" className="h-12 rounded-lg bg-muted animate-pulse" />
          ) : customer ? (
            <Link
              href="/account"
              onClick={close}
              className="lens-block flex items-center gap-3 rounded-lg bg-accent px-3 py-2 text-sm transition hover:bg-accent/80"
            >
              <span className="flex h-9 w-9 items-center justify-center rounded-full bg-foreground text-background">
                <UserIcon className="h-4 w-4" />
              </span>
              <span className="flex-1 min-w-0">
                <span className="block font-medium">
                  {tm("hi", { name: customer.firstName })}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {tm("viewAccount")}
                </span>
              </span>
            </Link>
          ) : (
            <div className="flex gap-2">
              <Link
                href="/account/login"
                onClick={close}
                className="flex-1 flex items-center justify-center gap-2 rounded-lg bg-foreground px-3 py-2 text-sm font-medium text-background transition hover:opacity-90"
              >
                <LogInIcon className="h-4 w-4" />
                {tc("signIn")}
              </Link>
              <Link
                href="/account/register"
                onClick={close}
                className="flex-1 flex items-center justify-center rounded-lg border border-border px-3 py-2 text-sm font-medium transition hover:bg-accent"
              >
                {tc("register")}
              </Link>
            </div>
          )}
        </div>

        {/* Outer container masks overflow so panels slide in from the right; the inner flex row
        holds every panel in the stack side by side */}
        <div className="relative flex-1 overflow-hidden">
          <div
            className="absolute inset-0 flex transition-transform ease-out"
            style={{
              width: `${stack.length * 100}%`,
              transform: `translateX(-${(100 / stack.length) * depth}%)`,
              transitionDuration: `${PANEL_TRANSITION_MS}ms`,
            }}
          >
            {stack.map((node, i) => (
              <div
                key={`${i}-${nodeKey(node)}`}
                className="overflow-y-auto"
                style={{ width: `${100 / stack.length}%` }}
                aria-hidden={i !== depth}
              >
                <PanelView
                  node={node}
                  categories={categories}
                  collections={collections}
                  activeFilters={activeFilters}
                  onDrill={push}
                  onBack={pop}
                  onNavigate={close}
                />
              </div>
            ))}
          </div>
        </div>

        <footer className="border-t border-border px-3 py-3 space-y-3">
          {locales.length > 1 && (
            <SwitcherRow
              label={tc("language")}
              icon={GlobeIcon}
              items={locales.map((l) => ({
                key: l.code,
                isActive: l.code === currentLocale,
                label: l.nativeName,
                short: l.code.toUpperCase(),
                formAction: setLocale,
                fieldName: "code",
                fieldValue: l.code,
                ariaLabel: tm("switchTo", { language: l.name }),
                onClick: close,
              }))}
            />
          )}
        </footer>
      </Drawer>
    </>
  );
}

// Stable key per node so React doesn't recycle DOM nodes across different drill targets
// while the animation is running
function nodeKey(node: DrillNode): string {
  switch (node.kind) {
    case "root":
      return "root";
    case "categories":
      return "categories";
    case "collections":
      return "collections";
    case "category":
      return `cat:${node.category.id}`;
  }
}

type PanelProps = {
  node: DrillNode;
  categories: HeaderCategory[];
  collections: HeaderCollection[];
  activeFilters: { slug: string; values: string[] }[];
  onDrill: (node: DrillNode) => void;
  onBack: () => void;
  onNavigate: () => void;
};

function PanelView({
  node,
  categories,
  collections,
  activeFilters,
  onDrill,
  onBack,
  onNavigate,
}: PanelProps) {
  const t = useTranslations("header");
  const tc = useTranslations("common");
  switch (node.kind) {
    case "root":
      return (
        <PanelBody>
          <Section>
            <MenuLink href="/" icon={HomeIcon} onClick={onNavigate}>
              {tc("home")}
            </MenuLink>
            <MenuLink href="/products" icon={PackageIcon} onClick={onNavigate}>
              {t("allProducts")}
            </MenuLink>
            {categories.length > 0 && (
              <DrillRow
                icon={TagIcon}
                onClick={() => onDrill({ kind: "categories" })}
              >
                {t("categories")}
              </DrillRow>
            )}
            {collections.length > 0 && (
              <DrillRow
                icon={SparklesIcon}
                onClick={() => onDrill({ kind: "collections" })}
              >
                {t("collections")}
              </DrillRow>
            )}
            <MenuLink href="/blog" icon={NewspaperIcon} onClick={onNavigate}>
              {t("blog")}
            </MenuLink>
          </Section>
        </PanelBody>
      );

    case "categories":
      return (
        <PanelBody header={<PanelHeader title={t("categories")} onBack={onBack} />}>
          <Section>
            {categories.map((c) => (
              <CategoryRow
                key={c.id}
                category={c}
                activeFilters={activeFilters}
                onDrill={onDrill}
                onNavigate={onNavigate}
              />
            ))}
          </Section>
        </PanelBody>
      );

    case "collections":
      return (
        <PanelBody header={<PanelHeader title={t("collections")} onBack={onBack} />}>
          <Section>
            {collections.map((c) => (
              <MenuLink
                key={c.id}
                href={collectionHref(c, activeFilters)}
                icon={SparklesIcon}
                onClick={onNavigate}
              >
                {displayName(c.name, c.slug)}
              </MenuLink>
            ))}
          </Section>
        </PanelBody>
      );

    case "category": {
      const { category } = node;
      return (
        <PanelBody
          header={
            <PanelHeader
              title={displayName(category.name, category.slug)}
              onBack={onBack}
            />
          }
        >
          <Section>
            <MenuLink
              href={categoryHref(category, activeFilters)}
              icon={TagIcon}
              onClick={onNavigate}
            >
              {t("shopAllIn", { name: displayName(category.name, category.slug) })}
            </MenuLink>
            {category.children.map((child) => (
              <CategoryRow
                key={child.id}
                category={child}
                activeFilters={activeFilters}
                onDrill={onDrill}
                onNavigate={onNavigate}
              />
            ))}
          </Section>
        </PanelBody>
      );
    }
  }
}

function PanelBody({
  header,
  children,
}: {
  header?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col">
      {header}
      <nav className="px-3 py-2">{children}</nav>
    </div>
  );
}

function PanelHeader({
  title,
  onBack,
}: {
  title: string;
  onBack: () => void;
}) {
  const tc = useTranslations("common");
  return (
    <div className="flex items-center gap-1 border-b border-border px-2 py-2">
      <button
        type="button"
        onClick={onBack}
        aria-label={tc("back")}
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-foreground transition hover:bg-accent"
      >
        <ArrowLeftIcon className="h-4 w-4" aria-hidden="true" />
      </button>
      <h3 className="truncate text-base font-semibold tracking-tight">
        {title}
      </h3>
    </div>
  );
}

// A category row that drills into its own panel when it has children, otherwise
// navigates straight to the category page
function CategoryRow({
  category,
  activeFilters,
  onDrill,
  onNavigate,
}: {
  category: HeaderCategory;
  activeFilters: { slug: string; values: string[] }[];
  onDrill: (node: DrillNode) => void;
  onNavigate: () => void;
}) {
  if (category.children.length === 0) {
    return (
      <MenuLink
        href={categoryHref(category, activeFilters)}
        icon={TagIcon}
        onClick={onNavigate}
      >
        {displayName(category.name, category.slug)}
      </MenuLink>
    );
  }
  return (
    <DrillRow
      icon={TagIcon}
      onClick={() => onDrill({ kind: "category", category })}
    >
      {displayName(category.name, category.slug)}
    </DrillRow>
  );
}

function Section({
  label,
  children,
}: {
  label?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="py-2 first:pt-0 last:pb-0">
      {label && (
        <h3 className="px-2 pb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {label}
        </h3>
      )}
      <ul className="flex flex-col">{children}</ul>
    </section>
  );
}

function MenuLink({
  href,
  onClick,
  icon: Icon,
  children,
}: {
  href: string;
  onClick: () => void;
  icon: LucideIcon;
  children: React.ReactNode;
}) {
  return (
    <li>
      <Link
        href={href}
        onClick={onClick}
        className="group flex items-center gap-3 rounded-md px-2 py-2 text-sm text-foreground transition hover:bg-accent"
      >
        <Icon
          className="h-4 w-4 text-muted-foreground transition group-hover:text-foreground"
          aria-hidden="true"
        />
        <span className="flex-1 truncate">{children}</span>
      </Link>
    </li>
  );
}

// Tappable row that drills into a deeper panel rather than navigating
function DrillRow({
  icon: Icon,
  onClick,
  children,
}: {
  icon: LucideIcon;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        className="group flex w-full items-center gap-3 rounded-md px-2 py-2 text-sm text-foreground transition hover:bg-accent"
      >
        <Icon
          className="h-4 w-4 text-muted-foreground transition group-hover:text-foreground"
          aria-hidden="true"
        />
        <span className="flex-1 truncate text-left">{children}</span>
        <ChevronRightIcon
          className="h-4 w-4 text-muted-foreground transition group-hover:text-foreground"
          aria-hidden="true"
        />
      </button>
    </li>
  );
}

type SwitcherItem = {
  key: string;
  isActive: boolean;
  label: string;
  short: string;
  formAction: (formData: FormData) => Promise<void> | void;
  fieldName: string;
  fieldValue: string;
  ariaLabel: string;
  onClick: () => void;
};

function SwitcherRow({
  label,
  icon: Icon,
  items,
}: {
  label: string;
  icon: LucideIcon;
  items: SwitcherItem[];
}) {
  return (
    <div>
      <div className="mb-1.5 flex items-center gap-2 px-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        <Icon className="h-3.5 w-3.5" aria-hidden="true" />
        {label}
      </div>
      <div className="flex flex-wrap gap-1.5">
        {items.map((it) => (
          <form key={it.key} action={it.formAction}>
            <input type="hidden" name={it.fieldName} value={it.fieldValue} />
            <button
              type="submit"
              onClick={it.onClick}
              aria-current={it.isActive ? "true" : undefined}
              aria-label={it.ariaLabel}
              title={it.label}
              className={[
                "flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-sm font-medium transition",
                it.isActive
                  ? "border-foreground bg-foreground text-background"
                  : "border-border text-muted-foreground hover:bg-accent hover:text-foreground",
              ].join(" ")}
            >
              <span className="font-mono">{it.short}</span>
              <span className="hidden sm:inline">{it.label}</span>
            </button>
          </form>
        ))}
      </div>
    </div>
  );
}
