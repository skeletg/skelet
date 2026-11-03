// Pure channel types

// Price rendering hints for this store - symbol, its position, and decimal digits
// symbolPosition is optional to callers; unknown values fall back to prefix
export type Currency = {
  code: string;
  symbol: string;
  symbolPosition: string;
  decimals: number;
};

export type Channel = {
  id: string;
  slug: string;
  name: string;
  defaultLocale: string;
  currencyCode: string;
  currency: Currency;
  publicUrl: string;
};
