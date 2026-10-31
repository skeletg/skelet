// Server-side fetcher for the storefront's active country list

import { cache } from "react";

import { gqlFetch } from "@/app/lib/graphql";
import {
  CountriesDocument,
  CountrySettingsDocument,
  PhoneSettingsDocument,
  RegionsDocument,
  type PhoneSettingsQuery,
} from "@/app/lib/countries/operations.gql";

export type CountryOption = {
  code: string;
  name: string;
  emojiFlag: string;
  // The company identifiers a business purchase from the country asks for - empty means the generic tax ID
  businessIdFields?: BusinessIdField[];
};

export type BusinessIdField = {
  key: string;
  label: string;
  required: boolean;
  pattern: string;
  hint: string;
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

export type AddressFormOptions = {
  countries: CountryOption[];
  regionsByCountry: Record<string, RegionOption[]>;
  defaultCountry: string;
  phone: PhoneSettings;
};

// What an address form outside the checkout needs to offer the store's countries and each one's regions
export const getAddressFormOptions = cache(async (): Promise<AddressFormOptions> => {
  const [countries, settings, phone] = await Promise.all([getCountries(), getCountrySettings(), getPhoneSettings()]);
  const regionsByCountry = await getRegionsByCountry(countries.map((c) => c.code));
  return { countries, regionsByCountry, defaultCountry: settings.defaultCountryCode, phone };
});

export type PhoneSettings = PhoneSettingsQuery["phoneSettings"];

// The store's phone policy and the formats of the countries its forms offer. A read failure falls back to taking any
// number with no examples - the backend still enforces the real policy on save
export const getPhoneSettings = cache(async (): Promise<PhoneSettings> => {
  try {
    const res = await gqlFetch(PhoneSettingsDocument);
    return res.data.phoneSettings;
  } catch {
    return { restrictedToStoreCountries: false, lockedCountryCode: null, defaultCountryCode: "", formats: [] };
  }
});
