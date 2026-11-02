// Field-schema types + conditional-visibility evaluator for the storefront form
// renderer

export type FieldType =
  | "text" | "textarea" | "email" | "number" | "tel" | "url"
  | "date" | "datetime" | "time"
  | "select" | "radio" | "checkbox_group" | "consent"
  | "file" | "hidden"
  | "heading" | "paragraph" | "divider"
  | "section" | "matrix" | "repeater";

export type ConditionOp =
  | "equals" | "notEquals" | "contains" | "oneOf" | "empty" | "notEmpty" | "gt" | "lt";

export type Rule = { field: string; op: ConditionOp; value?: string; values?: string[] };
export type ConditionGroup = { logic?: "and" | "or"; rules?: Rule[]; groups?: ConditionGroup[] };
export type FieldOption = { value: string; label: string };
export type MatrixRow = { key: string; label: string };
export type Validation = {
  minLength?: number; maxLength?: number; min?: number; max?: number; pattern?: string;
};

export type Field = {
  key: string;
  type: FieldType;
  label: string;
  placeholder?: string;
  helpText?: string;
  required?: boolean;
  options?: FieldOption[];
  validation?: Validation;
  visibleIf?: ConditionGroup;
  defaultValue?: string;
  maxFileBytes?: number;
  acceptMimes?: string[];
  boxed?: boolean;
  rows?: MatrixRow[];
  columns?: FieldOption[];
  multi?: boolean;
  subFields?: Field[];
  minItems?: number;
  maxItems?: number;
};

export type FormSettings = {
  submitLabel?: string;
  successMessage?: string;
  redirectUrl?: string;
  requireCaptcha?: boolean;
};

// The inlined `form` prop shape the backend puts on a `form` block
export type InlineForm = {
  id: string;
  name: string;
  slug: string;
  fields: Field[];
  settings: FormSettings;
  captchaProvider?: string;
  captchaSiteKey?: string;
};

export type MatrixValue = Record<string, string | string[]>;
export type RepeaterValue = Array<Record<string, AnswerValue>>;
export type AnswerValue =
  | string
  | number
  | boolean
  | string[]
  | MatrixValue
  | RepeaterValue
  | undefined;

const LAYOUT: FieldType[] = ["heading", "paragraph", "divider"];
export function isLayout(t: FieldType): boolean {
  return LAYOUT.includes(t);
}

export function visibleFields(fields: Field[], answers: Record<string, AnswerValue>): Field[] {
  const out: Field[] = [];
  let sectionVisible = true;
  for (const f of fields) {
    if (f.type === "section") {
      sectionVisible = evalGroup(f.visibleIf, answers);
      continue;
    }
    if (isLayout(f.type)) continue;
    if (sectionVisible && evalGroup(f.visibleIf, answers)) out.push(f);
  }
  return out;
}

export function isSectionVisible(section: Field, answers: Record<string, AnswerValue>): boolean {
  return evalGroup(section.visibleIf, answers);
}

function evalGroup(group: ConditionGroup | undefined, answers: Record<string, AnswerValue>): boolean {
  if (!group || ((group.rules?.length ?? 0) === 0 && (group.groups?.length ?? 0) === 0)) return true;
  const results = [
    ...(group.rules ?? []).map((r) => evalRule(r, answers)),
    ...(group.groups ?? []).map((g) => evalGroup(g, answers)),
  ];
  return group.logic === "or" ? results.some(Boolean) : results.every(Boolean);
}

function evalRule(rule: Rule, answers: Record<string, AnswerValue>): boolean {
  const got = answers[rule.field];
  switch (rule.op) {
    case "empty": return isEmpty(got);
    case "notEmpty": return !isEmpty(got);
    case "equals": return matches(got, rule.value ?? "", true);
    case "notEquals": return !matches(got, rule.value ?? "", true);
    case "contains": return matches(got, rule.value ?? "", false);
    case "oneOf": return (rule.values ?? []).some((v) => matches(got, v, true));
    case "gt":
    case "lt": {
      const a = toNumber(got);
      const b = toNumber(rule.value);
      if (a === undefined || b === undefined) return false;
      return rule.op === "gt" ? a > b : a < b;
    }
    default: return false;
  }
}

function isEmpty(v: AnswerValue): boolean {
  if (v === undefined || v === null) return true;
  if (typeof v === "string") return v.trim() === "";
  if (typeof v === "boolean") return !v;
  if (Array.isArray(v)) return v.length === 0;
  return false;
}

function matches(v: AnswerValue, target: string, exact: boolean): boolean {
  const cmp = (s: string) => (exact ? s === target : s.includes(target));
  if (typeof v === "string") return cmp(v);
  if (typeof v === "boolean") return cmp(String(v));
  if (typeof v === "number") return cmp(String(v));
  if (Array.isArray(v)) return v.some((s) => cmp(String(s)));
  return false;
}

function toNumber(v: AnswerValue): number | undefined {
  if (typeof v === "number") return v;
  if (typeof v === "string" && v.trim() !== "") {
    const n = Number(v);
    return Number.isFinite(n) ? n : undefined;
  }
  return undefined;
}
