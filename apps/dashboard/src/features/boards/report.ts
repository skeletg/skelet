import type { BoardReportSize } from '@/graphql/schema.types'
import { cleanDefinition, type ReportDefinition } from '@/shared/analytics/kinds/definition'
import type { BoardReportFieldsFragment } from './Boards.gql'

export type ReportSize = BoardReportSize
export const REPORT_SIZE_LABEL: Record<ReportSize, string> = { SMALL: 'Small', MEDIUM: 'Medium', FULL: 'Full row' }

// A report as a board holds it: its title, its width on the board, and what it runs
export type BoardReport = { title: string; size: ReportSize; definition: ReportDefinition }

export const cleanReport = (r: BoardReportFieldsFragment): BoardReport => ({ title: r.title, size: r.size ?? 'MEDIUM', definition: cleanDefinition(r.definition) })
