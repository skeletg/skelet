"use client";

import { useEffect, useMemo, useState, type ChangeEvent } from "react";

import { requestProductOptionFileUploadAction } from "@/app/lib/cart/actions";
import { displayName } from "@/app/lib/display";
import { formatPrice, roundHalfAwayFromZero } from "@/app/lib/format";
import type {
  ProductCustomOption,
  ProductCustomOptionValue,
} from "@/app/lib/catalog/read";
import { visibleOptionIds } from "@/app/lib/optionVisibility";
import {
  ChoiceIndicator,
  optionCardClass,
  optionInputClass,
  optionInputInvalidClass,
} from "./parts";

const SELECT_TYPES = ["dropdown", "radio", "checkbox", "multiselect"];
const SINGLE_SELECT_TYPES = ["dropdown", "radio"];

export function isSelect(t: string) {
  return SELECT_TYPES.includes(t);
}
export function isSingleSelect(t: string) {
  return SINGLE_SELECT_TYPES.includes(t);
}

function textInputType(t: string): string {
  switch (t) {
    case "date":
      return "date";
    case "date_time":
      return "datetime-local";
    case "time":
      return "time";
    default:
      return "text"; // field
  }
}

export function priceLabel(
  priceValue: number | null,
  priceType: string,
  currency: string,
): string | null {
  if (priceValue == null || priceValue === 0) return null;
  if (priceType === "percent") return `+${priceValue}%`;
  return `+${formatPrice(priceValue, currency)}`;
}

// Per-unit cents one selected value contributes — mirrors the backend's valueContribution so the live total previews
// what the server will charge (source-product price + delta; else the value's fixed/percent adjustment)
function valueContribution(
  basePrice: number,
  v: ProductCustomOptionValue,
): number {
  if (v.sourceProduct) return v.sourceProduct.defaultPrice + v.priceValue;
  if (v.priceType === "percent")
    return roundHalfAwayFromZero((basePrice * v.priceValue) / 100);
  return v.priceValue;
}

function optionContribution(basePrice: number, o: ProductCustomOption): number {
  if (o.priceValue == null) return 0;
  if (o.priceType === "percent")
    return roundHalfAwayFromZero((basePrice * o.priceValue) / 100);
  return o.priceValue;
}

type SelState = Record<string, Set<string>>; // optionId → valueIds
type TextState = Record<string, string>; // optionId → text/date value

export type FileEntry =
  | { status: "uploading"; filename: string }
  | { status: "ready"; assetId: string; filename: string }
  | { status: "error"; error: string };
type FileState = Record<string, FileEntry>; // optionId → entry

function acceptAttr(fileExtensions: string | null): string | undefined {
  if (!fileExtensions) return undefined;
  const exts = fileExtensions
    .split(",")
    .map((e) => e.trim().replace(/^\./, ""))
    .filter(Boolean)
    .map((e) => `.${e}`);
  return exts.length > 0 ? exts.join(",") : undefined;
}

function fileExt(name: string): string {
  const i = name.lastIndexOf(".");
  return i >= 0 ? name.slice(i + 1).toLowerCase() : "";
}

function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.round(n / 1024)} KB`;
  return `${n} bytes`;
}

// Mirror of the server-side constraint check so the customer gets instant feedback
function validateFile(option: ProductCustomOption, file: File): string | null {
  if (option.maxFileBytes != null && file.size > option.maxFileBytes) {
    return `File is too large (max ${formatBytes(option.maxFileBytes)}).`;
  }
  if (option.fileExtensions) {
    const allow = option.fileExtensions
      .split(",")
      .map((e) => e.trim().replace(/^\./, "").toLowerCase())
      .filter(Boolean);
    if (allow.length > 0 && !allow.includes(fileExt(file.name))) {
      return `Allowed file types: ${allow.join(", ")}.`;
    }
  }
  return null;
}

function initialSelections(options: ProductCustomOption[]): SelState {
  const out: SelState = {};
  for (const opt of options) {
    if (!isSelect(opt.inputType)) continue;
    const defaults = opt.values.filter((v) => v.isDefault).map((v) => v.id);
    out[opt.id] = new Set(
      isSingleSelect(opt.inputType) ? defaults.slice(0, 1) : defaults,
    );
  }
  return out;
}

export type ConfiguredOptionPayload =
  | { optionId: string; valueIds: string[] }
  | { optionId: string; fileAssetId: string }
  | { optionId: string; textValue: string };

export type UseConfiguredOptions = {
  visibleOptions: ProductCustomOption[];
  sel: SelState;
  text: TextState;
  files: FileState;
  toggleValue: (option: ProductCustomOption, valueId: string) => void;
  setOptionText: (optionId: string, value: string) => void;
  onFileChange: (option: ProductCustomOption, e: ChangeEvent<HTMLInputElement>) => void;
  missingRequired: string[];
  missingRequiredItems: Array<{ id: string; label: string }>;
  missingRequiredIds: string[];
  uploading: boolean;
  payload: ConfiguredOptionPayload[];
  forcedQty: number | null;
  forcedQtyConflict: boolean;
  configuredTotal: number;
  optionsDelta: number;
};

export function useConfiguredOptions(
  options: ProductCustomOption[],
  cfg: { defaultPrice: number; effectivePrice: number; productSku: string },
): UseConfiguredOptions {
  const sorted = useMemo(
    () => [...options].sort((a, b) => a.sortOrder - b.sortOrder),
    [options],
  );

  const [sel, setSel] = useState<SelState>(() => initialSelections(sorted));
  const [text, setText] = useState<TextState>({});
  const [files, setFiles] = useState<FileState>({});

  // Options can appear after mount (variant picked later) - seed their configured
  // defaults then too, but never clobber picks the shopper already made
  useEffect(() => {
    setSel((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const opt of sorted) {
        if (!isSelect(opt.inputType) || opt.id in next) continue;
        const defaults = opt.values.filter((v) => v.isDefault).map((v) => v.id);
        next[opt.id] = new Set(
          isSingleSelect(opt.inputType) ? defaults.slice(0, 1) : defaults,
        );
        changed = true;
      }
      return changed ? next : prev;
    });
  }, [sorted]);

  const onFileChange = async (
    option: ProductCustomOption,
    e: ChangeEvent<HTMLInputElement>,
  ) => {
    const file = e.target.files?.[0];
    e.target.value = ""; // allow re-picking the same file
    if (!file) return;
    const clientError = validateFile(option, file);
    if (clientError) {
      setFiles((p) => ({ ...p, [option.id]: { status: "error", error: clientError } }));
      return;
    }
    setFiles((p) => ({
      ...p,
      [option.id]: { status: "uploading", filename: file.name },
    }));
    try {
      const req = await requestProductOptionFileUploadAction({
        productSku: cfg.productSku,
        optionId: option.id,
        filename: file.name,
        mimeType: file.type || "application/octet-stream",
        sizeBytes: file.size,
      });
      if (!req.ok) throw new Error(req.error);
      const put = await fetch(req.uploadUrl, {
        method: "PUT",
        body: file,
        headers: { "Content-Type": file.type || "application/octet-stream" },
      });
      if (!put.ok) throw new Error(`Upload failed (HTTP ${put.status})`);
      setFiles((p) => ({
        ...p,
        [option.id]: { status: "ready", assetId: req.assetId, filename: file.name },
      }));
    } catch (err) {
      setFiles((p) => ({
        ...p,
        [option.id]: {
          status: "error",
          error: err instanceof Error ? err.message : "Upload failed",
        },
      }));
    }
  };

  const toggleValue = (option: ProductCustomOption, valueId: string) => {
    setSel((prev) => {
      const next: SelState = {};
      for (const k of Object.keys(prev)) next[k] = new Set(prev[k]);
      const bucket = next[option.id] ?? new Set<string>();
      if (isSingleSelect(option.inputType)) {
        if (bucket.has(valueId) && option.isRequired) return prev;
        bucket.clear();
        if (!prev[option.id]?.has(valueId)) bucket.add(valueId);
      } else if (bucket.has(valueId)) {
        bucket.delete(valueId);
      } else {
        bucket.add(valueId);
      }
      next[option.id] = bucket;
      return next;
    });
  };

  const setOptionText = (optionId: string, value: string) => {
    setText((prev) => ({ ...prev, [optionId]: value }));
  };

  const isSatisfied = (opt: ProductCustomOption): boolean => {
    if (isSelect(opt.inputType)) return (sel[opt.id]?.size ?? 0) > 0;
    if (opt.inputType === "file") return files[opt.id]?.status === "ready";
    return (text[opt.id] ?? "").trim() !== "";
  };

  // Conditional visibility: one ordered pass mirroring the server's evaluator
  const byId = useMemo(() => new Map(sorted.map((o) => [o.id, o])), [sorted]);
  const visibleIds = visibleOptionIds(
    sorted,
    (id) => {
      const o = byId.get(id);
      return o && isSelect(o.inputType) ? Array.from(sel[id] ?? []) : [];
    },
    (id) => {
      const o = byId.get(id);
      return o ? isSatisfied(o) : false;
    },
  );
  const visibleOptions = sorted.filter((o) => visibleIds.has(o.id));

  const unsatisfiedRequired = visibleOptions.filter(
    (o) => o.isRequired && !isSatisfied(o),
  );
  const missingRequired = unsatisfiedRequired.map((o) => o.title);
  const missingRequiredIds = unsatisfiedRequired.map((o) => o.id);
  const missingRequiredItems = unsatisfiedRequired.map((o) => ({
    id: o.id,
    label: o.title,
  }));
  // Only visible options gate the add - a controlling pick can hide a FILE option
  // mid-upload and that upload must not wedge the button at "Uploading"
  const uploading = visibleOptions.some(
    (o) => files[o.id]?.status === "uploading",
  );

  // Submission payload: one entry per VISIBLE option engaged with — a pick on a hidden
  // option is rejected server-side (PRODUCT_OPTION_HIDDEN)
  const payload = visibleOptions
    .map((opt): ConfiguredOptionPayload | null => {
      if (isSelect(opt.inputType)) {
        const ids = Array.from(sel[opt.id] ?? []);
        return ids.length > 0 ? { optionId: opt.id, valueIds: ids } : null;
      }
      if (opt.inputType === "file") {
        const f = files[opt.id];
        return f?.status === "ready"
          ? { optionId: opt.id, fileAssetId: f.assetId }
          : null;
      }
      const t = (text[opt.id] ?? "").trim();
      return t !== "" ? { optionId: opt.id, textValue: t } : null;
    })
    .filter((x): x is ConfiguredOptionPayload => x !== null);

  let forcedQty: number | null = null;
  // Two checked values forcing different quantities can only lose server-side - flag
  // the clash here so the shopper sees it before the round trip
  let forcedQtyConflict = false;
  for (const opt of visibleOptions) {
    if (!isSelect(opt.inputType)) continue;
    for (const vid of sel[opt.id] ?? []) {
      const v = opt.values.find((x) => x.id === vid);
      if (v?.forcedLineQty != null) {
        if (forcedQty != null && forcedQty !== v.forcedLineQty) {
          forcedQtyConflict = true;
        }
        forcedQty = v.forcedLineQty;
      }
    }
  }

  const configuredTotal = useMemo(() => {
    let total = cfg.effectivePrice;
    for (const opt of visibleOptions) {
      if (isSelect(opt.inputType)) {
        for (const vid of sel[opt.id] ?? []) {
          const v = opt.values.find((x) => x.id === vid);
          if (v)
            total += roundHalfAwayFromZero(
              valueContribution(cfg.defaultPrice, v) * v.quantity,
            );
        }
      } else if (opt.inputType === "file") {
        if (files[opt.id]?.status === "ready") total += optionContribution(cfg.defaultPrice, opt);
      } else if ((text[opt.id] ?? "").trim() !== "") {
        total += optionContribution(cfg.defaultPrice, opt);
      }
    }
    return total;
  }, [visibleOptions, sel, text, files, cfg.defaultPrice, cfg.effectivePrice]);
  const optionsDelta = configuredTotal - cfg.effectivePrice;

  return {
    visibleOptions,
    sel,
    text,
    files,
    toggleValue,
    setOptionText,
    onFileChange,
    missingRequired,
    missingRequiredIds,
    missingRequiredItems,
    uploading,
    payload,
    forcedQty,
    forcedQtyConflict,
    configuredTotal,
    optionsDelta,
  };
}

export function OptionControl({
  option,
  currency,
  picked,
  textValue,
  file,
  onToggle,
  onText,
  onFile,
  invalid = false,
  fileDisabled = false,
  fileDisabledHint,
}: {
  option: ProductCustomOption;
  currency: string;
  picked: Set<string>;
  textValue: string;
  file?: FileEntry;
  onToggle: (valueId: string) => void;
  onText: (value: string) => void;
  onFile: (e: ChangeEvent<HTMLInputElement>) => void;
  // Required-but-unset after an add-to-cart attempt
  invalid?: boolean;
  // FILE uploads presign against a concrete SKU - block them until the caller has one
  fileDisabled?: boolean;
  fileDisabledHint?: string;
}) {
  const requiredMark = option.isRequired ? (
    <span className="ml-1 text-red-500">*</span>
  ) : (
    <span className="ml-1 normal-case text-muted-foreground">(optional)</span>
  );

  if (option.inputType === "file") {
    return (
      <FileControl
        option={option}
        currency={currency}
        file={file}
        requiredMark={requiredMark}
        onFile={onFile}
        invalid={invalid}
        disabled={fileDisabled}
        disabledHint={fileDisabledHint}
      />
    );
  }

  if (isSelect(option.inputType)) {
    // Values arrive ordered by sort_order from the backend
    const sorted = option.values;
    const multi = !isSingleSelect(option.inputType);
    return (
      <fieldset>
        <legend className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {option.title}
          {requiredMark}
        </legend>
        {option.helpText ? (
          <p className="mt-1 text-xs text-muted-foreground">{option.helpText}</p>
        ) : null}
        <ul className="mt-2 space-y-2">
          {sorted.map((v) => (
            <ValueRow
              key={v.id}
              value={v}
              currency={currency}
              multi={multi}
              optionId={option.id}
              checked={picked.has(v.id)}
              onToggle={() => onToggle(v.id)}
              invalid={invalid}
            />
          ))}
        </ul>
      </fieldset>
    );
  }

  const isArea = option.inputType === "area";
  const label = (
    <label
      htmlFor={`opt-${option.id}`}
      className="text-xs font-semibold uppercase tracking-wider text-muted-foreground"
    >
      {option.title}
      {requiredMark}
      {priceLabel(option.priceValue, option.priceType, currency) ? (
        <span className="ml-2 normal-case text-muted-foreground">
          {priceLabel(option.priceValue, option.priceType, currency)}
        </span>
      ) : null}
    </label>
  );

  return (
    <div>
      {label}
      {option.helpText ? (
        <p className="mt-1 text-xs text-muted-foreground">{option.helpText}</p>
      ) : null}
      {isArea ? (
        <textarea
          id={`opt-${option.id}`}
          required={option.isRequired}
          maxLength={option.maxChars ?? undefined}
          value={textValue}
          onChange={(e) => onText(e.target.value)}
          rows={3}
          className={`mt-2 ${invalid ? optionInputInvalidClass : optionInputClass}`}
        />
      ) : (
        <input
          id={`opt-${option.id}`}
          type={textInputType(option.inputType)}
          required={option.isRequired}
          maxLength={option.maxChars ?? undefined}
          value={textValue}
          onChange={(e) => onText(e.target.value)}
          className={`mt-2 ${invalid ? optionInputInvalidClass : optionInputClass}`}
        />
      )}
      {option.maxChars ? (
        <p className="mt-1 text-xs text-muted-foreground">
          {textValue.length}/{option.maxChars}
        </p>
      ) : null}
    </div>
  );
}
function FileControl({
  option,
  currency,
  file,
  requiredMark,
  onFile,
  invalid = false,
  disabled = false,
  disabledHint,
}: {
  option: ProductCustomOption;
  currency: string;
  file?: FileEntry;
  requiredMark: React.ReactNode;
  onFile: (e: ChangeEvent<HTMLInputElement>) => void;
  invalid?: boolean;
  disabled?: boolean;
  disabledHint?: string;
}) {
  const price = priceLabel(option.priceValue, option.priceType, currency);
  const constraints: string[] = [];
  if (option.fileExtensions) constraints.push(option.fileExtensions);
  if (option.maxFileBytes != null)
    constraints.push(`up to ${formatBytes(option.maxFileBytes)}`);
  return (
    <div>
      <label
        htmlFor={`opt-${option.id}`}
        className="text-xs font-semibold uppercase tracking-wider text-muted-foreground"
      >
        {option.title}
        {requiredMark}
        {price ? (
          <span className="ml-2 normal-case text-muted-foreground">{price}</span>
        ) : null}
      </label>
      {option.helpText ? (
        <p className="mt-1 text-xs text-muted-foreground">{option.helpText}</p>
      ) : null}
      <input
        id={`opt-${option.id}`}
        type="file"
        accept={acceptAttr(option.fileExtensions)}
        onChange={onFile}
        disabled={disabled}
        className={`mt-2 block w-full text-sm text-muted-foreground file:mr-3 file:cursor-pointer file:rounded-lg file:border file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-foreground file:transition hover:file:bg-muted/70 disabled:opacity-50 disabled:file:cursor-not-allowed ${
          invalid
            ? "file:border-destructive file:bg-destructive/5"
            : "file:border-border file:bg-muted"
        }`}
      />
      {disabled && disabledHint ? (
        <p className="mt-1 text-xs text-muted-foreground">{disabledHint}</p>
      ) : null}
      {constraints.length > 0 ? (
        <p className="mt-1 text-xs text-muted-foreground">{constraints.join(" · ")}</p>
      ) : null}
      {file?.status === "uploading" ? (
        <p className="mt-1 text-xs text-muted-foreground">Uploading {file.filename}…</p>
      ) : null}
      {file?.status === "ready" ? (
        <p className="mt-1 text-xs text-emerald-600 dark:text-emerald-400">
          ✓ {file.filename}
        </p>
      ) : null}
      {file?.status === "error" ? (
        <p className="mt-1 text-xs text-red-600 dark:text-red-400">{file.error}</p>
      ) : null}
    </div>
  );
}

function ValueRow({
  value,
  currency,
  multi,
  optionId,
  checked,
  onToggle,
  invalid = false,
}: {
  value: ProductCustomOptionValue;
  currency: string;
  multi: boolean;
  optionId: string;
  checked: boolean;
  onToggle: () => void;
  invalid?: boolean;
}) {
  const price = priceLabel(value.priceValue, value.priceType, currency);
  return (
    <li>
      <label className={`${optionCardClass(checked, invalid)} cursor-pointer`}>
        <input
          type={multi ? "checkbox" : "radio"}
          name={`opt-${optionId}`}
          checked={checked}
          onChange={onToggle}
          className="sr-only"
        />
        <ChoiceIndicator checked={checked} multi={multi} />
        <span className="min-w-0 flex-1 text-sm">
          {value.title}
          {value.sourceProduct ? (
            <span className="mt-0.5 block text-xs font-normal text-muted-foreground">
              {displayName(value.sourceProduct.name, value.sourceProduct.sku)}
            </span>
          ) : null}
        </span>
        {value.forcedLineQty != null ? (
          <span className="shrink-0 text-xs text-muted-foreground">
            qty {value.forcedLineQty}
          </span>
        ) : null}
        {value.quantity !== 1 ? (
          <span className="shrink-0 text-xs text-muted-foreground">
            ×{value.quantity}
          </span>
        ) : null}
        {price ? (
          <span className="shrink-0 text-xs font-medium text-foreground">
            {price}
          </span>
        ) : null}
      </label>
    </li>
  );
}
