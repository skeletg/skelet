import { browseProducts } from "./read";
import { getBrowsePageSize, getSeoConfig } from "../config/read";
import { evaluateFilterIndexability } from "../filter-path";

// Whether a filtered listing URL is worth indexing - the facet rules plus a thin-page guard. Shared by the
// product, category and collection listings, which differ only in the base filter they browse under
export async function listingIndexability(
  baseFilter: { categoryId?: string; collectionId?: string },
  pathFilters: { slug: string; values: string[] }[],
) {
  const { data } = await browseProducts({
    filter: {
      ...baseFilter,
      attributes: pathFilters.length > 0 ? pathFilters : null,
    },
    limit: await getBrowsePageSize(), // the grid's page size, so both share one cached fetch
    offset: 0,
  });
  const seo = await getSeoConfig();
  const idx = evaluateFilterIndexability(pathFilters, data.facets, {
    combinations: seo.facetCombinations,
  });
  // Thin-page guard - only for FILTERED pages, a sparse base listing stays indexable
  if (
    pathFilters.length > 0 &&
    seo.minFacetResults > 0 &&
    data.totalCount < seo.minFacetResults
  ) {
    idx.indexable = false;
  }
  return idx;
}
