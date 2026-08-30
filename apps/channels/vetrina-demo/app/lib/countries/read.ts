// Server-side fetcher for the storefront's active country list

import { cache } from "react";

import { gqlFetch } from "@/app/lib/graphql";
import {
  CountriesDocument,
  CountrySettingsDocument,
  RegionsDocument,
} from "@/app/lib/countries/operations.gql";

export type CountryOption = {
  code: string;
  name: string;
  emojiFlag: string;
};

export type RegionOption = { code: string; name: string };

export type CountrySettings = {
  defaultCountryCode: string;
  allowOptionalState: boolean;
  requiredRegionCountryCodes: string[];
};

export const getCountries = cache(async (): Promise<CountryOption[]> => {
  try {
    const res = await gqlFetch(CountriesDocument);
    return res.data.countries;
  } catch {
    // Read failure → empty list. The checkout form falls back to a
    // disabled, empty <select>; better than 500'ing the whole page
    return [];
  }
});
export const getCountrySettings = cache(async (): Promise<CountrySettings> => {
  try {
    const res = await gqlFetch(CountrySettingsDocument);
    return res.data.countrySettings;
  } catch {
    return { defaultCountryCode: "", allowOptionalState: false, requiredRegionCountryCodes: [] };
  }
});

// getRegionsByCountry resolves the seeded ISO 3166-2 subdivisions for the given countries into a map (countries with
// none are omitted so the form falls back to a free-text region field)
export const getRegionsByCountry = cache(
  async (codes: string[]): Promise<Record<string, RegionOption[]>> => {
    const entries = await Promise.all(
      codes.map(async (code) => {
        try {
          const res = await gqlFetch(RegionsDocument, { countryCode: code });
          return [code, res.data.regions] as const;
        } catch {
          return [code, [] as RegionOption[]] as const;
        }
      }),
    );
    const map: Record<string, RegionOption[]> = {};
    for (const [code, regions] of entries) {
      if (regions.length > 0) map[code] = regions;
    }
    return map;
  },
);
