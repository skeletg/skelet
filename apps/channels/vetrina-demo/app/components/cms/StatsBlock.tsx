import type { Block } from "./blocks";
import { readString, readItems } from "./blocks";
import { getContainer } from "./variants";

const COLS: Record<string, string> = {
  two: "grid-cols-2",
  three: "grid-cols-2 sm:grid-cols-3",
  four: "grid-cols-2 sm:grid-cols-4",
};

export function StatsBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const cols = readString(block.props, "cols") ?? "three";
  const items = readItems(block.props, "items").filter((it) => readString(it, "value"));
  if (items.length === 0) return null;
  const Container = getContainer(readString(block.props, "container"));

  return Container({
    heading,
    children: (
      <dl className={`grid gap-6 ${COLS[cols] ?? COLS.three}`}>
        {items.map((it, i) => (
          <div key={i} className="text-center">
            <dt className="font-mono text-3xl font-bold tracking-tight text-foreground sm:text-4xl">
              {readString(it, "value")}
            </dt>
            <dd className="mt-1 text-sm font-medium text-foreground">{readString(it, "label")}</dd>
            {readString(it, "description") && (
              <dd className="mt-1 text-xs text-muted-foreground">{readString(it, "description")}</dd>
            )}
          </div>
        ))}
      </dl>
    ),
  });
}
