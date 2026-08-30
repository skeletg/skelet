import type { Block } from "./blocks";
import { readString } from "./blocks";
import { GoalLink } from "./GoalLink";

export function CtaBannerBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const text = readString(block.props, "text");
  const ctaLabel = readString(block.props, "ctaLabel");
  const ctaHref = readString(block.props, "ctaHref");
  const goalEvent = readString(block.props, "goalEvent");

  if (!heading) return null;

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-8xl px-6 md:px-10 py-14">
        <div className="glass flex flex-col items-start gap-4 rounded-2xl p-8 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <h2 className="text-2xl font-extrabold tracking-tight">{heading}</h2>
            {text && (
              <p className="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">
                {text}
              </p>
            )}
          </div>
          {ctaLabel && ctaHref && (
            <GoalLink
              href={ctaHref}
              goalEvent={goalEvent}
              className="inline-flex h-11 shrink-0 items-center gap-2 rounded-md bg-primary px-6 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
            >
              {ctaLabel}
            </GoalLink>
          )}
        </div>
      </div>
    </section>
  );
}
