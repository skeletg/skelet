import { getTranslations } from "next-intl/server";

import type { Block } from "./blocks";
import { readNumber, readString } from "./blocks";
import { getCategoryCard, getContainer, getLayout } from "./variants";
import { loadHeader } from "@skelet/storefront-next/header/read";

export async function CategoryGridBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const limit = readNumber(block.props, "limit") ?? 5;
  const [{ categories }, t] = await Promise.all([
    loadHeader(),
    getTranslations("home"),
  ]);
  const shown = categories.slice(0, limit);

  if (shown.length === 0) return null;

  const renderCard = getCategoryCard(readString(block.props, "card"));
  const renderLayout = getLayout(readString(block.props, "variant"));
  const renderContainer = getContainer(readString(block.props, "container"));

  return renderContainer({
    heading,
    href: "/products",
    hrefLabel: t("allProducts"),
    children: renderLayout({
      items: shown,
      renderItem: (c) => renderCard({ category: c as (typeof shown)[number] }),
    }),
  });
}
