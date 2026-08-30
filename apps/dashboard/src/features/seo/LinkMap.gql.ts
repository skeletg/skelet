/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type CrawlChangeKind =
  | 'CANONICAL_CHANGED'
  | 'DEEPER'
  | 'FIXED'
  | 'GONE'
  | 'H1_CHANGED'
  | 'LOST_IN_LINKS'
  | 'META_CHANGED'
  | 'NEW'
  | 'NOW_BROKEN'
  | 'NOW_INDEXABLE'
  | 'NOW_NOINDEX'
  | 'NOW_ORPHAN'
  | 'NOW_REDIRECT'
  | 'TITLE_CHANGED';

export type CrawlPageKind =
  | 'FILTER'
  | 'PAGE'
  | 'PARAM';

export type LinkNodeState =
  | 'CRAWLED'
  | 'DISCOVERED'
  | 'UNSEEN';

export type LinkNodeType =
  | 'BLOG_AUTHOR'
  | 'BLOG_CATEGORY'
  | 'BLOG_POST'
  | 'BLOG_TAG'
  | 'CATEGORY'
  | 'CMS_PAGE'
  | 'COLLECTION'
  | 'OTHER'
  | 'PRODUCT';

export type SeoIssue =
  | 'BROKEN'
  | 'CANONICAL_NONE'
  | 'CANONICAL_OTHER'
  | 'DEEP'
  | 'H1_MISSING'
  | 'H1_MULTIPLE'
  | 'HEADING_SKIPPED'
  | 'META_DUPLICATE'
  | 'META_LONG'
  | 'META_MISSING'
  | 'META_SHORT'
  | 'NOFOLLOW'
  | 'NOINDEX'
  | 'NO_LINKS_OUT'
  | 'ORPHAN'
  | 'REDIRECT'
  | 'THIN'
  | 'TITLE_DUPLICATE'
  | 'TITLE_LONG'
  | 'TITLE_MISSING'
  | 'TITLE_SHORT';

export type SiteCrawlStatus =
  | 'CANCELLED'
  | 'COMPLETED'
  | 'FAILED'
  | 'RUNNING';

export type StartSiteCrawlInput = {
  baseUrl?: string | null | undefined;
  channelId: string | number;
  maxDepth?: number | null | undefined;
  maxPages?: number | null | undefined;
  respectRobots?: boolean | null | undefined;
};

export type SiteCrawlFieldsFragment = { id: string, baseUrl: string, status: Types.SiteCrawlStatus, maxPages: number, maxDepth: number, respectRobots: boolean, pagesCrawled: number, linksFound: number, hitBudget: boolean, error: string, startedAt: string, finishedAt: string | null };

export type LinkMapQueryVariables = Exact<{
  channelId: string | number;
  crawlId?: string | number | null | undefined;
  maxNodes?: number | null | undefined;
}>;


export type LinkMapQuery = { linkGraph: { nodes: Array<{ key: string, type: Types.LinkNodeType, entityId: string | null, label: string, path: string, kind: Types.CrawlPageKind, state: Types.LinkNodeState, statusCode: number, noindex: boolean, isHomepage: boolean, inLinks: number, outLinks: number, depth: number }>, edges: Array<{ from: string, to: string, nofollow: boolean, boilerplate: boolean }>, stats: { totalNodes: number, totalEdges: number, crawled: number, discovered: number, unseen: number, orphans: number, noindexed: number, broken: number, redirected: number, boilerplate: number, contextual: number, maxDepth: number, returnedNodes: number, dropped: number, complete: boolean }, crawl: { id: string, baseUrl: string, status: Types.SiteCrawlStatus, pagesCrawled: number, linksFound: number, hitBudget: boolean } } | null };

export type SiteCrawlsQueryVariables = Exact<{
  channelId: string | number;
  limit?: number | null | undefined;
}>;


export type SiteCrawlsQuery = { siteCrawls: Array<{ id: string, baseUrl: string, status: Types.SiteCrawlStatus, maxPages: number, maxDepth: number, respectRobots: boolean, pagesCrawled: number, linksFound: number, hitBudget: boolean, error: string, startedAt: string, finishedAt: string | null }> };

export type StartSiteCrawlMutationVariables = Exact<{
  input: Types.StartSiteCrawlInput;
}>;


export type StartSiteCrawlMutation = { startSiteCrawl: { id: string, baseUrl: string, status: Types.SiteCrawlStatus, maxPages: number, maxDepth: number, respectRobots: boolean, pagesCrawled: number, linksFound: number, hitBudget: boolean, error: string, startedAt: string, finishedAt: string | null } };

export type CancelSiteCrawlMutationVariables = Exact<{
  channelId: string | number;
}>;


export type CancelSiteCrawlMutation = { cancelSiteCrawl: boolean };

export type CrawlAuditQueryVariables = Exact<{
  channelId: string | number;
  issue?: Types.SeoIssue | null | undefined;
  kind?: Types.CrawlPageKind | null | undefined;
  search?: string | null | undefined;
  limit?: number | null | undefined;
  offset?: number | null | undefined;
}>;


export type CrawlAuditQuery = { crawlAudit: { totalPages: number, issueCounts: Array<{ issue: Types.SeoIssue, count: number }>, pages: Array<{ path: string, statusCode: number, kind: Types.CrawlPageKind, title: string, titleLength: number, metaDesc: string, metaLength: number, robotsMeta: string, canonical: string, redirectTo: string, noindex: boolean, nofollow: boolean, h1: string, h1Count: number, headingCounts: Array<number>, wordCount: number, internalLinks: number, externalLinks: number, inLinks: number, depth: number, entityType: Types.LinkNodeType | null, entityId: string | null, issues: Array<Types.SeoIssue> }> } | null };

export type CrawlDiffQueryVariables = Exact<{
  channelId: string | number;
  regressionsOnly?: boolean | null | undefined;
  limit?: number | null | undefined;
}>;


export type CrawlDiffQuery = { crawlDiff: { compared: number, regressions: number, coverageComparable: boolean, base: { id: string, pagesCrawled: number, startedAt: string }, current: { id: string, pagesCrawled: number, startedAt: string }, counts: Array<{ kind: Types.CrawlChangeKind, count: number }>, changes: Array<{ path: string, kind: Types.CrawlChangeKind, before: string, after: string, isRegression: boolean }> } | null };

export const SiteCrawlFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SiteCrawlFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SiteCrawl"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"baseUrl"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"maxPages"}},{"kind":"Field","name":{"kind":"Name","value":"maxDepth"}},{"kind":"Field","name":{"kind":"Name","value":"respectRobots"}},{"kind":"Field","name":{"kind":"Name","value":"pagesCrawled"}},{"kind":"Field","name":{"kind":"Name","value":"linksFound"}},{"kind":"Field","name":{"kind":"Name","value":"hitBudget"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"finishedAt"}}]}}]} as unknown as DocumentNode<SiteCrawlFieldsFragment, unknown>;
export const LinkMapDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LinkMap"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"crawlId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"maxNodes"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"linkGraph"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"crawlId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"crawlId"}}},{"kind":"Argument","name":{"kind":"Name","value":"maxNodes"},"value":{"kind":"Variable","name":{"kind":"Name","value":"maxNodes"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"nodes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"key"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"entityId"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"statusCode"}},{"kind":"Field","name":{"kind":"Name","value":"noindex"}},{"kind":"Field","name":{"kind":"Name","value":"isHomepage"}},{"kind":"Field","name":{"kind":"Name","value":"inLinks"}},{"kind":"Field","name":{"kind":"Name","value":"outLinks"}},{"kind":"Field","name":{"kind":"Name","value":"depth"}}]}},{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"from"}},{"kind":"Field","name":{"kind":"Name","value":"to"}},{"kind":"Field","name":{"kind":"Name","value":"nofollow"}},{"kind":"Field","name":{"kind":"Name","value":"boilerplate"}}]}},{"kind":"Field","name":{"kind":"Name","value":"stats"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalNodes"}},{"kind":"Field","name":{"kind":"Name","value":"totalEdges"}},{"kind":"Field","name":{"kind":"Name","value":"crawled"}},{"kind":"Field","name":{"kind":"Name","value":"discovered"}},{"kind":"Field","name":{"kind":"Name","value":"unseen"}},{"kind":"Field","name":{"kind":"Name","value":"orphans"}},{"kind":"Field","name":{"kind":"Name","value":"noindexed"}},{"kind":"Field","name":{"kind":"Name","value":"broken"}},{"kind":"Field","name":{"kind":"Name","value":"redirected"}},{"kind":"Field","name":{"kind":"Name","value":"boilerplate"}},{"kind":"Field","name":{"kind":"Name","value":"contextual"}},{"kind":"Field","name":{"kind":"Name","value":"maxDepth"}},{"kind":"Field","name":{"kind":"Name","value":"returnedNodes"}},{"kind":"Field","name":{"kind":"Name","value":"dropped"}},{"kind":"Field","name":{"kind":"Name","value":"complete"}}]}},{"kind":"Field","name":{"kind":"Name","value":"crawl"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"baseUrl"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"pagesCrawled"}},{"kind":"Field","name":{"kind":"Name","value":"linksFound"}},{"kind":"Field","name":{"kind":"Name","value":"hitBudget"}}]}}]}}]}}]} as unknown as DocumentNode<LinkMapQuery, LinkMapQueryVariables>;
export const SiteCrawlsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SiteCrawls"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"siteCrawls"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SiteCrawlFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SiteCrawlFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SiteCrawl"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"baseUrl"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"maxPages"}},{"kind":"Field","name":{"kind":"Name","value":"maxDepth"}},{"kind":"Field","name":{"kind":"Name","value":"respectRobots"}},{"kind":"Field","name":{"kind":"Name","value":"pagesCrawled"}},{"kind":"Field","name":{"kind":"Name","value":"linksFound"}},{"kind":"Field","name":{"kind":"Name","value":"hitBudget"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"finishedAt"}}]}}]} as unknown as DocumentNode<SiteCrawlsQuery, SiteCrawlsQueryVariables>;
export const StartSiteCrawlDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartSiteCrawl"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartSiteCrawlInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startSiteCrawl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SiteCrawlFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SiteCrawlFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SiteCrawl"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"baseUrl"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"maxPages"}},{"kind":"Field","name":{"kind":"Name","value":"maxDepth"}},{"kind":"Field","name":{"kind":"Name","value":"respectRobots"}},{"kind":"Field","name":{"kind":"Name","value":"pagesCrawled"}},{"kind":"Field","name":{"kind":"Name","value":"linksFound"}},{"kind":"Field","name":{"kind":"Name","value":"hitBudget"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"finishedAt"}}]}}]} as unknown as DocumentNode<StartSiteCrawlMutation, StartSiteCrawlMutationVariables>;
export const CancelSiteCrawlDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelSiteCrawl"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelSiteCrawl"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}]}]}}]} as unknown as DocumentNode<CancelSiteCrawlMutation, CancelSiteCrawlMutationVariables>;
export const CrawlAuditDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CrawlAudit"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"issue"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"SeoIssue"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"kind"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"CrawlPageKind"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"crawlAudit"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"issue"},"value":{"kind":"Variable","name":{"kind":"Name","value":"issue"}}},{"kind":"Argument","name":{"kind":"Name","value":"kind"},"value":{"kind":"Variable","name":{"kind":"Name","value":"kind"}}},{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalPages"}},{"kind":"Field","name":{"kind":"Name","value":"issueCounts"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"issue"}},{"kind":"Field","name":{"kind":"Name","value":"count"}}]}},{"kind":"Field","name":{"kind":"Name","value":"pages"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"statusCode"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"titleLength"}},{"kind":"Field","name":{"kind":"Name","value":"metaDesc"}},{"kind":"Field","name":{"kind":"Name","value":"metaLength"}},{"kind":"Field","name":{"kind":"Name","value":"robotsMeta"}},{"kind":"Field","name":{"kind":"Name","value":"canonical"}},{"kind":"Field","name":{"kind":"Name","value":"redirectTo"}},{"kind":"Field","name":{"kind":"Name","value":"noindex"}},{"kind":"Field","name":{"kind":"Name","value":"nofollow"}},{"kind":"Field","name":{"kind":"Name","value":"h1"}},{"kind":"Field","name":{"kind":"Name","value":"h1Count"}},{"kind":"Field","name":{"kind":"Name","value":"headingCounts"}},{"kind":"Field","name":{"kind":"Name","value":"wordCount"}},{"kind":"Field","name":{"kind":"Name","value":"internalLinks"}},{"kind":"Field","name":{"kind":"Name","value":"externalLinks"}},{"kind":"Field","name":{"kind":"Name","value":"inLinks"}},{"kind":"Field","name":{"kind":"Name","value":"depth"}},{"kind":"Field","name":{"kind":"Name","value":"entityType"}},{"kind":"Field","name":{"kind":"Name","value":"entityId"}},{"kind":"Field","name":{"kind":"Name","value":"issues"}}]}}]}}]}}]} as unknown as DocumentNode<CrawlAuditQuery, CrawlAuditQueryVariables>;
export const CrawlDiffDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CrawlDiff"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"regressionsOnly"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"crawlDiff"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}},{"kind":"Argument","name":{"kind":"Name","value":"regressionsOnly"},"value":{"kind":"Variable","name":{"kind":"Name","value":"regressionsOnly"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"compared"}},{"kind":"Field","name":{"kind":"Name","value":"regressions"}},{"kind":"Field","name":{"kind":"Name","value":"coverageComparable"}},{"kind":"Field","name":{"kind":"Name","value":"base"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"pagesCrawled"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"current"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"pagesCrawled"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"counts"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"count"}}]}},{"kind":"Field","name":{"kind":"Name","value":"changes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"before"}},{"kind":"Field","name":{"kind":"Name","value":"after"}},{"kind":"Field","name":{"kind":"Name","value":"isRegression"}}]}}]}}]}}]} as unknown as DocumentNode<CrawlDiffQuery, CrawlDiffQueryVariables>;