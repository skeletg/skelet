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
  experiment?: Maybe<Experiment>;
  experimentGoalEvents: Array<Scalars['String']['output']>;
  experimentResults: ExperimentResults;
  experiments: Array<Experiment>;
  experimentsOverview: ExperimentsOverview;
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
