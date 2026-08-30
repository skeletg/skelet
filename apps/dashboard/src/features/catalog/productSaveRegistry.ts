// Aliases - the staged-save registry is dashboard-wide, so these re-export shared/saving/saveRegistry.tsx under the names the product pages import
export {
  SaveRegistryContext as ProductSaveContext,
  useSaveSection as useProductSaveSection,
} from '@/shared/saving/saveRegistry'
export type {
  SectionDiffEntry,
  SectionRegistration,
  SaveRegistryValue as ProductSaveContextValue,
} from '@/shared/saving/saveRegistry'
