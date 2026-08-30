import type { Block } from "./blocks";
import { readString, readItems } from "./blocks";
import { GoalLink } from "./GoalLink";

const ALIGN: Record<string, string> = {
  left: "justify-start",
  center: "justify-center",
  right: "justify-end",
};

const STYLES: Record<string, string> = {
  primary: "bg-primary text-primary-foreground hover:bg-primary/90",
  secondary: "glass glass-hover text-foreground",
  outline: "border border-border text-foreground hover:bg-accent",
};

export function ButtonsBlock({ block }: { block: Block }) {
  const align = readString(block.props, "align") ?? "left";
  const items = readItems(block.props, "items").filter(
    (it) => readString(it, "label") && readString(it, "href"),
  );
  if (items.length === 0) return null;

  return (
    <section className="border-b border-border">
      <div className={`mx-auto flex w-full max-w-8xl flex-wrap gap-3 px-6 md:px-10 py-8 ${ALIGN[align] ?? ALIGN.left}`}>
        {items.map((it, i) => {
          const label = readString(it, "label")!;
          const href = readString(it, "href")!;
          const style = readString(it, "style") ?? "primary";
          const goalEvent = readString(it, "goalEvent");
          return (
            <GoalLink
              key={i}
              href={href}
              goalEvent={goalEvent}
              className={`inline-flex h-11 items-center gap-2 rounded-md px-5 text-sm font-medium transition ${STYLES[style] ?? STYLES.primary}`}
            >
              {label}
            </GoalLink>
          );
        })}
      </div>
    </section>
  );
}
