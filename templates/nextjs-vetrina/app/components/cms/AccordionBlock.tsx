import { PlusIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import type { Block } from "./blocks";
import { readString, readItems } from "./blocks";

export function AccordionBlock({ block }: { block: Block }) {
  const t = useTranslations("cms");
  const heading = readString(block.props, "heading");
  const items = readItems(block.props, "items").filter((it) =>
    readString(it, "heading"),
  );
  if (items.length === 0) return null;

  return (
    <section className="border-b border-border">
      <div
        className={[
          "mx-auto grid w-full max-w-8xl gap-8 px-6 py-14 md:px-10",
          heading
            ? "lg:grid-cols-[minmax(0,18rem)_minmax(0,1fr)] lg:gap-16"
            : "",
        ].join(" ")}
      >
        {heading && (
          <div className="lg:sticky lg:top-28 lg:self-start">
            <h2 className="text-3xl font-extrabold tracking-tight text-foreground">
              {heading}
            </h2>
            <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
              {t("tapQuestion")}
            </p>
          </div>
        )}

        <div className="grid gap-3">
          {items.map((item, i) => {
            const q = readString(item, "heading")!;
            const a = readString(item, "body");
            return (
              <details
                key={i}
                className="glass group rounded-xl px-5 py-4 [&_summary::-webkit-details-marker]:hidden"
              >
                <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-base font-semibold text-foreground marker:content-['']">
                  {q}
                  <span
                    aria-hidden="true"
                    className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground transition-transform duration-200 group-open:rotate-45"
                  >
                    <PlusIcon className="size-4" />
                  </span>
                </summary>
                {a && (
                  <p className="mt-3 max-w-2xl whitespace-pre-wrap text-sm leading-relaxed text-muted-foreground">
                    {a}
                  </p>
                )}
              </details>
            );
          })}
        </div>
      </div>
    </section>
  );
}
