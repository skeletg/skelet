import { money, type Currency } from "../lib/format";

type Props = {
  amount: number;
  compareAt?: number | null;
  onSale?: boolean | null;
  currency: Currency;
  className?: string;
};

export function Price({ amount, compareAt, onSale, currency, className = "" }: Props) {
  const showWas = !!onSale && !!compareAt && compareAt > amount;
  return (
    <span className={`tabular-nums ${className}`}>
      <span className={showWas ? "text-destructive" : undefined}>{money(amount, currency)}</span>
      {showWas && (
        <>
          <span className="sr-only">, was </span>
          <s className="ml-2 font-normal text-muted-foreground">{money(compareAt, currency)}</s>
        </>
      )}
    </span>
  );
}
