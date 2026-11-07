import { Suspense } from "react";

import { overviewExchange } from "@skelet/storefront-core/search/overview";
import { searchOverview } from "@skelet/storefront-next/search/read";

import { OverviewBox, OverviewPending, OverviewSlot } from "./OverviewBox";

export async function Overview({ query }: { query: string }) {
  const overview = await searchOverview(query);
  if (!overview) return null;
  return <OverviewBox overview={overview} chat={overviewExchange(query, overview)} />;
}

export function SearchOverview({ query }: { query: string }) {
  return (
    <OverviewSlot>
      <Suspense key={query} fallback={<OverviewPending />}>
        <Overview query={query} />
      </Suspense>
    </OverviewSlot>
  );
}
