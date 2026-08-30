"use client";

import { useState } from "react";

import type { Block } from "./blocks";
import { readString, readItems } from "./blocks";

export function TabsBlock({ block }: { block: Block }) {
  const items = readItems(block.props, "items").filter((it) => readString(it, "label"));
  const [active, setActive] = useState(0);
  if (items.length === 0) return null;
  const current = items[Math.min(active, items.length - 1)];

  return (
    <section className="border-b border-border">
      <div className="mx-auto w-full max-w-3xl px-6 py-12">
        <div className="flex flex-wrap gap-1 border-b border-border">
          {items.map((it, i) => (
            <button
              key={i}
              type="button"
              onClick={() => setActive(i)}
              className={[
                "-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors",
                i === active
                  ? "border-foreground text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              ].join(" ")}
            >
              {readString(it, "label")}
            </button>
          ))}
        </div>
        <p className="mt-4 whitespace-pre-wrap text-sm leading-relaxed text-foreground">
          {readString(current, "body")}
        </p>
      </div>
    </section>
  );
}
