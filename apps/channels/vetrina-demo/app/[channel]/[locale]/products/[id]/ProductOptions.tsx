"use client";

import type { ChangeEvent } from "react";
import { useTranslations } from "next-intl";

import { useErrorText } from "@/app/components/account/AccountUI";

import { displayName } from "@/app/lib/display";
import type {
  ProductCustomOption,
  ProductCustomOptionValue,
} from "@/app/lib/catalog/read";
import { acceptAttr, formatBytes, type FileEntry } from "./option-files";
import { isSelect, isSingleSelect, priceLabel, textInputType } from "./option-rules";
import {
  ChoiceIndicator,
  optionCardClass,
  optionInputClass,
  optionInputInvalidClass,
} from "./parts";

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
  const t = useTranslations("product.options");
  const errorText = useErrorText();
  const price = priceLabel(option.priceValue, option.priceType, currency);
  const constraints: string[] = [];
  if (option.fileExtensions) constraints.push(option.fileExtensions);
  if (option.maxFileBytes != null)
    constraints.push(t("upTo", { size: formatBytes(option.maxFileBytes) }));
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
        <p className="mt-1 text-xs text-muted-foreground">{t("uploading", { name: file.filename })}</p>
      ) : null}
      {file?.status === "ready" ? (
        <p className="mt-1 text-xs text-emerald-600 dark:text-emerald-400">
          ✓ {file.filename}
        </p>
      ) : null}
      {file?.status === "error" ? (
        <p className="mt-1 text-xs text-red-600 dark:text-red-400">{errorText(file.error)}</p>
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
