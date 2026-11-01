// A quantity box between - and + buttons. The buttons need scripts (enhance.ts steps the box), so without
// them only the box shows - typing and the keyboard's arrow keys still work
type Props = {
  id: string;
  value: number;
  min?: number;
  max?: number | null;
  step?: number | null;
  // The screen-reader name of the box, and what the buttons say they add or take away
  label: string;
  item?: string;
  // Changes post the form by themselves, once the clicks or typing settle (the cart's lines)
  autosubmit?: boolean;
  // Show the box's native up/down arrows as well as - and +
  arrows?: boolean;
};

const stepClass = "hidden size-11 shrink-0 place-items-center rounded-full text-lg text-muted-foreground transition-colors hover:text-foreground js:grid";

export function QuantityInput({ id, value, min = 0, max, step, label, item, autosubmit = false, arrows = false }: Props) {
  const what = item ? ` ${item}` : "";
  return (
    <div className="inline-flex items-center rounded-full border border-border bg-card">
      <button type="button" id={`${id}-less`} data-step="-1" aria-label={`One fewer${what}`} className={stepClass}>
        −
      </button>
      <label>
        <span className="sr-only">{label}</span>
        <input
          type="number"
          id={id}
          name="quantity"
          defaultValue={value}
          min={min}
          max={max ?? undefined}
          step={step ?? "any"}
          inputMode="decimal"
          data-autosubmit={autosubmit ? "" : undefined}
          data-arrows={arrows ? "" : undefined}
          // The arrows take room on the right, so the number keeps off them
          className={`w-20 rounded-full border-0 bg-transparent text-center focus:border-0 ${arrows ? "pl-4 pr-2 js:w-16 js:pl-2" : "px-3 js:w-12 js:px-0"}`}
        />
      </label>
      <button type="button" id={`${id}-more`} data-step="1" aria-label={`One more${what}`} className={stepClass}>
        +
      </button>
    </div>
  );
}
