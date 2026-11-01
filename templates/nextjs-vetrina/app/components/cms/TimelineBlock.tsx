import type { Block } from "./blocks";
import { readString, readItems } from "./blocks";

export function TimelineBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const variant = readString(block.props, "variant") ?? "vertical";
  const items = readItems(block.props, "items").filter((it) => readString(it, "title"));
  if (items.length === 0) return null;
  const alternating = variant === "alternating";

  function Event({ it }: { it: Record<string, unknown> }) {
    const label = readString(it, "label");
    const title = readString(it, "title")!;
    const body = readString(it, "body");
    return (
      <>
        {label && <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</span>}
        <h3 className="mt-0.5 text-base font-medium text-foreground">{title}</h3>
        {body && <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">{body}</p>}
      </>
    );
  }

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-3xl px-6 py-12">
        {heading && (
          <h2 className="mb-8 text-center text-2xl font-extrabold tracking-tight text-foreground">{heading}</h2>
        )}
        {alternating ? (
          <ol className="relative before:absolute before:inset-y-0 before:left-2 before:w-px before:bg-border sm:before:left-1/2">
            {items.map((it, i) => (
              <li
                key={i}
                className={`relative pb-8 pl-8 last:pb-0 sm:w-1/2 ${
                  i % 2 === 0 ? "sm:pr-10 sm:text-right" : "sm:ml-auto sm:pl-10"
                }`}
              >
                <span className="absolute left-[3px] top-1.5 size-3 rounded-full border-2 border-background bg-foreground sm:left-auto sm:right-[-6px] sm:odd:right-auto" />
                <Event it={it} />
              </li>
            ))}
          </ol>
        ) : (
          <ol className="relative ml-2 border-l border-border">
            {items.map((it, i) => (
              <li key={i} className="relative pb-8 pl-6 last:pb-0">
                <span className="absolute -left-[6.5px] top-1.5 size-3 rounded-full border-2 border-background bg-foreground" />
                <Event it={it} />
              </li>
            ))}
          </ol>
        )}
      </div>
    </section>
  );
}
