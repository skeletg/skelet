"use client";

// Pure controlled-input picker. Renders one fieldset per configurable option with value
// buttons; reports the chosen value back via `onChange`. Owns no selection state itself
// - the parent uses the selection to drive the whole PDP (gallery, name, descriptions,
// specs, etc.), so it has to live one level up

import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useTranslations } from "next-intl";

import { Drawer } from "@/app/components/Drawer";
import { displayName } from "@/app/lib/display";
import { formatPrice } from "@/app/lib/format";
import { mediaThumb } from "@/app/lib/media-thumb";
import type {
  ConfigurableOption,
  ProductVariant,
} from "@/app/lib/catalog/read";
import { ChoiceIndicator, optionAnchorId, optionCardClass } from "./parts";

export type Selection = Record<string, string | undefined>;

// Past this many values the inline swatch grid stops being a picker and starts
// being a wall - tyre sizes run to 150. Those get a trigger + searchable drawer
const INLINE_VALUE_LIMIT = 15;

// The list-mode equivalent, and a much lower bar: a LIST variant is a whole card - thumbnail, name,
// SKU and price - so ten of them already fill the column. A 1000-variant product rendered every one
// inline: 1030 buttons and an 84,000px page
const INLINE_VARIANT_LIMIT = 10;

// How many matches the drawer draws at once. The search is the way through a list this long, and
// mounting a thousand cards to scroll costs more than it gives
const VARIANT_RESULT_CAP = 60;

type Props = {
  options: ConfigurableOption[];
  variants: ProductVariant[];
  selection: Selection;
  onChange: (next: Selection) => void;
  invalid?: boolean;
};

export function VariantPicker({
  options,
  variants,
  selection,
  onChange,
  invalid = false,
}: Props) {
  const valuesByOption = useMemo(() => {
    const map = new Map<
      string,
      Map<string, { id: string; name: string; value: string }>
    >();
    for (const opt of options) {
      map.set(opt.id, new Map());
    }
    for (const v of variants) {
      for (const av of v.attributeValues) {
        const bucket = map.get(av.attribute.id);
        if (!bucket) continue;
        for (const sv of av.selectedValues) {
          if (!bucket.has(sv.id)) {
            bucket.set(sv.id, { id: sv.id, name: sv.name, value: sv.value });
          }
        }
      }
    }
    return map;
  }, [options, variants]);

  // Given the current partial selection, a value is "reachable" iff there's at least one variant whose attribute → value matches that value
  // AND every other currently-selected option matches that variant's own value. Lets us grey out impossible combinations
  const isReachable = (attributeId: string, valueId: string) => {
    return variants.some((variant) => {
      const variantValueForAttr = (attrId: string): string | undefined => {
        const av = variant.attributeValues.find(
          (x) => x.attribute.id === attrId,
        );
        return av?.selectedValues[0]?.id;
      };
      if (variantValueForAttr(attributeId) !== valueId) return false;
      for (const opt of options) {
        if (opt.id === attributeId) continue;
        const picked = selection[opt.id];
        if (picked && variantValueForAttr(opt.id) !== picked) return false;
      }
      return true;
    });
  };

  return (
    <div className="space-y-5">
      {options.map((opt) => {
        const values = Array.from(valuesByOption.get(opt.id)?.values() ?? []);
        const picked = selection[opt.id];
        const groupInvalid = invalid && !picked;
        return (
          <fieldset key={opt.id} id={optionAnchorId(opt.id)}>
            <legend className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {opt.name}
              {picked && (
                <span className="ml-2 normal-case text-foreground">
                  {values.find((v) => v.id === picked)?.name}
                </span>
              )}
            </legend>
            {values.length > INLINE_VALUE_LIMIT ? (
              <ValueSearchPicker
                optionName={opt.name}
                values={values}
                picked={picked}
                invalid={groupInvalid}
                isReachable={(valueId) => isReachable(opt.id, valueId)}
                onPick={(valueId) =>
                  onChange({ ...selection, [opt.id]: valueId })
                }
              />
            ) : (
            <div className="mt-2 flex flex-wrap gap-2">
              {values.map((v) => {
                const isPicked = picked === v.id;
                const reachable = isReachable(opt.id, v.id);
                return (
                  <button
                    key={v.id}
                    type="button"
                    onClick={() =>
                      onChange({
                        ...selection,
                        [opt.id]: isPicked ? undefined : v.id,
                      })
                    }
                    disabled={!reachable && !isPicked}
                    className={[
                      "min-w-[3rem] rounded-lg border px-3.5 py-2 text-sm font-medium transition",
                      isPicked
                        ? "border-foreground bg-foreground text-background shadow-sm"
                        : !reachable
                          ? "border-border/50 text-muted-foreground/50 line-through"
                          : groupInvalid
                            ? "border-destructive bg-destructive/5 hover:bg-destructive/10"
                            : "border-border hover:border-foreground/50 hover:bg-muted/50",
                    ].join(" ")}
                    aria-pressed={isPicked}
                  >
                    {v.name}
                  </button>
                );
              })}
            </div>
            )}
          </fieldset>
        );
      })}
    </div>
  );
}

type PickerValue = { id: string; name: string; value: string };

// Trigger + drawer for options with too many values to lay out inline Search filters on the value name; unreachable
// values stay listed but disabled so the shopper sees the size exists and just isn't available in this combo
function ValueSearchPicker({
  optionName,
  values,
  picked,
  invalid,
  isReachable,
  onPick,
}: {
  optionName: string;
  values: PickerValue[];
  picked: string | undefined;
  invalid: boolean;
  isReachable: (valueId: string) => boolean;
  onPick: (valueId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const pickedRef = useRef<HTMLButtonElement>(null);
  const pickedValue = values.find((v) => v.id === picked);
  const t = useTranslations("product.variants");

  // showModal() runs in Modal's effect (child effects fire first) and parks focus on the drawer's close button, so
  // autoFocus on the input never sticks - take it back here, where the effect runs after
  useEffect(() => {
    if (!open) return;
    searchRef.current?.focus();
    // 150 rows deep, opening at the top hides the shopper's current size
    pickedRef.current?.scrollIntoView({ block: "center" });
  }, [open]);

  const q = query.trim().toLowerCase();
  const shown = q
    ? values.filter((v) => v.name.toLowerCase().includes(q))
    : values;

  return (
    <>
      <button
        type="button"
        onClick={() => {
          setQuery("");
          setOpen(true);
        }}
        className={[
          "mt-2 flex w-full items-center justify-between gap-3 rounded-lg border px-4 py-3 text-left text-sm transition",
          invalid
            ? "border-destructive bg-destructive/5"
            : "border-border hover:border-foreground/50 hover:bg-muted/40",
        ].join(" ")}
      >
        <span className={pickedValue ? "font-medium" : "text-muted-foreground"}>
          {pickedValue ? pickedValue.name : t("choose", { option: optionName.toLowerCase() })}
        </span>
        <span className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
          {values.length} options
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
            className="size-4"
            aria-hidden="true"
          >
            <path d="M9 18l6-6-6-6" />
          </svg>
        </span>
      </button>

      {/* Only mount the rows while open - otherwise 150 buttons ship in the
          initial HTML for a list nobody has asked to see yet */}
      <Drawer open={open} onOpenChange={setOpen} side="right" title={optionName}>
        {open ? (
        <>
        <div className="border-b border-border p-4 pt-0">
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("search", { option: optionName.toLowerCase() })}
            aria-label={t("search", { option: optionName.toLowerCase() })}
            ref={searchRef}
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none focus:border-foreground/50"
          />
        </div>
        <ul className="min-h-0 flex-1 overflow-y-auto p-2">
          {shown.map((v) => {
            const isPicked = picked === v.id;
            const reachable = isReachable(v.id);
            return (
              <li key={v.id}>
                <button
                  type="button"
                  ref={isPicked ? pickedRef : undefined}
                  disabled={!reachable && !isPicked}
                  aria-pressed={isPicked}
                  onClick={() => {
                    onPick(v.id);
                    setOpen(false);
                  }}
                  className={[
                    "flex w-full items-center justify-between gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition",
                    isPicked
                      ? "bg-foreground font-medium text-background"
                      : !reachable
                        ? "text-muted-foreground/50 line-through"
                        : "hover:bg-muted",
                  ].join(" ")}
                >
                  <span>{v.name}</span>
                  {!reachable && !isPicked ? (
                    <span className="shrink-0 text-xs no-underline">
                      unavailable
                    </span>
                  ) : null}
                </button>
              </li>
            );
          })}
          {shown.length === 0 ? (
            <li className="px-3 py-6 text-center text-sm text-muted-foreground">
              {t("noMatch", { query })}
            </li>
          ) : null}
        </ul>
        </>
        ) : null}
      </Drawer>
    </>
  );
}

// Matches the picker's selection against the variants and returns the concrete child
// product when every option has a value AND a child exists with that exact combination
export function matchVariant(
  options: ConfigurableOption[],
  variants: ProductVariant[],
  selection: Selection,
): ProductVariant | null {
  if (options.some((o) => !selection[o.id])) return null;
  return (
    variants.find((variant) =>
      options.every((opt) => {
        const av = variant.attributeValues.find(
          (x) => x.attribute.id === opt.id,
        );
        return av?.selectedValues[0]?.id === selection[opt.id];
      }),
    ) ?? null
  );
}

export function ListVariantPicker({
  variants,
  selectedId,
  onChange,
  invalid = false,
}: {
  variants: ProductVariant[];
  selectedId: string | null;
  onChange: (id: string | null) => void;
  // No variant chosen after an add-to-cart attempt - flags the choice buttons
  // themselves rather than boxing the whole picker
  invalid?: boolean;
}) {
  const t = useTranslations("product.variants");
  if (variants.length > INLINE_VARIANT_LIMIT) {
    return (
      <VariantSearchPicker
        variants={variants}
        selectedId={selectedId}
        onChange={onChange}
        invalid={invalid}
      />
    );
  }
  return (
    <fieldset className="space-y-2">
      <legend className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {t("chooseVariant")}
      </legend>
      <div className="mt-2 grid gap-2">
        {variants.map((v) => (
          <VariantRow
            key={v.id}
            variant={v}
            isPicked={selectedId === v.id}
            invalid={invalid}
            onPick={() => onChange(selectedId === v.id ? null : v.id)}
          />
        ))}
      </div>
    </fieldset>
  );
}

// One variant as a row, shared by the inline list and the drawer so the two can't drift apart
function VariantRow({
  variant: v,
  isPicked,
  invalid,
  onPick,
  rowRef,
}: {
  variant: ProductVariant;
  isPicked: boolean;
  invalid: boolean;
  onPick: () => void;
  rowRef?: RefObject<HTMLButtonElement | null>;
}) {
  const tc = useTranslations("common");
  const out = !v.inStock;
  return (
    <button
      type="button"
      ref={rowRef}
      onClick={onPick}
      disabled={out && !isPicked}
      aria-pressed={isPicked}
      className={`${optionCardClass(isPicked, invalid)} ${out && !isPicked ? "opacity-50" : ""}`}
    >
      <ChoiceIndicator checked={isPicked} />
      {/* With no option attributes to label the choice, the name and the picture are all
          a shopper has to tell one variant from another - the placeholder keeps the rows
          aligned when only some children have art */}
      {v.mainImage ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={mediaThumb(v.mainImage.url, 96, v.mainImage.width)}
          alt={v.mainImage.altText || displayName(v.name, v.sku)}
          className="size-12 shrink-0 rounded-md border border-border object-cover"
          loading="lazy"
          decoding="async"
        />
      ) : (
        <span
          aria-hidden="true"
          className="flex size-12 shrink-0 items-center justify-center rounded-md border border-border bg-muted text-[10px] text-muted-foreground"
        >
          {tc("noImage")}
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium">
          {displayName(v.name, v.sku)}
        </span>
        <span className="block text-xs text-muted-foreground">
          SKU {v.sku}
          {out ? " · Out of stock" : ""}
        </span>
      </span>
      <span className="shrink-0 font-semibold tabular-nums">
        {formatPrice(v.price ?? v.defaultPrice, v.defaultCurrencyCode)}
      </span>
    </button>
  );
}

// Trigger + searchable drawer for list-mode products with more variants than fit a column - the same
// shape ValueSearchPicker gives an over-long attribute option, so both long pickers behave alike
//
// Search covers the SKU as well as the name: past a few hundred variants the SKU IS how a shopper
// (or someone reading it off an order) finds the one they mean
function VariantSearchPicker({
  variants,
  selectedId,
  onChange,
  invalid,
}: {
  variants: ProductVariant[];
  selectedId: string | null;
  onChange: (id: string | null) => void;
  invalid: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const pickedRef = useRef<HTMLButtonElement>(null);
  const picked = variants.find((v) => v.id === selectedId) ?? null;
  const t = useTranslations("product.variants");

  // Same reason as ValueSearchPicker: Modal parks focus on its close button in a child effect, so
  // the input has to claim it back here, where the effect runs later
  useEffect(() => {
    if (!open) return;
    searchRef.current?.focus();
    pickedRef.current?.scrollIntoView({ block: "center" });
  }, [open]);

  const q = query.trim().toLowerCase();
  const matches = useMemo(() => {
    if (!q) return variants;
    return variants.filter(
      (v) =>
        displayName(v.name, v.sku).toLowerCase().includes(q) ||
        v.sku.toLowerCase().includes(q),
    );
  }, [variants, q]);
  const shown = matches.slice(0, VARIANT_RESULT_CAP);
  const hidden = matches.length - shown.length;

  return (
    <fieldset className="space-y-2">
      <legend className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        {t("chooseVariant")}
      </legend>
      <button
        type="button"
        onClick={() => {
          setQuery("");
          setOpen(true);
        }}
        className={[
          "mt-2 flex w-full items-center justify-between gap-3 rounded-lg border px-4 py-3 text-left text-sm transition",
          invalid
            ? "border-destructive bg-destructive/5"
            : "border-border hover:border-foreground/50 hover:bg-muted/40",
        ].join(" ")}
      >
        <span className={picked ? "font-medium" : "text-muted-foreground"}>
          {picked ? displayName(picked.name, picked.sku) : t("chooseVariant")}
        </span>
        <span className="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
          {variants.length} variants
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            strokeLinecap="round"
            strokeLinejoin="round"
            className="size-4"
            aria-hidden="true"
          >
            <path d="M9 18l6-6-6-6" />
          </svg>
        </span>
      </button>

      {/* Rows only exist while the drawer is open - otherwise a thousand cards ship in the
          initial HTML for a list nobody has asked to see */}
      <Drawer
        open={open}
        onOpenChange={setOpen}
        side="right"
        title={t("chooseVariant")}
      >
        {open ? (
          <>
            <div className="border-b border-border p-4 pt-0">
              <input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("searchVariants")}
                aria-label={t("searchVariantsLabel")}
                ref={searchRef}
                className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm outline-none focus:border-foreground/50"
              />
            </div>
            <ul className="min-h-0 flex-1 space-y-2 overflow-y-auto p-2">
              {shown.map((v) => (
                <li key={v.id}>
                  <VariantRow
                    variant={v}
                    isPicked={selectedId === v.id}
                    invalid={false}
                    rowRef={selectedId === v.id ? pickedRef : undefined}
                    onPick={() => {
                      onChange(selectedId === v.id ? null : v.id);
                      setOpen(false);
                    }}
                  />
                </li>
              ))}
              {hidden > 0 ? (
                <li className="px-3 py-4 text-center text-xs text-muted-foreground">
                  {hidden} more match - keep typing to narrow it down
                </li>
              ) : null}
              {matches.length === 0 ? (
                <li className="px-3 py-6 text-center text-sm text-muted-foreground">
                  {t("noMatch", { query })}
                </li>
              ) : null}
            </ul>
          </>
        ) : null}
      </Drawer>
    </fieldset>
  );
}
