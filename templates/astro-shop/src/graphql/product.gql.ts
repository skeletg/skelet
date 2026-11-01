/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '@skelet/storefront-core/graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type ProductKind =
  | 'BUNDLE'
  | 'CONFIGURABLE'
  | 'GIFTCARD'
  | 'GROUPED'
  | 'KIT'
  | 'SIMPLE'
  | 'SUBSCRIPTION';

export type RouteKind =
  | 'BLOG_AUTHOR'
  | 'BLOG_CATEGORY'
  | 'BLOG_POST'
  | 'BLOG_TAG'
  | 'CATEGORY'
  | 'CMS_PAGE'
  | 'COLLECTION'
  | 'PLUGIN'
  | 'PRODUCT';

export type ImageFieldsFragment = { url: string, altText: string, width: number, height: number };

export type RouteQueryVariables = Exact<{
  path: string;
}>;


export type RouteQuery = { route: { kind: Types.RouteKind, entityId: string, redirectTo: string | null, redirectStatusCode: number | null } | null };

export type ProductPageQueryVariables = Exact<{
  id: string | number;
}>;


export type ProductPageQuery = { product: { id: string, url: string, sku: string, name: string | null, kind: Types.ProductKind, inStock: boolean, price: number, compareAtPrice: number | null, onSale: boolean, minOrderQty: number | null, quantityStep: number, maxOrderQty: number | null, metaTitle: string | null, metaDescription: string | null, description: unknown, mainImage: { url: string, altText: string, width: number, height: number } | null, children: Array<{ id: string, sku: string, name: string | null, inStock: boolean, price: number }> } };

export const ImageFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ImageFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProductMedia"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"altText"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}}]}}]} as unknown as DocumentNode<ImageFieldsFragment, unknown>;
export const RouteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Route"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"path"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"route"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"path"},"value":{"kind":"Variable","name":{"kind":"Name","value":"path"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"entityId"}},{"kind":"Field","name":{"kind":"Name","value":"redirectTo"}},{"kind":"Field","name":{"kind":"Name","value":"redirectStatusCode"}}]}}]}}]} as unknown as DocumentNode<RouteQuery, RouteQueryVariables>;
export const ProductPageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ProductPage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"product"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"sku"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"inStock"}},{"kind":"Field","name":{"kind":"Name","value":"price"}},{"kind":"Field","name":{"kind":"Name","value":"compareAtPrice"}},{"kind":"Field","name":{"kind":"Name","value":"onSale"}},{"kind":"Field","name":{"kind":"Name","value":"minOrderQty"}},{"kind":"Field","name":{"kind":"Name","value":"quantityStep"}},{"kind":"Field","name":{"kind":"Name","value":"maxOrderQty"}},{"kind":"Field","name":{"kind":"Name","value":"metaTitle"}},{"kind":"Field","name":{"kind":"Name","value":"metaDescription"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"mainImage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ImageFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"children"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sku"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"inStock"}},{"kind":"Field","name":{"kind":"Name","value":"price"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ImageFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ProductMedia"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"altText"}},{"kind":"Field","name":{"kind":"Name","value":"width"}},{"kind":"Field","name":{"kind":"Name","value":"height"}}]}}]} as unknown as DocumentNode<ProductPageQuery, ProductPageQueryVariables>;