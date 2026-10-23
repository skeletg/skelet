import {
  AlignLeftIcon,
  CalendarClockIcon,
  CalendarIcon,
  ChevronsUpDownIcon,
  CircleDotIcon,
  ClockIcon,
  ListChecksIcon,
  PaperclipIcon,
  SquareCheckIcon,
  TypeIcon,
  type LucideIcon,
} from 'lucide-react'
import type { ProductOptionInputType } from '@/graphql/schema.types'

// Switching within a family keeps everything an option holds; across families its choices or its limits would mean nothing
export type OptionFamily = 'choice' | 'text' | 'date' | 'file'

export type InputTypeInfo = { label: string; hint: string; Icon: LucideIcon; family: OptionFamily; multiple?: boolean }

export const INPUT_TYPES: Record<ProductOptionInputType, InputTypeInfo> = {
  DROPDOWN: { label: 'Dropdown', hint: 'Pick one from a list', Icon: ChevronsUpDownIcon, family: 'choice' },
  RADIO: { label: 'Radio buttons', hint: 'Pick one, all shown', Icon: CircleDotIcon, family: 'choice' },
  CHECKBOX: { label: 'Checkboxes', hint: 'Tick any number', Icon: SquareCheckIcon, family: 'choice', multiple: true },
  MULTISELECT: { label: 'Multi-select list', hint: 'Pick several from a list', Icon: ListChecksIcon, family: 'choice', multiple: true },
  FIELD: { label: 'Short text', hint: 'One line, like a name', Icon: TypeIcon, family: 'text' },
  AREA: { label: 'Long text', hint: 'A paragraph, like a message', Icon: AlignLeftIcon, family: 'text' },
  DATE: { label: 'Date', hint: 'A calendar day', Icon: CalendarIcon, family: 'date' },
  DATE_TIME: { label: 'Date and time', hint: 'A day and a time', Icon: CalendarClockIcon, family: 'date' },
  TIME: { label: 'Time', hint: 'A time of day', Icon: ClockIcon, family: 'date' },
  FILE: { label: 'File upload', hint: 'An image or document', Icon: PaperclipIcon, family: 'file' },
}

export const TYPE_GROUPS: { title: string; types: ProductOptionInputType[] }[] = [
  { title: 'Choices you define', types: ['DROPDOWN', 'RADIO', 'CHECKBOX', 'MULTISELECT'] },
  { title: 'Something the customer provides', types: ['FIELD', 'AREA', 'DATE', 'DATE_TIME', 'TIME', 'FILE'] },
]

export const isChoice = (t: ProductOptionInputType) => INPUT_TYPES[t].family === 'choice'
export const isSingleChoice = (t: ProductOptionInputType) => isChoice(t) && !INPUT_TYPES[t].multiple
