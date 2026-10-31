"use client";

import { useEffect, useMemo, useState, type ChangeEvent } from "react";
import { useTranslations } from "next-intl";

import { requestProductOptionFileUploadAction } from "@/app/lib/cart/actions";
import { roundHalfAwayFromZero } from "@/app/lib/format";
import type { ProductCustomOption } from "@/app/lib/catalog/read";
import { evaluateOptionRules, type ProductOptionRule } from "@/app/lib/optionRules";
import { visibleOptionPicks } from "@/app/lib/optionVisibility";
import { validateFile, type FileEntry } from "./option-files";
import {
  isSelect,
  isSingleSelect,
  optionContribution,
  valueContribution,
} from "./option-rules";

type SelState = Record<string, Set<string>>; // optionId → valueIds
type TextState = Record<string, string>; // optionId → text/date value
type FileState = Record<string, FileEntry>; // optionId → entry

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
  // Whether an option is required right now - its own setting, as the product's option rules change it
  isRequired: (option: ProductCustomOption) => boolean;
  // The price changes the fired rules add, as the cart line will list them
  ruleAdjustments: { id: string; label: string; amount: number }[];
  // A fired rule charges an amount this store has none of, so the cart would refuse this configuration
  rulePriceUnavailable: boolean;
};

export function useConfiguredOptions(
  options: ProductCustomOption[],
  cfg: { defaultPrice: number; effectivePrice: number; productSku: string },
  rules: ProductOptionRule[] = [],
): UseConfiguredOptions {
  const sorted = useMemo(
    () => [...options].sort((a, b) => a.sortOrder - b.sortOrder),
    [options],
  );

  const t = useTranslations("product.options");
  const tr = useTranslations("product.reviewForm");
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
    const clientError = validateFile(option, file, t);
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
      if (!put.ok) throw new Error(tr("uploadFailedStatus", { status: put.status }));
      setFiles((p) => ({
        ...p,
        [option.id]: { status: "ready", assetId: req.assetId, filename: file.name },
      }));
    } catch (err) {
      setFiles((p) => ({
        ...p,
        [option.id]: {
          status: "error",
          error: err instanceof Error ? err.message : tr("uploadFailed"),
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
  const { visible: visibleIds, picks } = visibleOptionPicks(
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
  const effects = evaluateOptionRules(rules, picks, cfg.defaultPrice);
  // Handed back already carrying what the rules make of isRequired, so every renderer marks and gates on it
  const visibleOptions = sorted
    .filter((o) => visibleIds.has(o.id))
    .map((o) => {
      const required = effects.isRequired(o);
      return required === o.isRequired ? o : { ...o, isRequired: required };
    });

  const unsatisfiedRequired = visibleOptions.filter(
    (o) => effects.isRequired(o) && !isSatisfied(o),
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
  if (effects.forcedQty != null) {
    if (effects.qtyConflict || (forcedQty != null && forcedQty !== effects.forcedQty)) {
      forcedQtyConflict = true;
    }
    forcedQty = effects.forcedQty;
  }
  const ruleTotal = effects.adjustments.reduce((sum, a) => sum + a.amount, 0);

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
    return total + ruleTotal;
  }, [visibleOptions, sel, text, files, cfg.defaultPrice, cfg.effectivePrice, ruleTotal]);
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
    isRequired: effects.isRequired,
    ruleAdjustments: effects.adjustments,
    rulePriceUnavailable: effects.priceUnavailable,
  };
}
