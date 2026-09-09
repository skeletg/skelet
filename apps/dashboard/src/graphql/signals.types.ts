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
  Time: { input: string; output: string; }
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

export type Mutation = {
  __typename?: 'Mutation';
  createExperiment: Experiment;
  deleteExperiment: Scalars['Boolean']['output'];
  updateExperiment: Experiment;
};


export type MutationCreateExperimentArgs = {
  input: SaveExperimentInput;
};


export type MutationDeleteExperimentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationUpdateExperimentArgs = {
  id: Scalars['ID']['input'];
  input: SaveExperimentInput;
};

export type Query = {
  __typename?: 'Query';
  botTraffic: BotTraffic;
  experiment?: Maybe<Experiment>;
  experimentGoalEvents: Array<Scalars['String']['output']>;
  experimentResults: ExperimentResults;
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
};


export type QueryBotTrafficArgs = {
  bot?: InputMaybe<Scalars['String']['input']>;
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
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
