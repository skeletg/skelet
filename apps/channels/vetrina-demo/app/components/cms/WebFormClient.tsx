"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";

import {
  presignWebFormUploadAction,
  submitWebFormAction,
  type WebFormFile,
} from "@/app/lib/webforms/actions";
import {
  isLayout,
  isSectionVisible,
  visibleFields,
  type AnswerValue,
  type Field,
  type InlineForm,
  type MatrixValue,
  type RepeaterValue,
} from "./webform";

const INPUT_CLASS =
  "w-full rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-foreground/40";

const CAPTCHA_SRC: Record<string, string> = {
  turnstile: "https://challenges.cloudflare.com/turnstile/v0/api.js",
  hcaptcha: "https://js.hcaptcha.com/1/api.js",
};
const CAPTCHA_RESPONSE_FIELD: Record<string, string> = {
  turnstile: "cf-turnstile-response",
  hcaptcha: "h-captcha-response",
};
const CAPTCHA_WIDGET_CLASS: Record<string, string> = {
  turnstile: "cf-turnstile",
  hcaptcha: "h-captcha",
};

export function WebFormClient({ form }: { form: InlineForm }) {
  const t = useTranslations("webform");
  const [answers, setAnswers] = useState<Record<string, AnswerValue>>(() => {
    const seed: Record<string, AnswerValue> = {};
    for (const f of form.fields) {
      if (f.type === "hidden" && f.defaultValue) seed[f.key] = f.defaultValue;
    }
    return seed;
  });
  const [files, setFiles] = useState<Record<string, WebFormFile>>({});
  const [uploading, setUploading] = useState<Record<string, boolean>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  const honeypotRef = useRef<HTMLInputElement>(null);
  const rootRef = useRef<HTMLFormElement>(null);

  const provider = form.captchaProvider ?? "";
  const captchaOn = !!(form.settings.requireCaptcha && provider && form.captchaSiteKey);

  useEffect(() => {
    if (!captchaOn) return;
    const src = CAPTCHA_SRC[provider];
    if (!src || document.querySelector(`script[src="${src}"]`)) return;
    const s = document.createElement("script");
    s.src = src;
    s.async = true;
    s.defer = true;
    document.head.appendChild(s);
  }, [captchaOn, provider]);

  if (done) {
    return (
      <div className="mx-auto max-w-lg rounded-md border border-border bg-card p-6 text-center">
        <p className="text-sm">{done}</p>
      </div>
    );
  }

  const visible = visibleFields(form.fields, answers).filter((f) => f.type !== "hidden");

  function setAnswer(key: string, val: AnswerValue) {
    setAnswers((a) => ({ ...a, [key]: val }));
  }

  async function onFile(field: Field, file: File | null) {
    if (!file) {
      setFiles((f) => {
        const n = { ...f };
        delete n[field.key];
        return n;
      });
      return;
    }
    setUploading((u) => ({ ...u, [field.key]: true }));
    setErrors((e) => ({ ...e, [field.key]: "" }));
    try {
      const ticket = await presignWebFormUploadAction({
        formId: form.id,
        fieldKey: field.key,
        filename: file.name,
        contentType: file.type || "application/octet-stream",
        sizeBytes: file.size,
      });
      if ("error" in ticket) {
        setErrors((e) => ({ ...e, [field.key]: ticket.error }));
        return;
      }
      const put = await fetch(ticket.url, {
        method: "PUT",
        headers: { "Content-Type": file.type || "application/octet-stream" },
        body: file,
      });
      if (!put.ok) {
        setErrors((e) => ({ ...e, [field.key]: t("uploadFailed") }));
        return;
      }
      setFiles((f) => ({
        ...f,
        [field.key]: {
          fieldKey: field.key,
          objectKey: ticket.objectKey,
          name: file.name,
          size: file.size,
          mimeType: file.type || "application/octet-stream",
        },
      }));
    } finally {
      setUploading((u) => ({ ...u, [field.key]: false }));
    }
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setFormError(null);
    setErrors({});

    const payload: Record<string, AnswerValue> = {};
    for (const f of form.fields) {
      if (isLayout(f.type) || f.type === "file") continue;
      if (f.type === "hidden") {
        if (f.defaultValue) payload[f.key] = f.defaultValue;
        continue;
      }
      if (visible.some((v) => v.key === f.key) && answers[f.key] !== undefined) {
        payload[f.key] = answers[f.key];
      }
    }

    let captchaToken = "";
    if (captchaOn && rootRef.current) {
      const respName = CAPTCHA_RESPONSE_FIELD[provider];
      const el = rootRef.current.querySelector<HTMLTextAreaElement | HTMLInputElement>(
        `[name="${respName}"]`,
      );
      captchaToken = el?.value ?? "";
    }

    try {
      const res = await submitWebFormAction({
        formId: form.id,
        values: payload,
        files: Object.values(files),
        honeypot: honeypotRef.current?.value ?? "",
        captchaToken,
      });
      if (res.ok) {
        if (res.redirectUrl) {
          window.location.href = res.redirectUrl;
          return;
        }
        setDone(res.successMessage || t("thanks"));
        return;
      }
      const fieldErrs: Record<string, string> = {};
      for (const fe of res.errors) {
        if (fe.field) fieldErrs[fe.field] = fe.message;
      }
      setErrors(fieldErrs);
      const general = res.errors.find((x) => !x.field);
      setFormError(general ? general.message : res.errors.length ? t("fixErrors") : t("failed"));
    } finally {
      setSubmitting(false);
    }
  }

  const visKeys = new Set(visible.map((v) => v.key));

  const groups: { section?: Field; fields: Field[] }[] = [{ fields: [] }];
  for (const f of form.fields) {
    if (f.type === "section") {
      groups.push({ section: f, fields: [] });
      continue;
    }
    groups[groups.length - 1].fields.push(f);
  }

  const renderField = (f: Field, key: string) => {
    if (isLayout(f.type)) return <LayoutField key={key} field={f} />;
    if (!visKeys.has(f.key)) return null;
    return (
      <div key={key} className="grid gap-1.5">
        {f.type !== "consent" && (
          <label className="text-sm font-medium" htmlFor={`wf-${f.key}`}>
            {f.label}
            {f.required && <span className="ml-0.5 text-red-600">*</span>}
          </label>
        )}
        <FieldInput
          field={f}
          value={answers[f.key]}
          uploading={!!uploading[f.key]}
          uploadedName={files[f.key]?.name}
          onChange={(v) => setAnswer(f.key, v)}
          onFile={(file) => onFile(f, file)}
        />
        {f.helpText && <p className="text-xs text-muted-foreground">{f.helpText}</p>}
        {errors[f.key] && <p className="text-xs text-red-600">{errors[f.key]}</p>}
      </div>
    );
  };

  return (
    <form ref={rootRef} onSubmit={onSubmit} className="mx-auto grid max-w-lg gap-4" noValidate>
      {/* Honeypot: hidden from users, must stay empty. */}
      <div aria-hidden className="absolute left-[-9999px] h-0 w-0 overflow-hidden">
        <label>
          {t("honeypot")}
          <input ref={honeypotRef} type="text" name="company_website" tabIndex={-1} autoComplete="off" />
        </label>
      </div>

      {groups.map((g, gi) => {
        if (g.section && !isSectionVisible(g.section, answers)) return null;
        const body = g.fields.map((f, fi) => renderField(f, `${gi}.${fi}`));
        if (!g.section) {
          return (
            <div key={gi} className="grid gap-4">
              {body}
            </div>
          );
        }
        return (
          <fieldset key={gi} className={g.section.boxed ? "grid gap-4 rounded-md border border-border p-4" : "grid gap-4"}>
            {g.section.label && <legend className="px-1 text-base font-semibold">{g.section.label}</legend>}
            {g.section.helpText && <p className="text-sm text-muted-foreground">{g.section.helpText}</p>}
            {body}
          </fieldset>
        );
      })}

      {captchaOn && (
        <div
          className={CAPTCHA_WIDGET_CLASS[provider]}
          data-sitekey={form.captchaSiteKey}
        />
      )}

      {formError && <p className="text-sm text-red-600">{formError}</p>}

      <button
        type="submit"
        disabled={submitting || Object.values(uploading).some(Boolean)}
        className="w-fit rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background disabled:opacity-60"
      >
        {submitting ? t("submitting") : form.settings.submitLabel || t("submit")}
      </button>
    </form>
  );
}

function LayoutField({ field }: { field: Field }) {
  if (field.type === "divider") return <hr className="border-border" />;
  if (field.type === "heading") return <h3 className="text-lg font-semibold">{field.label}</h3>;
  return <p className="text-sm text-muted-foreground">{field.label}</p>;
}

function FieldInput({
  field,
  value,
  uploading,
  uploadedName,
  onChange,
  onFile,
}: {
  field: Field;
  value: AnswerValue;
  uploading: boolean;
  uploadedName?: string;
  onChange: (v: AnswerValue) => void;
  onFile: (file: File | null) => void;
}) {
  const t = useTranslations("webform");
  const tc = useTranslations("common");
  const id = `wf-${field.key}`;
  const str = typeof value === "string" ? value : "";
  switch (field.type) {
    case "textarea":
      return <textarea id={id} className={INPUT_CLASS} rows={4} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />;
    case "select":
      return (
        <select id={id} className={INPUT_CLASS} value={str} onChange={(e) => onChange(e.target.value)}>
          <option value="">{field.placeholder || "Select…"}</option>
          {(field.options ?? []).map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
      );
    case "radio":
      return (
        <div className="grid gap-1.5">
          {(field.options ?? []).map((o) => (
            <label key={o.value} className="flex items-center gap-2 text-sm">
              <input type="radio" name={id} checked={str === o.value} onChange={() => onChange(o.value)} />
              {o.label}
            </label>
          ))}
        </div>
      );
    case "checkbox_group": {
      const arr = Array.isArray(value) ? (value as string[]) : [];
      return (
        <div className="grid gap-1.5">
          {(field.options ?? []).map((o) => (
            <label key={o.value} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={arr.includes(o.value)}
                onChange={(e) => onChange(e.target.checked ? [...arr, o.value] : arr.filter((x) => x !== o.value))}
              />
              {o.label}
            </label>
          ))}
        </div>
      );
    }
    case "consent":
      return (
        <label className="flex items-start gap-2 text-sm">
          <input type="checkbox" checked={value === true} onChange={(e) => onChange(e.target.checked)} className="mt-0.5" />
          <span>
            {field.label}
            {field.required && <span className="ml-0.5 text-red-600">*</span>}
          </span>
        </label>
      );
    case "file":
      return (
        <div className="grid gap-1">
          <input
            id={id}
            type="file"
            className="text-sm"
            accept={(field.acceptMimes ?? []).join(",") || undefined}
            onChange={(e) => onFile(e.target.files?.[0] ?? null)}
          />
          {uploading && <p className="text-xs text-muted-foreground">{t("uploading")}</p>}
          {uploadedName && !uploading && <p className="text-xs text-green-700">{t("attached", { name: uploadedName })}</p>}
        </div>
      );
    case "number":
      return <input id={id} type="number" className={INPUT_CLASS} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />;
    case "email":
      return <input id={id} type="email" className={INPUT_CLASS} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />;
    case "tel":
      return <input id={id} type="tel" className={INPUT_CLASS} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />;
    case "url":
      return <input id={id} type="url" className={INPUT_CLASS} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />;
    case "date":
      return <input id={id} type="date" className={INPUT_CLASS} value={str} onChange={(e) => onChange(e.target.value)} />;
    case "datetime":
      return <input id={id} type="datetime-local" className={INPUT_CLASS} value={str} onChange={(e) => onChange(e.target.value)} />;
    case "time":
      return <input id={id} type="time" className={INPUT_CLASS} value={str} onChange={(e) => onChange(e.target.value)} />;
    case "matrix": {
      const m: MatrixValue = value && typeof value === "object" && !Array.isArray(value) ? (value as MatrixValue) : {};
      const setCell = (rowKey: string, col: string) => {
        if (field.multi) {
          const cur = Array.isArray(m[rowKey]) ? (m[rowKey] as string[]) : [];
          onChange({ ...m, [rowKey]: cur.includes(col) ? cur.filter((x) => x !== col) : [...cur, col] });
        } else {
          onChange({ ...m, [rowKey]: col });
        }
      };
      return (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr>
                <th />
                {(field.columns ?? []).map((c) => (
                  <th key={c.value} className="px-2 pb-1 text-center text-xs font-normal text-muted-foreground">{c.label}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {(field.rows ?? []).map((r) => (
                <tr key={r.key} className="border-t border-border">
                  <td className="py-1.5 pr-3">{r.label}</td>
                  {(field.columns ?? []).map((c) => {
                    const on = field.multi
                      ? Array.isArray(m[r.key]) && (m[r.key] as string[]).includes(c.value)
                      : m[r.key] === c.value;
                    return (
                      <td key={c.value} className="px-2 text-center">
                        <input type={field.multi ? "checkbox" : "radio"} name={`${id}-${r.key}`} checked={!!on} onChange={() => setCell(r.key, c.value)} />
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    }
    case "repeater": {
      const items: RepeaterValue = Array.isArray(value) ? (value as RepeaterValue) : [];
      const setItem = (i: number, patch: Record<string, AnswerValue>) =>
        onChange(items.map((it, j) => (j === i ? { ...it, ...patch } : it)));
      const max = field.maxItems;
      return (
        <div className="grid gap-2">
          {items.map((it, i) => (
            <div key={i} className="grid gap-2 rounded-md border border-border p-3">
              {(field.subFields ?? []).map((sf) => (
                <div key={sf.key} className="grid gap-1">
                  <label className="text-xs font-medium">
                    {sf.label}
                    {sf.required && <span className="ml-0.5 text-red-600">*</span>}
                  </label>
                  <FieldInput field={sf} value={it[sf.key]} uploading={false} onChange={(v) => setItem(i, { [sf.key]: v })} onFile={() => {}} />
                </div>
              ))}
              <button type="button" className="w-fit text-xs text-red-600" onClick={() => onChange(items.filter((_, j) => j !== i))}>
                {tc("remove")}
              </button>
            </div>
          ))}
          <button
            type="button"
            disabled={!!max && items.length >= max}
            className="w-fit rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
            onClick={() => onChange([...items, {}])}
          >
            {t("add", { label: field.label || t("item") })}
          </button>
        </div>
      );
    }
    default:
      return <input id={id} type="text" className={INPUT_CLASS} value={str} placeholder={field.placeholder} onChange={(e) => onChange(e.target.value)} />;
  }
}
