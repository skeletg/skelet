import { gql } from '@apollo/client'
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core'
import type { CollectionCover } from './Collections.gql'

export type CategoryContent = {
  __typename?: 'CategoryContent'
  locale: string
  name: string
  shortDescription?: unknown | null
  description?: unknown | null
  metaTitle?: string | null
  metaDescription?: string | null
  metaKeywords?: string | null
  updatedAt: string
}

export type CategorySummary = {
  __typename?: 'Category'
  id: string
  slug: string
  urlPath: string
  position: number
  level: number
  isPublished: boolean
  seoNoindex: boolean
  name?: string | null
  productCount: number
  content: Array<{ locale: string; name: string }>
  children: CategorySummary[]
  createdAt: string
  updatedAt: string
}

export type CategoryStoreListing = {
  __typename?: 'CategoryStoreListing'
  categoryId: string
  channelId: string
  channel: { id: string; slug: string; name: string }
  isPublished: boolean
  sortOrderOverride?: number | null
  updatedAt: string
}

export type CategoryFacetOverride = {
  __typename?: 'CategoryFacetOverride'
  attributeId: string
  attributeSlug: string
  sortOrder: number
  hidden: boolean
}

export type CategoryDetail = {
  __typename?: 'Category'
  id: string
  slug: string
  urlPath: string
  position: number
  level: number
  isPublished: boolean
  seoNoindex: boolean
  name?: string | null
  shortDescription?: unknown | null
  description?: unknown | null
  metaTitle?: string | null
  metaDescription?: string | null
  contentLocale?: string | null
  content: CategoryContent[]
  coverImage?: CollectionCover | null
  parent?: { id: string; slug: string; urlPath: string; name?: string | null } | null
  ancestors: Array<{ id: string; slug: string; urlPath: string; name?: string | null }>
  products: Array<{ id: string; name?: string | null; sku?: string | null }>
  listings: CategoryStoreListing[]
  facetOverrides: CategoryFacetOverride[]
  createdAt: string
  updatedAt: string
}

export type RootCategoriesQueryVariables = { channelId?: string | null }
export type RootCategoriesQuery = { __typename?: 'Query'; rootCategories: CategorySummary[] }

export type CategoryQueryVariables = { id: string }
export type CategoryQuery = { __typename?: 'Query'; category: CategoryDetail }

export type CreateCategoryMutationVariables = {
  input: {
    parentId?: string | null
    slug: string
    position?: number | null
    isPublished?: boolean | null
    seoNoindex?: boolean | null
    locale: string
    name: string
    shortDescription?: unknown | null
    description?: unknown | null
    metaTitle?: string | null
    metaDescription?: string | null
    metaKeywords?: string | null
  }
}
export type CreateCategoryMutation = { __typename?: 'Mutation'; createCategory: CategoryDetail }

export type UpdateCategoryMutationVariables = {
  input: {
    id: string
    parentId?: string | null
    clearParent?: boolean | null
    slug?: string | null
    position?: number | null
    isPublished?: boolean | null
    seoNoindex?: boolean | null
  }
}
export type UpdateCategoryMutation = { __typename?: 'Mutation'; updateCategory: CategoryDetail }

export type DeleteCategoryMutationVariables = { id: string }
export type DeleteCategoryMutation = { __typename?: 'Mutation'; deleteCategory: boolean }

export type DeleteCategoryContentMutationVariables = { categoryId: string; locale: string }
export type DeleteCategoryContentMutation = { __typename?: 'Mutation'; deleteCategoryContent: boolean }

export type UpsertCategoryContentMutationVariables = {
  categoryId: string
  input: {
    locale: string
    name: string
    shortDescription?: unknown | null
    description?: unknown | null
    metaTitle?: string | null
    metaDescription?: string | null
    metaKeywords?: string | null
  }
}
export type UpsertCategoryContentMutation = { __typename?: 'Mutation'; upsertCategoryContent: CategoryDetail }

export type SetCategoryListingMutationVariables = {
  input: {
    categoryId: string
    channelId: string
    isPublished: boolean
    sortOrderOverride?: number | null
  }
}
export type SetCategoryListingMutation = { __typename?: 'Mutation'; setCategoryListing: CategoryDetail }

export type ClearCategoryListingMutationVariables = { categoryId: string; channelId: string }
export type ClearCategoryListingMutation = { __typename?: 'Mutation'; clearCategoryListing: CategoryDetail }

export type RequestCategoryCoverUploadMutationVariables = {
  input: {
    categoryId: string
    filename: string
    mimeType: string
    sizeBytes: number
    altText?: string | null
  }
}
export type RequestCategoryCoverUploadMutation = {
  __typename?: 'Mutation'
  requestCategoryCoverUpload: {
    __typename?: 'RequestCoverUploadPayload'
    uploadUrl: string
    expiresAt: string
  }
}

export type ConfirmCategoryCoverUploadMutationVariables = {
  categoryId: string
  width: number
  height: number
}
export type ConfirmCategoryCoverUploadMutation = {
  __typename?: 'Mutation'
  confirmCategoryCoverUpload: {
    __typename?: 'Category'
    id: string
    updatedAt: string
    coverImage?: CollectionCover | null
  }
}

export type ClearCategoryCoverMutationVariables = { categoryId: string }
export type ClearCategoryCoverMutation = {
  __typename?: 'Mutation'
  clearCategoryCover: {
    __typename?: 'Category'
    id: string
    updatedAt: string
    coverImage?: CollectionCover | null
  }
}

const CategorySummaryFragment = gql`
  fragment CategorySummaryFields on Category {
    id slug urlPath position level isPublished seoNoindex name productCount
    content { locale name }
    children {
      id slug urlPath position level isPublished seoNoindex name productCount
      content { locale name }
      children {
        id slug urlPath position level isPublished seoNoindex name productCount
        content { locale name }
        children { id slug urlPath position level isPublished name productCount content { locale name } children { id } }
      }
    }
    createdAt updatedAt
  }
`

const CategoryDetailFragment = gql`
  fragment CategoryDetailFields on Category {
    id slug urlPath position level isPublished seoNoindex
    name shortDescription description metaTitle metaDescription contentLocale
    content { locale name shortDescription description metaTitle metaDescription metaKeywords updatedAt }
    coverImage { url altText width height mimeType status }
    parent { id slug urlPath name }
    ancestors { id slug urlPath name }
    products { id name sku }
    listings {
      categoryId channelId
      channel { id slug name }
      isPublished sortOrderOverride updatedAt
    }
    facetOverrides { attributeId attributeSlug sortOrder hidden }
    createdAt updatedAt
  }
`

export const RootCategoriesDocument: DocumentNode<RootCategoriesQuery, RootCategoriesQueryVariables> = gql`
  ${CategorySummaryFragment}
  query RootCategories($channelId: ID) {
    rootCategories(channelId: $channelId) {
      ...CategorySummaryFields
    }
  }
`

export const CategoryDocument: DocumentNode<CategoryQuery, CategoryQueryVariables> = gql`
  ${CategoryDetailFragment}
  query Category($id: ID!) {
    category(id: $id) {
      ...CategoryDetailFields
    }
  }
`

export const CreateCategoryDocument: DocumentNode<CreateCategoryMutation, CreateCategoryMutationVariables> = gql`
  ${CategoryDetailFragment}
  mutation CreateCategory($input: CreateCategoryInput!) {
    createCategory(input: $input) {
      ...CategoryDetailFields
    }
  }
`

export const UpdateCategoryDocument: DocumentNode<UpdateCategoryMutation, UpdateCategoryMutationVariables> = gql`
  ${CategoryDetailFragment}
  mutation UpdateCategory($input: UpdateCategoryInput!) {
    updateCategory(input: $input) {
      ...CategoryDetailFields
    }
  }
`

export const DeleteCategoryDocument: DocumentNode<DeleteCategoryMutation, DeleteCategoryMutationVariables> = gql`
  mutation DeleteCategory($id: ID!) {
    deleteCategory(id: $id)
  }
`

// Refuses the default locale while other translations exist - the caller
// surfaces that error in its confirm dialog
export const DeleteCategoryContentDocument: DocumentNode<DeleteCategoryContentMutation, DeleteCategoryContentMutationVariables> = gql`
  mutation DeleteCategoryContent($categoryId: ID!, $locale: String!) {
    deleteCategoryContent(categoryId: $categoryId, locale: $locale)
  }
`

export const UpsertCategoryContentDocument: DocumentNode<UpsertCategoryContentMutation, UpsertCategoryContentMutationVariables> = gql`
  ${CategoryDetailFragment}
  mutation UpsertCategoryContent($categoryId: ID!, $input: UpsertCategoryContentInput!) {
    upsertCategoryContent(categoryId: $categoryId, input: $input) {
      ...CategoryDetailFields
    }
  }
`

export const SetCategoryListingDocument: DocumentNode<SetCategoryListingMutation, SetCategoryListingMutationVariables> = gql`
  ${CategoryDetailFragment}
  mutation SetCategoryListing($input: SetCategoryListingInput!) {
    setCategoryListing(input: $input) {
      ...CategoryDetailFields
    }
  }
`

export const ClearCategoryListingDocument: DocumentNode<ClearCategoryListingMutation, ClearCategoryListingMutationVariables> = gql`
  ${CategoryDetailFragment}
  mutation ClearCategoryListing($categoryId: ID!, $channelId: ID!) {
    clearCategoryListing(categoryId: $categoryId, channelId: $channelId) {
      ...CategoryDetailFields
    }
  }
`

export type SetCategoryFacetOverridesMutationVariables = {
  categoryId: string
  overrides: Array<{ attributeId: string; hidden?: boolean }>
}
export type SetCategoryFacetOverridesMutation = {
  __typename?: 'Mutation'
  setCategoryFacetOverrides: CategoryDetail
}

// List order IS the storefront order; an empty list resets the category to defaults
export const SetCategoryFacetOverridesDocument: DocumentNode<SetCategoryFacetOverridesMutation, SetCategoryFacetOverridesMutationVariables> = gql`
  ${CategoryDetailFragment}
  mutation SetCategoryFacetOverrides($categoryId: ID!, $overrides: [FacetOverrideInput!]!) {
    setCategoryFacetOverrides(categoryId: $categoryId, overrides: $overrides) {
      ...CategoryDetailFields
    }
  }
`

// The payload's non-null collection field stays unselected on purpose - the
// category resolver only fills uploadUrl/expiresAt
export const RequestCategoryCoverUploadDocument: DocumentNode<RequestCategoryCoverUploadMutation, RequestCategoryCoverUploadMutationVariables> = gql`
  mutation RequestCategoryCoverUpload($input: RequestCategoryCoverUploadInput!) {
    requestCategoryCoverUpload(input: $input) {
      uploadUrl
      expiresAt
    }
  }
`

export const ConfirmCategoryCoverUploadDocument: DocumentNode<ConfirmCategoryCoverUploadMutation, ConfirmCategoryCoverUploadMutationVariables> = gql`
  mutation ConfirmCategoryCoverUpload($categoryId: ID!, $width: Int!, $height: Int!) {
    confirmCategoryCoverUpload(categoryId: $categoryId, width: $width, height: $height) {
      id
      updatedAt
      coverImage { url altText width height mimeType status }
    }
  }
`

export const ClearCategoryCoverDocument: DocumentNode<ClearCategoryCoverMutation, ClearCategoryCoverMutationVariables> = gql`
  mutation ClearCategoryCover($categoryId: ID!) {
    clearCategoryCover(categoryId: $categoryId) {
      id
      updatedAt
      coverImage { url altText width height mimeType status }
    }
  }
`
