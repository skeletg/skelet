// Field-schema types for the custom form builder - These mirror the Go domain
// (apps/backend/internal/webform/domain.go) exactly - the `fields` and
// `settings` JSON scalars round-trip through this shape - The backend
// re-validates everything; this type is the authoring contract

export type FieldType =
  | 'text'
  | 'textarea'
  | 'email'
  | 'number'
  | 'tel'
  | 'url'
  | 'date'
  | 'datetime'
  | 'time'
  | 'select'
  | 'radio'
  | 'checkbox_group'
  | 'consent'
  | 'file'
  | 'hidden'
  | 'heading'
  | 'paragraph'
  | 'divider'
  | 'section'
  | 'matrix'
  | 'repeater'

export const LAYOUT_TYPES: FieldType[] = ['heading', 'paragraph', 'divider']
export const CHOICE_TYPES: FieldType[] = ['select', 'radio', 'checkbox_group']
// Field types a repeater sub-field may be (no composites/files/layout)
export const SUBFIELD_TYPES: FieldType[] = [
  'text', 'textarea', 'email', 'number', 'tel', 'url', 'date', 'datetime', 'time',
  'select', 'radio', 'checkbox_group', 'consent',
]

export function isLayout(t: FieldType): boolean {
  return LAYOUT_TYPES.includes(t)
}
export function isChoice(t: FieldType): boolean {
  return CHOICE_TYPES.includes(t)
}

export type ConditionOp =
  | 'equals'
  | 'notEquals'
  | 'contains'
  | 'oneOf'
  | 'empty'
  | 'notEmpty'
  | 'gt'
  | 'lt'

export type Rule = {
  field: string
  op: ConditionOp
  value?: string
  values?: string[]
}

export type ConditionGroup = {
  logic: 'and' | 'or'
  rules?: Rule[]
  groups?: ConditionGroup[]
}

export type FieldOption = { value: string; label: string }
export type MatrixRow = { key: string; label: string }

export type Validation = {
  minLength?: number
  maxLength?: number
  min?: number
  max?: number
  pattern?: string
}

export type Field = {
  key: string
  type: FieldType
  label: string
  placeholder?: string
  helpText?: string
  required?: boolean
  options?: FieldOption[]
  validation?: Validation
  visibleIf?: ConditionGroup
  defaultValue?: string
  maxFileBytes?: number
  acceptMimes?: string[]
  // section
  boxed?: boolean
  // matrix
  rows?: MatrixRow[]
  columns?: FieldOption[]
  multi?: boolean
  // repeater
  subFields?: Field[]
  minItems?: number
  maxItems?: number
}

export type ConfirmationEmail = {
  enabled?: boolean
  toFieldKey?: string
  subject?: string
  body?: string
}

export type FormSettings = {
  submitLabel?: string
  successMessage?: string
  redirectUrl?: string
  notifyEmails?: string[]
  confirmation?: ConfirmationEmail
  consentKind?: string
  flowTrigger?: boolean
  requireCaptcha?: boolean
}

export type FormStatus = 'draft' | 'active'

// A submission answer value: scalar, list (multi-select/checkbox), boolean (consent), a matrix map (row ->
// selection[s]), or a repeater array (one object per instance)
export type MatrixValue = Record<string, string | string[]>
export type RepeaterValue = Array<Record<string, AnswerValue>>
export type AnswerValue =
  | string
  | number
  | boolean
  | string[]
  | MatrixValue
  | RepeaterValue
  | undefined

export const FIELD_TYPE_LABELS: Record<FieldType, string> = {
  text: 'Text',
  textarea: 'Paragraph text',
  email: 'Email',
  number: 'Number',
  tel: 'Phone',
  url: 'URL',
  date: 'Date',
  datetime: 'Date & time',
  time: 'Time',
  select: 'Dropdown',
  radio: 'Radio group',
  checkbox_group: 'Checkbox group',
  consent: 'Consent checkbox',
  file: 'File upload',
  hidden: 'Hidden',
  heading: 'Heading',
  paragraph: 'Paragraph',
  divider: 'Divider',
  section: 'Section',
  matrix: 'Matrix grid',
  repeater: 'Repeatable group',
}
