import type { CellData, RowData, TableFeatures } from '@tanstack/react-table'

declare module '@tanstack/react-table' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- the interface being augmented requires all three params
  interface ColumnMeta<TFeatures extends TableFeatures, TData extends RowData, TValue extends CellData> {
    label?: string // a column id is a slug and a header can be JSX, so the controls and exporter need this
    noExport?: boolean // selection and action columns have nothing to put in a file
  }
}
