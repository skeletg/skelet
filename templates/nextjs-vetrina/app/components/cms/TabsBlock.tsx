"use client";

import { useId, useState } from "react";

import type { Block } from "./blocks";
import { readString, readItems } from "./blocks";

export function TabsBlock({ block }: { block: Block }) {
  const items = readItems(block.props, "items").filter((it) => readString(it, "label"));
  const [active, setActive] = useState(0);
  const id = useId();
  if (items.length === 0) return null;
  const current = Math.min(active, items.length - 1);

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-3xl px-6 py-12">
        <div role="tablist" className="flex flex-wrap gap-1 border-b border-border">
          {items.map((it, i) => (
            <button
              key={i}
              id={`${id}-tab-${i}`}
              type="button"
              role="tab"
              aria-selected={i === current}
              aria-controls={`${id}-panel-${i}`}
              onClick={() => setActive(i)}
              className={[
                "-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors",
                i === current
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              ].join(" ")}
            >
              {readString(it, "label")}
            </button>
          ))}
        </div>
        {/* Every panel is in the HTML, inactive ones hidden - rendering only the active body
            left the other tabs' copy out of the page for anything that doesn't run JS */}
        {items.map((it, i) => (
          <p
            key={i}
            id={`${id}-panel-${i}`}
            role="tabpanel"
            aria-labelledby={`${id}-tab-${i}`}
            hidden={i !== current}
            className="mt-4 whitespace-pre-wrap text-sm leading-relaxed text-foreground"
          >
            {readString(it, "body")}
          </p>
        ))}
      </div>
    </section>
  );
}
