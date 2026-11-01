import type { Block } from "./blocks";
import { readItems } from "./blocks";
import { ValueStrip } from "@/app/components/ValueStrip";
import type { ValueItem } from "@/app/components/ValueStrip";

export function ValueStripBlock({ block }: { block: Block }) {
  const items: ValueItem[] = readItems(block.props, "items").flatMap((it) => {
    const title = typeof it.title === "string" ? it.title.trim() : "";
    if (!title) return [];
    return [
      {
        icon: typeof it.icon === "string" ? it.icon : "shipping",
        title,
        text: typeof it.text === "string" ? it.text : "",
      },
    ];
  });
  return <ValueStrip items={items} />;
}
