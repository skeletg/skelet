// A display currency the store offers: its rate from the base currency, and how its amounts are written
export type PresentmentCurrency = {
  code: string;
  name: string;
  symbol: string;
  decimals: number;
  symbolPosition: string;
  factorFromBase: number;
};
