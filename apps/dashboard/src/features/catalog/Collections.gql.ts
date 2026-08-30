import { gql } from '@apollo/client'
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core'
import type { ProductMediaStatus } from '@/graphql/schema.types'

export type MetadataEntry = { __typename?: 'MetadataItem'; key: string; value: string }

export type CollectionCover = {
  __typename?: 'CollectionCover'
  url: string
  altText: string
  width: number
  height: number
  mimeType: string
  status: ProductMediaStatus
}

export type CollectionContent = {
  __typename?: 'CollectionContent'
  locale: string
  name: string
  description?: unknown | null
  metaTitle?: string | null
  metaDescription?: string | null
  metaKeywords?: string | null
  updatedAt: string
}

export type CollectionProduct = {
  __typename?: 'Product'
  id: string
  name?: string | null
  sku?: string | null
  defaultPrice: number
  defaultCurrencyCode: string
  thumbnail?: {
    __typename?: 'ProductMedia'
    id: string
    url: string
    altText: string
    status: ProductMediaStatus
  } | null
}

export type CollectionSummary = {
  __typename?: 'Collection'
  id: string
  slug: string
  isPublished: boolean
  seoNoindex: boolean
  name?: string | null
  content: Array<Pick<CollectionContent, '__typename' | 'locale' | 'name'>>
  coverImage?: Pick<CollectionCover, '__typename' | 'url' | 'altText' | 'status'> | null
  products: Array<Pick<CollectionProduct, '__typename' | 'id'>>
  createdAt: string
  updatedAt: string
}

export type CollectionDetail = {
  __typename?: 'Collection'
  id: string
  slug: string
  isPublished: boolean
  seoNoindex: boolean
  name?: string | null
  description?: unknown | null
  metaTitle?: string | null
  metaDescription?: string | null
  metaKeywords?: string | null
  contentLocale?: string | null
  content: CollectionContent[]
  coverImage?: CollectionCover | null
  products: CollectionProduct[]
  metadata: MetadataEntry[]
  privateMetadata: MetadataEntry[]
  facetOverrides: Array<{
    attributeId: string
    attributeSlug: string
    sortOrder: number
    hidden: boolean
  }>
  createdAt: string
  updatedAt: string
}

export type CollectionsQueryVariables = Record<string, never>
export type CollectionsQuery = { __typename?: 'Query'; collections: CollectionSummary[] }

export type CollectionQueryVariables = { id: string }
export type CollectionQuery = { __typename?: 'Query'; collection: CollectionDetail }

export type CreateCollectionInput = {
  slug: string
  isPublished?: boolean
  seoNoindex?: boolean
  metadata?: Array<{ key: string; value: string }>
  privateMetadata?: Array<{ key: string; value: string }>
  locale: string
  name: string
  description?: unknown
  metaTitle?: string
  metaDescription?: string
  metaKeywords?: string
}

export type UpdateCollectionInput = {
  id: string
  slug?: string
  isPublished?: boolean
  seoNoindex?: boolean
  metadata?: Array<{ key: string; value: string }>
  privateMetadata?: Array<{ key: string; value: string }>
}

export type UpsertCollectionContentInput = {
  name: string
  description?: unknown
  metaTitle?: string
  metaDescription?: string
  metaKeywords?: string
}

export type RequestCollectionCoverUploadInput = {
  collectionId: string
  filename: string
  mimeType: string
  sizeBytes: number
  altText?: string
}

export type CreateCollectionMutationVariables = { input: CreateCollectionInput }
export type CreateCollectionMutation = {
  __typename?: 'Mutation'
  createCollection: { __typename?: 'Collection'; id: string; slug: string; name?: string | null }
}

export type UpdateCollectionMutationVariables = { input: UpdateCollectionInput }
export type UpdateCollectionMutation = {
  __typename?: 'Mutation'
  updateCollection: {
    __typename?: 'Collection'
    id: string
    slug: string
    isPublished: boolean
    seoNoindex: boolean
    updatedAt: string
    metadata: MetadataEntry[]
    privateMetadata: MetadataEntry[]
  }
}

export type DeleteCollectionMutationVariables = { id: string }
export type DeleteCollectionMutation = { __typename?: 'Mutation'; deleteCollection: boolean }

export type UpsertCollectionContentMutationVariables = {
  collectionId: string
  locale: string
  input: UpsertCollectionContentInput
}
export type UpsertCollectionContentMutation = {
  __typename?: 'Mutation'
  upsertCollectionContent: {
    __typename?: 'Collection'
    id: string
    name?: string | null
    description?: unknown | null
    metaTitle?: string | null
    metaDescription?: string | null
    metaKeywords?: string | null
    updatedAt: string
    content: CollectionContent[]
  }
}

export type DeleteCollectionContentMutationVariables = { collectionId: string; locale: string }
export type DeleteCollectionContentMutation = {
  __typename?: 'Mutation'
  deleteCollectionContent: boolean
}

export type AddProductToCollectionMutationVariables = {
  collectionId: string
  productId: string
  sortOrder?: number
}
export type RemoveProductFromCollectionMutationVariables = {
  collectionId: string
  productId: string
}
export type ReorderCollectionProductsMutationVariables = {
  collectionId: string
  productIds: string[]
}

type CollectionProductsMutationPayload = {
  __typename?: 'Mutation'
  addProductToCollection?: {
    __typename?: 'Collection'
    id: string
    updatedAt: string
    products: CollectionProduct[]
  }
  removeProductFromCollection?: {
    __typename?: 'Collection'
    id: string
    updatedAt: string
    products: CollectionProduct[]
  }
  reorderCollectionProducts?: {
    __typename?: 'Collection'
    id: string
    updatedAt: string
    products: CollectionProduct[]
  }
}

export type AddProductToCollectionMutation = Required<
  Pick<CollectionProductsMutationPayload, '__typename' | 'addProductToCollection'>
>
export type RemoveProductFromCollectionMutation = Required<
  Pick<CollectionProductsMutationPayload, '__typename' | 'removeProductFromCollection'>
>
export type ReorderCollectionProductsMutation = Required<
  Pick<CollectionProductsMutationPayload, '__typename' | 'reorderCollectionProducts'>
>

export type RequestCollectionCoverUploadMutationVariables = {
  input: RequestCollectionCoverUploadInput
}
export type RequestCollectionCoverUploadMutation = {
  __typename?: 'Mutation'
  requestCollectionCoverUpload: {
    __typename?: 'RequestCoverUploadPayload'
    uploadUrl: string
    expiresAt: string
    collection: { __typename?: 'Collection'; id: string; slug: string }
  }
}

export type ConfirmCollectionCoverUploadMutationVariables = {
  collectionId: string
  width: number
  height: number
}
export type ConfirmCollectionCoverUploadMutation = {
  __typename?: 'Mutation'
  confirmCollectionCoverUpload: {
    __typename?: 'Collection'
    id: string
    updatedAt: string
    coverImage?: CollectionCover | null
  }
}

export type ClearCollectionCoverMutationVariables = { collectionId: string }
export type ClearCollectionCoverMutation = {
  __typename?: 'Mutation'
  clearCollectionCover: {
    __typename?: 'Collection'
    id: string
    updatedAt: string
    coverImage?: CollectionCover | null
  }
}

// Flat product list for the "add product" picker on the collection page.
// Keyed by id because addProductToCollection takes a productId, not a SKU.
export type CollectionProductPickerQueryVariables = Record<string, never>
export type CollectionProductPickerQuery = {
  __typename?: 'Query'
  products: Array<{
    __typename?: 'Product'
    id: string
    sku?: string | null
    name?: string | null
  }>
}

export const CollectionProductPickerDocument = gql`
  query CollectionProductPicker {
    products {
      id
      sku
      name
    }
  }
` as unknown as DocumentNode<
  CollectionProductPickerQuery,
  CollectionProductPickerQueryVariables
>

export const CollectionsDocument = gql`
  query Collections {
    collections {
      id
      slug
      isPublished
      seoNoindex
      name
      content {
        locale
        name
      }
      coverImage {
        url
        altText
        status
      }
      products {
        id
      }
      createdAt
      updatedAt
    }
  }
` as unknown as DocumentNode<CollectionsQuery, CollectionsQueryVariables>

export const CollectionDocument = gql`
  query Collection($id: ID!) {
    collection(id: $id) {
      id
      slug
      isPublished
      seoNoindex
      name
      description
      metaTitle
      metaDescription
      metaKeywords
      contentLocale
      content {
        locale
        name
        description
        metaTitle
        metaDescription
        metaKeywords
        updatedAt
      }
      coverImage {
        url
        altText
        width
        height
        mimeType
        status
      }
      products {
        id
        name
        sku
        defaultPrice
        defaultCurrencyCode
        thumbnail {
          id
          url
          altText
          status
        }
      }
      metadata {
        key
        value
      }
      privateMetadata {
        key
        value
      }
      facetOverrides {
        attributeId
        attributeSlug
        sortOrder
        hidden
      }
      createdAt
      updatedAt
    }
  }
` as unknown as DocumentNode<CollectionQuery, CollectionQueryVariables>

export type SetCollectionFacetOverridesMutationVariables = {
  collectionId: string
  overrides: Array<{ attributeId: string; hidden?: boolean }>
}
export type SetCollectionFacetOverridesMutation = {
  __typename?: 'Mutation'
  setCollectionFacetOverrides: { id: string; updatedAt: string }
}

// List order IS the storefront order; an empty list resets the collection to defaults
export const SetCollectionFacetOverridesDocument = gql`
  mutation SetCollectionFacetOverrides($collectionId: ID!, $overrides: [FacetOverrideInput!]!) {
    setCollectionFacetOverrides(collectionId: $collectionId, overrides: $overrides) {
      id
      updatedAt
    }
  }
` as unknown as DocumentNode<
  SetCollectionFacetOverridesMutation,
  SetCollectionFacetOverridesMutationVariables
>

export const CreateCollectionDocument = gql`
  mutation CreateCollection($input: CreateCollectionInput!) {
    createCollection(input: $input) {
      id
      slug
      name
    }
  }
` as unknown as DocumentNode<CreateCollectionMutation, CreateCollectionMutationVariables>

export const UpdateCollectionDocument = gql`
  mutation UpdateCollection($input: UpdateCollectionInput!) {
    updateCollection(input: $input) {
      id
      slug
      isPublished
      seoNoindex
      updatedAt
      metadata {
        key
        value
      }
      privateMetadata {
        key
        value
      }
    }
  }
` as unknown as DocumentNode<UpdateCollectionMutation, UpdateCollectionMutationVariables>

export const DeleteCollectionDocument = gql`
  mutation DeleteCollection($id: ID!) {
    deleteCollection(id: $id)
  }
` as unknown as DocumentNode<DeleteCollectionMutation, DeleteCollectionMutationVariables>

export const UpsertCollectionContentDocument = gql`
  mutation UpsertCollectionContent(
    $collectionId: ID!
    $locale: String!
    $input: UpsertCollectionContentInput!
  ) {
    upsertCollectionContent(collectionId: $collectionId, locale: $locale, input: $input) {
      id
      name
      description
      metaTitle
      metaDescription
      metaKeywords
      updatedAt
      content {
        locale
        name
        description
        metaTitle
        metaDescription
        metaKeywords
        updatedAt
      }
    }
  }
` as unknown as DocumentNode<
  UpsertCollectionContentMutation,
  UpsertCollectionContentMutationVariables
>

export const DeleteCollectionContentDocument = gql`
  mutation DeleteCollectionContent($collectionId: ID!, $locale: String!) {
    deleteCollectionContent(collectionId: $collectionId, locale: $locale)
  }
` as unknown as DocumentNode<
  DeleteCollectionContentMutation,
  DeleteCollectionContentMutationVariables
>

const collectionProductsSelection = gql`
  fragment CollectionProductsSelection on Collection {
    id
    updatedAt
    products {
      id
      name
      sku
      defaultPrice
      defaultCurrencyCode
      thumbnail {
        id
        url
        altText
        status
      }
    }
  }
`

export const AddProductToCollectionDocument = gql`
  mutation AddProductToCollection($collectionId: ID!, $productId: ID!, $sortOrder: Int) {
    addProductToCollection(collectionId: $collectionId, productId: $productId, sortOrder: $sortOrder) {
      ...CollectionProductsSelection
    }
  }
  ${collectionProductsSelection}
` as unknown as DocumentNode<
  AddProductToCollectionMutation,
  AddProductToCollectionMutationVariables
>

export const RemoveProductFromCollectionDocument = gql`
  mutation RemoveProductFromCollection($collectionId: ID!, $productId: ID!) {
    removeProductFromCollection(collectionId: $collectionId, productId: $productId) {
      ...CollectionProductsSelection
    }
  }
  ${collectionProductsSelection}
` as unknown as DocumentNode<
  RemoveProductFromCollectionMutation,
  RemoveProductFromCollectionMutationVariables
>

export const ReorderCollectionProductsDocument = gql`
  mutation ReorderCollectionProducts($collectionId: ID!, $productIds: [ID!]!) {
    reorderCollectionProducts(collectionId: $collectionId, productIds: $productIds) {
      ...CollectionProductsSelection
    }
  }
  ${collectionProductsSelection}
` as unknown as DocumentNode<
  ReorderCollectionProductsMutation,
  ReorderCollectionProductsMutationVariables
>

export const RequestCollectionCoverUploadDocument = gql`
  mutation RequestCollectionCoverUpload($input: RequestCollectionCoverUploadInput!) {
    requestCollectionCoverUpload(input: $input) {
      uploadUrl
      expiresAt
      collection {
        id
        slug
      }
    }
  }
` as unknown as DocumentNode<
  RequestCollectionCoverUploadMutation,
  RequestCollectionCoverUploadMutationVariables
>

export const ConfirmCollectionCoverUploadDocument = gql`
  mutation ConfirmCollectionCoverUpload($collectionId: ID!, $width: Int!, $height: Int!) {
    confirmCollectionCoverUpload(collectionId: $collectionId, width: $width, height: $height) {
      id
      updatedAt
      coverImage {
        url
        altText
        width
        height
        mimeType
        status
      }
    }
  }
` as unknown as DocumentNode<
  ConfirmCollectionCoverUploadMutation,
  ConfirmCollectionCoverUploadMutationVariables
>

export const ClearCollectionCoverDocument = gql`
  mutation ClearCollectionCover($collectionId: ID!) {
    clearCollectionCover(collectionId: $collectionId) {
      id
      updatedAt
      coverImage {
        url
        altText
        width
        height
        mimeType
        status
      }
    }
  }
` as unknown as DocumentNode<
  ClearCollectionCoverMutation,
  ClearCollectionCoverMutationVariables
>
