import type { Block } from "./blocks";
import { readString } from "./blocks";
import { HeroBlock } from "./HeroBlock";
import type { HeroChip } from "./HeroBlock";
import { displayName } from "@/app/lib/display";
import { categoryHref } from "@/app/lib/entity-href";
import { loadHeader } from "@/app/lib/header/read";

export async function HeroBlockServer({ block }: { block: Block }) {
  const showSearch = readString(block.props, "showSearch") === "show";
  let popularCategories: HeroChip[] = [];
  if (showSearch) {
    const { categories } = await loadHeader();
    popularCategories = categories.slice(0, 4).map((c) => ({
      id: c.id,
      url: categoryHref(c),
      label: displayName(c.name, c.slug),
    }));
  }
  return <HeroBlock block={block} popularCategories={popularCategories} />;
}
