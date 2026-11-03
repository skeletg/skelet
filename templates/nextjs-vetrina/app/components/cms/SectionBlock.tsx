import type { Block } from "./blocks";
import { readString } from "./blocks";
import { CmsBlocks } from "./CmsBlocks";

export async function SectionBlock({ block }: { block: Block }) {
  const children = block.blocks ?? [];
  if (children.length === 0) return null;

  const background = readString(block.props, "background") ?? "none";
  const width = readString(block.props, "width") ?? "normal";
  const padding = readString(block.props, "padding") ?? "normal";

  const bg =
    background === "muted"
      ? "bg-muted/40 border-y border-border"
      : background === "foreground"
        ? "bg-foreground text-background"
        : "";
  const pad = padding === "none" ? "" : padding === "large" ? "py-16 sm:py-24" : "py-10";
  const maxW = width === "full" ? "max-w-none" : width === "wide" ? "max-w-7xl" : "max-w-8xl";

  return (
    <section className={bg}>
      <div className={`mx-auto w-full px-6 ${maxW} ${pad}`.trim()}>
        <CmsBlocks blocks={children} />
      </div>
    </section>
  );
}
