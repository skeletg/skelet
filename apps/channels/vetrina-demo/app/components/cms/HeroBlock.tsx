import { Link } from "@/app/components/LocaleLink";
import { useTranslations } from "next-intl";
import { SearchIcon } from "lucide-react";

import type { Block } from "./blocks";
import { readImage, readString } from "./blocks";
import { GoalLink } from "./GoalLink";

// hero - full-bleed band, every piece prop-driven. With artwork it runs dark with white copy;
// without, it's a typographic hero on paper. Client-safe so the dashboard preview can render it;
// the popular-category chips need server data, so published pages go through HeroBlockServer
export type HeroChip = { id: string; url: string; label: string };

export function HeroBlock({
  block,
  popularCategories = [],
}: {
  block: Block;
  popularCategories?: HeroChip[];
}) {
  const t = useTranslations("cms");
  const ts = useTranslations("search");
  const th = useTranslations("home.hero");
  const eyebrow = readString(block.props, "eyebrow");
  const heading = readString(block.props, "heading");
  const subheading = readString(block.props, "subheading");
  const image = readImage(block.props, "imageUrl");
  const ctaLabel = readString(block.props, "ctaLabel");
  const ctaHref = readString(block.props, "ctaHref");
  const cta2Label = readString(block.props, "cta2Label");
  const cta2Href = readString(block.props, "cta2Href");
  const goalEvent = readString(block.props, "goalEvent");
  const isCenter = block.props.align === "center";
  const showSearch = readString(block.props, "showSearch") === "show";

  // Nothing worth rendering without a heading - skip rather than emit an empty
  // hero band
  if (!heading) return null;

  const dark = !!image;

  return (
    <section
      className={[
        "relative overflow-hidden border-b border-border",
        dark ? "bg-midnight-deep text-white" : "bg-background text-foreground",
      ].join(" ")}
    >
      {image && (
        <>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            aria-hidden="true"
            src={image.src}
            alt=""
            className="pointer-events-none absolute inset-0 h-full w-full object-cover opacity-40"
          />
          {/* Wash keeps white copy legible over any artwork */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 bg-gradient-to-br from-midnight-deep/85 via-midnight-deep/55 to-midnight-deep/90"
          />
        </>
      )}
      <div
        className={[
          "relative mx-auto flex w-full max-w-8xl flex-col gap-6 px-6 py-16 md:px-10 sm:py-24",
          isCenter ? "items-center text-center" : "items-start",
        ].join(" ")}
      >
        {eyebrow && (
          <div>
            <span className="font-mono text-xs font-medium uppercase tracking-[0.22em] text-signal">
              {eyebrow}
            </span>
          </div>
        )}
        <div>
          <h1 className="max-w-4xl text-5xl font-extrabold leading-[0.98] tracking-[-0.03em] sm:text-7xl">
            {heading}
          </h1>
        </div>
        {subheading && (
          <div>
            <p
              className={[
                "max-w-xl text-base leading-relaxed sm:text-lg",
                dark ? "text-white/70" : "text-muted-foreground",
              ].join(" ")}
            >
              {subheading}
            </p>
          </div>
        )}
        {showSearch && (
          <div className="w-full">
            <form
              action="/search"
              method="GET"
              className={[
                "flex w-full max-w-xl items-center gap-2 rounded-md border p-1.5 pl-4",
                dark
                  ? "border-transparent bg-white"
                  : "border-input bg-background",
              ].join(" ")}
            >
              <label htmlFor="hero-block-search" className="sr-only">
                {ts("label")}
              </label>
              <input
                id="hero-block-search"
                type="search"
                name="q"
                placeholder={t("searchPlaceholder")}
                autoComplete="off"
                className="h-10 min-w-0 flex-1 bg-transparent text-base text-foreground outline-none placeholder:text-muted-foreground"
              />
              <button
                type="submit"
                className="flex h-10 shrink-0 items-center gap-2 rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition hover:bg-primary/85"
              >
                <SearchIcon className="h-4 w-4" aria-hidden="true" />
                {ts("submit")}
              </button>
            </form>
          </div>
        )}
        {showSearch && popularCategories.length > 0 && (
          <div>
            <div className="flex flex-wrap items-center gap-2 font-mono text-xs">
              <span className={dark ? "text-white/50" : "text-muted-foreground"}>
                {th("popular")}
              </span>
              {popularCategories.slice(0, 4).map((c) => (
                <Link
                  key={c.id}
                  href={c.url}
                  className={[
                    "rounded-md border px-3 py-1 uppercase tracking-[0.06em] transition",
                    dark
                      ? "border-white/25 text-white/80 hover:border-white/60 hover:text-white"
                      : "border-border text-foreground/70 hover:border-foreground/50 hover:text-foreground",
                  ].join(" ")}
                >
                  {c.label}
                </Link>
              ))}
            </div>
          </div>
        )}
        {((ctaLabel && ctaHref) || (cta2Label && cta2Href)) && (
          <div>
            <div className="flex flex-wrap gap-3">
              {ctaLabel && ctaHref && (
                <GoalLink
                  href={ctaHref}
                  goalEvent={goalEvent}
                  className={[
                    "inline-flex h-12 items-center gap-2 rounded-md px-7 text-sm font-semibold transition-colors",
                    dark
                      ? "bg-white text-midnight-deep hover:bg-white/90"
                      : "bg-primary text-primary-foreground hover:bg-primary/85",
                  ].join(" ")}
                >
                  {ctaLabel}
                </GoalLink>
              )}
              {cta2Label && cta2Href && (
                <GoalLink
                  href={cta2Href}
                  goalEvent={goalEvent}
                  className={[
                    "inline-flex h-12 items-center gap-2 rounded-md border px-7 text-sm font-semibold transition",
                    dark
                      ? "border-white/40 text-white hover:border-white/70 hover:bg-white/10"
                      : "border-foreground/25 text-foreground hover:border-foreground",
                  ].join(" ")}
                >
                  {cta2Label}
                </GoalLink>
              )}
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
