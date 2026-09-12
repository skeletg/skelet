export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  Int64: { input: number; output: number; }
  JSON: { input: unknown; output: unknown; }
  Time: { input: string; output: string; }
};

export type ActivePoint = {
  __typename?: 'ActivePoint';
  count: Scalars['Int']['output'];
  day: Scalars['Time']['output'];
};

export type AttributeAffinity = {
  __typename?: 'AttributeAffinity';
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  values: Array<ProfileValue>;
};

export type AttributedRevenue = {
  __typename?: 'AttributedRevenue';
  cents: Scalars['Int64']['output'];
  currency: Scalars['String']['output'];
  orders: Scalars['Int']['output'];
};

export type CustomerActivityItem = {
  __typename?: 'CustomerActivityItem';
  detailsJson: Scalars['String']['output'];
  eventName: Scalars['String']['output'];
  occurredAt: Scalars['Time']['output'];
  source: Scalars['String']['output'];
  summary: Scalars['String']['output'];
};

export type CustomerEventCount = {
  __typename?: 'CustomerEventCount';
  count: Scalars['Int']['output'];
  name: Scalars['String']['output'];
};

export type CustomerExperimentArm = {
  __typename?: 'CustomerExperimentArm';
  experiment: Scalars['String']['output'];
  lastExposedAt: Scalars['Time']['output'];
  variation: Scalars['String']['output'];
};

export type CustomerProfile = {
  __typename?: 'CustomerProfile';
  affinities: Array<AttributeAffinity>;
  browsers: Array<ProfileValue>;
  campaigns: Array<ProfileValue>;
  conversions: Scalars['Int']['output'];
  countries: Array<ProfileValue>;
  counts: Array<CustomerEventCount>;
  devices: Array<ProfileValue>;
  experiments: Array<CustomerExperimentArm>;
  firstSeen?: Maybe<Scalars['Time']['output']>;
  ipAddresses: Array<ProfileValue>;
  lastSeen?: Maybe<Scalars['Time']['output']>;
  operatingSystems: Array<ProfileValue>;
  pageViews: Scalars['Int']['output'];
  pagesViewedTotal: Scalars['Int']['output'];
  referrers: Array<ProfileValue>;
  revenue: Array<AttributedRevenue>;
  screens: Array<ProfileValue>;
  searches: Array<ProfileValue>;
  sessions: Scalars['Int']['output'];
  sources: Array<ProfileValue>;
  timeline: Array<CustomerActivityItem>;
  topPages: Array<ProfilePageView>;
  topProducts: Array<ProfileProductView>;
  totalEvents: Scalars['Int']['output'];
  traits?: Maybe<CustomerTraits>;
};

export type CustomerTraits = {
  __typename?: 'CustomerTraits';
  churnRisk: Scalars['Int']['output'];
  computedAt: Scalars['Time']['output'];
  engagementScore: Scalars['Int']['output'];
  frequency: Scalars['Int']['output'];
  lifecycle: Scalars['String']['output'];
  monetaryCents: Scalars['Int64']['output'];
  nextOrderDays: Scalars['Int']['output'];
  predictedClvCents: Scalars['Int64']['output'];
  recencyDays: Scalars['Int']['output'];
  rfmFrequency: Scalars['Int']['output'];
  rfmMonetary: Scalars['Int']['output'];
  rfmRecency: Scalars['Int']['output'];
};

export type InsightPoint = {
  __typename?: 'InsightPoint';
  bucket: Scalars['Time']['output'];
  key: Scalars['String']['output'];
  value: Scalars['Int']['output'];
};

export type InsightSeries = {
  __typename?: 'InsightSeries';
  key: Scalars['String']['output'];
  total: Scalars['Int']['output'];
};

export type Mutation = {
  __typename?: 'Mutation';
  synapseRefreshSegment: SynapseSegment;
  synapseSaveSegment: SynapseSegment;
  synapseSetSegmentLifecycleWindows: SynapseSegment;
};


export type MutationSynapseRefreshSegmentArgs = {
  segmentId: Scalars['ID']['input'];
};


export type MutationSynapseSaveSegmentArgs = {
  affinity?: InputMaybe<SegmentAffinityInput>;
  channelId: Scalars['ID']['input'];
  conditions?: InputMaybe<Array<SegmentConditionInput>>;
  name: Scalars['String']['input'];
  root?: InputMaybe<SegmentNodeInput>;
  segmentId?: InputMaybe<Scalars['ID']['input']>;
  trait?: InputMaybe<SegmentTraitInput>;
};


export type MutationSynapseSetSegmentLifecycleWindowsArgs = {
  segmentId: Scalars['ID']['input'];
  windows?: InputMaybe<SegmentLifecycleWindowsInput>;
};

export type ProfilePageView = {
  __typename?: 'ProfilePageView';
  dwellSeconds: Scalars['Int']['output'];
  pageKey: Scalars['String']['output'];
  title: Scalars['String']['output'];
  views: Scalars['Int']['output'];
};

export type ProfileProductView = {
  __typename?: 'ProfileProductView';
  image: Scalars['String']['output'];
  name: Scalars['String']['output'];
  price: Scalars['String']['output'];
  sku: Scalars['String']['output'];
  views: Scalars['Int']['output'];
};

export type ProfileValue = {
  __typename?: 'ProfileValue';
  count: Scalars['Int']['output'];
  value: Scalars['String']['output'];
};

export type Query = {
  __typename?: 'Query';
  customerAudiences: Array<SignalSegment>;
  customerProfile: CustomerProfile;
  synapseFlow: SynapseFlow;
  synapseFunnel: SynapseFunnel;
  synapseInsights: SynapseInsights;
  synapseOverview: SynapseOverview;
  synapseRetention: SynapseRetention;
  synapseSegment: SynapseSegmentDetail;
  synapseSegmentPreview: Scalars['Int']['output'];
  synapseSegments: Array<SynapseSegment>;
  synapseStickiness: SynapseStickiness;
  synapseTrackedAttributes: Array<SynapseTrackedAttribute>;
  visitorProfile: CustomerProfile;
};


export type QueryCustomerAudiencesArgs = {
  customerId: Scalars['ID']['input'];
};


export type QueryCustomerProfileArgs = {
  customerId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySynapseFlowArgs = {
  channelId: Scalars['ID']['input'];
  depth?: InputMaybe<Scalars['Int']['input']>;
  event: Scalars['String']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySynapseFunnelArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
  steps: Array<Scalars['String']['input']>;
};


export type QuerySynapseInsightsArgs = {
  breakdown?: InputMaybe<Scalars['String']['input']>;
  channelId: Scalars['ID']['input'];
  event?: InputMaybe<Scalars['String']['input']>;
  measure?: InputMaybe<Scalars['String']['input']>;
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
  unit?: InputMaybe<Scalars['String']['input']>;
};


export type QuerySynapseOverviewArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySynapseRetentionArgs = {
  channelId: Scalars['ID']['input'];
  periods?: InputMaybe<Scalars['Int']['input']>;
  returnEvent: Scalars['String']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
  startEvent: Scalars['String']['input'];
  unit?: InputMaybe<Scalars['String']['input']>;
};


export type QuerySynapseSegmentArgs = {
  id: Scalars['ID']['input'];
};


export type QuerySynapseSegmentPreviewArgs = {
  affinity?: InputMaybe<SegmentAffinityInput>;
  channelId: Scalars['ID']['input'];
  conditions?: InputMaybe<Array<SegmentConditionInput>>;
  root?: InputMaybe<SegmentNodeInput>;
  trait?: InputMaybe<SegmentTraitInput>;
};


export type QuerySynapseSegmentsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QuerySynapseStickinessArgs = {
  channelId: Scalars['ID']['input'];
  event?: InputMaybe<Scalars['String']['input']>;
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
  windowDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySynapseTrackedAttributesArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryVisitorProfileArgs = {
  anonId: Scalars['String']['input'];
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};

export type RetentionBucket = {
  __typename?: 'RetentionBucket';
  count: Scalars['Int']['output'];
  eligible: Scalars['Int']['output'];
  period: Scalars['Int']['output'];
};

export type RetentionCohort = {
  __typename?: 'RetentionCohort';
  buckets: Array<RetentionBucket>;
  size: Scalars['Int']['output'];
  startAt: Scalars['Time']['output'];
};

export type SegmentAffinityCount = {
  __typename?: 'SegmentAffinityCount';
  attr: Scalars['String']['output'];
  members: Scalars['Int']['output'];
  value: Scalars['String']['output'];
};

export type SegmentAffinityInput = {
  attr: Scalars['String']['input'];
  days?: InputMaybe<Scalars['Int']['input']>;
  value: Scalars['String']['input'];
};

export type SegmentBoolOp =
  | 'AND'
  | 'OR';

export type SegmentConditionInput = {
  count?: InputMaybe<Scalars['Int']['input']>;
  days?: InputMaybe<Scalars['Int']['input']>;
  event: Scalars['String']['input'];
  negate?: InputMaybe<Scalars['Boolean']['input']>;
};

export type SegmentGrowthPoint = {
  __typename?: 'SegmentGrowthPoint';
  count: Scalars['Int']['output'];
  day: Scalars['Time']['output'];
};

export type SegmentLifecycleWindows = {
  __typename?: 'SegmentLifecycleWindows';
  activeDays: Scalars['Int']['output'];
  atRiskDays: Scalars['Int']['output'];
  newDays: Scalars['Int']['output'];
};

export type SegmentLifecycleWindowsInput = {
  activeDays: Scalars['Int']['input'];
  atRiskDays: Scalars['Int']['input'];
  newDays: Scalars['Int']['input'];
};

export type SegmentMember = {
  __typename?: 'SegmentMember';
  customerId: Scalars['ID']['output'];
  lastOrderAt?: Maybe<Scalars['Time']['output']>;
  lifecycle: Scalars['String']['output'];
  orders: Scalars['Int']['output'];
  recencyDays: Scalars['Int']['output'];
  spendCents: Scalars['Int64']['output'];
};

export type SegmentNode = {
  __typename?: 'SegmentNode';
  affinity?: Maybe<SynapseSegmentAffinity>;
  children?: Maybe<Array<SegmentNode>>;
  count?: Maybe<Scalars['Int']['output']>;
  days?: Maybe<Scalars['Int']['output']>;
  event?: Maybe<Scalars['String']['output']>;
  kind: SegmentNodeKind;
  negate?: Maybe<Scalars['Boolean']['output']>;
  op?: Maybe<SegmentBoolOp>;
  trait?: Maybe<SynapseSegmentTrait>;
};

export type SegmentNodeInput = {
  affinity?: InputMaybe<SegmentAffinityInput>;
  children?: InputMaybe<Array<SegmentNodeInput>>;
  count?: InputMaybe<Scalars['Int']['input']>;
  days?: InputMaybe<Scalars['Int']['input']>;
  event?: InputMaybe<Scalars['String']['input']>;
  kind: SegmentNodeKind;
  negate?: InputMaybe<Scalars['Boolean']['input']>;
  op?: InputMaybe<SegmentBoolOp>;
  trait?: InputMaybe<SegmentTraitInput>;
};

export type SegmentNodeKind =
  | 'AFFINITY'
  | 'EVENT'
  | 'GROUP'
  | 'TRAIT';

export type SegmentRfmBucket = {
  __typename?: 'SegmentRfmBucket';
  count: Scalars['Int']['output'];
  dimension: Scalars['String']['output'];
  score: Scalars['Int']['output'];
};

export type SegmentStageCount = {
  __typename?: 'SegmentStageCount';
  count: Scalars['Int']['output'];
  stage: Scalars['String']['output'];
};

export type SegmentTraitInput = {
  lifecycle?: InputMaybe<Array<Scalars['String']['input']>>;
  minEngagement?: InputMaybe<Scalars['Int']['input']>;
  minFrequencyScore?: InputMaybe<Scalars['Int']['input']>;
  minMonetaryScore?: InputMaybe<Scalars['Int']['input']>;
  minRecencyScore?: InputMaybe<Scalars['Int']['input']>;
};

export type SignalSegment = {
  __typename?: 'SignalSegment';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type SynapseActiveCustomer = {
  __typename?: 'SynapseActiveCustomer';
  customerId: Scalars['ID']['output'];
  events: Scalars['Int']['output'];
  lastSeen?: Maybe<Scalars['Time']['output']>;
};

export type SynapseFlow = {
  __typename?: 'SynapseFlow';
  avgSteps: Scalars['Float']['output'];
  cohort: Scalars['Int']['output'];
  continued: Scalars['Int']['output'];
  focus: Scalars['String']['output'];
  nodes: Array<SynapseFlowNode>;
  topPaths: Array<SynapseFlowPath>;
};

export type SynapseFlowNode = {
  __typename?: 'SynapseFlowNode';
  count: Scalars['Int']['output'];
  depth: Scalars['Int']['output'];
  event: Scalars['String']['output'];
  id: Scalars['String']['output'];
  parentId: Scalars['String']['output'];
};

export type SynapseFlowPath = {
  __typename?: 'SynapseFlowPath';
  count: Scalars['Int']['output'];
  events: Array<Scalars['String']['output']>;
};

export type SynapseFunnel = {
  __typename?: 'SynapseFunnel';
  steps: Array<SynapseFunnelStep>;
};

export type SynapseFunnelStep = {
  __typename?: 'SynapseFunnelStep';
  count: Scalars['Int']['output'];
  eventName: Scalars['String']['output'];
};

export type SynapseInsights = {
  __typename?: 'SynapseInsights';
  points: Array<InsightPoint>;
  prevTotal: Scalars['Int']['output'];
  prevUniques: Scalars['Int']['output'];
  series: Array<InsightSeries>;
  total: Scalars['Int']['output'];
  uniques: Scalars['Int']['output'];
};

export type SynapseOverview = {
  __typename?: 'SynapseOverview';
  activeCustomers: Array<SynapseActiveCustomer>;
  activeVisitors: Array<SynapseVisitor>;
  anonymousVisitors: Scalars['Int']['output'];
  conversions: Scalars['Int']['output'];
  identifiedCustomers: Scalars['Int']['output'];
  pageViews: Scalars['Int']['output'];
  revenue: Array<AttributedRevenue>;
  topEvents: Array<CustomerEventCount>;
  totalEvents: Scalars['Int']['output'];
};

export type SynapseRetention = {
  __typename?: 'SynapseRetention';
  buckets: Array<RetentionBucket>;
  cohort: Scalars['Int']['output'];
  cohorts: Array<RetentionCohort>;
};

export type SynapseSegment = {
  __typename?: 'SynapseSegment';
  affinity?: Maybe<SynapseSegmentAffinity>;
  channelId: Scalars['ID']['output'];
  conditions: Array<SynapseSegmentCondition>;
  definition?: Maybe<SegmentNode>;
  id: Scalars['ID']['output'];
  lifecycleWindows?: Maybe<SegmentLifecycleWindows>;
  memberCount: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  trait?: Maybe<SynapseSegmentTrait>;
};

export type SynapseSegmentAffinity = {
  __typename?: 'SynapseSegmentAffinity';
  attr: Scalars['String']['output'];
  auto: Scalars['Boolean']['output'];
  days: Scalars['Int']['output'];
  value: Scalars['String']['output'];
};

export type SynapseSegmentCondition = {
  __typename?: 'SynapseSegmentCondition';
  count: Scalars['Int']['output'];
  days: Scalars['Int']['output'];
  event: Scalars['String']['output'];
  negate: Scalars['Boolean']['output'];
};

export type SynapseSegmentDetail = {
  __typename?: 'SynapseSegmentDetail';
  aovCents: Scalars['Int64']['output'];
  currency: Scalars['String']['output'];
  growth: Array<SegmentGrowthPoint>;
  lifecycleMix: Array<SegmentStageCount>;
  members: Array<SegmentMember>;
  membersTruncated: Scalars['Boolean']['output'];
  orders: Scalars['Int']['output'];
  revenueCents: Scalars['Int64']['output'];
  rfmDistribution: Array<SegmentRfmBucket>;
  segment: SynapseSegment;
  topAffinities: Array<SegmentAffinityCount>;
};

export type SynapseSegmentTrait = {
  __typename?: 'SynapseSegmentTrait';
  lifecycle: Array<Scalars['String']['output']>;
  minEngagement: Scalars['Int']['output'];
  minFrequencyScore: Scalars['Int']['output'];
  minMonetaryScore: Scalars['Int']['output'];
  minRecencyScore: Scalars['Int']['output'];
};

export type SynapseStickiness = {
  __typename?: 'SynapseStickiness';
  activeNow: Scalars['Int']['output'];
  days: Array<ActivePoint>;
  mau: Scalars['Int']['output'];
  priorActive: Scalars['Int']['output'];
  returnRate: Scalars['Float']['output'];
  returning: Scalars['Int']['output'];
  wau: Scalars['Int']['output'];
  windowDays: Scalars['Int']['output'];
};

export type SynapseTrackedAttribute = {
  __typename?: 'SynapseTrackedAttribute';
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  values: Array<Scalars['String']['output']>;
};

export type SynapseVisitor = {
  __typename?: 'SynapseVisitor';
  anonId: Scalars['String']['output'];
  events: Scalars['Int']['output'];
  lastPage: Scalars['String']['output'];
  lastSeen?: Maybe<Scalars['Time']['output']>;
};
