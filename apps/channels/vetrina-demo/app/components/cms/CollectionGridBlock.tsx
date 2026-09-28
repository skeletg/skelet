import { getTranslations } from "next-intl/server";

import type { Block } from "./blocks";
import { readNumber, readString } from "./blocks";
import { getCollectionCard, getContainer, getLayout } from "./variants";
import { loadHeader } from "@/app/lib/header/read";

export async function CollectionGridBlock({ block }: { block: Block }) {
  const heading = readString(block.props, "heading");
  const limit = readNumber(block.props, "limit") ?? 4;
  const [{ collections }, t] = await Promise.all([
    loadHeader(),
    getTranslations("home"),
  ]);
  const shown = collections.slice(0, limit);

  if (shown.length === 0) return null;

  const renderCard = getCollectionCard(readString(block.props, "card"));
  const renderLayout = getLayout(readString(block.props, "variant"));
  const renderContainer = getContainer(readString(block.props, "container"));

  return renderContainer({
    heading,
    href: "/products",
    hrefLabel: t("shopAll"),
    children: renderLayout({
      items: shown,
      renderItem: (c) => renderCard({ collection: c as (typeof shown)[number] }),
    }),
  });
}
