"use client";

import { jumpToOption } from "./focus-intent";

export function MissingOptionLinks({
  items,
}: {
  items: Array<{ id: string; label: string }>;
}) {
  if (items.length === 0) return null;

  return (
    <>
      {items.map((it, i) => (
        <span key={it.id}>
          {i > 0 ? ", " : ""}
          <button
            type="button"
            onClick={() => jumpToOption(it.id)}
            className="font-semibold underline underline-offset-2 hover:no-underline"
          >
            {it.label}
          </button>
        </span>
      ))}
    </>
  );
}
