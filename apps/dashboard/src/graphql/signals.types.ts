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

export type BotAgent = {
  __typename?: 'BotAgent';
  category: Scalars['String']['output'];
  hits: Scalars['Int']['output'];
  lastSeen: Scalars['Time']['output'];
  name: Scalars['String']['output'];
  pages: Scalars['Int']['output'];
};

export type BotDayPoint = {
  __typename?: 'BotDayPoint';
  botHits: Scalars['Int']['output'];
  day: Scalars['String']['output'];
  humanViews: Scalars['Int']['output'];
};

export type BotPage = {
  __typename?: 'BotPage';
  bots: Scalars['Int']['output'];
  hits: Scalars['Int']['output'];
  pageKey: Scalars['String']['output'];
};

export type BotTraffic = {
  __typename?: 'BotTraffic';
  botCount: Scalars['Int']['output'];
  botHits: Scalars['Int']['output'];
  bots: Array<BotAgent>;
  daily: Array<BotDayPoint>;
  humanViews: Scalars['Int']['output'];
  pageCount: Scalars['Int']['output'];
  topPages: Array<BotPage>;
};

export type CreateCustomerSegmentInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
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

export type CustomerSegment = {
  __typename?: 'CustomerSegment';
  createdAt: Scalars['Time']['output'];
  description: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  memberCount: Scalars['Int']['output'];
  members: Array<CustomerSegmentMember>;
  name: Scalars['String']['output'];
  rules?: Maybe<Scalars['JSON']['output']>;
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};


export type CustomerSegmentMembersArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};

export type CustomerSegmentList = {
  __typename?: 'CustomerSegmentList';
  items: Array<CustomerSegment>;
  totalCount: Scalars['Int']['output'];
};

export type CustomerSegmentMember = {
  __typename?: 'CustomerSegmentMember';
  id: Scalars['ID']['output'];
  joinedAt: Scalars['Time']['output'];
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

export type EventDestination = {
  __typename?: 'EventDestination';
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  eventNames: Array<Scalars['String']['output']>;
  eventSources: Array<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  kind: Scalars['String']['output'];
  lastError: Scalars['String']['output'];
  lastForwardedAt?: Maybe<Scalars['Time']['output']>;
  name: Scalars['String']['output'];
  targetUrl: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type EventDestinationInput = {
  channelId: Scalars['ID']['input'];
  eventNames?: InputMaybe<Array<Scalars['String']['input']>>;
  eventSources?: InputMaybe<Array<Scalars['String']['input']>>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  kind: Scalars['String']['input'];
  name: Scalars['String']['input'];
  secret?: InputMaybe<Scalars['String']['input']>;
  targetUrl: Scalars['String']['input'];
};

export type Experiment = {
  __typename?: 'Experiment';
  channelId: Scalars['ID']['output'];
  coverage: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  goalEvent: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  key: Scalars['String']['output'];
  name: Scalars['String']['output'];
  segmentSlug: Scalars['String']['output'];
  status: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  variations: Array<ExperimentVariation>;
};

export type ExperimentResults = {
  __typename?: 'ExperimentResults';
  sampleRatioMismatch: Scalars['Boolean']['output'];
  sequentialWinner: Scalars['String']['output'];
  totalExposed: Scalars['Int']['output'];
  totalRevenue: Scalars['Int64']['output'];
  variations: Array<ExperimentVariationResult>;
  winner: Scalars['String']['output'];
};

export type ExperimentSummary = {
  __typename?: 'ExperimentSummary';
  conversions: Scalars['Int']['output'];
  coverage: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  key: Scalars['String']['output'];
  leader: Scalars['String']['output'];
  leaderChanceToWin: Scalars['Float']['output'];
  leaderIsControl: Scalars['Boolean']['output'];
  leaderRate: Scalars['Float']['output'];
  leaderUplift: Scalars['Float']['output'];
  name: Scalars['String']['output'];
  sampleRatioMismatch: Scalars['Boolean']['output'];
  totalExposed: Scalars['Int']['output'];
  variations: Scalars['Int']['output'];
  winner: Scalars['String']['output'];
};

export type ExperimentVariation = {
  __typename?: 'ExperimentVariation';
  key: Scalars['String']['output'];
  name: Scalars['String']['output'];
  value: Scalars['String']['output'];
  weight: Scalars['Int']['output'];
};

export type ExperimentVariationInput = {
  key: Scalars['String']['input'];
  name: Scalars['String']['input'];
  value: Scalars['String']['input'];
  weight: Scalars['Int']['input'];
};

export type ExperimentVariationResult = {
  __typename?: 'ExperimentVariationResult';
  alwaysValidP: Scalars['Float']['output'];
  chanceToWin: Scalars['Float']['output'];
  ciHigh: Scalars['Float']['output'];
  ciLow: Scalars['Float']['output'];
  converted: Scalars['Int']['output'];
  exposed: Scalars['Int']['output'];
  isControl: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  rate: Scalars['Float']['output'];
  revenueCents: Scalars['Int64']['output'];
  sequentialSignificant: Scalars['Boolean']['output'];
  significant: Scalars['Boolean']['output'];
  uplift: Scalars['Float']['output'];
  variation: Scalars['String']['output'];
};

export type ExperimentsOverview = {
  __typename?: 'ExperimentsOverview';
  decided: Scalars['Int']['output'];
  draft: Scalars['Int']['output'];
  experiments: Array<ExperimentSummary>;
  running: Scalars['Int']['output'];
  stopped: Scalars['Int']['output'];
  totalExposed: Scalars['Int']['output'];
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

export type LensCell = {
  __typename?: 'LensCell';
  count: Scalars['Int']['output'];
  x: Scalars['Int']['output'];
  y: Scalars['Int']['output'];
};

export type LensElementClick = {
  __typename?: 'LensElementClick';
  count: Scalars['Int']['output'];
  selector: Scalars['String']['output'];
  text: Scalars['String']['output'];
};

export type LensErrorStat = {
  __typename?: 'LensErrorStat';
  count: Scalars['Int']['output'];
  lastSeen: Scalars['Time']['output'];
  message: Scalars['String']['output'];
  pages: Scalars['Int']['output'];
};

export type LensFrustration = {
  __typename?: 'LensFrustration';
  deadPages: Array<LensSignalPage>;
  rageElements: Array<LensRageElement>;
  ragePages: Array<LensSignalPage>;
};

export type LensHeatmap = {
  __typename?: 'LensHeatmap';
  click: Array<LensCell>;
  elements: Array<LensElementClick>;
  height: Scalars['Int']['output'];
  move: Array<LensCell>;
  pageKey: Scalars['String']['output'];
  scroll: Array<LensCell>;
  snapshotJson: Scalars['String']['output'];
  state: Scalars['String']['output'];
  viewport: Scalars['String']['output'];
  width: Scalars['Int']['output'];
};

export type LensLabelCount = {
  __typename?: 'LensLabelCount';
  label: Scalars['String']['output'];
  views: Scalars['Int']['output'];
};

export type LensOverview = {
  __typename?: 'LensOverview';
  daily: Array<LensTimePoint>;
  devices: Array<LensLabelCount>;
  interactions: Array<LensLabelCount>;
  referrers: Array<LensLabelCount>;
  topPages: Array<LensPageStat>;
  views: Scalars['Int']['output'];
  visitors: Scalars['Int']['output'];
};

export type LensPage = {
  __typename?: 'LensPage';
  pageKey: Scalars['String']['output'];
  views: Scalars['Int']['output'];
};

export type LensPageStat = {
  __typename?: 'LensPageStat';
  avgScroll: Scalars['Int']['output'];
  clicks: Scalars['Int']['output'];
  pageKey: Scalars['String']['output'];
  views: Scalars['Int']['output'];
  visitors: Scalars['Int']['output'];
};

export type LensPageStats = {
  __typename?: 'LensPageStats';
  avgScroll: Scalars['Int']['output'];
  clicks: Scalars['Int']['output'];
  clients: Array<LensLabelCount>;
  devices: Array<LensLabelCount>;
  referrers: Array<LensLabelCount>;
  timeline: Array<LensTimePoint>;
  views: Scalars['Int']['output'];
  visitors: Scalars['Int']['output'];
};

export type LensPageVitals = {
  __typename?: 'LensPageVitals';
  cls?: Maybe<Scalars['Float']['output']>;
  inp?: Maybe<Scalars['Float']['output']>;
  lcp?: Maybe<Scalars['Float']['output']>;
  pageKey: Scalars['String']['output'];
  samples: Scalars['Int']['output'];
  ttfb?: Maybe<Scalars['Float']['output']>;
};

export type LensRageElement = {
  __typename?: 'LensRageElement';
  count: Scalars['Int']['output'];
  selector: Scalars['String']['output'];
  text: Scalars['String']['output'];
};

export type LensSearchAnalytics = {
  __typename?: 'LensSearchAnalytics';
  daily: Array<LensSearchDay>;
  distinctTerms: Scalars['Int']['output'];
  top: Array<LensSearchTerm>;
  totalSearches: Scalars['Int']['output'];
  zeroResult: Array<LensSearchTerm>;
  zeroResultSearches: Scalars['Int']['output'];
};

export type LensSearchDay = {
  __typename?: 'LensSearchDay';
  date: Scalars['String']['output'];
  searches: Scalars['Int']['output'];
  zeroResults: Scalars['Int']['output'];
};

export type LensSearchTerm = {
  __typename?: 'LensSearchTerm';
  count: Scalars['Int']['output'];
  term: Scalars['String']['output'];
};

export type LensSession = {
  __typename?: 'LensSession';
  anonId: Scalars['String']['output'];
  customerId?: Maybe<Scalars['ID']['output']>;
  durationMs: Scalars['Int']['output'];
  endedAt: Scalars['Time']['output'];
  entryPage: Scalars['String']['output'];
  errorCount: Scalars['Int']['output'];
  eventCount: Scalars['Int']['output'];
  exitPage: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  rageCount: Scalars['Int']['output'];
  startedAt: Scalars['Time']['output'];
  userAgent: Scalars['String']['output'];
  viewport: Scalars['String']['output'];
};

export type LensSessionError = {
  __typename?: 'LensSessionError';
  at: Scalars['Time']['output'];
  message: Scalars['String']['output'];
};

export type LensSessionSignals = {
  __typename?: 'LensSessionSignals';
  errors: Array<LensSessionError>;
  vitals: Array<LensSessionVital>;
};

export type LensSessionVital = {
  __typename?: 'LensSessionVital';
  metric: Scalars['String']['output'];
  value: Scalars['Float']['output'];
};

export type LensSignalPage = {
  __typename?: 'LensSignalPage';
  count: Scalars['Int']['output'];
  pageKey: Scalars['String']['output'];
};

export type LensTimePoint = {
  __typename?: 'LensTimePoint';
  date: Scalars['String']['output'];
  views: Scalars['Int']['output'];
};

export type LensVital = {
  __typename?: 'LensVital';
  metric: Scalars['String']['output'];
  p75: Scalars['Float']['output'];
  samples: Scalars['Int']['output'];
};

export type LinkClicks = {
  __typename?: 'LinkClicks';
  clicks: Scalars['Int']['output'];
  url: Scalars['String']['output'];
};

export type Mutation = {
  __typename?: 'Mutation';
  assignCustomersToSegment: Scalars['Boolean']['output'];
  clearSegmentMembership: Scalars['Boolean']['output'];
  createCustomerSegment: CustomerSegment;
  createEventDestination: EventDestination;
  createExperiment: Experiment;
  deleteCustomerSegment: Scalars['Boolean']['output'];
  deleteEventDestination: Scalars['Boolean']['output'];
  deleteExperiment: Scalars['Boolean']['output'];
  refreshSegment: Scalars['Int']['output'];
  removeCustomerFromSegment: Scalars['Boolean']['output'];
  setSegmentRules: CustomerSegment;
  synapseRefreshSegment: SynapseSegment;
  synapseSaveSegment: SynapseSegment;
  synapseSetSegmentLifecycleWindows: SynapseSegment;
  syncAudience: Scalars['Int']['output'];
  updateCustomerSegment: CustomerSegment;
  updateEventDestination: EventDestination;
  updateExperiment: Experiment;
};


export type MutationAssignCustomersToSegmentArgs = {
  customerIds: Array<Scalars['ID']['input']>;
  segmentId: Scalars['ID']['input'];
};


export type MutationClearSegmentMembershipArgs = {
  segmentId: Scalars['ID']['input'];
};


export type MutationCreateCustomerSegmentArgs = {
  input: CreateCustomerSegmentInput;
};


export type MutationCreateEventDestinationArgs = {
  input: EventDestinationInput;
};


export type MutationCreateExperimentArgs = {
  input: SaveExperimentInput;
};


export type MutationDeleteCustomerSegmentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteEventDestinationArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteExperimentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRefreshSegmentArgs = {
  segmentId: Scalars['ID']['input'];
};


export type MutationRemoveCustomerFromSegmentArgs = {
  customerId: Scalars['ID']['input'];
  segmentId: Scalars['ID']['input'];
};


export type MutationSetSegmentRulesArgs = {
  rules?: InputMaybe<Scalars['JSON']['input']>;
  segmentId: Scalars['ID']['input'];
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


export type MutationSyncAudienceArgs = {
  destinationId: Scalars['ID']['input'];
  segmentId: Scalars['ID']['input'];
};


export type MutationUpdateCustomerSegmentArgs = {
  input: UpdateCustomerSegmentInput;
};


export type MutationUpdateEventDestinationArgs = {
  id: Scalars['ID']['input'];
  input: EventDestinationInput;
};


export type MutationUpdateExperimentArgs = {
  id: Scalars['ID']['input'];
  input: SaveExperimentInput;
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

export type PulseEngagement = {
  __typename?: 'PulseEngagement';
  clicks: Scalars['Int']['output'];
  conversions: Scalars['Int']['output'];
  opensHuman: Scalars['Int']['output'];
  opensRaw: Scalars['Int']['output'];
  revenue: Array<AttributedRevenue>;
  topLinks: Array<LinkClicks>;
};

export type Query = {
  __typename?: 'Query';
  botTraffic: BotTraffic;
  customerAudiences: Array<SignalSegment>;
  customerProfile: CustomerProfile;
  customerSegment?: Maybe<CustomerSegment>;
  customerSegments: CustomerSegmentList;
  eventDestinations: Array<EventDestination>;
  experiment?: Maybe<Experiment>;
  experimentGoalEvents: Array<Scalars['String']['output']>;
  experimentResults: ExperimentResults;
  experimentSegments: Array<SegmentOption>;
  experiments: Array<Experiment>;
  experimentsOverview: ExperimentsOverview;
  lensCustomerSessions: Array<LensSession>;
  lensErrors: Array<LensErrorStat>;
  lensFrustration: LensFrustration;
  lensHeatmap?: Maybe<LensHeatmap>;
  lensOverview: LensOverview;
  lensPageStats: LensPageStats;
  lensPages: Array<LensPage>;
  lensPerformance: Array<LensPageVitals>;
  lensSearchAnalytics: LensSearchAnalytics;
  lensSessionEvents: Scalars['String']['output'];
  lensSessionSignals: LensSessionSignals;
  lensSessions: Array<LensSession>;
  lensStates: Array<Scalars['String']['output']>;
  lensVisitorSessions: Array<LensSession>;
  lensVitalsSummary: Array<LensVital>;
  pulseEngagement: PulseEngagement;
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


export type QueryBotTrafficArgs = {
  bot?: InputMaybe<Scalars['String']['input']>;
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCustomerAudiencesArgs = {
  customerId: Scalars['ID']['input'];
};


export type QueryCustomerProfileArgs = {
  customerId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCustomerSegmentArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCustomerSegmentsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryEventDestinationsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryExperimentArgs = {
  id: Scalars['ID']['input'];
};


export type QueryExperimentGoalEventsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryExperimentResultsArgs = {
  id: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryExperimentsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryExperimentsOverviewArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensCustomerSessionsArgs = {
  customerId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensErrorsArgs = {
  channelId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensFrustrationArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensHeatmapArgs = {
  channelId: Scalars['ID']['input'];
  pageKey: Scalars['String']['input'];
  state?: InputMaybe<Scalars['String']['input']>;
  viewport: Scalars['String']['input'];
};


export type QueryLensOverviewArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensPageStatsArgs = {
  channelId: Scalars['ID']['input'];
  pageKey: Scalars['String']['input'];
};


export type QueryLensPagesArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryLensPerformanceArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensSearchAnalyticsArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensSessionEventsArgs = {
  id: Scalars['ID']['input'];
};


export type QueryLensSessionSignalsArgs = {
  id: Scalars['ID']['input'];
};


export type QueryLensSessionsArgs = {
  channelId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensStatesArgs = {
  channelId: Scalars['ID']['input'];
  pageKey: Scalars['String']['input'];
  viewport: Scalars['String']['input'];
};


export type QueryLensVisitorSessionsArgs = {
  anonId: Scalars['String']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLensVitalsSummaryArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryPulseEngagementArgs = {
  channelId: Scalars['ID']['input'];
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

export type SaveExperimentInput = {
  channelId: Scalars['ID']['input'];
  coverage: Scalars['Int']['input'];
  goalEvent: Scalars['String']['input'];
  key: Scalars['String']['input'];
  name: Scalars['String']['input'];
  segmentSlug?: InputMaybe<Scalars['String']['input']>;
  status: Scalars['String']['input'];
  variations: Array<ExperimentVariationInput>;
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

export type SegmentOption = {
  __typename?: 'SegmentOption';
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

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

export type UpdateCustomerSegmentInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};
