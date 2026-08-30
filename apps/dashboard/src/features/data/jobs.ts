import type { DataJobFieldsFragment, DataResourceFieldsFragment } from './Data.gql'

export type DataJob = DataJobFieldsFragment
export type DataResource = DataResourceFieldsFragment
export type DataOption = DataResourceFieldsFragment['options'][number]
export type DataFormat = { format: string; label: string; extension: string }

export function isRunning(job: Pick<DataJob, 'status'>): boolean {
  return job.status === 'queued' || job.status === 'running'
}

export function formatWhen(value?: string | null): string {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

// A required option with no value yet is what blocks the run button - checked here so both panels agree
export function missingRequired(
  options: readonly DataOption[],
  values: Record<string, string>,
  direction: 'import' | 'export',
): string[] {
  return options
    .filter((o) => (!o.appliesTo || o.appliesTo === direction) && o.required)
    .filter((o) => !(values[o.name] ?? o.default ?? ''))
    .map((o) => o.label || o.name)
}
