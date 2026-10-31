import type { Block } from "./blocks";
import { readString } from "./blocks";

const SIZE: Record<string, string> = { sm: "py-3", md: "py-6", lg: "py-12" };

export function DividerBlock({ block }: { block: Block }) {
  const size = readString(block.props, "size") ?? "md";
  const style = readString(block.props, "style") ?? "line";
  return (
    <div className={`mx-auto w-full max-w-8xl px-6 md:px-10 ${SIZE[size] ?? SIZE.md}`}>
      {style === "line" && <hr className="border-border" />}
    </div>
  );
}
