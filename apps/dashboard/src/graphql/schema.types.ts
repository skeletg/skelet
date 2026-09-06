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

export type AiSeoSuggestion = {
  __typename?: 'AISeoSuggestion';
  metaDescription: Scalars['String']['output'];
  metaTitle: Scalars['String']['output'];
};

export type AiStatus = {
  __typename?: 'AIStatus';
  enabled: Scalars['Boolean']['output'];
};

export type ActionArgument = {
  __typename?: 'ActionArgument';
  default?: Maybe<Scalars['String']['output']>;
  description: Scalars['String']['output'];
  label: Scalars['String']['output'];
  name: Scalars['String']['output'];
  options: Array<ActionArgumentOption>;
  required: Scalars['Boolean']['output'];
  type: ActionArgumentType;
};

export type ActionArgumentOption = {
  __typename?: 'ActionArgumentOption';
  label: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type ActionArgumentType =
  | 'BOOL'
  | 'INT'
  | 'SELECT'
  | 'STRING';

export type ActivePoint = {
  __typename?: 'ActivePoint';
  count: Scalars['Int']['output'];
  day: Scalars['Time']['output'];
};

export type AddBundleOptionInput = {
  inputType: BundleInputType;
  isRequired?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  maxSelections?: InputMaybe<Scalars['Int']['input']>;
  minSelections?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  title: Scalars['String']['input'];
};

export type AddBundleSelectionInput = {
  isDefault?: InputMaybe<Scalars['Boolean']['input']>;
  optionId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
  quantity?: InputMaybe<Scalars['Float']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type AddGroupedMemberInput = {
  defaultQuantity?: InputMaybe<Scalars['Float']['input']>;
  memberId: Scalars['ID']['input'];
  parentId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type AddKitComponentInput = {
  componentProductId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
  quantity?: InputMaybe<Scalars['Float']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type AddProductDownloadInput = {
  maxDownloads?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  title: Scalars['String']['input'];
  url: Scalars['String']['input'];
};

export type AddProductOptionInput = {
  fileExtensions?: InputMaybe<Scalars['String']['input']>;
  helpText?: InputMaybe<Scalars['String']['input']>;
  inputType: ProductOptionInputType;
  isRequired?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  maxChars?: InputMaybe<Scalars['Int']['input']>;
  maxFileBytes?: InputMaybe<Scalars['Int']['input']>;
  priceType?: InputMaybe<ProductOptionPriceType>;
  priceValue?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  title: Scalars['String']['input'];
  visibleIf?: InputMaybe<Scalars['JSON']['input']>;
};

export type AddProductOptionValueInput = {
  forcedLineQty?: InputMaybe<Scalars['Float']['input']>;
  isDefault?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
  priceType?: InputMaybe<ProductOptionPriceType>;
  priceValue?: InputMaybe<Scalars['Int']['input']>;
  quantity?: InputMaybe<Scalars['Float']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  sourceProductId?: InputMaybe<Scalars['ID']['input']>;
  title: Scalars['String']['input'];
};

export type Address = {
  __typename?: 'Address';
  city: Scalars['String']['output'];
  company?: Maybe<Scalars['String']['output']>;
  countryCode: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  lastName: Scalars['String']['output'];
  phone?: Maybe<Scalars['String']['output']>;
  postalCode: Scalars['String']['output'];
  state?: Maybe<Scalars['String']['output']>;
  street1: Scalars['String']['output'];
  street2?: Maybe<Scalars['String']['output']>;
};

export type AddressInput = {
  city: Scalars['String']['input'];
  company?: InputMaybe<Scalars['String']['input']>;
  countryCode: Scalars['String']['input'];
  firstName: Scalars['String']['input'];
  lastName: Scalars['String']['input'];
  phone?: InputMaybe<Scalars['String']['input']>;
  postalCode: Scalars['String']['input'];
  state?: InputMaybe<Scalars['String']['input']>;
  street1: Scalars['String']['input'];
  street2?: InputMaybe<Scalars['String']['input']>;
};

export type AdjustInventoryLevelInput = {
  locationId: Scalars['ID']['input'];
  newQuantity: Scalars['Float']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
  productId: Scalars['ID']['input'];
};

export type AdminAction = {
  __typename?: 'AdminAction';
  arguments: Array<ActionArgument>;
  description: Scalars['String']['output'];
  group: Scalars['String']['output'];
  name: Scalars['String']['output'];
  permissionRequired: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type AdminActionResult = {
  __typename?: 'AdminActionResult';
  data?: Maybe<Scalars['JSON']['output']>;
  durationMs: Scalars['Int']['output'];
  output: Scalars['String']['output'];
  success: Scalars['Boolean']['output'];
};

export type AdminAuthPayload = {
  __typename?: 'AdminAuthPayload';
  accessToken: Scalars['String']['output'];
  admin: AdminUser;
  firstLogin: Scalars['Boolean']['output'];
  refreshToken: Scalars['String']['output'];
};

export type AdminCart = {
  __typename?: 'AdminCart';
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  customerEmail?: Maybe<Scalars['String']['output']>;
  customerId?: Maybe<Scalars['ID']['output']>;
  customerName?: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  itemCount: Scalars['Int']['output'];
  lines: Array<AdminCartLine>;
  status: Scalars['String']['output'];
  subtotal: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type AdminCartLine = {
  __typename?: 'AdminCartLine';
  components: Array<AdminCartLineComponent>;
  id: Scalars['ID']['output'];
  isSimple: Scalars['Boolean']['output'];
  options: Array<AdminCartLineOption>;
  productName: Scalars['String']['output'];
  productSku: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  totalPrice: Scalars['Int']['output'];
  unitPrice: Scalars['Int']['output'];
};

export type AdminCartLineComponent = {
  __typename?: 'AdminCartLineComponent';
  id: Scalars['ID']['output'];
  productName: Scalars['String']['output'];
  productSku: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  unitPrice: Scalars['Int']['output'];
};

export type AdminCartLineOption = {
  __typename?: 'AdminCartLineOption';
  fileUrl: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  optionTitle: Scalars['String']['output'];
  priceDelta: Scalars['Int']['output'];
  textValue: Scalars['String']['output'];
  valueLabel: Scalars['String']['output'];
};

export type AdminCartLineOptionInput = {
  fileAssetId?: InputMaybe<Scalars['ID']['input']>;
  optionId: Scalars['ID']['input'];
  textValue?: InputMaybe<Scalars['String']['input']>;
  valueIds?: InputMaybe<Array<Scalars['ID']['input']>>;
};

export type AdminCreateCustomerInput = {
  channelId: Scalars['ID']['input'];
  customerGroupSlug?: InputMaybe<Scalars['String']['input']>;
  email: Scalars['String']['input'];
  firstName: Scalars['String']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  isEmailVerified?: InputMaybe<Scalars['Boolean']['input']>;
  lastName: Scalars['String']['input'];
  password: Scalars['String']['input'];
  phone?: InputMaybe<Scalars['String']['input']>;
};

export type AdminLoginInput = {
  email: Scalars['String']['input'];
  password: Scalars['String']['input'];
  totpCode?: InputMaybe<Scalars['String']['input']>;
};

export type AdminNotification = {
  __typename?: 'AdminNotification';
  actionUrl: Scalars['String']['output'];
  body: Scalars['String']['output'];
  category: Scalars['String']['output'];
  channelId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  entityId: Scalars['String']['output'];
  entityType: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  read: Scalars['Boolean']['output'];
  severity: Scalars['String']['output'];
  title: Scalars['String']['output'];
};

export type AdminPreference = {
  __typename?: 'AdminPreference';
  key: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  value: Scalars['JSON']['output'];
};

export type AdminProductQuestion = {
  __typename?: 'AdminProductQuestion';
  answer: Scalars['String']['output'];
  answeredAt?: Maybe<Scalars['Time']['output']>;
  authorEmail: Scalars['String']['output'];
  authorName: Scalars['String']['output'];
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  productId: Scalars['ID']['output'];
  question: Scalars['String']['output'];
  status: Scalars['String']['output'];
};

export type AdminSession = {
  __typename?: 'AdminSession';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  ipAddress?: Maybe<Scalars['String']['output']>;
  isCurrent: Scalars['Boolean']['output'];
  lastActiveAt: Scalars['Time']['output'];
  userAgent?: Maybe<Scalars['String']['output']>;
};

export type AdminTotpSetup = {
  __typename?: 'AdminTotpSetup';
  otpauthUrl: Scalars['String']['output'];
  secret: Scalars['String']['output'];
};

export type AdminUpdateCustomerInput = {
  customerGroupSlug?: InputMaybe<Scalars['String']['input']>;
  defaultBillingAddressId?: InputMaybe<Scalars['ID']['input']>;
  defaultShippingAddressId?: InputMaybe<Scalars['ID']['input']>;
  email?: InputMaybe<Scalars['String']['input']>;
  firstName?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  isEmailVerified?: InputMaybe<Scalars['Boolean']['input']>;
  lastName?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  note?: InputMaybe<Scalars['String']['input']>;
  phone?: InputMaybe<Scalars['String']['input']>;
};

export type AdminUser = {
  __typename?: 'AdminUser';
  createdAt: Scalars['Time']['output'];
  email: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  isSuperuser: Scalars['Boolean']['output'];
  lastLoginAt?: Maybe<Scalars['Time']['output']>;
  lastName: Scalars['String']['output'];
  permissions: Array<Permission>;
  roles: Array<Role>;
  sessions: Array<AdminSession>;
  totpEnabled: Scalars['Boolean']['output'];
};

export type AppliedPromotion = {
  __typename?: 'AppliedPromotion';
  appliedAt: Scalars['Time']['output'];
  code: Scalars['String']['output'];
  discount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  ruleId?: Maybe<Scalars['ID']['output']>;
  ruleName: Scalars['String']['output'];
  ruleSlug: Scalars['String']['output'];
};

export type AssignCmsAttributeInput = {
  attributeId: Scalars['ID']['input'];
  conditionAttributeId?: InputMaybe<Scalars['ID']['input']>;
  conditionValue?: InputMaybe<Scalars['String']['input']>;
  groupId?: InputMaybe<Scalars['ID']['input']>;
  isRequired?: InputMaybe<Scalars['Boolean']['input']>;
  pageTypeId: Scalars['ID']['input'];
  position?: InputMaybe<Scalars['Int']['input']>;
};

export type AssignReviewQuestionInput = {
  productTypeId: Scalars['ID']['input'];
  questionId: Scalars['ID']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type Attribute = {
  __typename?: 'Attribute';
  autoSegment: Scalars['Boolean']['output'];
  createdAt: Scalars['Time']['output'];
  defaultValue?: Maybe<Scalars['JSON']['output']>;
  descriptions: Array<AttributeContent>;
  facetCollapsed: Scalars['Boolean']['output'];
  filterable: Scalars['Boolean']['output'];
  h1Position: AttributeH1Position;
  id: Scalars['ID']['output'];
  inputType: AttributeInputType;
  isIndexable: Scalars['Boolean']['output'];
  isPublic: Scalars['Boolean']['output'];
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  privateMetadata: Array<MetadataItem>;
  relationshipTarget?: Maybe<AttributeReferenceTarget>;
  searchable: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  synapseTrack: Scalars['Boolean']['output'];
  unit: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  valueRequired: Scalars['Boolean']['output'];
  values: Array<AttributeValue>;
};

export type AttributeAffinity = {
  __typename?: 'AttributeAffinity';
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  values: Array<ProfileValue>;
};

export type AttributeContent = {
  __typename?: 'AttributeContent';
  description: Scalars['String']['output'];
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type AttributeGroup = {
  __typename?: 'AttributeGroup';
  attributes: Array<ProductTypeAttribute>;
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type AttributeH1Position =
  | 'AFTER'
  | 'BEFORE';

export type AttributeInputType =
  | 'BOOLEAN'
  | 'DATE'
  | 'DATE_TIME'
  | 'DROPDOWN'
  | 'MULTISELECT'
  | 'NUMERIC'
  | 'PLAIN_TEXT'
  | 'REFERENCE'
  | 'SWATCH';

export type AttributeReferenceRefInput = {
  targetId: Scalars['ID']['input'];
  targetType: AttributeReferenceTarget;
};

export type AttributeReferenceTarget =
  | 'CATEGORY'
  | 'COLLECTION'
  | 'PRODUCT';

export type AttributeValue = {
  __typename?: 'AttributeValue';
  descriptions: Array<AttributeValueContent>;
  id: Scalars['ID']['output'];
  isIndexable: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  value: Scalars['String']['output'];
};

export type AttributeValueChannelSlug = {
  __typename?: 'AttributeValueChannelSlug';
  channelId: Scalars['ID']['output'];
  id: Scalars['ID']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  valueId: Scalars['ID']['output'];
};

export type AttributeValueContent = {
  __typename?: 'AttributeValueContent';
  description: Scalars['String']['output'];
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type AttributeValueInput = {
  isIndexable?: InputMaybe<Scalars['Boolean']['input']>;
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  value?: InputMaybe<Scalars['String']['input']>;
};

export type AttributedRevenue = {
  __typename?: 'AttributedRevenue';
  cents: Scalars['Int64']['output'];
  currency: Scalars['String']['output'];
  orders: Scalars['Int']['output'];
};

export type AuditLogEntry = {
  __typename?: 'AuditLogEntry';
  action: Scalars['String']['output'];
  adminEmail: Scalars['String']['output'];
  adminId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  detail?: Maybe<Scalars['JSON']['output']>;
  durationMs: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  ip: Scalars['String']['output'];
  success: Scalars['Boolean']['output'];
};

export type AuditLogPage = {
  __typename?: 'AuditLogPage';
  entries: Array<AuditLogEntry>;
  total: Scalars['Int']['output'];
};

export type BlogAuthor = {
  __typename?: 'BlogAuthor';
  allContent: Array<BlogAuthorContent>;
  avatarAsset?: Maybe<MediaAsset>;
  content?: Maybe<BlogAuthorContent>;
  createdAt: Scalars['Time']['output'];
  displayName: Scalars['String']['output'];
  email?: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  linkedAdmin?: Maybe<AdminUser>;
  metadata: Array<MetadataItem>;
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BlogAuthorContent = {
  __typename?: 'BlogAuthorContent';
  bio?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BlogCategory = {
  __typename?: 'BlogCategory';
  allContent: Array<BlogCategoryContent>;
  children: Array<BlogCategory>;
  content?: Maybe<BlogCategoryContent>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  parent?: Maybe<BlogCategory>;
  position: Scalars['Int']['output'];
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  urlPath: Scalars['String']['output'];
};

export type BlogCategoryContent = {
  __typename?: 'BlogCategoryContent';
  description?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BlogPost = {
  __typename?: 'BlogPost';
  allContent: Array<BlogPostContent>;
  author?: Maybe<BlogAuthor>;
  categories: Array<BlogCategory>;
  channelListings: Array<BlogPostChannelListing>;
  content?: Maybe<BlogPostContent>;
  coverAsset?: Maybe<MediaAsset>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  metadata: Array<MetadataItem>;
  privateMetadata: Array<MetadataItem>;
  publishedAt?: Maybe<Scalars['Time']['output']>;
  readingTimeMinutes?: Maybe<Scalars['Int']['output']>;
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  status: BlogPostStatus;
  tags: Array<BlogTag>;
  updatedAt: Scalars['Time']['output'];
};

export type BlogPostChannelListing = {
  __typename?: 'BlogPostChannelListing';
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  isPublished: Scalars['Boolean']['output'];
  seoNoindex: Scalars['Boolean']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BlogPostContent = {
  __typename?: 'BlogPostContent';
  body?: Maybe<Scalars['JSON']['output']>;
  excerpt?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BlogPostFilter = {
  authorSlug?: InputMaybe<Scalars['String']['input']>;
  categorySlug?: InputMaybe<Scalars['String']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  includeUnpublished?: InputMaybe<Scalars['Boolean']['input']>;
  status?: InputMaybe<BlogPostStatus>;
  tagSlug?: InputMaybe<Scalars['String']['input']>;
};

export type BlogPostList = {
  __typename?: 'BlogPostList';
  items: Array<BlogPost>;
  totalCount?: Maybe<Scalars['Int']['output']>;
};

export type BlogPostPreviewToken = {
  __typename?: 'BlogPostPreviewToken';
  expiresAt: Scalars['Time']['output'];
  token: Scalars['String']['output'];
};

export type BlogPostStatus =
  | 'ARCHIVED'
  | 'DRAFT'
  | 'PUBLISHED'
  | 'SCHEDULED';

export type BlogTag = {
  __typename?: 'BlogTag';
  allContent: Array<BlogTagContent>;
  content?: Maybe<BlogTagContent>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BlogTagContent = {
  __typename?: 'BlogTagContent';
  description?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
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

export type BulkAttributeValueResult = {
  __typename?: 'BulkAttributeValueResult';
  count: Scalars['Int']['output'];
};

export type BulkCmsPageResult = {
  __typename?: 'BulkCMSPageResult';
  count: Scalars['Int']['output'];
};

export type BundleInputType =
  | 'CHECKBOX'
  | 'DROPDOWN'
  | 'MULTISELECT'
  | 'RADIO';

export type BundleMembership = {
  __typename?: 'BundleMembership';
  bundle: Product;
  isDefault: Scalars['Boolean']['output'];
  option: BundleOption;
  quantity: Scalars['Float']['output'];
  siblings: Array<BundleSelection>;
  sortOrder: Scalars['Int']['output'];
};

export type BundleOption = {
  __typename?: 'BundleOption';
  content: Array<BundleOptionContent>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  inputType: BundleInputType;
  isRequired: Scalars['Boolean']['output'];
  maxSelections?: Maybe<Scalars['Int']['output']>;
  minSelections: Scalars['Int']['output'];
  selections: Array<BundleSelection>;
  sortOrder: Scalars['Int']['output'];
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type BundleOptionContent = {
  __typename?: 'BundleOptionContent';
  locale: Scalars['String']['output'];
  title: Scalars['String']['output'];
};

export type BundleSelection = {
  __typename?: 'BundleSelection';
  id: Scalars['ID']['output'];
  isDefault: Scalars['Boolean']['output'];
  product: Product;
  quantity: Scalars['Float']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type CmsAttribute = {
  __typename?: 'CMSAttribute';
  allContent: Array<CmsAttributeContent>;
  content?: Maybe<CmsAttributeContent>;
  createdAt: Scalars['Time']['output'];
  defaultValue?: Maybe<Scalars['JSON']['output']>;
  id: Scalars['ID']['output'];
  inputType: CmsInputType;
  isRequiredDefault: Scalars['Boolean']['output'];
  isTranslatable: Scalars['Boolean']['output'];
  metadata: Array<MetadataItem>;
  relationshipTarget?: Maybe<CmsRelationshipTarget>;
  slug: Scalars['String']['output'];
  unit?: Maybe<Scalars['String']['output']>;
  updatedAt: Scalars['Time']['output'];
  validation?: Maybe<Scalars['JSON']['output']>;
  values: Array<CmsAttributeValue>;
};

export type CmsAttributeContent = {
  __typename?: 'CMSAttributeContent';
  helperText?: Maybe<Scalars['String']['output']>;
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
  placeholder?: Maybe<Scalars['String']['output']>;
  updatedAt: Scalars['Time']['output'];
};

export type CmsAttributeGroup = {
  __typename?: 'CMSAttributeGroup';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  position: Scalars['Int']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsAttributeValue = {
  __typename?: 'CMSAttributeValue';
  allContent: Array<CmsAttributeValueContent>;
  attributeId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  position: Scalars['Int']['output'];
  slug: Scalars['String']['output'];
};

export type CmsAttributeValueContent = {
  __typename?: 'CMSAttributeValueContent';
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsAttributeValueInput = {
  position?: InputMaybe<Scalars['Int']['input']>;
  slug: Scalars['String']['input'];
};

export type CmsBlockType = {
  __typename?: 'CMSBlockType';
  createdAt: Scalars['Time']['output'];
  fields: Scalars['JSON']['output'];
  htmlTemplate: Scalars['String']['output'];
  icon: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  key: Scalars['String']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsInputType =
  | 'ASSET'
  | 'ASSETS'
  | 'BOOLEAN'
  | 'DATE'
  | 'DATETIME'
  | 'JSON'
  | 'MULTISELECT'
  | 'NUMBER'
  | 'RELATIONSHIP'
  | 'RICHTEXT'
  | 'SELECT'
  | 'TEXT'
  | 'TEXTAREA'
  | 'URL';

export type CmsPage = {
  __typename?: 'CMSPage';
  allContent: Array<CmsPageContent>;
  approvedAt?: Maybe<Scalars['Time']['output']>;
  approvedBy?: Maybe<Scalars['ID']['output']>;
  assignedTo?: Maybe<Scalars['ID']['output']>;
  basePath: Scalars['String']['output'];
  channelListings: Array<CmsPageChannelListing>;
  content?: Maybe<CmsPageContent>;
  createdAt: Scalars['Time']['output'];
  data?: Maybe<Scalars['JSON']['output']>;
  experimentKey: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  metadata: Array<MetadataItem>;
  pageTypeId: Scalars['ID']['output'];
  parentId?: Maybe<Scalars['ID']['output']>;
  position: Scalars['Int']['output'];
  privateMetadata: Array<MetadataItem>;
  publishedAt?: Maybe<Scalars['Time']['output']>;
  relationships: Array<CmsPageRelationship>;
  scheduledAt?: Maybe<Scalars['Time']['output']>;
  selections: Array<CmsPageSelection>;
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  submittedAt?: Maybe<Scalars['Time']['output']>;
  submittedBy?: Maybe<Scalars['ID']['output']>;
  tags: Array<Scalars['String']['output']>;
  updatedAt: Scalars['Time']['output'];
  urlPath: Scalars['String']['output'];
  variants: Array<CmsPageVariant>;
  workflowState: CmsWorkflowState;
};

export type CmsPageChannelListing = {
  __typename?: 'CMSPageChannelListing';
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  isHomepage: Scalars['Boolean']['output'];
  isPublished: Scalars['Boolean']['output'];
  seoNoindex: Scalars['Boolean']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsPageComment = {
  __typename?: 'CMSPageComment';
  authorId: Scalars['ID']['output'];
  blockId?: Maybe<Scalars['String']['output']>;
  body: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  locale?: Maybe<Scalars['String']['output']>;
  pageId: Scalars['ID']['output'];
  resolved: Scalars['Boolean']['output'];
  resolvedAt?: Maybe<Scalars['Time']['output']>;
  resolvedBy?: Maybe<Scalars['ID']['output']>;
};

export type CmsPageCommentInput = {
  blockId?: InputMaybe<Scalars['String']['input']>;
  body: Scalars['String']['input'];
  locale?: InputMaybe<Scalars['String']['input']>;
  pageId: Scalars['ID']['input'];
};

export type CmsPageContent = {
  __typename?: 'CMSPageContent';
  body?: Maybe<Scalars['JSON']['output']>;
  data?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsPageFilter = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  onlyPublished?: InputMaybe<Scalars['Boolean']['input']>;
  pageTypeId?: InputMaybe<Scalars['ID']['input']>;
  parentId?: InputMaybe<Scalars['ID']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
  workflowState?: InputMaybe<CmsWorkflowState>;
};

export type CmsPagePreviewToken = {
  __typename?: 'CMSPagePreviewToken';
  expiresAt: Scalars['Time']['output'];
  token: Scalars['String']['output'];
};

export type CmsPageRelationship = {
  __typename?: 'CMSPageRelationship';
  attributeId: Scalars['ID']['output'];
  position: Scalars['Int']['output'];
  targetId: Scalars['ID']['output'];
  targetType: CmsRelationshipTarget;
};

export type CmsPageRelationshipInput = {
  attributeId: Scalars['ID']['input'];
  position?: InputMaybe<Scalars['Int']['input']>;
  targetId: Scalars['ID']['input'];
  targetType: CmsRelationshipTarget;
};

export type CmsPageRevision = {
  __typename?: 'CMSPageRevision';
  authorAdminId?: Maybe<Scalars['ID']['output']>;
  body?: Maybe<Scalars['JSON']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  pageId: Scalars['ID']['output'];
  title: Scalars['String']['output'];
};

export type CmsPageSelection = {
  __typename?: 'CMSPageSelection';
  attributeId: Scalars['ID']['output'];
  valueId: Scalars['ID']['output'];
};

export type CmsPageSelectionInput = {
  attributeId: Scalars['ID']['input'];
  valueId: Scalars['ID']['input'];
};

export type CmsPageType = {
  __typename?: 'CMSPageType';
  allContent: Array<CmsPageTypeContent>;
  attributes: Array<CmsPageTypeAttribute>;
  channelRoots: Array<CmsPageTypeChannelRoot>;
  content?: Maybe<CmsPageTypeContent>;
  createdAt: Scalars['Time']['output'];
  defaultBlocks?: Maybe<Scalars['JSON']['output']>;
  defaultMetaTitle?: Maybe<Scalars['String']['output']>;
  defaultSeoNoindex: Scalars['Boolean']['output'];
  icon?: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  isSingleton: Scalars['Boolean']['output'];
  isSystem: Scalars['Boolean']['output'];
  pageCount: Scalars['Int']['output'];
  position: Scalars['Int']['output'];
  rootSlug?: Maybe<Scalars['String']['output']>;
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsPageTypeAttribute = {
  __typename?: 'CMSPageTypeAttribute';
  attribute: CmsAttribute;
  conditionAttributeId?: Maybe<Scalars['ID']['output']>;
  conditionValue?: Maybe<Scalars['String']['output']>;
  groupId?: Maybe<Scalars['ID']['output']>;
  isRequired: Scalars['Boolean']['output'];
  position: Scalars['Int']['output'];
};

export type CmsPageTypeChannelRoot = {
  __typename?: 'CMSPageTypeChannelRoot';
  channelId: Scalars['ID']['output'];
  rootSlug: Scalars['String']['output'];
};

export type CmsPageTypeContent = {
  __typename?: 'CMSPageTypeContent';
  description?: Maybe<Scalars['String']['output']>;
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsPageVariant = {
  __typename?: 'CMSPageVariant';
  body?: Maybe<Scalars['JSON']['output']>;
  variantKey: Scalars['String']['output'];
};

export type CmsRelationshipRefInput = {
  targetId: Scalars['ID']['input'];
  targetType: CmsRelationshipTarget;
};

export type CmsRelationshipTarget =
  | 'BLOG_POST'
  | 'CATEGORY'
  | 'CMS_PAGE'
  | 'COLLECTION'
  | 'PRODUCT';

export type CmsSnippet = {
  __typename?: 'CMSSnippet';
  blocks: Scalars['JSON']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CmsTag = {
  __typename?: 'CMSTag';
  createdAt: Scalars['Time']['output'];
  name: Scalars['String']['output'];
  pageCount: Scalars['Int']['output'];
  slug: Scalars['String']['output'];
};

export type CmsWorkflowState =
  | 'APPROVED'
  | 'DRAFT'
  | 'IN_REVIEW'
  | 'PUBLISHED';

export type CacheRevalidation = {
  __typename?: 'CacheRevalidation';
  actorId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  paths: Array<Scalars['String']['output']>;
  reason: Scalars['String']['output'];
  surface: Scalars['String']['output'];
  tags: Array<Scalars['String']['output']>;
};

export type CacheSurface = {
  __typename?: 'CacheSurface';
  bulkInvalidatable: Scalars['Boolean']['output'];
  enabled: Scalars['Boolean']['output'];
  key: Scalars['String']['output'];
  label: Scalars['String']['output'];
  ttlSeconds: Scalars['Int']['output'];
};

export type CampaignAudienceEstimate = {
  __typename?: 'CampaignAudienceEstimate';
  eligible: Scalars['Int']['output'];
  total: Scalars['Int']['output'];
};

export type CampaignVariantStat = {
  __typename?: 'CampaignVariantStat';
  clicked: Scalars['Int']['output'];
  opened: Scalars['Int']['output'];
  sent: Scalars['Int']['output'];
  variant: Scalars['String']['output'];
};

export type CartList = {
  __typename?: 'CartList';
  items: Array<CartListItem>;
  totalCount: Scalars['Int']['output'];
};

export type CartListItem = {
  __typename?: 'CartListItem';
  channelId: Scalars['ID']['output'];
  currencyCode: Scalars['String']['output'];
  customerEmail?: Maybe<Scalars['String']['output']>;
  customerId?: Maybe<Scalars['ID']['output']>;
  customerName?: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  itemCount: Scalars['Int']['output'];
  status: Scalars['String']['output'];
  subtotal: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CascadeAttributeIndexableResult = {
  __typename?: 'CascadeAttributeIndexableResult';
  affectedProductsCount: Scalars['Int']['output'];
  affectedValuesCount: Scalars['Int']['output'];
  attribute: Attribute;
};

export type Category = {
  __typename?: 'Category';
  ancestors: Array<Category>;
  children: Array<Category>;
  content: Array<CategoryContent>;
  contentLocale?: Maybe<Scalars['String']['output']>;
  coverImage?: Maybe<CollectionCover>;
  createdAt: Scalars['Time']['output'];
  description?: Maybe<Scalars['JSON']['output']>;
  facetOverrides: Array<FacetOverride>;
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  level: Scalars['Int']['output'];
  listings: Array<CategoryChannelListing>;
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  metadata: Array<MetadataItem>;
  name?: Maybe<Scalars['String']['output']>;
  parent?: Maybe<Category>;
  position: Scalars['Int']['output'];
  privateMetadata: Array<MetadataItem>;
  productCount: Scalars['Int']['output'];
  products: Array<Product>;
  seoNoindex: Scalars['Boolean']['output'];
  shortDescription?: Maybe<Scalars['JSON']['output']>;
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  urlPath: Scalars['String']['output'];
};

export type CategoryChannelListing = {
  __typename?: 'CategoryChannelListing';
  categoryId: Scalars['ID']['output'];
  channel: Channel;
  channelId: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  sortOrderOverride?: Maybe<Scalars['Int']['output']>;
  updatedAt: Scalars['Time']['output'];
};

export type CategoryContent = {
  __typename?: 'CategoryContent';
  description?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  shortDescription?: Maybe<Scalars['JSON']['output']>;
  updatedAt: Scalars['Time']['output'];
};

export type Channel = {
  __typename?: 'Channel';
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  defaultLocale: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  inventoryLocations: Array<ChannelInventoryLocation>;
  isActive: Scalars['Boolean']['output'];
  isDefault: Scalars['Boolean']['output'];
  locales: Array<Locale>;
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  publicUrl: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type ChannelInventoryLocation = {
  __typename?: 'ChannelInventoryLocation';
  location: InventoryLocation;
  priority: Scalars['Int']['output'];
};

export type ChatActionResult = {
  __typename?: 'ChatActionResult';
  message: Scalars['String']['output'];
  ok: Scalars['Boolean']['output'];
};

export type ChatConversation = {
  __typename?: 'ChatConversation';
  id: Scalars['ID']['output'];
  messages: Array<ChatMessage>;
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ChatConversationSummary = {
  __typename?: 'ChatConversationSummary';
  id: Scalars['ID']['output'];
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ChatMessage = {
  __typename?: 'ChatMessage';
  content: Scalars['String']['output'];
  role: Scalars['String']['output'];
};

export type Collection = {
  __typename?: 'Collection';
  content: Array<CollectionContent>;
  contentLocale?: Maybe<Scalars['String']['output']>;
  coverImage?: Maybe<CollectionCover>;
  createdAt: Scalars['Time']['output'];
  description?: Maybe<Scalars['JSON']['output']>;
  facetOverrides: Array<FacetOverride>;
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  metadata: Array<MetadataItem>;
  name?: Maybe<Scalars['String']['output']>;
  privateMetadata: Array<MetadataItem>;
  products: Array<Product>;
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CollectionContent = {
  __typename?: 'CollectionContent';
  description?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CollectionCover = {
  __typename?: 'CollectionCover';
  altText: Scalars['String']['output'];
  height: Scalars['Int']['output'];
  mimeType: Scalars['String']['output'];
  status: ProductMediaStatus;
  url: Scalars['String']['output'];
  width: Scalars['Int']['output'];
};

export type ConfigGroup = {
  __typename?: 'ConfigGroup';
  fields: Array<ConfigValue>;
  label: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type ConfigOption = {
  __typename?: 'ConfigOption';
  label: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type ConfigScope = {
  __typename?: 'ConfigScope';
  channelId?: Maybe<Scalars['ID']['output']>;
  kind: ConfigScopeKind;
  locale?: Maybe<Scalars['String']['output']>;
};

export type ConfigScopeInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  kind: ConfigScopeKind;
  locale?: InputMaybe<Scalars['String']['input']>;
};

export type ConfigScopeKind =
  | 'CHANNEL'
  | 'DEFAULT';

export type ConfigScoping =
  | 'CHANNEL'
  | 'GLOBAL';

export type ConfigSection = {
  __typename?: 'ConfigSection';
  groups: Array<ConfigGroup>;
  label: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type ConfigType =
  | 'BOOL'
  | 'COLOR'
  | 'IMAGE'
  | 'INT'
  | 'JSON'
  | 'MULTISELECT'
  | 'PHONE'
  | 'SEARCH_ATTRIBUTES'
  | 'SECRET'
  | 'SELECT'
  | 'SORT_OPTIONS'
  | 'STRING'
  | 'TEXTAREA'
  | 'WORKING_HOURS';

export type ConfigValue = {
  __typename?: 'ConfigValue';
  description: Scalars['String']['output'];
  field: Scalars['String']['output'];
  group: Scalars['String']['output'];
  isDefault: Scalars['Boolean']['output'];
  isSet: Scalars['Boolean']['output'];
  label: Scalars['String']['output'];
  options?: Maybe<Array<ConfigOption>>;
  path: Scalars['String']['output'];
  public: Scalars['Boolean']['output'];
  scope: ConfigScope;
  scoping: ConfigScoping;
  section: Scalars['String']['output'];
  type: ConfigType;
  updatedAt?: Maybe<Scalars['Time']['output']>;
  value: Scalars['String']['output'];
};

export type ConfigurableMembership = {
  __typename?: 'ConfigurableMembership';
  optionValues: Array<ProductAttributeValue>;
  parent: Product;
  siblings: Array<Product>;
  sortOrder: Scalars['Int']['output'];
  variantMode?: Maybe<VariantMode>;
};

export type ConsentChannel =
  | 'EMAIL'
  | 'PUSH'
  | 'SMS';

export type ConsentDefinition = {
  __typename?: 'ConsentDefinition';
  channel: ConsentChannel;
  defaultOptIn: Scalars['Boolean']['output'];
  description: Scalars['String']['output'];
  name: Scalars['String']['output'];
  purpose: ConsentPurpose;
  required: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
};

export type ConsentPurpose =
  | 'MARKETING'
  | 'TRANSACTIONAL';

export type Country = {
  __typename?: 'Country';
  code: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  defaultCurrency?: Maybe<Currency>;
  defaultCurrencyCode: Scalars['String']['output'];
  emojiFlag: Scalars['String']['output'];
  isActive: Scalars['Boolean']['output'];
  iso3Code: Scalars['String']['output'];
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  nativeName: Scalars['String']['output'];
  numericCode: Scalars['String']['output'];
  phoneCode: Scalars['String']['output'];
  regions: Array<Region>;
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type CrawlAudit = {
  __typename?: 'CrawlAudit';
  crawl: SiteCrawl;
  issueCounts: Array<SeoIssueCount>;
  pages: Array<CrawlAuditPage>;
  totalPages: Scalars['Int']['output'];
};

export type CrawlAuditPage = {
  __typename?: 'CrawlAuditPage';
  byteSize: Scalars['Int']['output'];
  canonical: Scalars['String']['output'];
  depth: Scalars['Int']['output'];
  entityId?: Maybe<Scalars['ID']['output']>;
  entityType?: Maybe<LinkNodeType>;
  externalLinks: Scalars['Int']['output'];
  fetchMs: Scalars['Int']['output'];
  h1: Scalars['String']['output'];
  h1Count: Scalars['Int']['output'];
  headingCounts: Array<Scalars['Int']['output']>;
  headings: Array<CrawlHeading>;
  inLinks: Scalars['Int']['output'];
  internalLinks: Scalars['Int']['output'];
  issues: Array<SeoIssue>;
  kind: CrawlPageKind;
  metaDesc: Scalars['String']['output'];
  metaLength: Scalars['Int']['output'];
  nofollow: Scalars['Boolean']['output'];
  noindex: Scalars['Boolean']['output'];
  path: Scalars['String']['output'];
  redirectTo: Scalars['String']['output'];
  robotsMeta: Scalars['String']['output'];
  statusCode: Scalars['Int']['output'];
  title: Scalars['String']['output'];
  titleLength: Scalars['Int']['output'];
  wordCount: Scalars['Int']['output'];
};

export type CrawlChange = {
  __typename?: 'CrawlChange';
  after: Scalars['String']['output'];
  before: Scalars['String']['output'];
  isRegression: Scalars['Boolean']['output'];
  kind: CrawlChangeKind;
  path: Scalars['String']['output'];
};

export type CrawlChangeCount = {
  __typename?: 'CrawlChangeCount';
  count: Scalars['Int']['output'];
  kind: CrawlChangeKind;
};

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

export type CrawlDiff = {
  __typename?: 'CrawlDiff';
  base: SiteCrawl;
  changes: Array<CrawlChange>;
  compared: Scalars['Int']['output'];
  counts: Array<CrawlChangeCount>;
  coverageComparable: Scalars['Boolean']['output'];
  current: SiteCrawl;
  regressions: Scalars['Int']['output'];
};

export type CrawlHeading = {
  __typename?: 'CrawlHeading';
  level: Scalars['Int']['output'];
  text: Scalars['String']['output'];
};

export type CrawlPageKind =
  | 'FILTER'
  | 'PAGE'
  | 'PARAM';

export type CreateAdminInput = {
  email: Scalars['String']['input'];
  firstName: Scalars['String']['input'];
  lastName: Scalars['String']['input'];
  password: Scalars['String']['input'];
  roleSlugs: Array<Scalars['String']['input']>;
};

export type CreateAttributeInput = {
  autoSegment?: InputMaybe<Scalars['Boolean']['input']>;
  defaultValue?: InputMaybe<Scalars['JSON']['input']>;
  facetCollapsed?: InputMaybe<Scalars['Boolean']['input']>;
  filterable?: InputMaybe<Scalars['Boolean']['input']>;
  h1Position?: InputMaybe<AttributeH1Position>;
  inputType: AttributeInputType;
  isIndexable?: InputMaybe<Scalars['Boolean']['input']>;
  isPublic?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  relationshipTarget?: InputMaybe<AttributeReferenceTarget>;
  searchable?: InputMaybe<Scalars['Boolean']['input']>;
  slug: Scalars['String']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  synapseTrack?: InputMaybe<Scalars['Boolean']['input']>;
  unit?: InputMaybe<Scalars['String']['input']>;
  valueRequired?: InputMaybe<Scalars['Boolean']['input']>;
  values?: InputMaybe<Array<AttributeValueInput>>;
};

export type CreateBlogAuthorInput = {
  avatarAssetId?: InputMaybe<Scalars['ID']['input']>;
  bio?: InputMaybe<Scalars['JSON']['input']>;
  displayName: Scalars['String']['input'];
  email?: InputMaybe<Scalars['String']['input']>;
  linkedAdminId?: InputMaybe<Scalars['ID']['input']>;
  locale?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateBlogCategoryInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  parentId?: InputMaybe<Scalars['ID']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateBlogPostInput = {
  authorId?: InputMaybe<Scalars['ID']['input']>;
  body?: InputMaybe<Scalars['JSON']['input']>;
  categoryIds?: InputMaybe<Array<Scalars['ID']['input']>>;
  coverAssetId?: InputMaybe<Scalars['ID']['input']>;
  excerpt?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  publishedAt?: InputMaybe<Scalars['Time']['input']>;
  readingTimeMinutes?: InputMaybe<Scalars['Int']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug: Scalars['String']['input'];
  status?: InputMaybe<BlogPostStatus>;
  tagIds?: InputMaybe<Array<Scalars['ID']['input']>>;
  title: Scalars['String']['input'];
};

export type CreateBlogTagInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  locale?: InputMaybe<Scalars['String']['input']>;
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateCmsAttributeInput = {
  defaultValue?: InputMaybe<Scalars['JSON']['input']>;
  inputType: CmsInputType;
  isRequiredDefault?: InputMaybe<Scalars['Boolean']['input']>;
  isTranslatable?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  relationshipTarget?: InputMaybe<CmsRelationshipTarget>;
  slug: Scalars['String']['input'];
  unit?: InputMaybe<Scalars['String']['input']>;
  validation?: InputMaybe<Scalars['JSON']['input']>;
  values?: InputMaybe<Array<CmsAttributeValueInput>>;
};

export type CreateCmsBlockTypeInput = {
  fields?: InputMaybe<Scalars['JSON']['input']>;
  htmlTemplate?: InputMaybe<Scalars['String']['input']>;
  icon?: InputMaybe<Scalars['String']['input']>;
  key: Scalars['String']['input'];
  name: Scalars['String']['input'];
};

export type CreateCmsPageInput = {
  body?: InputMaybe<Scalars['JSON']['input']>;
  contentData?: InputMaybe<Scalars['JSON']['input']>;
  data?: InputMaybe<Scalars['JSON']['input']>;
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  pageTypeId: Scalars['ID']['input'];
  parentId?: InputMaybe<Scalars['ID']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  publishedAt?: InputMaybe<Scalars['Time']['input']>;
  relationships?: InputMaybe<Array<CmsPageRelationshipInput>>;
  selections?: InputMaybe<Array<CmsPageSelectionInput>>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug: Scalars['String']['input'];
  title: Scalars['String']['input'];
};

export type CreateCmsPageTypeInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  icon?: InputMaybe<Scalars['String']['input']>;
  isSystem?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  name: Scalars['String']['input'];
  position?: InputMaybe<Scalars['Int']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateCmsSnippetInput = {
  blocks?: InputMaybe<Scalars['JSON']['input']>;
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
};

export type CreateCategoryInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  parentId?: InputMaybe<Scalars['ID']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  shortDescription?: InputMaybe<Scalars['JSON']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateChannelInput = {
  currencyCode?: InputMaybe<Scalars['String']['input']>;
  defaultLocale?: InputMaybe<Scalars['String']['input']>;
  locales?: InputMaybe<Array<Scalars['String']['input']>>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  publicUrl?: InputMaybe<Scalars['String']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateCollectionInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug: Scalars['String']['input'];
};

export type CreateCountryInput = {
  code: Scalars['String']['input'];
  defaultCurrencyCode?: InputMaybe<Scalars['String']['input']>;
  emojiFlag?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  nativeName?: InputMaybe<Scalars['String']['input']>;
  phoneCode?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type CreateCurrencyInput = {
  code: Scalars['String']['input'];
  decimals?: InputMaybe<Scalars['Int']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  symbol?: InputMaybe<Scalars['String']['input']>;
  symbolPosition?: InputMaybe<Scalars['String']['input']>;
};

export type CreateCustomerGroupInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
};

export type CreateCustomerSegmentInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
};

export type CreateDraftOrderInput = {
  billingAddress: AddressInput;
  channelId: Scalars['ID']['input'];
  customerId?: InputMaybe<Scalars['ID']['input']>;
  customerNote?: InputMaybe<Scalars['String']['input']>;
  email: Scalars['String']['input'];
  lines: Array<DraftOrderLineInput>;
  manualDiscountCents?: InputMaybe<Scalars['Int']['input']>;
  shippingAddress: AddressInput;
  shippingCarrierSlug?: InputMaybe<Scalars['String']['input']>;
  shippingMethodCode?: InputMaybe<Scalars['String']['input']>;
  sourceCartId?: InputMaybe<Scalars['ID']['input']>;
};

export type CreateFulfillmentInput = {
  carrier?: InputMaybe<Scalars['String']['input']>;
  insuredValueCents?: InputMaybe<Scalars['Int']['input']>;
  lines: Array<FulfillmentLineInput>;
  orderId: Scalars['ID']['input'];
  trackingNumber?: InputMaybe<Scalars['String']['input']>;
};

export type CreateIntegrationInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  expiresAt?: InputMaybe<Scalars['Time']['input']>;
  name: Scalars['String']['input'];
  permissionSlugs: Array<Scalars['String']['input']>;
};

export type CreateIntegrationPayload = {
  __typename?: 'CreateIntegrationPayload';
  integration: Integration;
  rawToken: Scalars['String']['output'];
};

export type CreateInventoryLocationInput = {
  city?: InputMaybe<Scalars['String']['input']>;
  contactName?: InputMaybe<Scalars['String']['input']>;
  countryCode?: InputMaybe<Scalars['String']['input']>;
  email?: InputMaybe<Scalars['String']['input']>;
  externalReference?: InputMaybe<Scalars['String']['input']>;
  hours?: InputMaybe<Scalars['JSON']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  latitude?: InputMaybe<Scalars['Float']['input']>;
  line1?: InputMaybe<Scalars['String']['input']>;
  line2?: InputMaybe<Scalars['String']['input']>;
  longitude?: InputMaybe<Scalars['Float']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  phone?: InputMaybe<Scalars['String']['input']>;
  postalCode?: InputMaybe<Scalars['String']['input']>;
  region?: InputMaybe<Scalars['String']['input']>;
  slug: Scalars['String']['input'];
  timezone?: InputMaybe<Scalars['String']['input']>;
  type?: InputMaybe<Scalars['String']['input']>;
};

export type CreateLocaleInput = {
  code: Scalars['String']['input'];
  dateFormat?: InputMaybe<Scalars['String']['input']>;
  decimalSeparator?: InputMaybe<Scalars['String']['input']>;
  defaultCountryCode?: InputMaybe<Scalars['String']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  emailFromNameOverride?: InputMaybe<Scalars['String']['input']>;
  firstDayOfWeek?: InputMaybe<Scalars['Int']['input']>;
  flagEmoji?: InputMaybe<Scalars['String']['input']>;
  hreflangValue?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  isRTL?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  nativeName?: InputMaybe<Scalars['String']['input']>;
  pluralRule?: InputMaybe<Scalars['String']['input']>;
  script?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  thousandsSeparator?: InputMaybe<Scalars['String']['input']>;
  timeFormat?: InputMaybe<Scalars['String']['input']>;
};

export type CreateMenuInput = {
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
};

export type CreateMenuItemInput = {
  label: Scalars['String']['input'];
  menuId: Scalars['ID']['input'];
  parentId?: InputMaybe<Scalars['ID']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  targetId?: InputMaybe<Scalars['ID']['input']>;
  targetType?: InputMaybe<MenuItemTargetType>;
  url?: InputMaybe<Scalars['String']['input']>;
};

export type CreateProductInput = {
  canonicalUrl?: InputMaybe<Scalars['String']['input']>;
  defaultCurrencyCode?: InputMaybe<Scalars['String']['input']>;
  defaultPrice?: InputMaybe<Scalars['Int']['input']>;
  descriptionRichText?: InputMaybe<Scalars['JSON']['input']>;
  kind: ProductKind;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  productTypeId: Scalars['ID']['input'];
  quantityDecimals?: InputMaybe<Scalars['Int']['input']>;
  quantityStep?: InputMaybe<Scalars['Float']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  shortDescription?: InputMaybe<Scalars['JSON']['input']>;
  sku: Scalars['String']['input'];
  tracksInventoryOverride?: InputMaybe<Scalars['Boolean']['input']>;
  url?: InputMaybe<Scalars['String']['input']>;
  urlAuto?: InputMaybe<Scalars['Boolean']['input']>;
  urlTemplateOverride?: InputMaybe<Scalars['String']['input']>;
};

export type CreateProductTypeInput = {
  isDigital?: InputMaybe<Scalars['Boolean']['input']>;
  isShippingRequired?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name: Scalars['String']['input'];
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  slug: Scalars['String']['input'];
  tracksInventory?: InputMaybe<Scalars['Boolean']['input']>;
  urlTemplate?: InputMaybe<Scalars['String']['input']>;
};

export type CreatePromotionRuleInput = {
  actions?: InputMaybe<Array<PromotionActionInput>>;
  bannerMediaId?: InputMaybe<Scalars['ID']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  conditions?: InputMaybe<Array<PromotionConditionInput>>;
  description?: InputMaybe<Scalars['String']['input']>;
  exclusivityGroup?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  labelMediaId?: InputMaybe<Scalars['ID']['input']>;
  maxUses?: InputMaybe<Scalars['Int']['input']>;
  name: Scalars['String']['input'];
  perCustomerLimit?: InputMaybe<Scalars['Int']['input']>;
  pillBgColor?: InputMaybe<Scalars['String']['input']>;
  pillText?: InputMaybe<Scalars['String']['input']>;
  pillTextColor?: InputMaybe<Scalars['String']['input']>;
  priority?: InputMaybe<Scalars['Int']['input']>;
  publicLabel?: InputMaybe<Scalars['String']['input']>;
  requiresCode?: InputMaybe<Scalars['Boolean']['input']>;
  scheduleJson?: InputMaybe<Scalars['JSON']['input']>;
  slug: Scalars['String']['input'];
  stopAfterApply?: InputMaybe<PromotionStopMode>;
  validFrom?: InputMaybe<Scalars['Time']['input']>;
  validTo?: InputMaybe<Scalars['Time']['input']>;
};

export type CreateRemoteProviderInput = {
  cacheTtlS?: InputMaybe<Scalars['Int']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  config?: InputMaybe<Scalars['JSON']['input']>;
  failurePolicy?: InputMaybe<RemoteProviderFailurePolicy>;
  kind: RemoteProviderKind;
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  targetUrl: Scalars['String']['input'];
  timeoutMs?: InputMaybe<Scalars['Int']['input']>;
};

export type CreateRemoteProviderPayload = {
  __typename?: 'CreateRemoteProviderPayload';
  provider: RemoteProvider;
  rawSecret: Scalars['String']['output'];
};

export type CreateReviewCategoryInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  isActive: Scalars['Boolean']['input'];
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type CreateReviewModerationTagInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  isActive: Scalars['Boolean']['input'];
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type CreateReviewQuestionInput = {
  allowComment: Scalars['Boolean']['input'];
  helpText?: InputMaybe<Scalars['String']['input']>;
  inputType: ReviewQuestionInputType;
  isRequired: Scalars['Boolean']['input'];
  label: Scalars['String']['input'];
  locale: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type CreateRoleInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  scopes: Array<RolePermissionScopeInput>;
  slug: Scalars['String']['input'];
};

export type CreateShippingMethodInput = {
  allowedCountries?: InputMaybe<Array<Scalars['String']['input']>>;
  calcType: ShippingCalcType;
  code: Scalars['String']['input'];
  countryMode?: InputMaybe<Scalars['String']['input']>;
  flatCents?: InputMaybe<Scalars['Int']['input']>;
  freeAboveCents?: InputMaybe<Scalars['Int']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  zoneId: Scalars['ID']['input'];
};

export type CreateShippingZoneInput = {
  channelId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
  countries?: InputMaybe<Array<Scalars['String']['input']>>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type CreateTaxRuleInput = {
  channelId: Scalars['ID']['input'];
  conditions?: InputMaybe<Array<TaxConditionInput>>;
  effects?: InputMaybe<Array<TaxEffectInput>>;
  exclusivityGroup?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name: Scalars['String']['input'];
  priority?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type CreateUrlRewriteInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  entityId: Scalars['ID']['input'];
  entityType: UrlEntityType;
  redirectType: Scalars['Int']['input'];
  requestPath: Scalars['String']['input'];
  targetPath?: InputMaybe<Scalars['String']['input']>;
};

export type CreateUrlRewriteRedirectInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  entityId: Scalars['ID']['input'];
  entityType: Scalars['String']['input'];
  fromPath: Scalars['String']['input'];
  redirectType: Scalars['Int']['input'];
};

export type CreateWebhookInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  customHeaders?: InputMaybe<Scalars['JSON']['input']>;
  eventTypes: Array<Scalars['String']['input']>;
  failurePolicy?: InputMaybe<WebhookFailurePolicy>;
  kind: WebhookKind;
  name: Scalars['String']['input'];
  targetUrl: Scalars['String']['input'];
  timeoutMs?: InputMaybe<Scalars['Int']['input']>;
};

export type CreateWebhookJobInput = {
  alertAfterFailures?: InputMaybe<Scalars['Int']['input']>;
  body?: InputMaybe<Scalars['String']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  enabled?: InputMaybe<Scalars['Boolean']['input']>;
  headers?: InputMaybe<Array<HttpHeaderInput>>;
  maxAttempts?: InputMaybe<Scalars['Int']['input']>;
  method?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  retryBackoffSeconds?: InputMaybe<Scalars['Int']['input']>;
  schedule: Scalars['String']['input'];
  secret?: InputMaybe<Scalars['String']['input']>;
  timezone?: InputMaybe<Scalars['String']['input']>;
  url: Scalars['String']['input'];
};

export type CreateWebhookPayload = {
  __typename?: 'CreateWebhookPayload';
  rawSecret: Scalars['String']['output'];
  webhook: Webhook;
};

export type CreditMemo = {
  __typename?: 'CreditMemo';
  adjustmentFee: Scalars['Int']['output'];
  adjustmentRefund: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  itemsTotal: Scalars['Int']['output'];
  lines: Array<CreditMemoLine>;
  number: Scalars['String']['output'];
  offline: Scalars['Boolean']['output'];
  orderId: Scalars['ID']['output'];
  reason: Scalars['String']['output'];
  refundTarget: Scalars['String']['output'];
  returnId?: Maybe<Scalars['ID']['output']>;
  shippingRefund: Scalars['Int']['output'];
  total: Scalars['Int']['output'];
};

export type CreditMemoLine = {
  __typename?: 'CreditMemoLine';
  orderLineId: Scalars['ID']['output'];
  productName: Scalars['String']['output'];
  productSku: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  totalAmount: Scalars['Int']['output'];
  unitAmount: Scalars['Int']['output'];
};

export type CreditMemoLineInput = {
  orderLineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
};

export type Currency = {
  __typename?: 'Currency';
  code: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  decimals: Scalars['Int']['output'];
  exchangeRate?: Maybe<Scalars['Float']['output']>;
  isActive: Scalars['Boolean']['output'];
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  rateUpdatedAt?: Maybe<Scalars['Time']['output']>;
  sortOrder: Scalars['Int']['output'];
  symbol: Scalars['String']['output'];
  symbolPosition: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type Customer = {
  __typename?: 'Customer';
  addresses: Array<Address>;
  channelId: Scalars['ID']['output'];
  consents: Array<CustomerConsent>;
  createdAt: Scalars['Time']['output'];
  defaultBillingAddress?: Maybe<Address>;
  defaultShippingAddress?: Maybe<Address>;
  email: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  group: CustomerGroup;
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  isEmailVerified: Scalars['Boolean']['output'];
  lastName: Scalars['String']['output'];
  metadata: Array<MetadataItem>;
  note?: Maybe<Scalars['String']['output']>;
  phone?: Maybe<Scalars['String']['output']>;
  taxExempt: Scalars['Boolean']['output'];
  taxExemptionNote: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  vatId: Scalars['String']['output'];
  vatIdCheckedAt?: Maybe<Scalars['Time']['output']>;
  vatIdStatus: Scalars['String']['output'];
};

export type CustomerActivityItem = {
  __typename?: 'CustomerActivityItem';
  detailsJson: Scalars['String']['output'];
  eventName: Scalars['String']['output'];
  occurredAt: Scalars['Time']['output'];
  source: Scalars['String']['output'];
  summary: Scalars['String']['output'];
};

export type CustomerAuthPayload = {
  __typename?: 'CustomerAuthPayload';
  accessToken?: Maybe<Scalars['String']['output']>;
  customer: Customer;
  refreshToken?: Maybe<Scalars['String']['output']>;
  verificationRequired: Scalars['Boolean']['output'];
};

export type CustomerCommerce = {
  __typename?: 'CustomerCommerce';
  activeSubscriptions: Scalars['Int']['output'];
  firstOrderAt?: Maybe<Scalars['Time']['output']>;
  group: Scalars['String']['output'];
  lastOrderAt?: Maybe<Scalars['Time']['output']>;
  marketingOptedIn: Scalars['Boolean']['output'];
  memberSince: Scalars['Time']['output'];
  orders: Scalars['Int']['output'];
  returns: Scalars['Int']['output'];
  spend: Array<AttributedRevenue>;
};

export type CustomerConsent = {
  __typename?: 'CustomerConsent';
  definition: ConsentDefinition;
  isDefault: Scalars['Boolean']['output'];
  kind: Scalars['String']['output'];
  optedIn: Scalars['Boolean']['output'];
  source: Scalars['String']['output'];
  updatedAt?: Maybe<Scalars['Time']['output']>;
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

export type CustomerGroup = {
  __typename?: 'CustomerGroup';
  createdAt: Scalars['Time']['output'];
  description: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isDefault: Scalars['Boolean']['output'];
  memberCount: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type CustomerLoginInput = {
  email: Scalars['String']['input'];
  password: Scalars['String']['input'];
};

export type CustomerPage = {
  __typename?: 'CustomerPage';
  items: Array<Customer>;
  totalCount: Scalars['Int']['output'];
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

export type CustomerRegisterInput = {
  email: Scalars['String']['input'];
  firstName: Scalars['String']['input'];
  lastName: Scalars['String']['input'];
  marketingOptIn?: InputMaybe<Scalars['Boolean']['input']>;
  password: Scalars['String']['input'];
  referralCode?: InputMaybe<Scalars['String']['input']>;
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
  email: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  joinedAt: Scalars['Time']['output'];
  lastName: Scalars['String']['output'];
};

export type CustomerSignals = {
  __typename?: 'CustomerSignals';
  consents: Array<SignalConsent>;
  email: SignalEmail;
  flows: Array<SignalFlowRun>;
  segments: Array<SignalSegment>;
};

export type CustomerSubscription = {
  __typename?: 'CustomerSubscription';
  canceledAt?: Maybe<Scalars['Time']['output']>;
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  currentPeriodEnd: Scalars['Time']['output'];
  currentPeriodStart: Scalars['Time']['output'];
  customerId?: Maybe<Scalars['ID']['output']>;
  id: Scalars['ID']['output'];
  intervalCount: Scalars['Int']['output'];
  intervalUnit: SubscriptionInterval;
  invoices: Array<SubscriptionInvoice>;
  nextBillingAt: Scalars['Time']['output'];
  orderId?: Maybe<Scalars['ID']['output']>;
  paymentMethodId?: Maybe<Scalars['ID']['output']>;
  price: Scalars['Int']['output'];
  productId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
  sku: Scalars['String']['output'];
  status: SubscriptionStatus;
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
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

/** A column in a transfer file. The admin UI renders its reference table straight off these, so adding one needs no frontend work */
export type DataField = {
  __typename?: 'DataField';
  description: Scalars['String']['output'];
  enum: Array<Scalars['String']['output']>;
  example: Scalars['String']['output'];
  /** Part of the natural key rows upsert on */
  identifier: Scalars['Boolean']['output'];
  label: Scalars['String']['output'];
  name: Scalars['String']['output'];
  /** Exported for context, ignored on import */
  readOnly: Scalars['Boolean']['output'];
  /** Required to CREATE a record - an update may leave it out */
  required: Scalars['Boolean']['output'];
  type: Scalars['String']['output'];
};

export type DataFormat = {
  __typename?: 'DataFormat';
  extension: Scalars['String']['output'];
  format: Scalars['String']['output'];
  label: Scalars['String']['output'];
};

export type DataJob = {
  __typename?: 'DataJob';
  adminId?: Maybe<Scalars['ID']['output']>;
  attempts: Scalars['Int']['output'];
  byteSize: Scalars['Int']['output'];
  channelId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  createdRows: Scalars['Int']['output'];
  direction: Scalars['String']['output'];
  dryRun: Scalars['Boolean']['output'];
  errorSummary: Scalars['String']['output'];
  /** More errors than were stored - the report holds the first batch */
  errorsTruncated: Scalars['Boolean']['output'];
  expiresAt?: Maybe<Scalars['Time']['output']>;
  failedRows: Scalars['Int']['output'];
  filename: Scalars['String']['output'];
  finishedAt?: Maybe<Scalars['Time']['output']>;
  format: Scalars['String']['output'];
  /** False once the retention sweep has removed the file - the row outlives it */
  hasArtifact: Scalars['Boolean']['output'];
  id: Scalars['ID']['output'];
  options: Scalars['JSON']['output'];
  processedRows: Scalars['Int']['output'];
  resource: Scalars['String']['output'];
  resourceLabel: Scalars['String']['output'];
  skippedRows: Scalars['Int']['output'];
  startedAt?: Maybe<Scalars['Time']['output']>;
  status: Scalars['String']['output'];
  totalRows: Scalars['Int']['output'];
  updatedRows: Scalars['Int']['output'];
  warnings: Array<Scalars['String']['output']>;
};

export type DataJobError = {
  __typename?: 'DataJobError';
  line: Scalars['Int']['output'];
  message: Scalars['String']['output'];
  /** The row's natural key, so an error is findable in the source file */
  ref: Scalars['String']['output'];
};

export type DataJobErrorList = {
  __typename?: 'DataJobErrorList';
  items: Array<DataJobError>;
  totalCount: Scalars['Int']['output'];
};

export type DataJobList = {
  __typename?: 'DataJobList';
  items: Array<DataJob>;
  totalCount: Scalars['Int']['output'];
};

/** A per-run knob. `appliesTo` is import, export, or empty for both */
export type DataOption = {
  __typename?: 'DataOption';
  appliesTo: Scalars['String']['output'];
  choices: Array<DataOptionChoice>;
  default: Scalars['String']['output'];
  description: Scalars['String']['output'];
  label: Scalars['String']['output'];
  name: Scalars['String']['output'];
  required: Scalars['Boolean']['output'];
  type: Scalars['String']['output'];
};

export type DataOptionChoice = {
  __typename?: 'DataOptionChoice';
  label: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type DataOptionValue = {
  name: Scalars['String']['input'];
  value: Scalars['String']['input'];
};

/** One importable or exportable thing. `canImport` already accounts for the caller's permissions */
export type DataResource = {
  __typename?: 'DataResource';
  canExport: Scalars['Boolean']['output'];
  canImport: Scalars['Boolean']['output'];
  description: Scalars['String']['output'];
  /** Column prefixes the file may carry beyond the declared fields, e.g. attr: for product attributes */
  dynamicPrefixes: Array<Scalars['String']['output']>;
  fields: Array<DataField>;
  group: Scalars['String']['output'];
  importPermission: Scalars['String']['output'];
  label: Scalars['String']['output'];
  options: Array<DataOption>;
  slug: Scalars['String']['output'];
  viewPermission: Scalars['String']['output'];
};

export type DefaultValueUpdate = {
  clear?: InputMaybe<Scalars['Boolean']['input']>;
  value?: InputMaybe<Scalars['JSON']['input']>;
};

export type DraftOrderLineInput = {
  productSku: Scalars['String']['input'];
  quantity: Scalars['Float']['input'];
  unitPriceCents?: InputMaybe<Scalars['Int']['input']>;
};

export type EditAddLineInput = {
  productSku: Scalars['String']['input'];
  quantity: Scalars['Float']['input'];
};

export type EditLineQtyInput = {
  orderLineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
};

export type EditOrderAddressInput = {
  billingAddress?: InputMaybe<AddressInput>;
  orderId: Scalars['ID']['input'];
  shippingAddress?: InputMaybe<AddressInput>;
};

export type EditOrderInput = {
  addLines?: InputMaybe<Array<EditAddLineInput>>;
  orderId: Scalars['ID']['input'];
  removeLines?: InputMaybe<Array<Scalars['ID']['input']>>;
  updateLines?: InputMaybe<Array<EditLineQtyInput>>;
};

export type EmailBlockSnippet = {
  __typename?: 'EmailBlockSnippet';
  blocks: Scalars['JSON']['output'];
  channelId: Scalars['ID']['output'];
  css: Scalars['String']['output'];
  fields: Scalars['JSON']['output'];
  htmlTemplate: Scalars['String']['output'];
  icon: Scalars['String']['output'];
  name: Scalars['String']['output'];
  scripted: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  textTemplate: Scalars['String']['output'];
};

export type EmailCampaign = {
  __typename?: 'EmailCampaign';
  abEnabled: Scalars['Boolean']['output'];
  abSubjectB: Scalars['String']['output'];
  abTestMinutes: Scalars['Int']['output'];
  abTestPercent: Scalars['Int']['output'];
  abTestStartedAt?: Maybe<Scalars['Time']['output']>;
  abWinner: Scalars['String']['output'];
  abWinnerMetric: Scalars['String']['output'];
  audienceId?: Maybe<Scalars['ID']['output']>;
  audienceKind: Scalars['String']['output'];
  blocks: Scalars['JSON']['output'];
  channelId: Scalars['ID']['output'];
  clickedCount: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  deletedAt?: Maybe<Scalars['Time']['output']>;
  emailId?: Maybe<Scalars['ID']['output']>;
  failedCount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  openedCount: Scalars['Int']['output'];
  scheduledFor?: Maybe<Scalars['Time']['output']>;
  sentAt?: Maybe<Scalars['Time']['output']>;
  sentCount: Scalars['Int']['output'];
  skippedCount: Scalars['Int']['output'];
  status: Scalars['String']['output'];
  styles: Scalars['JSON']['output'];
  subject: Scalars['String']['output'];
  totalRecipients: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type EmailGroup = {
  __typename?: 'EmailGroup';
  emailCount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  isSystem: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type EmailPreview = {
  __typename?: 'EmailPreview';
  html: Scalars['String']['output'];
  subject: Scalars['String']['output'];
};

export type EmailRevision = {
  __typename?: 'EmailRevision';
  blocks: Scalars['JSON']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  preheader: Scalars['String']['output'];
  styles: Scalars['JSON']['output'];
  subject: Scalars['String']['output'];
};

export type EmailSuppression = {
  __typename?: 'EmailSuppression';
  createdAt: Scalars['Time']['output'];
  detail: Scalars['String']['output'];
  email: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  reason: Scalars['String']['output'];
  softBounces: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type EmailSuppressionPage = {
  __typename?: 'EmailSuppressionPage';
  items: Array<EmailSuppression>;
  totalCount: Scalars['Int']['output'];
};

export type EmailWarning = {
  __typename?: 'EmailWarning';
  message: Scalars['String']['output'];
  severity: Scalars['String']['output'];
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

export type FacetOverride = {
  __typename?: 'FacetOverride';
  attributeId: Scalars['ID']['output'];
  attributeSlug: Scalars['String']['output'];
  hidden: Scalars['Boolean']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type FacetOverrideInput = {
  attributeId: Scalars['ID']['input'];
  hidden?: Scalars['Boolean']['input'];
};

export type Flow = {
  __typename?: 'Flow';
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  deletedAt?: Maybe<Scalars['Time']['output']>;
  graph: Scalars['JSON']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  status: Scalars['String']['output'];
  triggerConfig: Scalars['JSON']['output'];
  triggerType: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type Fulfillment = {
  __typename?: 'Fulfillment';
  carrier: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  insuredValueCents: Scalars['Int']['output'];
  labels: Array<ShippingLabel>;
  lines: Array<FulfillmentLine>;
  sequence: Scalars['Int']['output'];
  shippedAt?: Maybe<Scalars['Time']['output']>;
  status: Scalars['String']['output'];
  trackingNumber: Scalars['String']['output'];
};

export type FulfillmentLine = {
  __typename?: 'FulfillmentLine';
  id: Scalars['ID']['output'];
  orderLineId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
};

export type FulfillmentLineInput = {
  orderLineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
};

export type GenerateConfigurableChildInput = {
  attributeValueIds: Array<Scalars['ID']['input']>;
  defaultCurrencyCode?: InputMaybe<Scalars['String']['input']>;
  defaultPrice?: InputMaybe<Scalars['Int']['input']>;
  locale?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  sku?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type GiftCard = {
  __typename?: 'GiftCard';
  balance: Scalars['Int']['output'];
  code: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  expiresAt?: Maybe<Scalars['Time']['output']>;
  id: Scalars['ID']['output'];
  initialBalance: Scalars['Int']['output'];
  issuedOrderId?: Maybe<Scalars['ID']['output']>;
  recipientEmail: Scalars['String']['output'];
  status: Scalars['String']['output'];
  transactions: Array<GiftCardTransaction>;
};

export type GiftCardTransaction = {
  __typename?: 'GiftCardTransaction';
  amount: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  note: Scalars['String']['output'];
  orderId?: Maybe<Scalars['ID']['output']>;
  reason: Scalars['String']['output'];
};

export type GroupedMember = {
  __typename?: 'GroupedMember';
  createdAt: Scalars['Time']['output'];
  defaultQuantity: Scalars['Float']['output'];
  id: Scalars['ID']['output'];
  member: Product;
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type GroupedMembership = {
  __typename?: 'GroupedMembership';
  defaultQuantity: Scalars['Float']['output'];
  groupedProduct: Product;
  siblings: Array<GroupedMember>;
  sortOrder: Scalars['Int']['output'];
};

export type HttpHeader = {
  __typename?: 'HTTPHeader';
  name: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type HttpHeaderInput = {
  name: Scalars['String']['input'];
  value: Scalars['String']['input'];
};

export type HealthCheck = {
  __typename?: 'HealthCheck';
  detail: Scalars['String']['output'];
  hint?: Maybe<Scalars['String']['output']>;
  key: Scalars['String']['output'];
  label: Scalars['String']['output'];
  status: HealthStatus;
};

export type HealthGroup = {
  __typename?: 'HealthGroup';
  checks: Array<HealthCheck>;
  key: Scalars['String']['output'];
  label: Scalars['String']['output'];
};

export type HealthStatus =
  | 'BAD'
  | 'OFF'
  | 'OK'
  | 'WARN';

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

export type Integration = {
  __typename?: 'Integration';
  channelId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  description: Scalars['String']['output'];
  expiresAt?: Maybe<Scalars['Time']['output']>;
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  permissions: Array<Permission>;
};

export type InventoryItem = {
  __typename?: 'InventoryItem';
  backorderable: Scalars['Boolean']['output'];
  id: Scalars['ID']['output'];
  lastSyncedAt?: Maybe<Scalars['Time']['output']>;
  lastSyncedSource?: Maybe<Scalars['String']['output']>;
  levels: Array<InventoryLevel>;
  lowStockThreshold: Scalars['Float']['output'];
  product: Product;
  quantity: Scalars['Float']['output'];
  tracked: Scalars['Boolean']['output'];
};

export type InventoryLevel = {
  __typename?: 'InventoryLevel';
  available: Scalars['Float']['output'];
  location: InventoryLocation;
  quantity: Scalars['Float']['output'];
  reservedQuantity: Scalars['Float']['output'];
};

export type InventoryLocation = {
  __typename?: 'InventoryLocation';
  city: Scalars['String']['output'];
  contactName: Scalars['String']['output'];
  country?: Maybe<Country>;
  countryCode: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  email: Scalars['String']['output'];
  externalReference: Scalars['String']['output'];
  hours: Scalars['JSON']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  latitude?: Maybe<Scalars['Float']['output']>;
  line1: Scalars['String']['output'];
  line2: Scalars['String']['output'];
  longitude?: Maybe<Scalars['Float']['output']>;
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  phone: Scalars['String']['output'];
  pickupEnabled: Scalars['Boolean']['output'];
  pickupInstructions: Scalars['String']['output'];
  postalCode: Scalars['String']['output'];
  region: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  timezone: Scalars['String']['output'];
  type: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type InventoryMode =
  | 'FLAT'
  | 'MULTI_LOCATION';

export type InventoryReservation = {
  __typename?: 'InventoryReservation';
  adminId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  inventoryItem: InventoryItem;
  location: InventoryLocation;
  note: Scalars['String']['output'];
  orderId?: Maybe<Scalars['ID']['output']>;
  orderLineId?: Maybe<Scalars['ID']['output']>;
  quantityDelta: Scalars['Float']['output'];
  reason: ReservationReason;
};

export type IssueCreditMemoInput = {
  adjustmentFee?: InputMaybe<Scalars['Int']['input']>;
  adjustmentRefund?: InputMaybe<Scalars['Int']['input']>;
  lines?: InputMaybe<Array<CreditMemoLineInput>>;
  orderId: Scalars['ID']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
  refundTarget?: InputMaybe<Scalars['String']['input']>;
  shippingRefund?: InputMaybe<Scalars['Int']['input']>;
};

export type KitComponent = {
  __typename?: 'KitComponent';
  component: Product;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
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
  customerEmail?: Maybe<Scalars['String']['output']>;
  customerId?: Maybe<Scalars['ID']['output']>;
  customerName?: Maybe<Scalars['String']['output']>;
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

export type LibraryEmail = {
  __typename?: 'LibraryEmail';
  blocks: Scalars['JSON']['output'];
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  groupId?: Maybe<Scalars['ID']['output']>;
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  preheader: Scalars['String']['output'];
  slot: Scalars['String']['output'];
  styles: Scalars['JSON']['output'];
  subject: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type LinkClicks = {
  __typename?: 'LinkClicks';
  clicks: Scalars['Int']['output'];
  url: Scalars['String']['output'];
};

export type LinkEdge = {
  __typename?: 'LinkEdge';
  boilerplate: Scalars['Boolean']['output'];
  from: Scalars['String']['output'];
  nofollow: Scalars['Boolean']['output'];
  to: Scalars['String']['output'];
};

export type LinkGraph = {
  __typename?: 'LinkGraph';
  crawl: SiteCrawl;
  edges: Array<LinkEdge>;
  nodes: Array<LinkNode>;
  stats: LinkGraphStats;
};

export type LinkGraphStats = {
  __typename?: 'LinkGraphStats';
  boilerplate: Scalars['Int']['output'];
  broken: Scalars['Int']['output'];
  complete: Scalars['Boolean']['output'];
  contextual: Scalars['Int']['output'];
  crawled: Scalars['Int']['output'];
  discovered: Scalars['Int']['output'];
  dropped: Scalars['Int']['output'];
  maxDepth: Scalars['Int']['output'];
  noindexed: Scalars['Int']['output'];
  orphans: Scalars['Int']['output'];
  redirected: Scalars['Int']['output'];
  returnedNodes: Scalars['Int']['output'];
  totalEdges: Scalars['Int']['output'];
  totalNodes: Scalars['Int']['output'];
  unseen: Scalars['Int']['output'];
};

export type LinkNode = {
  __typename?: 'LinkNode';
  depth: Scalars['Int']['output'];
  entityId?: Maybe<Scalars['ID']['output']>;
  inLinks: Scalars['Int']['output'];
  isHomepage: Scalars['Boolean']['output'];
  key: Scalars['String']['output'];
  kind: CrawlPageKind;
  label: Scalars['String']['output'];
  noindex: Scalars['Boolean']['output'];
  outLinks: Scalars['Int']['output'];
  path: Scalars['String']['output'];
  state: LinkNodeState;
  statusCode: Scalars['Int']['output'];
  type: LinkNodeType;
};

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

export type Linkable = {
  __typename?: 'Linkable';
  entityId: Scalars['ID']['output'];
  entityType: UrlEntityType;
  label: Scalars['String']['output'];
  path: Scalars['String']['output'];
};

export type Locale = {
  __typename?: 'Locale';
  code: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  dateFormat: Scalars['String']['output'];
  decimalSeparator: Scalars['String']['output'];
  defaultCountryCode: Scalars['String']['output'];
  description: Scalars['String']['output'];
  emailFromNameOverride: Scalars['String']['output'];
  firstDayOfWeek: Scalars['Int']['output'];
  flagEmoji: Scalars['String']['output'];
  hreflangValue: Scalars['String']['output'];
  isActive: Scalars['Boolean']['output'];
  isRTL: Scalars['Boolean']['output'];
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  nativeName: Scalars['String']['output'];
  pluralRule: Scalars['String']['output'];
  script: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  thousandsSeparator: Scalars['String']['output'];
  timeFormat: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type LocationAssignmentInput = {
  locationId: Scalars['ID']['input'];
  priority: Scalars['Int']['input'];
};

export type LowStockLevel = {
  __typename?: 'LowStockLevel';
  backorderable: Scalars['Boolean']['output'];
  inventoryItemId: Scalars['ID']['output'];
  locationId?: Maybe<Scalars['ID']['output']>;
  locationName?: Maybe<Scalars['String']['output']>;
  locationSlug?: Maybe<Scalars['String']['output']>;
  lowStockThreshold: Scalars['Float']['output'];
  productId: Scalars['ID']['output'];
  productSku: Scalars['String']['output'];
  productSlug: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  quantityDecimals: Scalars['Int']['output'];
  quantityStep: Scalars['Float']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type LoyaltyEntry = {
  __typename?: 'LoyaltyEntry';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  orderId?: Maybe<Scalars['ID']['output']>;
  points: Scalars['Int']['output'];
  reason: Scalars['String']['output'];
};

export type LoyaltySummary = {
  __typename?: 'LoyaltySummary';
  balancePoints: Scalars['Int']['output'];
  entries: Array<LoyaltyEntry>;
};

export type MasqueradeSession = {
  __typename?: 'MasqueradeSession';
  accessToken: Scalars['String']['output'];
  expiresInSeconds: Scalars['Int']['output'];
  storefrontPath: Scalars['String']['output'];
  storefrontUrl: Scalars['String']['output'];
};

export type MediaAsset = {
  __typename?: 'MediaAsset';
  altText: Scalars['String']['output'];
  caption: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  directoryPath: Scalars['String']['output'];
  filename: Scalars['String']['output'];
  focalX?: Maybe<Scalars['Float']['output']>;
  focalY?: Maybe<Scalars['Float']['output']>;
  height?: Maybe<Scalars['Int']['output']>;
  id: Scalars['ID']['output'];
  mimeGroup: MediaMimeGroup;
  mimeType: Scalars['String']['output'];
  notes: Scalars['String']['output'];
  path: Scalars['String']['output'];
  sizeBytes: Scalars['Int']['output'];
  status: MediaAssetStatus;
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  url: Scalars['String']['output'];
  width?: Maybe<Scalars['Int']['output']>;
};

export type MediaAssetSearchResult = {
  __typename?: 'MediaAssetSearchResult';
  items: Array<MediaAsset>;
  totalCount: Scalars['Int']['output'];
};

export type MediaAssetStatus =
  | 'PENDING'
  | 'READY';

export type MediaDirectory = {
  __typename?: 'MediaDirectory';
  assets: Array<MediaAsset>;
  directories: Array<MediaDirectoryChild>;
  name: Scalars['String']['output'];
  path: Scalars['String']['output'];
  untrackedFiles: Array<UntrackedMediaFile>;
};

export type MediaDirectoryChild = {
  __typename?: 'MediaDirectoryChild';
  name: Scalars['String']['output'];
  path: Scalars['String']['output'];
};

export type MediaMimeGroup =
  | 'DOCUMENT'
  | 'IMAGE';

export type Menu = {
  __typename?: 'Menu';
  channelIds: Array<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  itemCount: Scalars['Int']['output'];
  items: Array<MenuItem>;
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type MenuItem = {
  __typename?: 'MenuItem';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  label: Scalars['String']['output'];
  menuId: Scalars['ID']['output'];
  parentId?: Maybe<Scalars['ID']['output']>;
  sortOrder: Scalars['Int']['output'];
  targetId?: Maybe<Scalars['ID']['output']>;
  targetType?: Maybe<MenuItemTargetType>;
  translations: Array<MenuItemTranslation>;
  updatedAt: Scalars['Time']['output'];
  url?: Maybe<Scalars['String']['output']>;
};

export type MenuItemTargetType =
  | 'BLOG_AUTHOR'
  | 'BLOG_CATEGORY'
  | 'BLOG_POST'
  | 'BLOG_TAG'
  | 'CATEGORY'
  | 'CMS_PAGE'
  | 'COLLECTION'
  | 'PRODUCT';

export type MenuItemTranslation = {
  __typename?: 'MenuItemTranslation';
  createdAt: Scalars['Time']['output'];
  label: Scalars['String']['output'];
  locale: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type MetadataInput = {
  key: Scalars['String']['input'];
  value: Scalars['String']['input'];
};

export type MetadataItem = {
  __typename?: 'MetadataItem';
  key: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type MintBlogPostPreviewTokenInput = {
  locale?: InputMaybe<Scalars['String']['input']>;
  postId: Scalars['ID']['input'];
};

export type MintCmsPagePreviewTokenInput = {
  locale?: InputMaybe<Scalars['String']['input']>;
  pageId: Scalars['ID']['input'];
};

export type Mutation = {
  __typename?: 'Mutation';
  addAttributeValue: AttributeValue;
  addBundleOption: Product;
  addBundleSelection: Product;
  addCMSAttributeValue: CmsAttributeValue;
  addCMSPageComment: CmsPageComment;
  addGroupedMember: Product;
  addKitComponent: Product;
  addProductChild: Product;
  addProductDownload: Product;
  addProductOption: Product;
  addProductOptionValue: Product;
  addProductToCategory: Product;
  addProductToCollection: Collection;
  addSearchSynonym: SearchSynonym;
  adjustGiftCard: GiftCard;
  adjustInventoryLevel: InventoryLevel;
  adjustLoyaltyPoints: LoyaltySummary;
  adjustStoreCredit: StoreCreditSummary;
  adminAddAddress: Address;
  adminAddCartLine: AdminCart;
  adminClearCart: AdminCart;
  adminCreateCart: AdminCart;
  adminCreateCustomer: Customer;
  adminDeleteAddress: Scalars['Boolean']['output'];
  adminDuplicateCart: AdminCart;
  adminLogin: AdminAuthPayload;
  adminRemoveCartLine: AdminCart;
  adminSetCustomerConsent: CustomerConsent;
  adminTotpDisable: Scalars['Boolean']['output'];
  adminTotpEnable: Array<Scalars['String']['output']>;
  adminTotpSetup: AdminTotpSetup;
  adminUpdateCartLine: AdminCart;
  adminUpdateCustomer: Customer;
  anonymizeCustomer: Scalars['Boolean']['output'];
  answerProductQuestion: AdminProductQuestion;
  approveCMSPage: CmsPage;
  approveReturn: OrderReturn;
  approveReview: Review;
  archiveBlogPost: BlogPost;
  assignAttribute: ProductType;
  assignCMSAttribute: CmsPageType;
  assignCustomersToSegment: Scalars['Boolean']['output'];
  assignEmailSlot: Scalars['Boolean']['output'];
  assignReviewQuestion: Scalars['Boolean']['output'];
  bulkDeleteCMSPages: BulkCmsPageResult;
  bulkPublishCMSPages: BulkCmsPageResult;
  bulkTagCMSPages: BulkCmsPageResult;
  /** Only a job that has not started yet - a running one is stopped by its runner noticing */
  cancelDataJob: DataJob;
  cancelEmailCampaign: EmailCampaign;
  cancelOrder: Order;
  cancelSiteCrawl: Scalars['Boolean']['output'];
  cancelSubscription: CustomerSubscription;
  capturePayment: Order;
  changePassword: Scalars['Boolean']['output'];
  claimCMSPage: CmsPage;
  clearCategoryCover: Category;
  clearCategoryListing: Category;
  clearCollectionCover: Collection;
  clearProductAttribute: Product;
  clearProductListing: Scalars['Boolean']['output'];
  clearProductMediaRole: Product;
  clearSegmentMembership: Scalars['Boolean']['output'];
  clearSubscriptionTerms: Product;
  clearWebhookDeadLetters: Scalars['Int']['output'];
  clearWebhookDeliveries: Scalars['Int']['output'];
  cloneCMSPage: CmsPage;
  confirmCategoryCoverUpload: Category;
  confirmCollectionCoverUpload: Collection;
  confirmMediaAssetUpload: MediaAsset;
  confirmOrder: Order;
  confirmProductMediaUpload: ProductMedia;
  createAdmin: AdminUser;
  createAttribute: Attribute;
  createAttributeGroup: ProductType;
  createBlogAuthor: BlogAuthor;
  createBlogCategory: BlogCategory;
  createBlogPost: BlogPost;
  createBlogTag: BlogTag;
  createCMSAttribute: CmsAttribute;
  createCMSAttributeGroup: CmsAttributeGroup;
  createCMSBlockType: CmsBlockType;
  createCMSPage: CmsPage;
  createCMSPageType: CmsPageType;
  createCMSSnippet: CmsSnippet;
  createCMSTag: CmsTag;
  createCategory: Category;
  createChannel: Channel;
  createCollection: Collection;
  createCountry: Country;
  createCurrency: Currency;
  createCustomerGroup: CustomerGroup;
  createCustomerSegment: CustomerSegment;
  createDraftOrder: Order;
  createEmailCampaign: EmailCampaign;
  createEmailGroup: EmailGroup;
  createEventDestination: EventDestination;
  createExperiment: Experiment;
  createFlow: Flow;
  createFulfillment: Order;
  createIntegration: CreateIntegrationPayload;
  createInventoryLocation: InventoryLocation;
  createLocale: Locale;
  createMediaDirectory: MediaDirectoryChild;
  createMenu: Menu;
  createMenuItem: MenuItem;
  createProduct: Product;
  createProductType: ProductType;
  createPromotionRule: PromotionRule;
  createRemoteProvider: CreateRemoteProviderPayload;
  createReviewCategory: ReviewCategory;
  createReviewModerationTag: ReviewModerationTag;
  createReviewQuestion: ReviewQuestion;
  createRole: Role;
  createShippingMethod: ShippingMethod;
  createShippingZone: ShippingZone;
  createSignupForm: SignupForm;
  createTaxRule: TaxRule;
  createUrlRewrite: UrlRewrite;
  createUrlRewriteRedirect: UrlRewrite;
  createWebForm: WebForm;
  createWebhook: CreateWebhookPayload;
  createWebhookJob: ScheduledJob;
  deleteAdmin: Scalars['Boolean']['output'];
  deleteAttribute: Scalars['Boolean']['output'];
  deleteAttributeGroup: ProductType;
  deleteAttributeValue: Scalars['Boolean']['output'];
  deleteAttributeValueChannelSlug: Scalars['Boolean']['output'];
  deleteBlogAuthor: Scalars['Boolean']['output'];
  deleteBlogAuthorContent: Scalars['Boolean']['output'];
  deleteBlogCategory: Scalars['Boolean']['output'];
  deleteBlogCategoryContent: Scalars['Boolean']['output'];
  deleteBlogPost: Scalars['Boolean']['output'];
  deleteBlogPostChannelListing: BlogPost;
  deleteBlogPostContent: Scalars['Boolean']['output'];
  deleteBlogTag: Scalars['Boolean']['output'];
  deleteBlogTagContent: Scalars['Boolean']['output'];
  deleteBundleOption: Product;
  deleteBundleOptionContent: Product;
  deleteBundleSelection: Product;
  deleteCMSAttribute: Scalars['Boolean']['output'];
  deleteCMSAttributeGroup: Scalars['Boolean']['output'];
  deleteCMSAttributeValue: Scalars['Boolean']['output'];
  deleteCMSBlockType: Scalars['Boolean']['output'];
  deleteCMSPage: Scalars['Boolean']['output'];
  deleteCMSPageChannelListing: CmsPage;
  deleteCMSPageComment: Scalars['Boolean']['output'];
  deleteCMSPageType: Scalars['Boolean']['output'];
  deleteCMSSnippet: Scalars['Boolean']['output'];
  deleteCMSTag: Scalars['Boolean']['output'];
  deleteCategory: Scalars['Boolean']['output'];
  deleteCategoryContent: Scalars['Boolean']['output'];
  deleteChannel: Scalars['Boolean']['output'];
  deleteChatConversation: Scalars['Boolean']['output'];
  deleteCollection: Scalars['Boolean']['output'];
  deleteCollectionContent: Scalars['Boolean']['output'];
  deleteCountry: Scalars['Boolean']['output'];
  deleteCurrency: Scalars['Boolean']['output'];
  deleteCustomer: Scalars['Boolean']['output'];
  deleteCustomerGroup: Scalars['Boolean']['output'];
  deleteCustomerSegment: Scalars['Boolean']['output'];
  deleteEmailBlock: Scalars['Boolean']['output'];
  deleteEmailCampaign: Scalars['Boolean']['output'];
  deleteEmailGroup: Scalars['Boolean']['output'];
  deleteEventDestination: Scalars['Boolean']['output'];
  deleteExperiment: Scalars['Boolean']['output'];
  deleteFlow: Scalars['Boolean']['output'];
  deleteIntegration: Scalars['Boolean']['output'];
  deleteInventoryLocation: Scalars['Boolean']['output'];
  deleteLibraryEmail: Scalars['Boolean']['output'];
  deleteLocale: Scalars['Boolean']['output'];
  deleteMediaAsset: Scalars['Boolean']['output'];
  deleteMediaDirectory: Scalars['Boolean']['output'];
  deleteMenu: Scalars['Boolean']['output'];
  deleteMenuItem: Scalars['Boolean']['output'];
  deleteMenuItemTranslation: Scalars['Boolean']['output'];
  deleteProduct: Scalars['Boolean']['output'];
  deleteProductAttributeSlug: Scalars['Boolean']['output'];
  deleteProductContent: Scalars['Boolean']['output'];
  deleteProductMedia: Scalars['Boolean']['output'];
  deleteProductMediaBulk: Scalars['Int']['output'];
  deleteProductOption: Product;
  deleteProductOptionContent: Product;
  deleteProductOptionValue: Product;
  deleteProductOptionValueContent: Product;
  deleteProductQuestion: Scalars['Boolean']['output'];
  deleteProductType: Scalars['Boolean']['output'];
  deletePromotionCode: Scalars['Boolean']['output'];
  deletePromotionRule: Scalars['Boolean']['output'];
  deletePromotionRuleContent: Scalars['Boolean']['output'];
  deleteRemoteProvider: Scalars['Boolean']['output'];
  deleteReviewCategory: Scalars['Boolean']['output'];
  deleteReviewModerationTag: Scalars['Boolean']['output'];
  deleteReviewQuestion: Scalars['Boolean']['output'];
  deleteRole: Scalars['Boolean']['output'];
  deleteScheduledJob: Scalars['Boolean']['output'];
  deleteSearchBoost: Scalars['Boolean']['output'];
  deleteSearchPin: Scalars['Boolean']['output'];
  deleteSearchRedirect: Scalars['Boolean']['output'];
  deleteSearchSynonym: Scalars['Boolean']['output'];
  deleteShippingBox: Scalars['Boolean']['output'];
  deleteShippingMethod: Scalars['Boolean']['output'];
  deleteShippingZone: Scalars['Boolean']['output'];
  deleteSignupForm: Scalars['Boolean']['output'];
  deleteTaxRule: Scalars['Boolean']['output'];
  deleteUrlRewrite: Scalars['Boolean']['output'];
  deleteUrlRewritesForEntity: Scalars['Int']['output'];
  deleteWebForm: Scalars['Boolean']['output'];
  deleteWebhook: Scalars['Boolean']['output'];
  devImpersonate: AdminAuthPayload;
  disableGiftCard: GiftCard;
  draftReviewReply: Scalars['String']['output'];
  duplicateProductOption: Product;
  editOrder: Order;
  editOrderAddress: Order;
  enqueueRebuild: Scalars['Int']['output'];
  fulfillOrder: Order;
  generateCMSPageSeo: AiSeoSuggestion;
  generateConfigurableChildren: Product;
  generateMediaAltText: Scalars['String']['output'];
  generateProductReviewSummary: Scalars['String']['output'];
  generatePromotionCodes: Array<PromotionCode>;
  generateShippingLabel: ShippingLabel;
  holdOrder: Order;
  issueCreditMemo: CreditMemo;
  issueGiftCard: GiftCard;
  issueReturnRefund: OrderReturn;
  logout: Scalars['Boolean']['output'];
  logoutOtherDevices: Scalars['Boolean']['output'];
  markAllNotificationsRead: Scalars['Boolean']['output'];
  markNotificationRead: Scalars['Boolean']['output'];
  markProcessing: Order;
  masqueradeCustomer: MasqueradeSession;
  mintBlogPostPreviewToken: BlogPostPreviewToken;
  mintCMSPagePreviewToken: CmsPagePreviewToken;
  moveAttributeToGroup: ProductType;
  moveMediaAsset: MediaAsset;
  pauseSubscription: CustomerSubscription;
  placeDraftOrder: Order;
  publishBlogPost: BlogPost;
  publishCMSPage: CmsPage;
  purgeCache: Scalars['Boolean']['output'];
  rebuildCMSPage: Scalars['Boolean']['output'];
  rebuildCache: Scalars['Boolean']['output'];
  receiveReturn: OrderReturn;
  recheckReviewModeration: Review;
  recordOfflinePayment: Order;
  redeliverWebhookDelivery: WebhookQueueItem;
  refreshSegment: Scalars['Int']['output'];
  refreshToken: AdminAuthPayload;
  refundPayment: Order;
  regenerateIntegrationToken: RegenerateIntegrationTokenPayload;
  rejectCMSPage: CmsPage;
  rejectProductQuestion: AdminProductQuestion;
  rejectReturn: OrderReturn;
  rejectReview: Review;
  releaseOrder: Order;
  removeCustomerFromSegment: Scalars['Boolean']['output'];
  removeGroupedMember: Product;
  removeKitComponent: Product;
  removeProductChild: Product;
  removeProductDownload: Product;
  removeProductFromCategory: Product;
  removeProductFromCollection: Collection;
  renameCMSTag: CmsTag;
  renameEmailGroup: EmailGroup;
  renameMediaDirectory: MediaDirectoryChild;
  reopenCMSPage: CmsPage;
  reorderCollectionProducts: Collection;
  reorderMenuItems: Menu;
  reorderProductMedia: Array<ProductMedia>;
  reorderPromotionRules: Scalars['Boolean']['output'];
  replyToReview: Review;
  requestAdminPasswordReset: Scalars['Boolean']['output'];
  requestCategoryCoverUpload: RequestCoverUploadPayload;
  requestCollectionCoverUpload: RequestCoverUploadPayload;
  requestMediaAssetUpload: RequestMediaAssetUploadPayload;
  requestProductMediaUpload: RequestMediaUploadPayload;
  requestReturn: OrderReturn;
  resendOrderEmail: Scalars['Boolean']['output'];
  resetAdminPassword: Scalars['Boolean']['output'];
  resetConfigValue: ConfigValue;
  resolveCMSPageComment: CmsPageComment;
  restoreCMSPageRevision: CmsPage;
  restoreEmailCampaign: Scalars['Boolean']['output'];
  restoreEmailRevision: LibraryEmail;
  restoreFlow: Scalars['Boolean']['output'];
  restoreWebForm: Scalars['Boolean']['output'];
  resumeSubscription: CustomerSubscription;
  retryWebhookDeadLetter: WebhookQueueItem;
  rewriteEmailText: Scalars['String']['output'];
  rotateRemoteProviderSecret: RotateRemoteProviderSecretPayload;
  rotateWebhookSecret: RotateWebhookSecretPayload;
  runAdminAction: AdminActionResult;
  runChatAction: ChatActionResult;
  runScheduledJobNow: ScheduledJob;
  saveChatConversation: Scalars['ID']['output'];
  saveEmailBlock: EmailBlockSnippet;
  saveLibraryEmail: LibraryEmail;
  scheduleBlogPost: BlogPost;
  scheduleCMSPage: CmsPage;
  scheduleEmailCampaign: EmailCampaign;
  sendCampaignTest: Scalars['Boolean']['output'];
  setAttributeDescription: AttributeContent;
  setAttributeIndexable: CascadeAttributeIndexableResult;
  setAttributeValueChannelSlug: AttributeValueChannelSlug;
  setAttributeValueDescription: AttributeValueContent;
  setAttributeValuesIndexable: BulkAttributeValueResult;
  setBlogPostCategories: BlogPost;
  setBlogPostChannelListing: BlogPost;
  setBlogPostTags: BlogPost;
  setCMSPageChannelListing: CmsPage;
  setCMSPageExperiment: CmsPage;
  setCMSPageRelationships: CmsPage;
  setCMSPageSelections: CmsPage;
  setCMSPageTypeChannelRoot: CmsPageType;
  setCMSPageTypeDefaultBlocks: CmsPageType;
  setCategoryFacetOverrides: Category;
  setCategoryListing: Category;
  setChannelAsDefault: Channel;
  setChannelLocales: Channel;
  setChannelLocations: Channel;
  setCollectionFacetOverrides: Collection;
  setConfigValue: ConfigValue;
  setConfigurableOptions: Product;
  setCustomerActive: Customer;
  setCustomerGroup: Customer;
  setCustomerTaxStatus: Customer;
  setCustomerVatId: Customer;
  setDefaultCustomerGroup: CustomerGroup;
  setFlowStatus: Flow;
  setIntegrationActive: Integration;
  setInventoryItemBackorder: InventoryItem;
  setInventoryItemTracking: InventoryItem;
  setLocationPickup: InventoryLocation;
  setMediaFocalPoint: MediaAsset;
  setMenuStores: Menu;
  setMyPreference: Scalars['Boolean']['output'];
  setPaymentMethodCountries: PaymentMethodAdmin;
  setPaymentMethodEnabled: PaymentMethodAdmin;
  setProductAttributeLocaleValue: ProductAttributeLocaleValue;
  setProductAttributeReferences: Product;
  setProductAttributeSlug: ProductAttributeSlug;
  setProductAttributes: Product;
  setProductGroupPrices: Array<ProductGroupPrice>;
  setProductLinks: Scalars['Boolean']['output'];
  setProductListing: Product;
  setProductMediaRole: Product;
  setProductPrimaryCategory: Product;
  setProductTaxClass: Product;
  setProductTypeURLPathTemplate: ProductType;
  setProductVariantMode: Product;
  setRemoteProviderActive: RemoteProvider;
  setReviewCategories: Review;
  setReviewModerationTags: Review;
  setSegmentRules: CustomerSegment;
  setShippingCarrierCountries: ShippingCarrierAdmin;
  setShippingCarrierEnabled: ShippingCarrierAdmin;
  setSubscriptionTerms: Product;
  setWebhookActive: Webhook;
  signalsChat: SignalsChatReply;
  /** Queues an export - poll the returned job, then download it from /api/admin/data/jobs/{id}/download */
  startDataExport: DataJob;
  startSiteCrawl: SiteCrawl;
  submitCMSPageForReview: CmsPage;
  suggestEmailSubjects: Array<Scalars['String']['output']>;
  suppressEmail: Scalars['Boolean']['output'];
  synapseRefreshSegment: SynapseSegment;
  synapseSaveSegment: SynapseSegment;
  synapseSetSegmentLifecycleWindows: SynapseSegment;
  syncAudience: Scalars['Int']['output'];
  syncCanonicalUrlRewrite: UrlRewrite;
  syncProductInventory: InventoryItem;
  syncProductInventoryBatch: SyncProductInventoryBatchResult;
  translateCMSPage: CmsPage;
  unassignAttribute: ProductType;
  unassignCMSAttribute: CmsPageType;
  unassignCMSPage: CmsPage;
  unassignReviewQuestion: Scalars['Boolean']['output'];
  unpublishBlogPost: BlogPost;
  unpublishCMSPage: CmsPage;
  unsuppressEmail: Scalars['Boolean']['output'];
  updateAdmin: AdminUser;
  updateAttribute: Attribute;
  updateAttributeGroup: ProductType;
  updateAttributeValue: AttributeValue;
  updateBlogAuthor: BlogAuthor;
  updateBlogCategory: BlogCategory;
  updateBlogPost: BlogPost;
  updateBlogTag: BlogTag;
  updateBundleOption: Product;
  updateBundleSelection: Product;
  updateCMSAttribute: CmsAttribute;
  updateCMSAttributeGroup: CmsAttributeGroup;
  updateCMSAttributeValue: CmsAttributeValue;
  updateCMSBlockType: CmsBlockType;
  updateCMSPage: CmsPage;
  updateCMSPageType: CmsPageType;
  updateCMSPageTypeStructure: CmsPageType;
  updateCMSSnippet: CmsSnippet;
  updateCategory: Category;
  updateChannel: Channel;
  updateCollection: Collection;
  updateCountry: Country;
  updateCurrency: Currency;
  updateCustomerGroup: CustomerGroup;
  updateCustomerSegment: CustomerSegment;
  updateEmailCampaign: EmailCampaign;
  updateEventDestination: EventDestination;
  updateExperiment: Experiment;
  updateFlow: Flow;
  updateGroupedMember: Product;
  updateIntegration: Integration;
  updateInventoryLocation: InventoryLocation;
  updateKitComponent: Product;
  updateLocale: Locale;
  updateMediaAsset: MediaAsset;
  updateMenu: Menu;
  updateMenuItem: MenuItem;
  updateProduct: Product;
  updateProductDownload: Product;
  updateProductMedia: ProductMedia;
  updateProductOption: Product;
  updateProductOptionValue: Product;
  updateProductType: ProductType;
  updatePromotionCode: PromotionCode;
  updatePromotionRule: PromotionRule;
  updateRemoteProvider: RemoteProvider;
  updateReviewCategory: ReviewCategory;
  updateReviewModerationTag: ReviewModerationTag;
  updateReviewQuestion: ReviewQuestion;
  updateRole: Role;
  updateScheduledJob: ScheduledJob;
  updateShippingMethod: ShippingMethod;
  updateShippingZone: ShippingZone;
  updateSignupForm: SignupForm;
  updateTaxRule: TaxRule;
  updateUrlRewrite: UrlRewrite;
  updateWebForm: WebForm;
  updateWebhook: Webhook;
  updateWebhookJob: ScheduledJob;
  upsertBlogAuthorContent: BlogAuthor;
  upsertBlogCategoryContent: BlogCategory;
  upsertBlogPostContent: BlogPost;
  upsertBlogTagContent: BlogTag;
  upsertBundleOptionContent: Product;
  upsertCMSAttributeContent: CmsAttribute;
  upsertCMSAttributeValueContent: CmsAttribute;
  upsertCMSPageContent: CmsPage;
  upsertCMSPageTypeContent: CmsPageType;
  upsertCMSPageVariantContent: CmsPage;
  upsertCategoryContent: Category;
  upsertCollectionContent: Collection;
  upsertMenuItemTranslation: MenuItemTranslation;
  upsertProductContent: Product;
  upsertProductOptionContent: Product;
  upsertProductOptionValueContent: Product;
  upsertPromotionRuleContent: PromotionRuleContent;
  upsertReviewQuestionContent: ReviewQuestion;
  upsertSearchBoost: SearchBoost;
  upsertSearchPin: SearchPin;
  upsertSearchRedirect: SearchRedirect;
  upsertShippingBox: ShippingBox;
  voidPayment: Order;
};


export type MutationAddAttributeValueArgs = {
  attributeId: Scalars['ID']['input'];
  input: AttributeValueInput;
};


export type MutationAddBundleOptionArgs = {
  input: AddBundleOptionInput;
};


export type MutationAddBundleSelectionArgs = {
  input: AddBundleSelectionInput;
};


export type MutationAddCmsAttributeValueArgs = {
  attributeId: Scalars['ID']['input'];
  position?: InputMaybe<Scalars['Int']['input']>;
  slug: Scalars['String']['input'];
};


export type MutationAddCmsPageCommentArgs = {
  input: CmsPageCommentInput;
};


export type MutationAddGroupedMemberArgs = {
  input: AddGroupedMemberInput;
};


export type MutationAddKitComponentArgs = {
  input: AddKitComponentInput;
};


export type MutationAddProductChildArgs = {
  childId: Scalars['ID']['input'];
  parentId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationAddProductDownloadArgs = {
  input: AddProductDownloadInput;
};


export type MutationAddProductOptionArgs = {
  input: AddProductOptionInput;
};


export type MutationAddProductOptionValueArgs = {
  input: AddProductOptionValueInput;
};


export type MutationAddProductToCategoryArgs = {
  categoryId: Scalars['ID']['input'];
  isPrimary?: InputMaybe<Scalars['Boolean']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationAddProductToCollectionArgs = {
  collectionId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationAddSearchSynonymArgs = {
  channelId: Scalars['ID']['input'];
  terms: Scalars['String']['input'];
};


export type MutationAdjustGiftCardArgs = {
  amount: Scalars['Int']['input'];
  id: Scalars['ID']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
};


export type MutationAdjustInventoryLevelArgs = {
  input: AdjustInventoryLevelInput;
};


export type MutationAdjustLoyaltyPointsArgs = {
  customerId: Scalars['ID']['input'];
  points: Scalars['Int']['input'];
  reason: Scalars['String']['input'];
};


export type MutationAdjustStoreCreditArgs = {
  amountCents: Scalars['Int']['input'];
  customerId: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
};


export type MutationAdminAddAddressArgs = {
  customerId: Scalars['ID']['input'];
  input: AddressInput;
};


export type MutationAdminAddCartLineArgs = {
  cartId: Scalars['ID']['input'];
  options?: InputMaybe<Array<AdminCartLineOptionInput>>;
  productSku: Scalars['String']['input'];
  quantity: Scalars['Float']['input'];
};


export type MutationAdminClearCartArgs = {
  cartId: Scalars['ID']['input'];
};


export type MutationAdminCreateCartArgs = {
  channelId: Scalars['ID']['input'];
  customerId?: InputMaybe<Scalars['ID']['input']>;
};


export type MutationAdminCreateCustomerArgs = {
  input: AdminCreateCustomerInput;
};


export type MutationAdminDeleteAddressArgs = {
  id: Scalars['ID']['input'];
};


export type MutationAdminDuplicateCartArgs = {
  cartId: Scalars['ID']['input'];
  targetCustomerId?: InputMaybe<Scalars['ID']['input']>;
};


export type MutationAdminLoginArgs = {
  input: AdminLoginInput;
};


export type MutationAdminRemoveCartLineArgs = {
  cartId: Scalars['ID']['input'];
  lineId: Scalars['ID']['input'];
};


export type MutationAdminSetCustomerConsentArgs = {
  customerId: Scalars['ID']['input'];
  kind: Scalars['String']['input'];
  optedIn: Scalars['Boolean']['input'];
};


export type MutationAdminTotpDisableArgs = {
  code: Scalars['String']['input'];
};


export type MutationAdminTotpEnableArgs = {
  code: Scalars['String']['input'];
};


export type MutationAdminUpdateCartLineArgs = {
  cartId: Scalars['ID']['input'];
  lineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
};


export type MutationAdminUpdateCustomerArgs = {
  id: Scalars['ID']['input'];
  input: AdminUpdateCustomerInput;
};


export type MutationAnonymizeCustomerArgs = {
  customerId: Scalars['ID']['input'];
};


export type MutationAnswerProductQuestionArgs = {
  answer: Scalars['String']['input'];
  id: Scalars['ID']['input'];
};


export type MutationApproveCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationApproveReturnArgs = {
  id: Scalars['ID']['input'];
};


export type MutationApproveReviewArgs = {
  id: Scalars['ID']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
};


export type MutationArchiveBlogPostArgs = {
  id: Scalars['ID']['input'];
};


export type MutationAssignAttributeArgs = {
  attributeId: Scalars['ID']['input'];
  groupId?: InputMaybe<Scalars['ID']['input']>;
  productTypeId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationAssignCmsAttributeArgs = {
  input: AssignCmsAttributeInput;
};


export type MutationAssignCustomersToSegmentArgs = {
  customerIds: Array<Scalars['ID']['input']>;
  segmentId: Scalars['ID']['input'];
};


export type MutationAssignEmailSlotArgs = {
  emailId: Scalars['ID']['input'];
  slot: Scalars['String']['input'];
};


export type MutationAssignReviewQuestionArgs = {
  input: AssignReviewQuestionInput;
};


export type MutationBulkDeleteCmsPagesArgs = {
  ids: Array<Scalars['ID']['input']>;
};


export type MutationBulkPublishCmsPagesArgs = {
  ids: Array<Scalars['ID']['input']>;
  published: Scalars['Boolean']['input'];
};


export type MutationBulkTagCmsPagesArgs = {
  addTags?: InputMaybe<Array<Scalars['String']['input']>>;
  ids: Array<Scalars['ID']['input']>;
  removeTags?: InputMaybe<Array<Scalars['String']['input']>>;
};


export type MutationCancelDataJobArgs = {
  id: Scalars['ID']['input'];
};


export type MutationCancelEmailCampaignArgs = {
  id: Scalars['ID']['input'];
};


export type MutationCancelOrderArgs = {
  id: Scalars['ID']['input'];
};


export type MutationCancelSiteCrawlArgs = {
  channelId: Scalars['ID']['input'];
};


export type MutationCancelSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationCapturePaymentArgs = {
  amount?: InputMaybe<Scalars['Int']['input']>;
  orderId: Scalars['ID']['input'];
};


export type MutationChangePasswordArgs = {
  currentPassword: Scalars['String']['input'];
  newPassword: Scalars['String']['input'];
};


export type MutationClaimCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationClearCategoryCoverArgs = {
  categoryId: Scalars['ID']['input'];
};


export type MutationClearCategoryListingArgs = {
  categoryId: Scalars['ID']['input'];
  channelId: Scalars['ID']['input'];
};


export type MutationClearCollectionCoverArgs = {
  collectionId: Scalars['ID']['input'];
};


export type MutationClearProductAttributeArgs = {
  attributeId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
};


export type MutationClearProductListingArgs = {
  channelId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
};


export type MutationClearProductMediaRoleArgs = {
  productId: Scalars['ID']['input'];
  role: ProductMediaRole;
};


export type MutationClearSegmentMembershipArgs = {
  segmentId: Scalars['ID']['input'];
};


export type MutationClearSubscriptionTermsArgs = {
  productId: Scalars['ID']['input'];
};


export type MutationClearWebhookDeadLettersArgs = {
  webhookId: Scalars['ID']['input'];
};


export type MutationClearWebhookDeliveriesArgs = {
  webhookId: Scalars['ID']['input'];
};


export type MutationCloneCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationConfirmCategoryCoverUploadArgs = {
  categoryId: Scalars['ID']['input'];
  height: Scalars['Int']['input'];
  width: Scalars['Int']['input'];
};


export type MutationConfirmCollectionCoverUploadArgs = {
  collectionId: Scalars['ID']['input'];
  height: Scalars['Int']['input'];
  width: Scalars['Int']['input'];
};


export type MutationConfirmMediaAssetUploadArgs = {
  assetId: Scalars['ID']['input'];
  height?: InputMaybe<Scalars['Int']['input']>;
  width?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationConfirmOrderArgs = {
  id: Scalars['ID']['input'];
};


export type MutationConfirmProductMediaUploadArgs = {
  height: Scalars['Int']['input'];
  mediaId: Scalars['ID']['input'];
  width: Scalars['Int']['input'];
};


export type MutationCreateAdminArgs = {
  input: CreateAdminInput;
};


export type MutationCreateAttributeArgs = {
  input: CreateAttributeInput;
};


export type MutationCreateAttributeGroupArgs = {
  name: Scalars['String']['input'];
  productTypeId: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationCreateBlogAuthorArgs = {
  input: CreateBlogAuthorInput;
};


export type MutationCreateBlogCategoryArgs = {
  input: CreateBlogCategoryInput;
};


export type MutationCreateBlogPostArgs = {
  input: CreateBlogPostInput;
};


export type MutationCreateBlogTagArgs = {
  input: CreateBlogTagInput;
};


export type MutationCreateCmsAttributeArgs = {
  input: CreateCmsAttributeInput;
};


export type MutationCreateCmsAttributeGroupArgs = {
  position?: InputMaybe<Scalars['Int']['input']>;
  slug: Scalars['String']['input'];
};


export type MutationCreateCmsBlockTypeArgs = {
  input: CreateCmsBlockTypeInput;
};


export type MutationCreateCmsPageArgs = {
  input: CreateCmsPageInput;
};


export type MutationCreateCmsPageTypeArgs = {
  input: CreateCmsPageTypeInput;
};


export type MutationCreateCmsSnippetArgs = {
  input: CreateCmsSnippetInput;
};


export type MutationCreateCmsTagArgs = {
  name: Scalars['String']['input'];
};


export type MutationCreateCategoryArgs = {
  input: CreateCategoryInput;
};


export type MutationCreateChannelArgs = {
  input: CreateChannelInput;
};


export type MutationCreateCollectionArgs = {
  input: CreateCollectionInput;
};


export type MutationCreateCountryArgs = {
  input: CreateCountryInput;
};


export type MutationCreateCurrencyArgs = {
  input: CreateCurrencyInput;
};


export type MutationCreateCustomerGroupArgs = {
  input: CreateCustomerGroupInput;
};


export type MutationCreateCustomerSegmentArgs = {
  input: CreateCustomerSegmentInput;
};


export type MutationCreateDraftOrderArgs = {
  input: CreateDraftOrderInput;
};


export type MutationCreateEmailCampaignArgs = {
  input: SaveCampaignInput;
};


export type MutationCreateEmailGroupArgs = {
  name: Scalars['String']['input'];
};


export type MutationCreateEventDestinationArgs = {
  input: EventDestinationInput;
};


export type MutationCreateExperimentArgs = {
  input: SaveExperimentInput;
};


export type MutationCreateFlowArgs = {
  input: SaveFlowInput;
};


export type MutationCreateFulfillmentArgs = {
  input: CreateFulfillmentInput;
};


export type MutationCreateIntegrationArgs = {
  input: CreateIntegrationInput;
};


export type MutationCreateInventoryLocationArgs = {
  input: CreateInventoryLocationInput;
};


export type MutationCreateLocaleArgs = {
  input: CreateLocaleInput;
};


export type MutationCreateMediaDirectoryArgs = {
  name: Scalars['String']['input'];
  path: Scalars['String']['input'];
};


export type MutationCreateMenuArgs = {
  input: CreateMenuInput;
};


export type MutationCreateMenuItemArgs = {
  input: CreateMenuItemInput;
};


export type MutationCreateProductArgs = {
  input: CreateProductInput;
};


export type MutationCreateProductTypeArgs = {
  input: CreateProductTypeInput;
};


export type MutationCreatePromotionRuleArgs = {
  input: CreatePromotionRuleInput;
};


export type MutationCreateRemoteProviderArgs = {
  input: CreateRemoteProviderInput;
};


export type MutationCreateReviewCategoryArgs = {
  input: CreateReviewCategoryInput;
};


export type MutationCreateReviewModerationTagArgs = {
  input: CreateReviewModerationTagInput;
};


export type MutationCreateReviewQuestionArgs = {
  input: CreateReviewQuestionInput;
};


export type MutationCreateRoleArgs = {
  input: CreateRoleInput;
};


export type MutationCreateShippingMethodArgs = {
  input: CreateShippingMethodInput;
};


export type MutationCreateShippingZoneArgs = {
  input: CreateShippingZoneInput;
};


export type MutationCreateSignupFormArgs = {
  input: SaveSignupFormInput;
};


export type MutationCreateTaxRuleArgs = {
  input: CreateTaxRuleInput;
};


export type MutationCreateUrlRewriteArgs = {
  input: CreateUrlRewriteInput;
};


export type MutationCreateUrlRewriteRedirectArgs = {
  input: CreateUrlRewriteRedirectInput;
};


export type MutationCreateWebFormArgs = {
  channelId: Scalars['ID']['input'];
  input: SaveWebFormInput;
};


export type MutationCreateWebhookArgs = {
  input: CreateWebhookInput;
};


export type MutationCreateWebhookJobArgs = {
  input: CreateWebhookJobInput;
};


export type MutationDeleteAdminArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteAttributeArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteAttributeGroupArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteAttributeValueArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteAttributeValueChannelSlugArgs = {
  channelId: Scalars['ID']['input'];
  valueId: Scalars['ID']['input'];
};


export type MutationDeleteBlogAuthorArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteBlogAuthorContentArgs = {
  authorId: Scalars['ID']['input'];
  locale: Scalars['String']['input'];
};


export type MutationDeleteBlogCategoryArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteBlogCategoryContentArgs = {
  categoryId: Scalars['ID']['input'];
  locale: Scalars['String']['input'];
};


export type MutationDeleteBlogPostArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteBlogPostChannelListingArgs = {
  channelId: Scalars['ID']['input'];
  postId: Scalars['ID']['input'];
};


export type MutationDeleteBlogPostContentArgs = {
  locale: Scalars['String']['input'];
  postId: Scalars['ID']['input'];
};


export type MutationDeleteBlogTagArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteBlogTagContentArgs = {
  locale: Scalars['String']['input'];
  tagId: Scalars['ID']['input'];
};


export type MutationDeleteBundleOptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteBundleOptionContentArgs = {
  locale: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
};


export type MutationDeleteBundleSelectionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsAttributeArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsAttributeGroupArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsAttributeValueArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsBlockTypeArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsPageChannelListingArgs = {
  channelId: Scalars['ID']['input'];
  pageId: Scalars['ID']['input'];
};


export type MutationDeleteCmsPageCommentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsPageTypeArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsSnippetArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCmsTagArgs = {
  slug: Scalars['String']['input'];
};


export type MutationDeleteCategoryArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCategoryContentArgs = {
  categoryId: Scalars['ID']['input'];
  locale: Scalars['String']['input'];
};


export type MutationDeleteChannelArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteChatConversationArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCollectionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCollectionContentArgs = {
  collectionId: Scalars['ID']['input'];
  locale: Scalars['String']['input'];
};


export type MutationDeleteCountryArgs = {
  code: Scalars['String']['input'];
};


export type MutationDeleteCurrencyArgs = {
  code: Scalars['String']['input'];
};


export type MutationDeleteCustomerArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCustomerGroupArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteCustomerSegmentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteEmailBlockArgs = {
  channelId: Scalars['ID']['input'];
  slug: Scalars['String']['input'];
};


export type MutationDeleteEmailCampaignArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteEmailGroupArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteEventDestinationArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteExperimentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteFlowArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteIntegrationArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteInventoryLocationArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteLibraryEmailArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteLocaleArgs = {
  code: Scalars['String']['input'];
};


export type MutationDeleteMediaAssetArgs = {
  deleteFromStorage?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
};


export type MutationDeleteMediaDirectoryArgs = {
  path: Scalars['String']['input'];
  recursive: Scalars['Boolean']['input'];
};


export type MutationDeleteMenuArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteMenuItemArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteMenuItemTranslationArgs = {
  itemId: Scalars['ID']['input'];
  locale: Scalars['String']['input'];
};


export type MutationDeleteProductArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteProductAttributeSlugArgs = {
  attributeId: Scalars['ID']['input'];
  channelId?: InputMaybe<Scalars['ID']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationDeleteProductContentArgs = {
  locale?: InputMaybe<Scalars['String']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationDeleteProductMediaArgs = {
  deleteFromStorage?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
};


export type MutationDeleteProductMediaBulkArgs = {
  deleteFromStorage?: InputMaybe<Scalars['Boolean']['input']>;
  ids: Array<Scalars['ID']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationDeleteProductOptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteProductOptionContentArgs = {
  locale: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
};


export type MutationDeleteProductOptionValueArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteProductOptionValueContentArgs = {
  locale: Scalars['String']['input'];
  valueId: Scalars['ID']['input'];
};


export type MutationDeleteProductQuestionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteProductTypeArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeletePromotionCodeArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeletePromotionRuleArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeletePromotionRuleContentArgs = {
  locale: Scalars['String']['input'];
  ruleId: Scalars['ID']['input'];
};


export type MutationDeleteRemoteProviderArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteReviewCategoryArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteReviewModerationTagArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteReviewQuestionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteRoleArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteScheduledJobArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteSearchBoostArgs = {
  channelId: Scalars['ID']['input'];
  id: Scalars['ID']['input'];
};


export type MutationDeleteSearchPinArgs = {
  channelId: Scalars['ID']['input'];
  id: Scalars['ID']['input'];
};


export type MutationDeleteSearchRedirectArgs = {
  channelId: Scalars['ID']['input'];
  id: Scalars['ID']['input'];
};


export type MutationDeleteSearchSynonymArgs = {
  channelId: Scalars['ID']['input'];
  id: Scalars['ID']['input'];
};


export type MutationDeleteShippingBoxArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteShippingMethodArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteShippingZoneArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteSignupFormArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteTaxRuleArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteUrlRewriteArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteUrlRewritesForEntityArgs = {
  entityId: Scalars['ID']['input'];
  entityType: Scalars['String']['input'];
};


export type MutationDeleteWebFormArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteWebhookArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDevImpersonateArgs = {
  adminId: Scalars['ID']['input'];
};


export type MutationDisableGiftCardArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDraftReviewReplyArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDuplicateProductOptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationEditOrderArgs = {
  input: EditOrderInput;
};


export type MutationEditOrderAddressArgs = {
  input: EditOrderAddressInput;
};


export type MutationEnqueueRebuildArgs = {
  paths: Array<Scalars['String']['input']>;
};


export type MutationFulfillOrderArgs = {
  carrier?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  insuredValueCents?: InputMaybe<Scalars['Int']['input']>;
  trackingNumber?: InputMaybe<Scalars['String']['input']>;
};


export type MutationGenerateCmsPageSeoArgs = {
  locale: Scalars['String']['input'];
  pageId: Scalars['ID']['input'];
};


export type MutationGenerateConfigurableChildrenArgs = {
  parentId: Scalars['ID']['input'];
  variants: Array<GenerateConfigurableChildInput>;
};


export type MutationGenerateMediaAltTextArgs = {
  assetId: Scalars['ID']['input'];
};


export type MutationGenerateProductReviewSummaryArgs = {
  productId: Scalars['ID']['input'];
};


export type MutationGeneratePromotionCodesArgs = {
  count: Scalars['Int']['input'];
  pattern: Scalars['String']['input'];
  prefix?: InputMaybe<Scalars['String']['input']>;
  ruleId: Scalars['ID']['input'];
  suffix?: InputMaybe<Scalars['String']['input']>;
};


export type MutationGenerateShippingLabelArgs = {
  fulfillmentId: Scalars['ID']['input'];
};


export type MutationHoldOrderArgs = {
  id: Scalars['ID']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
};


export type MutationIssueCreditMemoArgs = {
  input: IssueCreditMemoInput;
};


export type MutationIssueGiftCardArgs = {
  amount: Scalars['Int']['input'];
  currencyCode: Scalars['String']['input'];
  expiresAt?: InputMaybe<Scalars['Time']['input']>;
  recipientEmail?: InputMaybe<Scalars['String']['input']>;
};


export type MutationIssueReturnRefundArgs = {
  id: Scalars['ID']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
};


export type MutationLogoutArgs = {
  token: Scalars['String']['input'];
};


export type MutationLogoutOtherDevicesArgs = {
  currentRefreshToken: Scalars['String']['input'];
};


export type MutationMarkNotificationReadArgs = {
  id: Scalars['ID']['input'];
};


export type MutationMarkProcessingArgs = {
  id: Scalars['ID']['input'];
};


export type MutationMasqueradeCustomerArgs = {
  customerId: Scalars['ID']['input'];
};


export type MutationMintBlogPostPreviewTokenArgs = {
  input: MintBlogPostPreviewTokenInput;
};


export type MutationMintCmsPagePreviewTokenArgs = {
  input: MintCmsPagePreviewTokenInput;
};


export type MutationMoveAttributeToGroupArgs = {
  attributeId: Scalars['ID']['input'];
  groupId?: InputMaybe<Scalars['ID']['input']>;
  productTypeId: Scalars['ID']['input'];
};


export type MutationMoveMediaAssetArgs = {
  filename?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  targetPath: Scalars['String']['input'];
};


export type MutationPauseSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationPlaceDraftOrderArgs = {
  id: Scalars['ID']['input'];
};


export type MutationPublishBlogPostArgs = {
  id: Scalars['ID']['input'];
};


export type MutationPublishCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationPurgeCacheArgs = {
  paths?: InputMaybe<Array<Scalars['String']['input']>>;
  surfaceKey?: InputMaybe<Scalars['String']['input']>;
  tags?: InputMaybe<Array<Scalars['String']['input']>>;
};


export type MutationRebuildCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRebuildCacheArgs = {
  paths: Array<Scalars['String']['input']>;
};


export type MutationReceiveReturnArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRecheckReviewModerationArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRecordOfflinePaymentArgs = {
  input: RecordOfflinePaymentInput;
};


export type MutationRedeliverWebhookDeliveryArgs = {
  deliveryId: Scalars['ID']['input'];
};


export type MutationRefreshSegmentArgs = {
  segmentId: Scalars['ID']['input'];
};


export type MutationRefreshTokenArgs = {
  token: Scalars['String']['input'];
};


export type MutationRefundPaymentArgs = {
  amount?: InputMaybe<Scalars['Int']['input']>;
  orderId: Scalars['ID']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
};


export type MutationRegenerateIntegrationTokenArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRejectCmsPageArgs = {
  id: Scalars['ID']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
};


export type MutationRejectProductQuestionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRejectReturnArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRejectReviewArgs = {
  id: Scalars['ID']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
};


export type MutationReleaseOrderArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRemoveCustomerFromSegmentArgs = {
  customerId: Scalars['ID']['input'];
  segmentId: Scalars['ID']['input'];
};


export type MutationRemoveGroupedMemberArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRemoveKitComponentArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRemoveProductChildArgs = {
  childId: Scalars['ID']['input'];
  parentId: Scalars['ID']['input'];
};


export type MutationRemoveProductDownloadArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRemoveProductFromCategoryArgs = {
  categoryId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
};


export type MutationRemoveProductFromCollectionArgs = {
  collectionId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
};


export type MutationRenameCmsTagArgs = {
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
};


export type MutationRenameEmailGroupArgs = {
  id: Scalars['ID']['input'];
  name: Scalars['String']['input'];
};


export type MutationRenameMediaDirectoryArgs = {
  newName: Scalars['String']['input'];
  path: Scalars['String']['input'];
};


export type MutationReopenCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationReorderCollectionProductsArgs = {
  collectionId: Scalars['ID']['input'];
  productIds: Array<Scalars['ID']['input']>;
};


export type MutationReorderMenuItemsArgs = {
  items: Array<ReorderMenuItemInput>;
  menuId: Scalars['ID']['input'];
};


export type MutationReorderProductMediaArgs = {
  mediaIds: Array<Scalars['ID']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationReorderPromotionRulesArgs = {
  ruleIds: Array<Scalars['ID']['input']>;
};


export type MutationReplyToReviewArgs = {
  body: Scalars['String']['input'];
  id: Scalars['ID']['input'];
};


export type MutationRequestAdminPasswordResetArgs = {
  email: Scalars['String']['input'];
};


export type MutationRequestCategoryCoverUploadArgs = {
  input: RequestCategoryCoverUploadInput;
};


export type MutationRequestCollectionCoverUploadArgs = {
  input: RequestCollectionCoverUploadInput;
};


export type MutationRequestMediaAssetUploadArgs = {
  input: RequestMediaAssetUploadInput;
};


export type MutationRequestProductMediaUploadArgs = {
  input: RequestProductMediaUploadInput;
};


export type MutationRequestReturnArgs = {
  input: RequestReturnInput;
};


export type MutationResendOrderEmailArgs = {
  fulfillmentId?: InputMaybe<Scalars['ID']['input']>;
  message: OrderEmailMessage;
  orderId: Scalars['ID']['input'];
};


export type MutationResetAdminPasswordArgs = {
  newPassword: Scalars['String']['input'];
  token: Scalars['String']['input'];
};


export type MutationResetConfigValueArgs = {
  path: Scalars['String']['input'];
  scope?: InputMaybe<ConfigScopeInput>;
};


export type MutationResolveCmsPageCommentArgs = {
  id: Scalars['ID']['input'];
  resolved: Scalars['Boolean']['input'];
};


export type MutationRestoreCmsPageRevisionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRestoreEmailCampaignArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRestoreEmailRevisionArgs = {
  revisionId: Scalars['ID']['input'];
};


export type MutationRestoreFlowArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRestoreWebFormArgs = {
  id: Scalars['ID']['input'];
};


export type MutationResumeSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRetryWebhookDeadLetterArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRewriteEmailTextArgs = {
  instruction?: InputMaybe<Scalars['String']['input']>;
  text: Scalars['String']['input'];
};


export type MutationRotateRemoteProviderSecretArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRotateWebhookSecretArgs = {
  id: Scalars['ID']['input'];
};


export type MutationRunAdminActionArgs = {
  args: Scalars['JSON']['input'];
  slug: Scalars['String']['input'];
};


export type MutationRunChatActionArgs = {
  action: Scalars['String']['input'];
  argsJson: Scalars['String']['input'];
};


export type MutationRunScheduledJobNowArgs = {
  id: Scalars['ID']['input'];
};


export type MutationSaveChatConversationArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  id?: InputMaybe<Scalars['ID']['input']>;
  messages: Array<SignalsChatMessage>;
  title?: InputMaybe<Scalars['String']['input']>;
};


export type MutationSaveEmailBlockArgs = {
  input: SaveEmailBlockInput;
};


export type MutationSaveLibraryEmailArgs = {
  input: SaveLibraryEmailInput;
};


export type MutationScheduleBlogPostArgs = {
  id: Scalars['ID']['input'];
  publishedAt: Scalars['Time']['input'];
};


export type MutationScheduleCmsPageArgs = {
  id: Scalars['ID']['input'];
  scheduledAt?: InputMaybe<Scalars['Time']['input']>;
};


export type MutationScheduleEmailCampaignArgs = {
  id: Scalars['ID']['input'];
  scheduledFor?: InputMaybe<Scalars['Time']['input']>;
};


export type MutationSendCampaignTestArgs = {
  id: Scalars['ID']['input'];
  to: Scalars['String']['input'];
};


export type MutationSetAttributeDescriptionArgs = {
  attributeId: Scalars['ID']['input'];
  description: Scalars['String']['input'];
  locale: Scalars['String']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
};


export type MutationSetAttributeIndexableArgs = {
  attributeId: Scalars['ID']['input'];
  isIndexable: Scalars['Boolean']['input'];
};


export type MutationSetAttributeValueChannelSlugArgs = {
  channelId: Scalars['ID']['input'];
  slug: Scalars['String']['input'];
  valueId: Scalars['ID']['input'];
};


export type MutationSetAttributeValueDescriptionArgs = {
  description: Scalars['String']['input'];
  locale: Scalars['String']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
  valueId: Scalars['ID']['input'];
};


export type MutationSetAttributeValuesIndexableArgs = {
  isIndexable: Scalars['Boolean']['input'];
  valueIds: Array<Scalars['ID']['input']>;
};


export type MutationSetBlogPostCategoriesArgs = {
  categoryIds: Array<Scalars['ID']['input']>;
  postId: Scalars['ID']['input'];
};


export type MutationSetBlogPostChannelListingArgs = {
  input: SetBlogPostChannelListingInput;
};


export type MutationSetBlogPostTagsArgs = {
  postId: Scalars['ID']['input'];
  tagIds: Array<Scalars['ID']['input']>;
};


export type MutationSetCmsPageChannelListingArgs = {
  input: SetCmsPageChannelListingInput;
};


export type MutationSetCmsPageExperimentArgs = {
  experimentKey: Scalars['String']['input'];
  pageId: Scalars['ID']['input'];
};


export type MutationSetCmsPageRelationshipsArgs = {
  attributeId: Scalars['ID']['input'];
  pageId: Scalars['ID']['input'];
  refs: Array<CmsRelationshipRefInput>;
};


export type MutationSetCmsPageSelectionsArgs = {
  attributeId: Scalars['ID']['input'];
  pageId: Scalars['ID']['input'];
  valueIds: Array<Scalars['ID']['input']>;
};


export type MutationSetCmsPageTypeChannelRootArgs = {
  channelId: Scalars['ID']['input'];
  pageTypeId: Scalars['ID']['input'];
  rootSlug: Scalars['String']['input'];
};


export type MutationSetCmsPageTypeDefaultBlocksArgs = {
  blocks?: InputMaybe<Scalars['JSON']['input']>;
  pageTypeId: Scalars['ID']['input'];
};


export type MutationSetCategoryFacetOverridesArgs = {
  categoryId: Scalars['ID']['input'];
  overrides: Array<FacetOverrideInput>;
};


export type MutationSetCategoryListingArgs = {
  input: SetCategoryListingInput;
};


export type MutationSetChannelAsDefaultArgs = {
  channelId: Scalars['ID']['input'];
};


export type MutationSetChannelLocalesArgs = {
  channelId: Scalars['ID']['input'];
  codes: Array<Scalars['String']['input']>;
};


export type MutationSetChannelLocationsArgs = {
  assignments: Array<LocationAssignmentInput>;
  channelId: Scalars['ID']['input'];
};


export type MutationSetCollectionFacetOverridesArgs = {
  collectionId: Scalars['ID']['input'];
  overrides: Array<FacetOverrideInput>;
};


export type MutationSetConfigValueArgs = {
  path: Scalars['String']['input'];
  scope?: InputMaybe<ConfigScopeInput>;
  value: Scalars['String']['input'];
};


export type MutationSetConfigurableOptionsArgs = {
  attributeIds: Array<Scalars['ID']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationSetCustomerActiveArgs = {
  active: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
};


export type MutationSetCustomerGroupArgs = {
  customerId: Scalars['ID']['input'];
  groupId: Scalars['ID']['input'];
};


export type MutationSetCustomerTaxStatusArgs = {
  customerId: Scalars['ID']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
  taxExempt: Scalars['Boolean']['input'];
};


export type MutationSetCustomerVatIdArgs = {
  customerId: Scalars['ID']['input'];
  vatId: Scalars['String']['input'];
};


export type MutationSetDefaultCustomerGroupArgs = {
  id: Scalars['ID']['input'];
};


export type MutationSetFlowStatusArgs = {
  id: Scalars['ID']['input'];
  status: Scalars['String']['input'];
};


export type MutationSetIntegrationActiveArgs = {
  active: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
};


export type MutationSetInventoryItemBackorderArgs = {
  backorderable: Scalars['Boolean']['input'];
  lowStockThreshold?: InputMaybe<Scalars['Float']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationSetInventoryItemTrackingArgs = {
  productId: Scalars['ID']['input'];
  tracked: Scalars['Boolean']['input'];
};


export type MutationSetLocationPickupArgs = {
  enabled: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
  instructions?: InputMaybe<Scalars['String']['input']>;
};


export type MutationSetMediaFocalPointArgs = {
  assetId: Scalars['ID']['input'];
  x?: InputMaybe<Scalars['Float']['input']>;
  y?: InputMaybe<Scalars['Float']['input']>;
};


export type MutationSetMenuStoresArgs = {
  channelIds: Array<Scalars['ID']['input']>;
  menuId: Scalars['ID']['input'];
};


export type MutationSetMyPreferenceArgs = {
  key: Scalars['String']['input'];
  value?: InputMaybe<Scalars['JSON']['input']>;
};


export type MutationSetPaymentMethodCountriesArgs = {
  channelId: Scalars['ID']['input'];
  countries: Array<Scalars['String']['input']>;
  mode: Scalars['String']['input'];
  providerSlug: Scalars['String']['input'];
};


export type MutationSetPaymentMethodEnabledArgs = {
  channelId: Scalars['ID']['input'];
  enabled: Scalars['Boolean']['input'];
  providerSlug: Scalars['String']['input'];
};


export type MutationSetProductAttributeLocaleValueArgs = {
  attributeId: Scalars['ID']['input'];
  locale: Scalars['String']['input'];
  productId: Scalars['ID']['input'];
  value: Scalars['String']['input'];
};


export type MutationSetProductAttributeReferencesArgs = {
  attributeId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
  refs: Array<AttributeReferenceRefInput>;
};


export type MutationSetProductAttributeSlugArgs = {
  input: SetProductAttributeSlugInput;
};


export type MutationSetProductAttributesArgs = {
  assignments: Array<ProductAttributeAssignmentInput>;
  productId: Scalars['ID']['input'];
};


export type MutationSetProductGroupPricesArgs = {
  prices: Array<ProductGroupPriceInput>;
  productId: Scalars['ID']['input'];
};


export type MutationSetProductLinksArgs = {
  kind: ProductLinkKind;
  linkedProductIds: Array<Scalars['ID']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationSetProductListingArgs = {
  input: SetProductListingInput;
};


export type MutationSetProductMediaRoleArgs = {
  mediaId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
  role: ProductMediaRole;
};


export type MutationSetProductPrimaryCategoryArgs = {
  categoryId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
};


export type MutationSetProductTaxClassArgs = {
  productId: Scalars['ID']['input'];
  taxClass: Scalars['String']['input'];
};


export type MutationSetProductTypeUrlPathTemplateArgs = {
  productTypeId: Scalars['ID']['input'];
  template: Scalars['String']['input'];
};


export type MutationSetProductVariantModeArgs = {
  mode: VariantMode;
  productId: Scalars['ID']['input'];
};


export type MutationSetRemoteProviderActiveArgs = {
  active: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
};


export type MutationSetReviewCategoriesArgs = {
  categorySlugs: Array<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
};


export type MutationSetReviewModerationTagsArgs = {
  id: Scalars['ID']['input'];
  tagSlugs: Array<Scalars['String']['input']>;
};


export type MutationSetSegmentRulesArgs = {
  rules?: InputMaybe<Scalars['JSON']['input']>;
  segmentId: Scalars['ID']['input'];
};


export type MutationSetShippingCarrierCountriesArgs = {
  carrierSlug: Scalars['String']['input'];
  channelId: Scalars['ID']['input'];
  countries: Array<Scalars['String']['input']>;
  mode: Scalars['String']['input'];
};


export type MutationSetShippingCarrierEnabledArgs = {
  carrierSlug: Scalars['String']['input'];
  channelId: Scalars['ID']['input'];
  enabled: Scalars['Boolean']['input'];
};


export type MutationSetSubscriptionTermsArgs = {
  input: SetSubscriptionTermsInput;
};


export type MutationSetWebhookActiveArgs = {
  active: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
};


export type MutationSignalsChatArgs = {
  channelId: Scalars['ID']['input'];
  messages: Array<SignalsChatMessage>;
  page?: InputMaybe<SignalsChatPage>;
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationStartDataExportArgs = {
  input: StartDataExportInput;
};


export type MutationStartSiteCrawlArgs = {
  input: StartSiteCrawlInput;
};


export type MutationSubmitCmsPageForReviewArgs = {
  id: Scalars['ID']['input'];
};


export type MutationSuggestEmailSubjectsArgs = {
  context: Scalars['String']['input'];
  count?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationSuppressEmailArgs = {
  email: Scalars['String']['input'];
  note?: InputMaybe<Scalars['String']['input']>;
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


export type MutationSyncCanonicalUrlRewriteArgs = {
  input: SyncCanonicalUrlRewriteInput;
};


export type MutationSyncProductInventoryArgs = {
  input: SyncProductInventoryInput;
};


export type MutationSyncProductInventoryBatchArgs = {
  items: Array<SyncProductInventoryInput>;
};


export type MutationTranslateCmsPageArgs = {
  pageId: Scalars['ID']['input'];
  sourceLocale: Scalars['String']['input'];
  targetLocale: Scalars['String']['input'];
};


export type MutationUnassignAttributeArgs = {
  attributeId: Scalars['ID']['input'];
  productTypeId: Scalars['ID']['input'];
};


export type MutationUnassignCmsAttributeArgs = {
  attributeId: Scalars['ID']['input'];
  pageTypeId: Scalars['ID']['input'];
};


export type MutationUnassignCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationUnassignReviewQuestionArgs = {
  productTypeId: Scalars['ID']['input'];
  questionId: Scalars['ID']['input'];
};


export type MutationUnpublishBlogPostArgs = {
  id: Scalars['ID']['input'];
};


export type MutationUnpublishCmsPageArgs = {
  id: Scalars['ID']['input'];
};


export type MutationUnsuppressEmailArgs = {
  email: Scalars['String']['input'];
};


export type MutationUpdateAdminArgs = {
  id: Scalars['ID']['input'];
  input: UpdateAdminInput;
};


export type MutationUpdateAttributeArgs = {
  input: UpdateAttributeInput;
};


export type MutationUpdateAttributeGroupArgs = {
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};


export type MutationUpdateAttributeValueArgs = {
  id: Scalars['ID']['input'];
  input: AttributeValueInput;
};


export type MutationUpdateBlogAuthorArgs = {
  input: UpdateBlogAuthorInput;
};


export type MutationUpdateBlogCategoryArgs = {
  input: UpdateBlogCategoryInput;
};


export type MutationUpdateBlogPostArgs = {
  input: UpdateBlogPostInput;
};


export type MutationUpdateBlogTagArgs = {
  input: UpdateBlogTagInput;
};


export type MutationUpdateBundleOptionArgs = {
  input: UpdateBundleOptionInput;
};


export type MutationUpdateBundleSelectionArgs = {
  input: UpdateBundleSelectionInput;
};


export type MutationUpdateCmsAttributeArgs = {
  input: UpdateCmsAttributeInput;
};


export type MutationUpdateCmsAttributeGroupArgs = {
  id: Scalars['ID']['input'];
  position: Scalars['Int']['input'];
  slug: Scalars['String']['input'];
};


export type MutationUpdateCmsAttributeValueArgs = {
  id: Scalars['ID']['input'];
  position: Scalars['Int']['input'];
  slug: Scalars['String']['input'];
};


export type MutationUpdateCmsBlockTypeArgs = {
  input: UpdateCmsBlockTypeInput;
};


export type MutationUpdateCmsPageArgs = {
  input: UpdateCmsPageInput;
};


export type MutationUpdateCmsPageTypeArgs = {
  input: UpdateCmsPageTypeInput;
};


export type MutationUpdateCmsPageTypeStructureArgs = {
  input: UpdateCmsPageTypeStructureInput;
};


export type MutationUpdateCmsSnippetArgs = {
  input: UpdateCmsSnippetInput;
};


export type MutationUpdateCategoryArgs = {
  input: UpdateCategoryInput;
};


export type MutationUpdateChannelArgs = {
  input: UpdateChannelInput;
};


export type MutationUpdateCollectionArgs = {
  input: UpdateCollectionInput;
};


export type MutationUpdateCountryArgs = {
  input: UpdateCountryInput;
};


export type MutationUpdateCurrencyArgs = {
  input: UpdateCurrencyInput;
};


export type MutationUpdateCustomerGroupArgs = {
  id: Scalars['ID']['input'];
  input: UpdateCustomerGroupInput;
};


export type MutationUpdateCustomerSegmentArgs = {
  input: UpdateCustomerSegmentInput;
};


export type MutationUpdateEmailCampaignArgs = {
  id: Scalars['ID']['input'];
  input: SaveCampaignInput;
};


export type MutationUpdateEventDestinationArgs = {
  id: Scalars['ID']['input'];
  input: EventDestinationInput;
};


export type MutationUpdateExperimentArgs = {
  id: Scalars['ID']['input'];
  input: SaveExperimentInput;
};


export type MutationUpdateFlowArgs = {
  id: Scalars['ID']['input'];
  input: SaveFlowInput;
};


export type MutationUpdateGroupedMemberArgs = {
  input: UpdateGroupedMemberInput;
};


export type MutationUpdateIntegrationArgs = {
  input: UpdateIntegrationInput;
};


export type MutationUpdateInventoryLocationArgs = {
  input: UpdateInventoryLocationInput;
};


export type MutationUpdateKitComponentArgs = {
  input: UpdateKitComponentInput;
};


export type MutationUpdateLocaleArgs = {
  input: UpdateLocaleInput;
};


export type MutationUpdateMediaAssetArgs = {
  input: UpdateMediaAssetInput;
};


export type MutationUpdateMenuArgs = {
  input: UpdateMenuInput;
};


export type MutationUpdateMenuItemArgs = {
  input: UpdateMenuItemInput;
};


export type MutationUpdateProductArgs = {
  input: UpdateProductInput;
};


export type MutationUpdateProductDownloadArgs = {
  input: UpdateProductDownloadInput;
};


export type MutationUpdateProductMediaArgs = {
  input: UpdateProductMediaInput;
};


export type MutationUpdateProductOptionArgs = {
  input: UpdateProductOptionInput;
};


export type MutationUpdateProductOptionValueArgs = {
  input: UpdateProductOptionValueInput;
};


export type MutationUpdateProductTypeArgs = {
  input: UpdateProductTypeInput;
};


export type MutationUpdatePromotionCodeArgs = {
  input: UpdatePromotionCodeInput;
};


export type MutationUpdatePromotionRuleArgs = {
  input: UpdatePromotionRuleInput;
};


export type MutationUpdateRemoteProviderArgs = {
  input: UpdateRemoteProviderInput;
};


export type MutationUpdateReviewCategoryArgs = {
  input: UpdateReviewCategoryInput;
};


export type MutationUpdateReviewModerationTagArgs = {
  input: UpdateReviewModerationTagInput;
};


export type MutationUpdateReviewQuestionArgs = {
  input: UpdateReviewQuestionInput;
};


export type MutationUpdateRoleArgs = {
  id: Scalars['ID']['input'];
  input: UpdateRoleInput;
};


export type MutationUpdateScheduledJobArgs = {
  input: UpdateScheduledJobInput;
};


export type MutationUpdateShippingMethodArgs = {
  input: UpdateShippingMethodInput;
};


export type MutationUpdateShippingZoneArgs = {
  input: UpdateShippingZoneInput;
};


export type MutationUpdateSignupFormArgs = {
  id: Scalars['ID']['input'];
  input: SaveSignupFormInput;
};


export type MutationUpdateTaxRuleArgs = {
  input: UpdateTaxRuleInput;
};


export type MutationUpdateUrlRewriteArgs = {
  input: UpdateUrlRewriteInput;
};


export type MutationUpdateWebFormArgs = {
  id: Scalars['ID']['input'];
  input: SaveWebFormInput;
};


export type MutationUpdateWebhookArgs = {
  input: UpdateWebhookInput;
};


export type MutationUpdateWebhookJobArgs = {
  input: UpdateWebhookJobInput;
};


export type MutationUpsertBlogAuthorContentArgs = {
  input: UpsertBlogAuthorContentInput;
};


export type MutationUpsertBlogCategoryContentArgs = {
  input: UpsertBlogCategoryContentInput;
};


export type MutationUpsertBlogPostContentArgs = {
  input: UpsertBlogPostContentInput;
};


export type MutationUpsertBlogTagContentArgs = {
  input: UpsertBlogTagContentInput;
};


export type MutationUpsertBundleOptionContentArgs = {
  locale: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
  title: Scalars['String']['input'];
};


export type MutationUpsertCmsAttributeContentArgs = {
  input: UpsertCmsAttributeContentInput;
};


export type MutationUpsertCmsAttributeValueContentArgs = {
  locale: Scalars['String']['input'];
  name: Scalars['String']['input'];
  valueId: Scalars['ID']['input'];
};


export type MutationUpsertCmsPageContentArgs = {
  input: UpsertCmsPageContentInput;
};


export type MutationUpsertCmsPageTypeContentArgs = {
  input: UpsertCmsPageTypeContentInput;
};


export type MutationUpsertCmsPageVariantContentArgs = {
  body?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  pageId: Scalars['ID']['input'];
  variantKey: Scalars['String']['input'];
};


export type MutationUpsertCategoryContentArgs = {
  categoryId: Scalars['ID']['input'];
  input: UpsertCategoryContentInput;
};


export type MutationUpsertCollectionContentArgs = {
  collectionId: Scalars['ID']['input'];
  input: UpsertCollectionContentInput;
  locale: Scalars['String']['input'];
};


export type MutationUpsertMenuItemTranslationArgs = {
  itemId: Scalars['ID']['input'];
  label: Scalars['String']['input'];
  locale: Scalars['String']['input'];
};


export type MutationUpsertProductContentArgs = {
  input: UpsertProductContentInput;
  locale?: InputMaybe<Scalars['String']['input']>;
  productId: Scalars['ID']['input'];
};


export type MutationUpsertProductOptionContentArgs = {
  helpText?: InputMaybe<Scalars['String']['input']>;
  locale: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
  title: Scalars['String']['input'];
};


export type MutationUpsertProductOptionValueContentArgs = {
  locale: Scalars['String']['input'];
  title: Scalars['String']['input'];
  valueId: Scalars['ID']['input'];
};


export type MutationUpsertPromotionRuleContentArgs = {
  input: UpsertPromotionRuleContentInput;
};


export type MutationUpsertReviewQuestionContentArgs = {
  input: UpsertReviewQuestionContentInput;
};


export type MutationUpsertSearchBoostArgs = {
  channelId: Scalars['ID']['input'];
  productId: Scalars['ID']['input'];
  query: Scalars['String']['input'];
  weight: Scalars['Float']['input'];
};


export type MutationUpsertSearchPinArgs = {
  channelId: Scalars['ID']['input'];
  position?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
  query: Scalars['String']['input'];
};


export type MutationUpsertSearchRedirectArgs = {
  channelId: Scalars['ID']['input'];
  query: Scalars['String']['input'];
  targetUrl: Scalars['String']['input'];
};


export type MutationUpsertShippingBoxArgs = {
  input: ShippingBoxInput;
};


export type MutationVoidPaymentArgs = {
  orderId: Scalars['ID']['input'];
};

export type Order = {
  __typename?: 'Order';
  appliedPromotions: Array<AppliedPromotion>;
  billingAddress: OrderAddress;
  carrier: Scalars['String']['output'];
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  customerId?: Maybe<Scalars['ID']['output']>;
  customerNote?: Maybe<Scalars['String']['output']>;
  discountAmount: Scalars['Int']['output'];
  email: Scalars['String']['output'];
  events: Array<OrderEvent>;
  fulfillments: Array<Fulfillment>;
  giftCardAmount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  lines: Array<OrderLine>;
  number: Scalars['String']['output'];
  paymentClientSecret?: Maybe<Scalars['String']['output']>;
  paymentProvider: Scalars['String']['output'];
  paymentReference: Scalars['String']['output'];
  paymentStatus: PaymentStatus;
  paymentTransactions: Array<PaymentTransaction>;
  poNumber?: Maybe<Scalars['String']['output']>;
  returns: Array<OrderReturn>;
  shippingAddress: OrderAddress;
  shippingAmount: Scalars['Int']['output'];
  status: OrderStatus;
  storeCreditAmount: Scalars['Int']['output'];
  subtotal: Scalars['Int']['output'];
  taxAmount: Scalars['Int']['output'];
  taxLines: Array<OrderTaxLine>;
  total: Scalars['Int']['output'];
  trackingNumber: Scalars['String']['output'];
  trackingUrl: Scalars['String']['output'];
};

export type OrderAddress = {
  __typename?: 'OrderAddress';
  city: Scalars['String']['output'];
  company?: Maybe<Scalars['String']['output']>;
  countryCode: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  lastName: Scalars['String']['output'];
  phone?: Maybe<Scalars['String']['output']>;
  postalCode: Scalars['String']['output'];
  state?: Maybe<Scalars['String']['output']>;
  street1: Scalars['String']['output'];
  street2?: Maybe<Scalars['String']['output']>;
};

export type OrderDocumentKind =
  | 'CREDIT_MEMO'
  | 'INVOICE'
  | 'PACKING_SLIP';

export type OrderEmailMessage =
  | 'CANCELLATION'
  | 'CONFIRMATION'
  | 'REFUND'
  | 'SHIPMENT';

export type OrderEvent = {
  __typename?: 'OrderEvent';
  actorEmail?: Maybe<Scalars['String']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  message: Scalars['String']['output'];
  type: Scalars['String']['output'];
};

export type OrderLine = {
  __typename?: 'OrderLine';
  components: Array<OrderLineComponent>;
  id: Scalars['ID']['output'];
  isGift: Scalars['Boolean']['output'];
  options: Array<OrderLineOption>;
  productName: Scalars['String']['output'];
  productSku: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  totalPrice: Scalars['Int']['output'];
  unitPrice: Scalars['Int']['output'];
};

export type OrderLineComponent = {
  __typename?: 'OrderLineComponent';
  id: Scalars['ID']['output'];
  productName: Scalars['String']['output'];
  productSku: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  unitPrice: Scalars['Int']['output'];
};

export type OrderLineOption = {
  __typename?: 'OrderLineOption';
  fileUrl: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  inputType: Scalars['String']['output'];
  optionTitle: Scalars['String']['output'];
  priceDelta: Scalars['Int']['output'];
  quantity: Scalars['Float']['output'];
  textValue: Scalars['String']['output'];
  valueLabel: Scalars['String']['output'];
};

export type OrderLineQuantity = {
  __typename?: 'OrderLineQuantity';
  orderLineId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
};

export type OrderPage = {
  __typename?: 'OrderPage';
  items: Array<Order>;
  totalCount: Scalars['Int']['output'];
};

export type OrderReturn = {
  __typename?: 'OrderReturn';
  createdAt: Scalars['Time']['output'];
  exchangeOrderId?: Maybe<Scalars['ID']['output']>;
  id: Scalars['ID']['output'];
  lines: Array<OrderReturnLine>;
  reason: Scalars['String']['output'];
  refundAmount: Scalars['Int']['output'];
  refundTarget: Scalars['String']['output'];
  resolution: Scalars['String']['output'];
  restock: Scalars['Boolean']['output'];
  rmaNumber: Scalars['String']['output'];
  status: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type OrderReturnLine = {
  __typename?: 'OrderReturnLine';
  id: Scalars['ID']['output'];
  orderLineId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
  reason: Scalars['String']['output'];
  refundUnitCents: Scalars['Int']['output'];
};

export type OrderStatus =
  | 'CANCELLED'
  | 'CONFIRMED'
  | 'DRAFT'
  | 'FULFILLED'
  | 'ON_HOLD'
  | 'PARTIALLY_FULFILLED'
  | 'PENDING'
  | 'PROCESSING';

export type OrderTaxLine = {
  __typename?: 'OrderTaxLine';
  amount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  rateBps: Scalars['Int']['output'];
  ruleId?: Maybe<Scalars['ID']['output']>;
};

export type PaymentMethodAdmin = {
  __typename?: 'PaymentMethodAdmin';
  allowedCountries: Array<Scalars['String']['output']>;
  countryMode: Scalars['String']['output'];
  displayName: Scalars['String']['output'];
  isEnabled: Scalars['Boolean']['output'];
  isOffline: Scalars['Boolean']['output'];
  providerSlug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type PaymentStatus =
  | 'AUTHORIZED'
  | 'PAID'
  | 'PARTIALLY_CAPTURED'
  | 'PARTIALLY_REFUNDED'
  | 'REFUNDED'
  | 'UNPAID'
  | 'VOIDED';

export type PaymentTransaction = {
  __typename?: 'PaymentTransaction';
  amount: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  kind: Scalars['String']['output'];
  providerReference: Scalars['String']['output'];
  providerSlug: Scalars['String']['output'];
  reason: Scalars['String']['output'];
  status: Scalars['String']['output'];
};

export type Permission = {
  __typename?: 'Permission';
  description: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type PermissionGroup = {
  __typename?: 'PermissionGroup';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  permissions: Array<Permission>;
  slug: Scalars['String']['output'];
};

export type Product = {
  __typename?: 'Product';
  attributeValues: Array<ProductAttributeValue>;
  bundleOptions: Array<BundleOption>;
  canonicalUrl?: Maybe<Scalars['String']['output']>;
  categories: Array<Category>;
  children: Array<Product>;
  collections: Array<Collection>;
  compareAtPrice?: Maybe<Scalars['Int']['output']>;
  compareAtPriceResolved?: Maybe<Scalars['Int']['output']>;
  configurableOptions: Array<Attribute>;
  content: Array<ProductContent>;
  contentLocale?: Maybe<Scalars['String']['output']>;
  costCents?: Maybe<Scalars['Int']['output']>;
  createdAt: Scalars['Time']['output'];
  defaultCurrencyCode: Scalars['String']['output'];
  defaultPrice: Scalars['Int']['output'];
  description?: Maybe<Scalars['JSON']['output']>;
  descriptionRichText?: Maybe<Scalars['JSON']['output']>;
  displayImage?: Maybe<ProductMedia>;
  downloads: Array<ProductDownload>;
  groupPrices: Array<ProductGroupPrice>;
  groupedMembers: Array<GroupedMember>;
  heightMm?: Maybe<Scalars['Int']['output']>;
  id: Scalars['ID']['output'];
  inStock: Scalars['Boolean']['output'];
  kind: ProductKind;
  kitComponents: Array<KitComponent>;
  lengthMm?: Maybe<Scalars['Int']['output']>;
  listings: Array<ProductChannelListing>;
  mainImage?: Maybe<ProductMedia>;
  maxOrderQty?: Maybe<Scalars['Float']['output']>;
  media: Array<ProductMedia>;
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  metadata: Array<MetadataItem>;
  minOrderQty?: Maybe<Scalars['Float']['output']>;
  name?: Maybe<Scalars['String']['output']>;
  ogImage?: Maybe<ProductMedia>;
  onSale: Scalars['Boolean']['output'];
  partOf: ProductMemberships;
  price: Scalars['Int']['output'];
  primaryCategory?: Maybe<Category>;
  privateMetadata: Array<MetadataItem>;
  productOptions: Array<ProductOption>;
  productType: ProductType;
  quantityDecimals: Scalars['Int']['output'];
  quantityStep: Scalars['Float']['output'];
  salableQuantity?: Maybe<Scalars['Float']['output']>;
  seoNoindex: Scalars['Boolean']['output'];
  shortDescription?: Maybe<Scalars['JSON']['output']>;
  sku: Scalars['String']['output'];
  specialFrom?: Maybe<Scalars['Time']['output']>;
  specialPrice?: Maybe<Scalars['Int']['output']>;
  specialTo?: Maybe<Scalars['Time']['output']>;
  subscriptionTerms?: Maybe<SubscriptionTerms>;
  taxClass: Scalars['String']['output'];
  thumbnail?: Maybe<ProductMedia>;
  tracksInventory: Scalars['Boolean']['output'];
  tracksInventoryOverride?: Maybe<Scalars['Boolean']['output']>;
  updatedAt: Scalars['Time']['output'];
  url: Scalars['String']['output'];
  urlAuto: Scalars['Boolean']['output'];
  urlTemplateOverride: Scalars['String']['output'];
  variantMode?: Maybe<VariantMode>;
  weightGrams?: Maybe<Scalars['Int']['output']>;
  widthMm?: Maybe<Scalars['Int']['output']>;
};


export type ProductInStockArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type ProductSalableQuantityArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};

export type ProductAttributeAssignmentInput = {
  attributeId: Scalars['ID']['input'];
  scalarValue?: InputMaybe<Scalars['JSON']['input']>;
  valueIds?: InputMaybe<Array<Scalars['ID']['input']>>;
};

export type ProductAttributeLocaleValue = {
  __typename?: 'ProductAttributeLocaleValue';
  attributeId: Scalars['ID']['output'];
  locale: Scalars['String']['output'];
  productId: Scalars['ID']['output'];
  updatedAt: Scalars['Time']['output'];
  value: Scalars['String']['output'];
};

export type ProductAttributeReference = {
  __typename?: 'ProductAttributeReference';
  label: Scalars['String']['output'];
  targetId: Scalars['ID']['output'];
  targetType: AttributeReferenceTarget;
  url: Scalars['String']['output'];
};

export type ProductAttributeSlug = {
  __typename?: 'ProductAttributeSlug';
  attributeId: Scalars['ID']['output'];
  channelId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  productId: Scalars['ID']['output'];
  slug: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ProductAttributeValue = {
  __typename?: 'ProductAttributeValue';
  attribute: Attribute;
  references: Array<ProductAttributeReference>;
  scalarValue?: Maybe<Scalars['JSON']['output']>;
  selectedValues: Array<AttributeValue>;
};

export type ProductChannelListing = {
  __typename?: 'ProductChannelListing';
  channel: Channel;
  channelId: Scalars['ID']['output'];
  compareAtPrice?: Maybe<Scalars['Int']['output']>;
  effectiveCurrency: Scalars['String']['output'];
  effectivePrice?: Maybe<Scalars['Int']['output']>;
  isPublished: Scalars['Boolean']['output'];
  price?: Maybe<Scalars['Int']['output']>;
  productId: Scalars['ID']['output'];
  specialFrom?: Maybe<Scalars['Time']['output']>;
  specialPrice?: Maybe<Scalars['Int']['output']>;
  specialTo?: Maybe<Scalars['Time']['output']>;
  url?: Maybe<Scalars['String']['output']>;
};

export type ProductContent = {
  __typename?: 'ProductContent';
  description?: Maybe<Scalars['JSON']['output']>;
  descriptionRichText?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  shortDescription?: Maybe<Scalars['JSON']['output']>;
  updatedAt: Scalars['Time']['output'];
};

export type ProductDownload = {
  __typename?: 'ProductDownload';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  maxDownloads?: Maybe<Scalars['Int']['output']>;
  sortOrder: Scalars['Int']['output'];
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  url: Scalars['String']['output'];
};

export type ProductGroupPrice = {
  __typename?: 'ProductGroupPrice';
  channelId?: Maybe<Scalars['ID']['output']>;
  customerGroupId?: Maybe<Scalars['ID']['output']>;
  id: Scalars['ID']['output'];
  minQty: Scalars['Float']['output'];
  price: Scalars['Int']['output'];
};

export type ProductGroupPriceInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  customerGroupId?: InputMaybe<Scalars['ID']['input']>;
  minQty: Scalars['Float']['input'];
  price: Scalars['Int']['input'];
};

export type ProductKind =
  | 'BUNDLE'
  | 'CONFIGURABLE'
  | 'GIFTCARD'
  | 'GROUPED'
  | 'KIT'
  | 'SIMPLE'
  | 'SUBSCRIPTION';

export type ProductLinkKind =
  | 'CROSSSELL'
  | 'RELATED'
  | 'UPSELL';

export type ProductMedia = {
  __typename?: 'ProductMedia';
  altText: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  height: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  mimeType: Scalars['String']['output'];
  sizeBytes: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  status: ProductMediaStatus;
  url: Scalars['String']['output'];
  width: Scalars['Int']['output'];
};

export type ProductMediaRole =
  | 'MAIN'
  | 'OG_IMAGE'
  | 'THUMBNAIL';

export type ProductMediaStatus =
  | 'PENDING'
  | 'READY';

export type ProductMediaUploadTarget =
  | 'DESCRIPTION'
  | 'GALLERY'
  | 'SHORT_DESCRIPTION';

export type ProductMemberships = {
  __typename?: 'ProductMemberships';
  bundles: Array<BundleMembership>;
  configurable?: Maybe<ConfigurableMembership>;
  groupedProducts: Array<GroupedMembership>;
};

export type ProductOption = {
  __typename?: 'ProductOption';
  content: Array<ProductOptionContent>;
  createdAt: Scalars['Time']['output'];
  fileExtensions?: Maybe<Scalars['String']['output']>;
  helpText: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  inputType: ProductOptionInputType;
  isRequired: Scalars['Boolean']['output'];
  maxChars?: Maybe<Scalars['Int']['output']>;
  maxFileBytes?: Maybe<Scalars['Int']['output']>;
  priceType?: Maybe<ProductOptionPriceType>;
  priceValue?: Maybe<Scalars['Int']['output']>;
  sortOrder: Scalars['Int']['output'];
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  values: Array<ProductOptionValue>;
  visibleIf?: Maybe<Scalars['JSON']['output']>;
};

export type ProductOptionContent = {
  __typename?: 'ProductOptionContent';
  helpText: Scalars['String']['output'];
  locale: Scalars['String']['output'];
  title: Scalars['String']['output'];
};

export type ProductOptionInputType =
  | 'AREA'
  | 'CHECKBOX'
  | 'DATE'
  | 'DATE_TIME'
  | 'DROPDOWN'
  | 'FIELD'
  | 'FILE'
  | 'MULTISELECT'
  | 'RADIO'
  | 'TIME';

export type ProductOptionPriceType =
  | 'FIXED'
  | 'PERCENT';

export type ProductOptionValue = {
  __typename?: 'ProductOptionValue';
  content: Array<ProductOptionValueContent>;
  createdAt: Scalars['Time']['output'];
  forcedLineQty?: Maybe<Scalars['Float']['output']>;
  id: Scalars['ID']['output'];
  isDefault: Scalars['Boolean']['output'];
  priceType: ProductOptionPriceType;
  priceValue: Scalars['Int']['output'];
  quantity: Scalars['Float']['output'];
  sortOrder: Scalars['Int']['output'];
  sourceProduct?: Maybe<Product>;
  title: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ProductOptionValueContent = {
  __typename?: 'ProductOptionValueContent';
  locale: Scalars['String']['output'];
  title: Scalars['String']['output'];
};

export type ProductPage = {
  __typename?: 'ProductPage';
  items: Array<Product>;
  totalCount: Scalars['Int']['output'];
};

export type ProductSales = {
  __typename?: 'ProductSales';
  cost: Scalars['Int64']['output'];
  days: Scalars['Int']['output'];
  margin: Scalars['Int64']['output'];
  orderCount: Scalars['Int']['output'];
  revenue: Scalars['Int64']['output'];
  unitsSold: Scalars['Int64']['output'];
};

export type ProductType = {
  __typename?: 'ProductType';
  attributes: Array<ProductTypeAttribute>;
  createdAt: Scalars['Time']['output'];
  groups: Array<AttributeGroup>;
  id: Scalars['ID']['output'];
  isDigital: Scalars['Boolean']['output'];
  isShippingRequired: Scalars['Boolean']['output'];
  metadata: Array<MetadataItem>;
  name: Scalars['String']['output'];
  privateMetadata: Array<MetadataItem>;
  reviewQuestions: Array<ReviewQuestion>;
  slug: Scalars['String']['output'];
  tracksInventory: Scalars['Boolean']['output'];
  updatedAt: Scalars['Time']['output'];
  urlTemplate: Scalars['String']['output'];
};

export type ProductTypeAttribute = {
  __typename?: 'ProductTypeAttribute';
  attribute: Attribute;
  groupId?: Maybe<Scalars['ID']['output']>;
  sortOrder: Scalars['Int']['output'];
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

export type PromotionAction = {
  __typename?: 'PromotionAction';
  id: Scalars['ID']['output'];
  params: Scalars['JSON']['output'];
  paramsVersion: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  type: Scalars['String']['output'];
};

export type PromotionActionInput = {
  params: Scalars['JSON']['input'];
  paramsVersion?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  type: Scalars['String']['input'];
};

export type PromotionAnalyticsDailyPoint = {
  __typename?: 'PromotionAnalyticsDailyPoint';
  date: Scalars['String']['output'];
  discountTotalCents: Scalars['Int64']['output'];
  redemptionCount: Scalars['Int']['output'];
};

export type PromotionAnalyticsTotals = {
  __typename?: 'PromotionAnalyticsTotals';
  discountTotalCents: Scalars['Int64']['output'];
  redemptionCount: Scalars['Int']['output'];
  uniqueCustomers: Scalars['Int']['output'];
};

export type PromotionCode = {
  __typename?: 'PromotionCode';
  code: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  maxUses?: Maybe<Scalars['Int']['output']>;
  perCustomerLimit?: Maybe<Scalars['Int']['output']>;
  restrictedToEmail: Scalars['String']['output'];
  ruleId: Scalars['ID']['output'];
  updatedAt: Scalars['Time']['output'];
  usedCount: Scalars['Int']['output'];
  validFrom?: Maybe<Scalars['Time']['output']>;
  validTo?: Maybe<Scalars['Time']['output']>;
};

export type PromotionCodeList = {
  __typename?: 'PromotionCodeList';
  items: Array<PromotionCode>;
  totalCount: Scalars['Int']['output'];
};

export type PromotionCondition = {
  __typename?: 'PromotionCondition';
  id: Scalars['ID']['output'];
  params: Scalars['JSON']['output'];
  paramsVersion: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  type: Scalars['String']['output'];
};

export type PromotionConditionInput = {
  params: Scalars['JSON']['input'];
  paramsVersion?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  type: Scalars['String']['input'];
};

export type PromotionRule = {
  __typename?: 'PromotionRule';
  actions: Array<PromotionAction>;
  bannerMedia?: Maybe<MediaAsset>;
  bannerMediaId?: Maybe<Scalars['ID']['output']>;
  channelId?: Maybe<Scalars['ID']['output']>;
  codes: PromotionCodeList;
  conditions: Array<PromotionCondition>;
  contents: Array<PromotionRuleContent>;
  createdAt: Scalars['Time']['output'];
  createdByAdminId?: Maybe<Scalars['ID']['output']>;
  description: Scalars['String']['output'];
  exclusivityGroup: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  labelMedia?: Maybe<MediaAsset>;
  labelMediaId?: Maybe<Scalars['ID']['output']>;
  maxUses?: Maybe<Scalars['Int']['output']>;
  name: Scalars['String']['output'];
  perCustomerLimit?: Maybe<Scalars['Int']['output']>;
  pillBgColor: Scalars['String']['output'];
  pillText: Scalars['String']['output'];
  pillTextColor: Scalars['String']['output'];
  priority: Scalars['Int']['output'];
  publicLabel: Scalars['String']['output'];
  requiresCode: Scalars['Boolean']['output'];
  scheduleJson?: Maybe<Scalars['JSON']['output']>;
  slug: Scalars['String']['output'];
  stopAfterApply: PromotionStopMode;
  updatedAt: Scalars['Time']['output'];
  updatedByAdminId?: Maybe<Scalars['ID']['output']>;
  usedCount: Scalars['Int']['output'];
  validFrom?: Maybe<Scalars['Time']['output']>;
  validTo?: Maybe<Scalars['Time']['output']>;
};


export type PromotionRuleCodesArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};

export type PromotionRuleAnalytics = {
  __typename?: 'PromotionRuleAnalytics';
  discountTotalCents: Scalars['Int64']['output'];
  orderRevenueCents: Scalars['Int64']['output'];
  redemptionCount: Scalars['Int']['output'];
  ruleId: Scalars['ID']['output'];
  ruleName: Scalars['String']['output'];
  ruleSlug: Scalars['String']['output'];
  uniqueCustomers: Scalars['Int']['output'];
};

export type PromotionRuleContent = {
  __typename?: 'PromotionRuleContent';
  label: Scalars['String']['output'];
  locale: Scalars['String']['output'];
  terms: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type PromotionRuleList = {
  __typename?: 'PromotionRuleList';
  items: Array<PromotionRule>;
  totalCount: Scalars['Int']['output'];
};

export type PromotionSimulationInput = {
  appliedCodes?: InputMaybe<Array<Scalars['String']['input']>>;
  countryCode?: InputMaybe<Scalars['String']['input']>;
  currency?: InputMaybe<Scalars['String']['input']>;
  customerGroupSlug?: InputMaybe<Scalars['String']['input']>;
  customerSegmentSlugs?: InputMaybe<Array<Scalars['String']['input']>>;
  lines: Array<PromotionSimulationLineInput>;
  subtotalCents: Scalars['Int']['input'];
};

export type PromotionSimulationLineInput = {
  categoryIds?: InputMaybe<Array<Scalars['ID']['input']>>;
  productKind?: InputMaybe<Scalars['String']['input']>;
  productSku: Scalars['String']['input'];
  quantity: Scalars['Float']['input'];
  unitPriceCents: Scalars['Int']['input'];
};

export type PromotionSimulationResult = {
  __typename?: 'PromotionSimulationResult';
  appliedRuleIds: Array<Scalars['ID']['output']>;
  freeShipping: Scalars['Boolean']['output'];
  subtotalDiscountCents: Scalars['Int']['output'];
  trace: Array<PromotionTraceEntry>;
};

export type PromotionStopMode =
  | 'ALL'
  | 'NONE'
  | 'SAME_ACTION_TYPE';

export type PromotionTraceEntry = {
  __typename?: 'PromotionTraceEntry';
  discountCents: Scalars['Int']['output'];
  matched: Scalars['Boolean']['output'];
  priority: Scalars['Int']['output'];
  ruleId: Scalars['ID']['output'];
  ruleSlug: Scalars['String']['output'];
  skipReason: Scalars['String']['output'];
  stopAfterApply: PromotionStopMode;
};

export type PromotionTypeDescriptor = {
  __typename?: 'PromotionTypeDescriptor';
  description: Scalars['String']['output'];
  label: Scalars['String']['output'];
  paramsSchema: Scalars['JSON']['output'];
  slug: Scalars['String']['output'];
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
  admin: AdminUser;
  adminActions: Array<AdminAction>;
  admins: Array<AdminUser>;
  aiStatus: AiStatus;
  allMediaDirectories: Array<MediaDirectoryChild>;
  attribute: Attribute;
  attributeValueChannelSlugs: Array<AttributeValueChannelSlug>;
  attributes: Array<Attribute>;
  auditLog: AuditLogPage;
  blogAuthor?: Maybe<BlogAuthor>;
  blogAuthors: Array<BlogAuthor>;
  blogCategories: Array<BlogCategory>;
  blogCategory?: Maybe<BlogCategory>;
  blogPost?: Maybe<BlogPost>;
  blogPosts: BlogPostList;
  blogTag?: Maybe<BlogTag>;
  blogTags: Array<BlogTag>;
  botTraffic: BotTraffic;
  cacheRevalidations: Array<CacheRevalidation>;
  cacheSurfaces: Array<CacheSurface>;
  campaignAudienceEstimate: CampaignAudienceEstimate;
  campaignMergeTags: Array<Scalars['String']['output']>;
  campaignVariantStats: Array<CampaignVariantStat>;
  cart: AdminCart;
  carts: CartList;
  category: Category;
  categoryChildren: Array<Category>;
  channel: Channel;
  channels: Array<Channel>;
  chatConversation?: Maybe<ChatConversation>;
  chatConversations: Array<ChatConversationSummary>;
  cmsAttribute?: Maybe<CmsAttribute>;
  cmsAttributeGroups: Array<CmsAttributeGroup>;
  cmsAttributes: Array<CmsAttribute>;
  cmsBlockType?: Maybe<CmsBlockType>;
  cmsBlockTypes: Array<CmsBlockType>;
  cmsPage?: Maybe<CmsPage>;
  cmsPageComments: Array<CmsPageComment>;
  cmsPageRevisions: Array<CmsPageRevision>;
  cmsPageType?: Maybe<CmsPageType>;
  cmsPageTypes: Array<CmsPageType>;
  cmsPages: Array<CmsPage>;
  cmsSnippet?: Maybe<CmsSnippet>;
  cmsSnippets: Array<CmsSnippet>;
  cmsTags: Array<CmsTag>;
  collection: Collection;
  collections: Array<Collection>;
  config: Array<ConfigValue>;
  configSections: Array<ConfigSection>;
  configValue: ConfigValue;
  countries: Array<Country>;
  country: Country;
  crawlAudit?: Maybe<CrawlAudit>;
  crawlDiff?: Maybe<CrawlDiff>;
  creditMemos: Array<CreditMemo>;
  creditableQuantities: Array<OrderLineQuantity>;
  currencies: Array<Currency>;
  currency: Currency;
  customer: Customer;
  customerCommerce: CustomerCommerce;
  customerGroup: CustomerGroup;
  customerGroups: Array<CustomerGroup>;
  customerProfile: CustomerProfile;
  customerSegment?: Maybe<CustomerSegment>;
  customerSegments: CustomerSegmentList;
  customerSignals: CustomerSignals;
  customers: Array<Customer>;
  customersPaged: CustomerPage;
  dataFormats: Array<DataFormat>;
  dataJob?: Maybe<DataJob>;
  dataJobErrors: DataJobErrorList;
  dataJobs: DataJobList;
  dataResource?: Maybe<DataResource>;
  /** Every resource the caller may at least export, with the columns and options its forms are built from */
  dataResources: Array<DataResource>;
  devAdmins: Array<AdminUser>;
  emailBlocks: Array<EmailBlockSnippet>;
  emailCampaign?: Maybe<EmailCampaign>;
  emailCampaigns: Array<EmailCampaign>;
  emailGroups: Array<EmailGroup>;
  emailPreflight: Array<EmailWarning>;
  emailRevisions: Array<EmailRevision>;
  emailSuppressions: EmailSuppressionPage;
  eventDestinations: Array<EventDestination>;
  experiment?: Maybe<Experiment>;
  experimentGoalEvents: Array<Scalars['String']['output']>;
  experimentResults: ExperimentResults;
  experimentSegments: Array<SegmentOption>;
  experiments: Array<Experiment>;
  experimentsOverview: ExperimentsOverview;
  exportCustomerData: Scalars['String']['output'];
  flow?: Maybe<Flow>;
  flowMergeTags: Array<Scalars['String']['output']>;
  flowTriggerTypes: Array<Scalars['String']['output']>;
  flows: Array<Flow>;
  giftCard?: Maybe<GiftCard>;
  giftCards: Array<GiftCard>;
  integrations: Array<Integration>;
  inventoryLocation: InventoryLocation;
  inventoryLocations: Array<InventoryLocation>;
  inventoryLocationsCount: Scalars['Int']['output'];
  inventoryMode: InventoryMode;
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
  libraryEmail?: Maybe<LibraryEmail>;
  libraryEmails: Array<LibraryEmail>;
  linkGraph?: Maybe<LinkGraph>;
  linkables: Array<Linkable>;
  locale: Locale;
  locales: Array<Locale>;
  lowStockLevels: Array<LowStockLevel>;
  loyalty: LoyaltySummary;
  me: AdminUser;
  mediaAsset: MediaAsset;
  mediaDirectory: MediaDirectory;
  menu: Menu;
  menuBySlug?: Maybe<Menu>;
  menus: Array<Menu>;
  myPreferences: Array<AdminPreference>;
  notificationUnreadCount: Scalars['Int']['output'];
  notifications: Array<AdminNotification>;
  order: Order;
  orderDocumentPdf: Scalars['String']['output'];
  orders: Array<Order>;
  ordersPaged: OrderPage;
  paymentMethods: Array<PaymentMethodAdmin>;
  permissionGroups: Array<PermissionGroup>;
  previewSchedule: SchedulePreview;
  product: Product;
  productAttributeLocaleValues: Array<ProductAttributeLocaleValue>;
  productAttributeSlugs: Array<ProductAttributeSlug>;
  productInventory?: Maybe<InventoryItem>;
  productLinks: Array<Scalars['ID']['output']>;
  productQuestions: Array<AdminProductQuestion>;
  productSales: ProductSales;
  productType: ProductType;
  productTypes: Array<ProductType>;
  products: Array<Product>;
  productsByIds: Array<Product>;
  productsPaged: ProductPage;
  promotionActionTypes: Array<PromotionTypeDescriptor>;
  promotionAnalytics: Array<PromotionRuleAnalytics>;
  promotionAnalyticsDaily: Array<PromotionAnalyticsDailyPoint>;
  promotionAnalyticsTotals: PromotionAnalyticsTotals;
  promotionConditionTypes: Array<PromotionTypeDescriptor>;
  promotionRule?: Maybe<PromotionRule>;
  promotionRules: PromotionRuleList;
  pulseEngagement: PulseEngagement;
  rebuildQueue: RebuildQueueStatus;
  referenceCandidates: Array<Linkable>;
  remoteProvider: RemoteProvider;
  remoteProviders: Array<RemoteProvider>;
  renderCampaignPreview: EmailPreview;
  renderLibraryPreview: EmailPreview;
  resolveUrl?: Maybe<UrlResolveResult>;
  review: Review;
  reviewCategories: Array<ReviewCategory>;
  reviewCategory: ReviewCategory;
  reviewCounts: ReviewCounts;
  reviewInsights: ReviewInsights;
  reviewModerationTags: Array<ReviewModerationTag>;
  reviewQuestion: ReviewQuestion;
  reviewQuestions: Array<ReviewQuestion>;
  reviewQuestionsForProduct: Array<ReviewQuestion>;
  reviews: ReviewListPage;
  role: Role;
  roles: Array<Role>;
  rootCategories: Array<Category>;
  salesReport: SalesReport;
  scheduledJobRuns: Array<ScheduledJobRun>;
  scheduledJobs: Array<ScheduledJob>;
  searchMediaAssets: MediaAssetSearchResult;
  searchMerchandising: SearchMerchandising;
  serverFeatures: ServerFeatures;
  shippingBoxes: Array<ShippingBox>;
  shippingCarriers: Array<ShippingCarrierAdmin>;
  shippingZone?: Maybe<ShippingZone>;
  shippingZones: Array<ShippingZone>;
  signalsInsights: Array<SignalsInsight>;
  signupForm?: Maybe<SignupForm>;
  signupFormSubmissions: Array<SignupFormSubmission>;
  signupForms: Array<SignupForm>;
  simulatePromotion: PromotionSimulationResult;
  siteCrawls: Array<SiteCrawl>;
  storeCredit: StoreCreditSummary;
  subscription?: Maybe<CustomerSubscription>;
  subscriptions: Array<CustomerSubscription>;
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
  systemHealth: SystemHealth;
  taxConditionTypes: Array<TaxTypeDescriptor>;
  taxEffectTypes: Array<TaxTypeDescriptor>;
  taxRule?: Maybe<TaxRule>;
  taxRules: Array<TaxRule>;
  transactionalSlots: Array<TransactionalSlot>;
  urlRewrite?: Maybe<UrlRewrite>;
  urlRewrites: UrlRewritePage;
  visitorProfile: CustomerProfile;
  webForm?: Maybe<WebForm>;
  webFormSubmissionCount: Scalars['Int']['output'];
  webFormSubmissions: Array<WebFormSubmission>;
  webForms: Array<WebForm>;
  webhook: Webhook;
  webhookDeadLetters: Array<WebhookQueueItem>;
  webhookDeliveries: Array<WebhookDelivery>;
  webhookDeliveryStats: WebhookDeliveryStats;
  webhookEventTypes: Array<WebhookEventType>;
  webhookJobConfig: WebhookJobConfig;
  webhooks: Array<Webhook>;
};


export type QueryAdminArgs = {
  id: Scalars['ID']['input'];
};


export type QueryAttributeArgs = {
  id: Scalars['ID']['input'];
};


export type QueryAttributeValueChannelSlugsArgs = {
  valueId: Scalars['ID']['input'];
};


export type QueryAuditLogArgs = {
  action?: InputMaybe<Scalars['String']['input']>;
  adminId?: InputMaybe<Scalars['ID']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryBlogAuthorArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryBlogCategoriesArgs = {
  parentId?: InputMaybe<Scalars['ID']['input']>;
};


export type QueryBlogCategoryArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryBlogPostArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  includeUnpublished?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryBlogPostsArgs = {
  filter?: InputMaybe<BlogPostFilter>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryBlogTagArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryBotTrafficArgs = {
  bot?: InputMaybe<Scalars['String']['input']>;
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCacheRevalidationsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCampaignAudienceEstimateArgs = {
  audienceId?: InputMaybe<Scalars['ID']['input']>;
  audienceKind: Scalars['String']['input'];
  channelId: Scalars['ID']['input'];
};


export type QueryCampaignVariantStatsArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCartArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCartsArgs = {
  abandoned?: InputMaybe<Scalars['Boolean']['input']>;
  channelId: Scalars['ID']['input'];
  customerId?: InputMaybe<Scalars['ID']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
  status?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCategoryArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCategoryChildrenArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  parentId: Scalars['ID']['input'];
};


export type QueryChannelArgs = {
  id: Scalars['ID']['input'];
};


export type QueryChatConversationArgs = {
  id: Scalars['ID']['input'];
};


export type QueryChatConversationsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCmsAttributeArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCmsBlockTypeArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCmsPageArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
  urlPath?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCmsPageCommentsArgs = {
  pageId: Scalars['ID']['input'];
};


export type QueryCmsPageRevisionsArgs = {
  locale: Scalars['String']['input'];
  pageId: Scalars['ID']['input'];
};


export type QueryCmsPageTypeArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCmsPagesArgs = {
  filter?: InputMaybe<CmsPageFilter>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCmsSnippetArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCollectionArgs = {
  id: Scalars['ID']['input'];
};


export type QueryConfigArgs = {
  scope?: InputMaybe<ConfigScopeInput>;
};


export type QueryConfigSectionsArgs = {
  scope?: InputMaybe<ConfigScopeInput>;
};


export type QueryConfigValueArgs = {
  path: Scalars['String']['input'];
  scope?: InputMaybe<ConfigScopeInput>;
};


export type QueryCountriesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryCountryArgs = {
  code: Scalars['String']['input'];
};


export type QueryCrawlAuditArgs = {
  channelId: Scalars['ID']['input'];
  crawlId?: InputMaybe<Scalars['ID']['input']>;
  issue?: InputMaybe<SeoIssue>;
  kind?: InputMaybe<CrawlPageKind>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCrawlDiffArgs = {
  baseId?: InputMaybe<Scalars['ID']['input']>;
  channelId: Scalars['ID']['input'];
  currentId?: InputMaybe<Scalars['ID']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  regressionsOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryCreditMemosArgs = {
  orderId: Scalars['ID']['input'];
};


export type QueryCreditableQuantitiesArgs = {
  orderId: Scalars['ID']['input'];
};


export type QueryCurrenciesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryCurrencyArgs = {
  code: Scalars['String']['input'];
};


export type QueryCustomerArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCustomerCommerceArgs = {
  customerId: Scalars['ID']['input'];
};


export type QueryCustomerGroupArgs = {
  id: Scalars['ID']['input'];
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


export type QueryCustomerSignalsArgs = {
  customerId: Scalars['ID']['input'];
};


export type QueryCustomersArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type QueryCustomersPagedArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  groupSlug?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};


export type QueryDataJobArgs = {
  id: Scalars['ID']['input'];
};


export type QueryDataJobErrorsArgs = {
  id: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryDataJobsArgs = {
  direction?: InputMaybe<Scalars['String']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  resources?: InputMaybe<Array<Scalars['String']['input']>>;
  statuses?: InputMaybe<Array<Scalars['String']['input']>>;
};


export type QueryDataResourceArgs = {
  slug: Scalars['String']['input'];
};


export type QueryEmailBlocksArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryEmailCampaignArgs = {
  id: Scalars['ID']['input'];
};


export type QueryEmailCampaignsArgs = {
  channelId: Scalars['ID']['input'];
  includeDeleted?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryEmailPreflightArgs = {
  id: Scalars['ID']['input'];
};


export type QueryEmailRevisionsArgs = {
  id: Scalars['ID']['input'];
};


export type QueryEmailSuppressionsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  reason?: InputMaybe<Scalars['String']['input']>;
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


export type QueryExportCustomerDataArgs = {
  customerId: Scalars['ID']['input'];
};


export type QueryFlowArgs = {
  id: Scalars['ID']['input'];
};


export type QueryFlowsArgs = {
  channelId: Scalars['ID']['input'];
  includeDeleted?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryGiftCardArgs = {
  code: Scalars['String']['input'];
};


export type QueryGiftCardsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryInventoryLocationArgs = {
  id: Scalars['ID']['input'];
};


export type QueryInventoryLocationsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};


export type QueryInventoryLocationsCountArgs = {
  search?: InputMaybe<Scalars['String']['input']>;
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


export type QueryLibraryEmailArgs = {
  id: Scalars['ID']['input'];
};


export type QueryLibraryEmailsArgs = {
  channelId: Scalars['ID']['input'];
  groupSlug?: InputMaybe<Scalars['String']['input']>;
};


export type QueryLinkGraphArgs = {
  channelId: Scalars['ID']['input'];
  crawlId?: InputMaybe<Scalars['ID']['input']>;
  maxNodes?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLinkablesArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};


export type QueryLocaleArgs = {
  code: Scalars['String']['input'];
};


export type QueryLocalesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryLowStockLevelsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryLoyaltyArgs = {
  customerId: Scalars['ID']['input'];
};


export type QueryMediaAssetArgs = {
  id: Scalars['ID']['input'];
};


export type QueryMediaDirectoryArgs = {
  path: Scalars['String']['input'];
};


export type QueryMenuArgs = {
  id: Scalars['ID']['input'];
};


export type QueryMenuBySlugArgs = {
  slug: Scalars['String']['input'];
};


export type QueryNotificationsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  unreadOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryOrderArgs = {
  id: Scalars['ID']['input'];
};


export type QueryOrderDocumentPdfArgs = {
  creditMemoId?: InputMaybe<Scalars['ID']['input']>;
  kind: OrderDocumentKind;
  orderId: Scalars['ID']['input'];
};


export type QueryOrdersArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type QueryOrdersPagedArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  paymentStatuses?: InputMaybe<Array<Scalars['String']['input']>>;
  search?: InputMaybe<Scalars['String']['input']>;
  statuses?: InputMaybe<Array<Scalars['String']['input']>>;
};


export type QueryPaymentMethodsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryPreviewScheduleArgs = {
  count?: InputMaybe<Scalars['Int']['input']>;
  schedule: Scalars['String']['input'];
  timezone?: InputMaybe<Scalars['String']['input']>;
};


export type QueryProductArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProductAttributeLocaleValuesArgs = {
  productId: Scalars['ID']['input'];
};


export type QueryProductAttributeSlugsArgs = {
  productId: Scalars['ID']['input'];
};


export type QueryProductInventoryArgs = {
  productId: Scalars['ID']['input'];
};


export type QueryProductLinksArgs = {
  kind: ProductLinkKind;
  productId: Scalars['ID']['input'];
};


export type QueryProductQuestionsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  status?: InputMaybe<Scalars['String']['input']>;
};


export type QueryProductSalesArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  days?: InputMaybe<Scalars['Int']['input']>;
  sku: Scalars['String']['input'];
};


export type QueryProductTypeArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProductsArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type QueryProductsByIdsArgs = {
  ids: Array<Scalars['ID']['input']>;
};


export type QueryProductsPagedArgs = {
  attributeValueIds?: InputMaybe<Array<Scalars['ID']['input']>>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  kinds?: InputMaybe<Array<ProductKind>>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  productTypeSlugs?: InputMaybe<Array<Scalars['String']['input']>>;
  published?: InputMaybe<Scalars['Boolean']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
  sortDesc?: InputMaybe<Scalars['Boolean']['input']>;
  sortKey?: InputMaybe<Scalars['String']['input']>;
};


export type QueryPromotionAnalyticsArgs = {
  end?: InputMaybe<Scalars['Time']['input']>;
  start?: InputMaybe<Scalars['Time']['input']>;
};


export type QueryPromotionAnalyticsDailyArgs = {
  end?: InputMaybe<Scalars['Time']['input']>;
  ruleId?: InputMaybe<Scalars['ID']['input']>;
  start?: InputMaybe<Scalars['Time']['input']>;
};


export type QueryPromotionAnalyticsTotalsArgs = {
  end?: InputMaybe<Scalars['Time']['input']>;
  start?: InputMaybe<Scalars['Time']['input']>;
};


export type QueryPromotionRuleArgs = {
  id: Scalars['ID']['input'];
};


export type QueryPromotionRulesArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryPulseEngagementArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryRebuildQueueArgs = {
  recentLimit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryReferenceCandidatesArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
  target: AttributeReferenceTarget;
};


export type QueryRemoteProviderArgs = {
  id: Scalars['ID']['input'];
};


export type QueryRemoteProvidersArgs = {
  kind?: InputMaybe<RemoteProviderKind>;
};


export type QueryRenderCampaignPreviewArgs = {
  blocks: Scalars['JSON']['input'];
  channelId: Scalars['ID']['input'];
  styles?: InputMaybe<Scalars['JSON']['input']>;
  subject: Scalars['String']['input'];
};


export type QueryRenderLibraryPreviewArgs = {
  blocks: Scalars['JSON']['input'];
  channelId: Scalars['ID']['input'];
  customerId?: InputMaybe<Scalars['ID']['input']>;
  preheader?: InputMaybe<Scalars['String']['input']>;
  styles?: InputMaybe<Scalars['JSON']['input']>;
  subject: Scalars['String']['input'];
};


export type QueryResolveUrlArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  path: Scalars['String']['input'];
};


export type QueryReviewArgs = {
  id: Scalars['ID']['input'];
};


export type QueryReviewCategoryArgs = {
  id: Scalars['ID']['input'];
};


export type QueryReviewCountsArgs = {
  productId?: InputMaybe<Scalars['ID']['input']>;
};


export type QueryReviewInsightsArgs = {
  bucket?: InputMaybe<Scalars['String']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  endDate?: InputMaybe<Scalars['Time']['input']>;
  startDate?: InputMaybe<Scalars['Time']['input']>;
};


export type QueryReviewQuestionArgs = {
  id: Scalars['ID']['input'];
};


export type QueryReviewQuestionsForProductArgs = {
  productId: Scalars['ID']['input'];
};


export type QueryReviewsArgs = {
  filter?: InputMaybe<ReviewFilter>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryRoleArgs = {
  id: Scalars['ID']['input'];
};


export type QueryRootCategoriesArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type QuerySalesReportArgs = {
  bucket?: InputMaybe<Scalars['String']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  endDate?: InputMaybe<Scalars['Time']['input']>;
  startDate?: InputMaybe<Scalars['Time']['input']>;
  topProductsLimit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryScheduledJobRunsArgs = {
  jobId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySearchMediaAssetsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  mimeGroup?: InputMaybe<MediaMimeGroup>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  path?: InputMaybe<Scalars['String']['input']>;
  query?: InputMaybe<Scalars['String']['input']>;
};


export type QuerySearchMerchandisingArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryShippingBoxesArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryShippingCarriersArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryShippingZoneArgs = {
  id: Scalars['ID']['input'];
};


export type QueryShippingZonesArgs = {
  channelId: Scalars['ID']['input'];
};


export type QuerySignalsInsightsArgs = {
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySignupFormArgs = {
  id: Scalars['ID']['input'];
};


export type QuerySignupFormSubmissionsArgs = {
  formId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QuerySignupFormsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QuerySimulatePromotionArgs = {
  input: PromotionSimulationInput;
};


export type QuerySiteCrawlsArgs = {
  channelId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryStoreCreditArgs = {
  customerId: Scalars['ID']['input'];
};


export type QuerySubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type QuerySubscriptionsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  status?: InputMaybe<SubscriptionStatus>;
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


export type QueryTaxRuleArgs = {
  id: Scalars['ID']['input'];
};


export type QueryTaxRulesArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type QueryTransactionalSlotsArgs = {
  channelId: Scalars['ID']['input'];
};


export type QueryUrlRewriteArgs = {
  id: Scalars['ID']['input'];
};


export type QueryUrlRewritesArgs = {
  autogenerated?: InputMaybe<Scalars['Boolean']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  entityId?: InputMaybe<Scalars['ID']['input']>;
  entityType?: InputMaybe<UrlEntityType>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  redirectTypes?: InputMaybe<Array<Scalars['Int']['input']>>;
  search?: InputMaybe<Scalars['String']['input']>;
};


export type QueryVisitorProfileArgs = {
  anonId: Scalars['String']['input'];
  channelId: Scalars['ID']['input'];
  sinceDays?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryWebFormArgs = {
  id: Scalars['ID']['input'];
};


export type QueryWebFormSubmissionCountArgs = {
  formId: Scalars['ID']['input'];
};


export type QueryWebFormSubmissionsArgs = {
  formId: Scalars['ID']['input'];
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryWebFormsArgs = {
  channelId: Scalars['ID']['input'];
  includeDeleted?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryWebhookArgs = {
  id: Scalars['ID']['input'];
};


export type QueryWebhookDeadLettersArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  webhookId: Scalars['ID']['input'];
};


export type QueryWebhookDeliveriesArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  webhookId: Scalars['ID']['input'];
};


export type QueryWebhookDeliveryStatsArgs = {
  webhookId: Scalars['ID']['input'];
};


export type QueryWebhookJobConfigArgs = {
  id: Scalars['ID']['input'];
};

export type RebuildQueueStatus = {
  __typename?: 'RebuildQueueStatus';
  done: Scalars['Int']['output'];
  failed: Scalars['Int']['output'];
  pending: Scalars['Int']['output'];
  recent: Array<RebuildTask>;
  running: Scalars['Int']['output'];
};

export type RebuildTask = {
  __typename?: 'RebuildTask';
  attempts: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  lastError: Scalars['String']['output'];
  path: Scalars['String']['output'];
  reason: Scalars['String']['output'];
  status: Scalars['String']['output'];
  surface: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type RecordOfflinePaymentInput = {
  amount?: InputMaybe<Scalars['Int']['input']>;
  method?: InputMaybe<Scalars['String']['input']>;
  note?: InputMaybe<Scalars['String']['input']>;
  orderId: Scalars['ID']['input'];
  receivedAt?: InputMaybe<Scalars['Time']['input']>;
  reference?: InputMaybe<Scalars['String']['input']>;
};

export type RegenerateIntegrationTokenPayload = {
  __typename?: 'RegenerateIntegrationTokenPayload';
  integration: Integration;
  rawToken: Scalars['String']['output'];
};

export type Region = {
  __typename?: 'Region';
  code: Scalars['String']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type RemoteProvider = {
  __typename?: 'RemoteProvider';
  cacheTtlS: Scalars['Int']['output'];
  channel?: Maybe<Channel>;
  circuit: WebhookCircuit;
  config?: Maybe<Scalars['JSON']['output']>;
  createdAt: Scalars['Time']['output'];
  failurePolicy: RemoteProviderFailurePolicy;
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  kind: RemoteProviderKind;
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  targetUrl: Scalars['String']['output'];
  timeoutMs: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type RemoteProviderFailurePolicy =
  | 'FAIL_CLOSED'
  | 'FAIL_OPEN';

export type RemoteProviderKind =
  | 'ADDRESS_VALIDATOR'
  | 'CURRENCY_RATE'
  | 'CUSTOMER_SEGMENTS'
  | 'FLOW_ACTION'
  | 'FRAUD_RISK'
  | 'INVENTORY_AVAILABILITY'
  | 'PRICING'
  | 'PROMOTION_ACTION'
  | 'PROMOTION_CONDITION'
  | 'RECOMMENDATION'
  | 'RETURNS_POLICY'
  | 'REVIEWS'
  | 'SEARCH'
  | 'SHIPPING_CARRIER'
  | 'SHIPPING_TRACKING'
  | 'TAX_PROVIDER';

export type ReorderMenuItemInput = {
  id: Scalars['ID']['input'];
  parentId?: InputMaybe<Scalars['ID']['input']>;
  sortOrder: Scalars['Int']['input'];
};

export type RequestCategoryCoverUploadInput = {
  altText?: InputMaybe<Scalars['String']['input']>;
  categoryId: Scalars['ID']['input'];
  filename: Scalars['String']['input'];
  mimeType: Scalars['String']['input'];
  sizeBytes: Scalars['Int']['input'];
};

export type RequestCollectionCoverUploadInput = {
  altText?: InputMaybe<Scalars['String']['input']>;
  collectionId: Scalars['ID']['input'];
  filename: Scalars['String']['input'];
  mimeType: Scalars['String']['input'];
  sizeBytes: Scalars['Int']['input'];
};

export type RequestCoverUploadPayload = {
  __typename?: 'RequestCoverUploadPayload';
  collection: Collection;
  expiresAt: Scalars['Time']['output'];
  uploadUrl: Scalars['String']['output'];
};

export type RequestMediaAssetUploadInput = {
  altText?: InputMaybe<Scalars['String']['input']>;
  caption?: InputMaybe<Scalars['String']['input']>;
  filename: Scalars['String']['input'];
  mimeType: Scalars['String']['input'];
  notes?: InputMaybe<Scalars['String']['input']>;
  path: Scalars['String']['input'];
  sizeBytes: Scalars['Int']['input'];
  title?: InputMaybe<Scalars['String']['input']>;
};

export type RequestMediaAssetUploadPayload = {
  __typename?: 'RequestMediaAssetUploadPayload';
  asset: MediaAsset;
  expiresAt: Scalars['Time']['output'];
  uploadUrl: Scalars['String']['output'];
};

export type RequestMediaUploadPayload = {
  __typename?: 'RequestMediaUploadPayload';
  expiresAt: Scalars['Time']['output'];
  media: ProductMedia;
  uploadUrl: Scalars['String']['output'];
};

export type RequestProductMediaUploadInput = {
  altText?: InputMaybe<Scalars['String']['input']>;
  filename: Scalars['String']['input'];
  mimeType: Scalars['String']['input'];
  productId: Scalars['ID']['input'];
  role?: InputMaybe<ProductMediaRole>;
  sizeBytes: Scalars['Int']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  target?: InputMaybe<ProductMediaUploadTarget>;
};

export type RequestReturnInput = {
  lines: Array<ReturnLineInput>;
  orderId: Scalars['ID']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
  refundTarget?: InputMaybe<Scalars['String']['input']>;
  resolution?: InputMaybe<Scalars['String']['input']>;
  restock?: InputMaybe<Scalars['Boolean']['input']>;
};

export type ReservationReason =
  | 'MANUAL_ADJUSTMENT'
  | 'ORDER_CANCELLED'
  | 'ORDER_FULFILLED'
  | 'ORDER_PLACED';

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

export type ReturnLineInput = {
  orderLineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
};

export type Review = {
  __typename?: 'Review';
  answers: Array<ReviewAnswer>;
  body: Scalars['String']['output'];
  categorySlugs: Array<Scalars['String']['output']>;
  channelId: Scalars['ID']['output'];
  customerId?: Maybe<Scalars['ID']['output']>;
  guestEmail: Scalars['String']['output'];
  guestName: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isVerifiedPurchase: Scalars['Boolean']['output'];
  locale: Scalars['String']['output'];
  media: Array<ReviewMedia>;
  merchantReply: Scalars['String']['output'];
  merchantReplyAt?: Maybe<Scalars['Time']['output']>;
  moderatedAt?: Maybe<Scalars['Time']['output']>;
  moderatedBy?: Maybe<Scalars['ID']['output']>;
  moderationLabel: Scalars['String']['output'];
  moderationReason: Scalars['String']['output'];
  moderationTags: Array<Scalars['String']['output']>;
  moderatorNote: Scalars['String']['output'];
  productId: Scalars['ID']['output'];
  status: ReviewStatus;
  submittedAt: Scalars['Time']['output'];
  title: Scalars['String']['output'];
};

export type ReviewAnswer = {
  __typename?: 'ReviewAnswer';
  boolValue?: Maybe<Scalars['Boolean']['output']>;
  question: ReviewQuestion;
  rating?: Maybe<Scalars['Int']['output']>;
  text: Scalars['String']['output'];
};

export type ReviewCategory = {
  __typename?: 'ReviewCategory';
  createdAt: Scalars['Time']['output'];
  description: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ReviewCategoryStat = {
  __typename?: 'ReviewCategoryStat';
  count: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  prevCount: Scalars['Int']['output'];
  slug: Scalars['String']['output'];
};

export type ReviewCounts = {
  __typename?: 'ReviewCounts';
  approved: Scalars['Int']['output'];
  pending: Scalars['Int']['output'];
  rejected: Scalars['Int']['output'];
  total: Scalars['Int']['output'];
};

export type ReviewFilter = {
  categorySlugs?: InputMaybe<Array<Scalars['String']['input']>>;
  productId?: InputMaybe<Scalars['ID']['input']>;
  status?: InputMaybe<Array<ReviewStatus>>;
};

export type ReviewInsightPoint = {
  __typename?: 'ReviewInsightPoint';
  count: Scalars['Int']['output'];
  period: Scalars['Time']['output'];
  slug: Scalars['String']['output'];
};

export type ReviewInsights = {
  __typename?: 'ReviewInsights';
  byCategory: Array<ReviewCategoryStat>;
  byStatus: Array<ReviewStatusStat>;
  byTag: Array<ReviewTagStat>;
  prevTotal: Scalars['Int']['output'];
  timeseries: Array<ReviewInsightPoint>;
  total: Scalars['Int']['output'];
  volume: Array<ReviewVolumePoint>;
};

export type ReviewListPage = {
  __typename?: 'ReviewListPage';
  items: Array<Review>;
  totalCount: Scalars['Int']['output'];
};

export type ReviewMedia = {
  __typename?: 'ReviewMedia';
  height?: Maybe<Scalars['Int']['output']>;
  id: Scalars['ID']['output'];
  mimeType: Scalars['String']['output'];
  sizeBytes: Scalars['Int']['output'];
  url: Scalars['String']['output'];
  width?: Maybe<Scalars['Int']['output']>;
};

export type ReviewModerationTag = {
  __typename?: 'ReviewModerationTag';
  createdAt: Scalars['Time']['output'];
  description: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ReviewQuestion = {
  __typename?: 'ReviewQuestion';
  allContent: Array<ReviewQuestionContent>;
  allowComment: Scalars['Boolean']['output'];
  content: ReviewQuestionContent;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  inputType: ReviewQuestionInputType;
  isRequired: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ReviewQuestionContent = {
  __typename?: 'ReviewQuestionContent';
  helpText: Scalars['String']['output'];
  label: Scalars['String']['output'];
  locale: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ReviewQuestionInputType =
  | 'FREE_TEXT'
  | 'RATING_1_5'
  | 'YES_NO';

export type ReviewStatus =
  | 'APPROVED'
  | 'PENDING'
  | 'REJECTED';

export type ReviewStatusStat = {
  __typename?: 'ReviewStatusStat';
  count: Scalars['Int']['output'];
  status: ReviewStatus;
};

export type ReviewTagStat = {
  __typename?: 'ReviewTagStat';
  count: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  prevCount: Scalars['Int']['output'];
  slug: Scalars['String']['output'];
};

export type ReviewVolumePoint = {
  __typename?: 'ReviewVolumePoint';
  count: Scalars['Int']['output'];
  period: Scalars['Time']['output'];
};

export type Role = {
  __typename?: 'Role';
  createdAt: Scalars['Time']['output'];
  description: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  memberCount: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  scopes: Array<RolePermissionScope>;
  slug: Scalars['String']['output'];
};

export type RolePermissionScope = {
  __typename?: 'RolePermissionScope';
  channelId?: Maybe<Scalars['ID']['output']>;
  locale?: Maybe<Scalars['String']['output']>;
  permission: Permission;
};

export type RolePermissionScopeInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  locale?: InputMaybe<Scalars['String']['input']>;
  permissionSlug: Scalars['String']['input'];
};

export type RotateRemoteProviderSecretPayload = {
  __typename?: 'RotateRemoteProviderSecretPayload';
  provider: RemoteProvider;
  rawSecret: Scalars['String']['output'];
};

export type RotateWebhookSecretPayload = {
  __typename?: 'RotateWebhookSecretPayload';
  rawSecret: Scalars['String']['output'];
  webhook: Webhook;
};

export type SalesReport = {
  __typename?: 'SalesReport';
  statusBreakdown: Array<SalesStatusBreakdown>;
  summary: Array<SalesSummary>;
  timeseries: Array<SalesTimeseriesPoint>;
  topProducts: Array<TopProduct>;
};

export type SalesStatusBreakdown = {
  __typename?: 'SalesStatusBreakdown';
  orderCount: Scalars['Int']['output'];
  revenue: Scalars['Int64']['output'];
  status: Scalars['String']['output'];
};

export type SalesSummary = {
  __typename?: 'SalesSummary';
  averageOrderValue: Scalars['Int64']['output'];
  costTotal: Scalars['Int64']['output'];
  currencyCode: Scalars['String']['output'];
  discountTotal: Scalars['Int64']['output'];
  grossRevenue: Scalars['Int64']['output'];
  marginTotal: Scalars['Int64']['output'];
  orderCount: Scalars['Int']['output'];
  refundCount: Scalars['Int']['output'];
  refundedAmount: Scalars['Int64']['output'];
  shippingTotal: Scalars['Int64']['output'];
  subtotalTotal: Scalars['Int64']['output'];
  taxTotal: Scalars['Int64']['output'];
  uniqueCustomers: Scalars['Int']['output'];
};

export type SalesTimeseriesPoint = {
  __typename?: 'SalesTimeseriesPoint';
  currencyCode: Scalars['String']['output'];
  orderCount: Scalars['Int']['output'];
  period: Scalars['Time']['output'];
  revenue: Scalars['Int64']['output'];
};

export type SaveCampaignInput = {
  abEnabled?: InputMaybe<Scalars['Boolean']['input']>;
  abSubjectB?: InputMaybe<Scalars['String']['input']>;
  abTestMinutes?: InputMaybe<Scalars['Int']['input']>;
  abTestPercent?: InputMaybe<Scalars['Int']['input']>;
  abWinnerMetric?: InputMaybe<Scalars['String']['input']>;
  audienceId?: InputMaybe<Scalars['ID']['input']>;
  audienceKind: Scalars['String']['input'];
  blocks: Scalars['JSON']['input'];
  channelId: Scalars['ID']['input'];
  emailId?: InputMaybe<Scalars['ID']['input']>;
  name: Scalars['String']['input'];
  styles?: InputMaybe<Scalars['JSON']['input']>;
  subject: Scalars['String']['input'];
};

export type SaveEmailBlockInput = {
  blocks: Scalars['JSON']['input'];
  channelId: Scalars['ID']['input'];
  css?: InputMaybe<Scalars['String']['input']>;
  fields?: InputMaybe<Scalars['JSON']['input']>;
  htmlTemplate?: InputMaybe<Scalars['String']['input']>;
  icon?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  scripted?: InputMaybe<Scalars['Boolean']['input']>;
  slug: Scalars['String']['input'];
  textTemplate?: InputMaybe<Scalars['String']['input']>;
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

export type SaveFlowInput = {
  channelId: Scalars['ID']['input'];
  graph: Scalars['JSON']['input'];
  name: Scalars['String']['input'];
  triggerConfig?: InputMaybe<Scalars['JSON']['input']>;
  triggerType: Scalars['String']['input'];
};

export type SaveLibraryEmailInput = {
  blocks: Scalars['JSON']['input'];
  channelId: Scalars['ID']['input'];
  groupId?: InputMaybe<Scalars['ID']['input']>;
  id?: InputMaybe<Scalars['ID']['input']>;
  name: Scalars['String']['input'];
  preheader?: InputMaybe<Scalars['String']['input']>;
  styles?: InputMaybe<Scalars['JSON']['input']>;
  subject: Scalars['String']['input'];
};

export type SaveSignupFormInput = {
  body: Scalars['String']['input'];
  buttonLabel: Scalars['String']['input'];
  channelId: Scalars['ID']['input'];
  collectName: Scalars['Boolean']['input'];
  consentKind: Scalars['String']['input'];
  display: SignupFormDisplayInput;
  heading: Scalars['String']['input'];
  name: Scalars['String']['input'];
  slug: Scalars['String']['input'];
  status: Scalars['String']['input'];
  successMessage: Scalars['String']['input'];
  targetSegmentId?: InputMaybe<Scalars['ID']['input']>;
  triggerFlow: Scalars['Boolean']['input'];
  type: Scalars['String']['input'];
};

export type SaveWebFormInput = {
  fields: Scalars['JSON']['input'];
  name: Scalars['String']['input'];
  settings: Scalars['JSON']['input'];
  slug?: InputMaybe<Scalars['String']['input']>;
  status: Scalars['String']['input'];
};

export type SchedulePreview = {
  __typename?: 'SchedulePreview';
  error: Scalars['String']['output'];
  nextRuns: Array<Scalars['Time']['output']>;
  valid: Scalars['Boolean']['output'];
};

export type ScheduledJob = {
  __typename?: 'ScheduledJob';
  alertAfterFailures: Scalars['Int']['output'];
  consecutiveFailures: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  currentAttempt: Scalars['Int']['output'];
  description: Scalars['String']['output'];
  enabled: Scalars['Boolean']['output'];
  id: Scalars['ID']['output'];
  kind: Scalars['String']['output'];
  lastDurationMs?: Maybe<Scalars['Int']['output']>;
  lastError: Scalars['String']['output'];
  lastRunAt?: Maybe<Scalars['Time']['output']>;
  lastStatus: Scalars['String']['output'];
  maxAttempts: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  nextRunAt?: Maybe<Scalars['Time']['output']>;
  retryBackoffSeconds: Scalars['Int']['output'];
  schedule: Scalars['String']['output'];
  timeoutSeconds: Scalars['Int']['output'];
  timezone: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
  webhook?: Maybe<ScheduledJobWebhook>;
};

export type ScheduledJobRun = {
  __typename?: 'ScheduledJobRun';
  attempt: Scalars['Int']['output'];
  durationMs: Scalars['Int']['output'];
  error: Scalars['String']['output'];
  finishedAt: Scalars['Time']['output'];
  startedAt: Scalars['Time']['output'];
  status: Scalars['String']['output'];
};

export type ScheduledJobWebhook = {
  __typename?: 'ScheduledJobWebhook';
  hasSecret: Scalars['Boolean']['output'];
  method: Scalars['String']['output'];
  url: Scalars['String']['output'];
};

export type SearchBoost = {
  __typename?: 'SearchBoost';
  id: Scalars['ID']['output'];
  productId: Scalars['ID']['output'];
  query: Scalars['String']['output'];
  weight: Scalars['Float']['output'];
};

export type SearchMerchandising = {
  __typename?: 'SearchMerchandising';
  boosts: Array<SearchBoost>;
  pins: Array<SearchPin>;
  redirects: Array<SearchRedirect>;
  synonyms: Array<SearchSynonym>;
};

export type SearchPin = {
  __typename?: 'SearchPin';
  id: Scalars['ID']['output'];
  position: Scalars['Int']['output'];
  productId: Scalars['ID']['output'];
  query: Scalars['String']['output'];
};

export type SearchRedirect = {
  __typename?: 'SearchRedirect';
  id: Scalars['ID']['output'];
  query: Scalars['String']['output'];
  targetUrl: Scalars['String']['output'];
};

export type SearchSynonym = {
  __typename?: 'SearchSynonym';
  id: Scalars['ID']['output'];
  terms: Scalars['String']['output'];
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
  email: Scalars['String']['output'];
  lastOrderAt?: Maybe<Scalars['Time']['output']>;
  lifecycle: Scalars['String']['output'];
  name: Scalars['String']['output'];
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

export type SeoIssueCount = {
  __typename?: 'SeoIssueCount';
  count: Scalars['Int']['output'];
  issue: SeoIssue;
};

export type ServerFeatures = {
  __typename?: 'ServerFeatures';
  ai: Scalars['Boolean']['output'];
  signals: Scalars['Boolean']['output'];
};

export type SetBlogPostChannelListingInput = {
  channelId: Scalars['ID']['input'];
  isPublished: Scalars['Boolean']['input'];
  postId: Scalars['ID']['input'];
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
};

export type SetCmsPageChannelListingInput = {
  channelId: Scalars['ID']['input'];
  isHomepage?: InputMaybe<Scalars['Boolean']['input']>;
  isPublished: Scalars['Boolean']['input'];
  pageId: Scalars['ID']['input'];
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
};

export type SetCategoryListingInput = {
  categoryId: Scalars['ID']['input'];
  channelId: Scalars['ID']['input'];
  isPublished: Scalars['Boolean']['input'];
  sortOrderOverride?: InputMaybe<Scalars['Int']['input']>;
};

export type SetProductAttributeSlugInput = {
  attributeId: Scalars['ID']['input'];
  channelId?: InputMaybe<Scalars['ID']['input']>;
  productId: Scalars['ID']['input'];
  slug: Scalars['String']['input'];
};

export type SetProductListingInput = {
  channelId: Scalars['ID']['input'];
  compareAtPrice?: InputMaybe<Scalars['Int']['input']>;
  isPublished: Scalars['Boolean']['input'];
  price?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
  specialFrom?: InputMaybe<Scalars['Time']['input']>;
  specialPrice?: InputMaybe<Scalars['Int']['input']>;
  specialTo?: InputMaybe<Scalars['Time']['input']>;
  url?: InputMaybe<Scalars['String']['input']>;
};

export type SetSubscriptionTermsInput = {
  intervalCount: Scalars['Int']['input'];
  intervalUnit: SubscriptionInterval;
  productId: Scalars['ID']['input'];
  trialDays?: InputMaybe<Scalars['Int']['input']>;
};

export type ShippingBox = {
  __typename?: 'ShippingBox';
  boxWeightGrams: Scalars['Int']['output'];
  channelId: Scalars['ID']['output'];
  code: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  innerHeightMm: Scalars['Int']['output'];
  innerLengthMm: Scalars['Int']['output'];
  innerWidthMm: Scalars['Int']['output'];
  isActive: Scalars['Boolean']['output'];
  maxWeightGrams: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type ShippingBoxInput = {
  boxWeightGrams?: InputMaybe<Scalars['Int']['input']>;
  channelId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
  innerHeightMm: Scalars['Int']['input'];
  innerLengthMm: Scalars['Int']['input'];
  innerWidthMm: Scalars['Int']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  maxWeightGrams?: InputMaybe<Scalars['Int']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type ShippingCalcType =
  | 'FLAT'
  | 'FREE'
  | 'FREE_ABOVE';

export type ShippingCarrierAdmin = {
  __typename?: 'ShippingCarrierAdmin';
  allowedCountries: Array<Scalars['String']['output']>;
  carrierSlug: Scalars['String']['output'];
  countryMode: Scalars['String']['output'];
  displayName: Scalars['String']['output'];
  isEnabled: Scalars['Boolean']['output'];
  sortOrder: Scalars['Int']['output'];
  supportsLabels: Scalars['Boolean']['output'];
  supportsTracking: Scalars['Boolean']['output'];
};

export type ShippingLabel = {
  __typename?: 'ShippingLabel';
  carrierSlug: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  dataUrl: Scalars['String']['output'];
  format: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  trackingNumber: Scalars['String']['output'];
};

export type ShippingMethod = {
  __typename?: 'ShippingMethod';
  allowedCountries: Array<Scalars['String']['output']>;
  calcType: ShippingCalcType;
  code: Scalars['String']['output'];
  countryMode: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  flatCents: Scalars['Int']['output'];
  freeAboveCents: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
  zoneId: Scalars['ID']['output'];
};

export type ShippingZone = {
  __typename?: 'ShippingZone';
  channelId: Scalars['ID']['output'];
  code: Scalars['String']['output'];
  countries: Array<Scalars['String']['output']>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  methods: Array<ShippingMethod>;
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type SignalConsent = {
  __typename?: 'SignalConsent';
  channel: Scalars['String']['output'];
  kind: Scalars['String']['output'];
  name: Scalars['String']['output'];
  optedIn: Scalars['Boolean']['output'];
  purpose: Scalars['String']['output'];
};

export type SignalEmail = {
  __typename?: 'SignalEmail';
  clicked: Scalars['Int']['output'];
  lastOpenAt?: Maybe<Scalars['Time']['output']>;
  lastSentAt?: Maybe<Scalars['Time']['output']>;
  opened: Scalars['Int']['output'];
  sent: Scalars['Int']['output'];
};

export type SignalFlowRun = {
  __typename?: 'SignalFlowRun';
  enrolledAt: Scalars['Time']['output'];
  flowId: Scalars['ID']['output'];
  flowName: Scalars['String']['output'];
  status: Scalars['String']['output'];
  trigger: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type SignalSegment = {
  __typename?: 'SignalSegment';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type SignalsChatMessage = {
  content: Scalars['String']['input'];
  role: Scalars['String']['input'];
};

export type SignalsChatPage = {
  entityId?: InputMaybe<Scalars['String']['input']>;
  entityType?: InputMaybe<Scalars['String']['input']>;
  label?: InputMaybe<Scalars['String']['input']>;
  path: Scalars['String']['input'];
};

export type SignalsChatReply = {
  __typename?: 'SignalsChatReply';
  reply: Scalars['String']['output'];
};

export type SignalsInsight = {
  __typename?: 'SignalsInsight';
  detail: Scalars['String']['output'];
  level: Scalars['String']['output'];
  title: Scalars['String']['output'];
};

export type SignupForm = {
  __typename?: 'SignupForm';
  body: Scalars['String']['output'];
  buttonLabel: Scalars['String']['output'];
  channelId: Scalars['ID']['output'];
  collectName: Scalars['Boolean']['output'];
  consentKind: Scalars['String']['output'];
  display: SignupFormDisplay;
  heading: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  status: Scalars['String']['output'];
  submissionCount: Scalars['Int']['output'];
  successMessage: Scalars['String']['output'];
  targetSegmentId?: Maybe<Scalars['ID']['output']>;
  triggerFlow: Scalars['Boolean']['output'];
  type: Scalars['String']['output'];
};

export type SignupFormDisplay = {
  __typename?: 'SignupFormDisplay';
  delaySeconds: Scalars['Int']['output'];
  frequencyDays: Scalars['Int']['output'];
  scrollPercent: Scalars['Int']['output'];
  trigger: Scalars['String']['output'];
};

export type SignupFormDisplayInput = {
  delaySeconds: Scalars['Int']['input'];
  frequencyDays: Scalars['Int']['input'];
  scrollPercent: Scalars['Int']['input'];
  trigger: Scalars['String']['input'];
};

export type SignupFormSubmission = {
  __typename?: 'SignupFormSubmission';
  createdAt: Scalars['Time']['output'];
  customerId?: Maybe<Scalars['ID']['output']>;
  email: Scalars['String']['output'];
};

export type SiteCrawl = {
  __typename?: 'SiteCrawl';
  baseUrl: Scalars['String']['output'];
  error: Scalars['String']['output'];
  finishedAt?: Maybe<Scalars['Time']['output']>;
  hitBudget: Scalars['Boolean']['output'];
  id: Scalars['ID']['output'];
  linksFound: Scalars['Int']['output'];
  maxDepth: Scalars['Int']['output'];
  maxPages: Scalars['Int']['output'];
  pagesCrawled: Scalars['Int']['output'];
  respectRobots: Scalars['Boolean']['output'];
  startedAt: Scalars['Time']['output'];
  status: SiteCrawlStatus;
};

export type SiteCrawlStatus =
  | 'CANCELLED'
  | 'COMPLETED'
  | 'FAILED'
  | 'RUNNING';

export type StartDataExportInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  format?: Scalars['String']['input'];
  locale?: InputMaybe<Scalars['String']['input']>;
  options?: InputMaybe<Array<DataOptionValue>>;
  resource: Scalars['String']['input'];
};

export type StartSiteCrawlInput = {
  baseUrl?: InputMaybe<Scalars['String']['input']>;
  channelId: Scalars['ID']['input'];
  maxDepth?: InputMaybe<Scalars['Int']['input']>;
  maxPages?: InputMaybe<Scalars['Int']['input']>;
  respectRobots?: InputMaybe<Scalars['Boolean']['input']>;
};

export type StoreCreditEntry = {
  __typename?: 'StoreCreditEntry';
  amountCents: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  orderId?: Maybe<Scalars['ID']['output']>;
  reason: Scalars['String']['output'];
};

export type StoreCreditSummary = {
  __typename?: 'StoreCreditSummary';
  balanceCents: Scalars['Int']['output'];
  currencyCode: Scalars['String']['output'];
  entries: Array<StoreCreditEntry>;
};

export type Subscription = {
  __typename?: 'Subscription';
  notificationAdded: AdminNotification;
};

export type SubscriptionInterval =
  | 'DAY'
  | 'MONTH'
  | 'WEEK'
  | 'YEAR';

export type SubscriptionInvoice = {
  __typename?: 'SubscriptionInvoice';
  amount: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  periodEnd: Scalars['Time']['output'];
  periodStart: Scalars['Time']['output'];
  sequence: Scalars['Int']['output'];
  status: SubscriptionInvoiceStatus;
};

export type SubscriptionInvoiceStatus =
  | 'DUE'
  | 'PAID'
  | 'VOID';

export type SubscriptionStatus =
  | 'ACTIVE'
  | 'CANCELED'
  | 'EXPIRED'
  | 'PAUSED';

export type SubscriptionTerms = {
  __typename?: 'SubscriptionTerms';
  createdAt: Scalars['Time']['output'];
  intervalCount: Scalars['Int']['output'];
  intervalUnit: SubscriptionInterval;
  trialDays: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type SynapseActiveCustomer = {
  __typename?: 'SynapseActiveCustomer';
  customerId: Scalars['ID']['output'];
  email: Scalars['String']['output'];
  events: Scalars['Int']['output'];
  lastSeen?: Maybe<Scalars['Time']['output']>;
  name: Scalars['String']['output'];
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

export type SyncCanonicalUrlRewriteInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  entityId: Scalars['ID']['input'];
  entityType: Scalars['String']['input'];
  requestPath: Scalars['String']['input'];
};

export type SyncProductInventoryBatchResult = {
  __typename?: 'SyncProductInventoryBatchResult';
  failed: Array<SyncProductInventoryFailure>;
  succeeded: Scalars['Int']['output'];
};

export type SyncProductInventoryFailure = {
  __typename?: 'SyncProductInventoryFailure';
  productId?: Maybe<Scalars['ID']['output']>;
  productSku?: Maybe<Scalars['String']['output']>;
  reason: Scalars['String']['output'];
};

export type SyncProductInventoryInput = {
  productId?: InputMaybe<Scalars['ID']['input']>;
  productSku?: InputMaybe<Scalars['String']['input']>;
  quantity: Scalars['Float']['input'];
  source?: InputMaybe<Scalars['String']['input']>;
};

export type SystemHealth = {
  __typename?: 'SystemHealth';
  checkedAt: Scalars['Time']['output'];
  groups: Array<HealthGroup>;
  status: HealthStatus;
};

export type TaxCondition = {
  __typename?: 'TaxCondition';
  id: Scalars['ID']['output'];
  params: Scalars['JSON']['output'];
  paramsVersion: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  type: Scalars['String']['output'];
};

export type TaxConditionInput = {
  params: Scalars['JSON']['input'];
  paramsVersion?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  type: Scalars['String']['input'];
};

export type TaxEffect = {
  __typename?: 'TaxEffect';
  id: Scalars['ID']['output'];
  params: Scalars['JSON']['output'];
  paramsVersion: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  type: Scalars['String']['output'];
};

export type TaxEffectInput = {
  params: Scalars['JSON']['input'];
  paramsVersion?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  type: Scalars['String']['input'];
};

export type TaxRule = {
  __typename?: 'TaxRule';
  channelId: Scalars['ID']['output'];
  conditions: Array<TaxCondition>;
  createdAt: Scalars['Time']['output'];
  effects: Array<TaxEffect>;
  exclusivityGroup: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  priority: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type TaxTypeDescriptor = {
  __typename?: 'TaxTypeDescriptor';
  description: Scalars['String']['output'];
  label: Scalars['String']['output'];
  paramsSchema: Scalars['JSON']['output'];
  slug: Scalars['String']['output'];
};

export type TopProduct = {
  __typename?: 'TopProduct';
  cost: Scalars['Int64']['output'];
  margin: Scalars['Int64']['output'];
  name: Scalars['String']['output'];
  orderCount: Scalars['Int']['output'];
  revenue: Scalars['Int64']['output'];
  sku: Scalars['String']['output'];
  unitsSold: Scalars['Int64']['output'];
};

export type TransactionalSlot = {
  __typename?: 'TransactionalSlot';
  emailId?: Maybe<Scalars['ID']['output']>;
  emailName?: Maybe<Scalars['String']['output']>;
  label: Scalars['String']['output'];
  slot: Scalars['String']['output'];
};

export type UntrackedMediaFile = {
  __typename?: 'UntrackedMediaFile';
  filename: Scalars['String']['output'];
  lastModified: Scalars['Time']['output'];
  mimeType: Scalars['String']['output'];
  objectKey: Scalars['String']['output'];
  sizeBytes: Scalars['Int']['output'];
  url: Scalars['String']['output'];
};

export type UpdateAdminInput = {
  firstName?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  lastName?: InputMaybe<Scalars['String']['input']>;
  roleSlugs?: InputMaybe<Array<Scalars['String']['input']>>;
};

export type UpdateAttributeInput = {
  autoSegment?: InputMaybe<Scalars['Boolean']['input']>;
  defaultValue?: InputMaybe<DefaultValueUpdate>;
  facetCollapsed?: InputMaybe<Scalars['Boolean']['input']>;
  filterable?: InputMaybe<Scalars['Boolean']['input']>;
  h1Position?: InputMaybe<AttributeH1Position>;
  id: Scalars['ID']['input'];
  isIndexable?: InputMaybe<Scalars['Boolean']['input']>;
  isPublic?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  searchable?: InputMaybe<Scalars['Boolean']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  synapseTrack?: InputMaybe<Scalars['Boolean']['input']>;
  unit?: InputMaybe<Scalars['String']['input']>;
  valueRequired?: InputMaybe<Scalars['Boolean']['input']>;
};

export type UpdateBlogAuthorInput = {
  avatarAssetId?: InputMaybe<Scalars['ID']['input']>;
  clearAvatar?: InputMaybe<Scalars['Boolean']['input']>;
  clearEmail?: InputMaybe<Scalars['Boolean']['input']>;
  clearLinkedAdmin?: InputMaybe<Scalars['Boolean']['input']>;
  displayName?: InputMaybe<Scalars['String']['input']>;
  email?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  linkedAdminId?: InputMaybe<Scalars['ID']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateBlogCategoryInput = {
  clearParent?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  parentId?: InputMaybe<Scalars['ID']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateBlogPostInput = {
  authorId?: InputMaybe<Scalars['ID']['input']>;
  clearAuthor?: InputMaybe<Scalars['Boolean']['input']>;
  clearCover?: InputMaybe<Scalars['Boolean']['input']>;
  coverAssetId?: InputMaybe<Scalars['ID']['input']>;
  id: Scalars['ID']['input'];
  metadata?: InputMaybe<Array<MetadataInput>>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  readingTimeMinutes?: InputMaybe<Scalars['Int']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateBlogTagInput = {
  id: Scalars['ID']['input'];
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateBundleOptionInput = {
  clearMaxSelections?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
  inputType?: InputMaybe<BundleInputType>;
  isRequired?: InputMaybe<Scalars['Boolean']['input']>;
  maxSelections?: InputMaybe<Scalars['Int']['input']>;
  minSelections?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateBundleSelectionInput = {
  id: Scalars['ID']['input'];
  isDefault?: InputMaybe<Scalars['Boolean']['input']>;
  quantity?: InputMaybe<Scalars['Float']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateCmsAttributeInput = {
  defaultValue?: InputMaybe<Scalars['JSON']['input']>;
  id: Scalars['ID']['input'];
  isRequiredDefault?: InputMaybe<Scalars['Boolean']['input']>;
  isTranslatable?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  relationshipTarget?: InputMaybe<CmsRelationshipTarget>;
  unit?: InputMaybe<Scalars['String']['input']>;
  validation?: InputMaybe<Scalars['JSON']['input']>;
};

export type UpdateCmsBlockTypeInput = {
  fields?: InputMaybe<Scalars['JSON']['input']>;
  htmlTemplate?: InputMaybe<Scalars['String']['input']>;
  icon?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCmsPageInput = {
  clearParent?: InputMaybe<Scalars['Boolean']['input']>;
  clearPublished?: InputMaybe<Scalars['Boolean']['input']>;
  data?: InputMaybe<Scalars['JSON']['input']>;
  id: Scalars['ID']['input'];
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  parentId?: InputMaybe<Scalars['ID']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  publishedAt?: InputMaybe<Scalars['Time']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
  tags?: InputMaybe<Array<Scalars['String']['input']>>;
};

export type UpdateCmsPageTypeInput = {
  icon?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  position?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateCmsPageTypeStructureInput = {
  defaultMetaTitle?: InputMaybe<Scalars['String']['input']>;
  defaultSeoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
  isSingleton?: InputMaybe<Scalars['Boolean']['input']>;
  rootSlug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCmsSnippetInput = {
  blocks?: InputMaybe<Scalars['JSON']['input']>;
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCategoryInput = {
  clearParent?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  parentId?: InputMaybe<Scalars['ID']['input']>;
  position?: InputMaybe<Scalars['Int']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateChannelInput = {
  currencyCode?: InputMaybe<Scalars['String']['input']>;
  defaultLocale?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  locales?: InputMaybe<Array<Scalars['String']['input']>>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  publicUrl?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCollectionInput = {
  id: Scalars['ID']['input'];
  isPublished?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCountryInput = {
  code: Scalars['String']['input'];
  defaultCurrencyCode?: InputMaybe<Scalars['String']['input']>;
  emojiFlag?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  nativeName?: InputMaybe<Scalars['String']['input']>;
  phoneCode?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateCurrencyInput = {
  code: Scalars['String']['input'];
  decimals?: InputMaybe<Scalars['Int']['input']>;
  exchangeRate?: InputMaybe<Scalars['Float']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  symbol?: InputMaybe<Scalars['String']['input']>;
  symbolPosition?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCustomerGroupInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateCustomerSegmentInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateGroupedMemberInput = {
  defaultQuantity?: InputMaybe<Scalars['Float']['input']>;
  id: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateIntegrationInput = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
  clearChannel?: InputMaybe<Scalars['Boolean']['input']>;
  clearExpiresAt?: InputMaybe<Scalars['Boolean']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  expiresAt?: InputMaybe<Scalars['Time']['input']>;
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
  permissionSlugs?: InputMaybe<Array<Scalars['String']['input']>>;
};

export type UpdateInventoryLocationInput = {
  city?: InputMaybe<Scalars['String']['input']>;
  clearCoords?: InputMaybe<Scalars['Boolean']['input']>;
  contactName?: InputMaybe<Scalars['String']['input']>;
  countryCode?: InputMaybe<Scalars['String']['input']>;
  email?: InputMaybe<Scalars['String']['input']>;
  externalReference?: InputMaybe<Scalars['String']['input']>;
  hours?: InputMaybe<Scalars['JSON']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  latitude?: InputMaybe<Scalars['Float']['input']>;
  line1?: InputMaybe<Scalars['String']['input']>;
  line2?: InputMaybe<Scalars['String']['input']>;
  longitude?: InputMaybe<Scalars['Float']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  phone?: InputMaybe<Scalars['String']['input']>;
  postalCode?: InputMaybe<Scalars['String']['input']>;
  region?: InputMaybe<Scalars['String']['input']>;
  timezone?: InputMaybe<Scalars['String']['input']>;
  type?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateKitComponentInput = {
  id: Scalars['ID']['input'];
  quantity?: InputMaybe<Scalars['Float']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateLocaleInput = {
  code: Scalars['String']['input'];
  dateFormat?: InputMaybe<Scalars['String']['input']>;
  decimalSeparator?: InputMaybe<Scalars['String']['input']>;
  defaultCountryCode?: InputMaybe<Scalars['String']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  emailFromNameOverride?: InputMaybe<Scalars['String']['input']>;
  firstDayOfWeek?: InputMaybe<Scalars['Int']['input']>;
  flagEmoji?: InputMaybe<Scalars['String']['input']>;
  hreflangValue?: InputMaybe<Scalars['String']['input']>;
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  isRTL?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  nativeName?: InputMaybe<Scalars['String']['input']>;
  pluralRule?: InputMaybe<Scalars['String']['input']>;
  script?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  thousandsSeparator?: InputMaybe<Scalars['String']['input']>;
  timeFormat?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateMediaAssetInput = {
  altText?: InputMaybe<Scalars['String']['input']>;
  caption?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  notes?: InputMaybe<Scalars['String']['input']>;
  title?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateMenuInput = {
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateMenuItemInput = {
  clearParent?: InputMaybe<Scalars['Boolean']['input']>;
  id: Scalars['ID']['input'];
  label?: InputMaybe<Scalars['String']['input']>;
  parentId?: InputMaybe<Scalars['ID']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  targetId?: InputMaybe<Scalars['ID']['input']>;
  targetType?: InputMaybe<MenuItemTargetType>;
  url?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateProductDownloadInput = {
  id: Scalars['ID']['input'];
  maxDownloads?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  title?: InputMaybe<Scalars['String']['input']>;
  url?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateProductInput = {
  canonicalUrl?: InputMaybe<Scalars['String']['input']>;
  clearCompareAtPrice?: InputMaybe<Scalars['Boolean']['input']>;
  clearSpecialPrice?: InputMaybe<Scalars['Boolean']['input']>;
  clearTracksInventoryOverride?: InputMaybe<Scalars['Boolean']['input']>;
  compareAtPrice?: InputMaybe<Scalars['Int']['input']>;
  costCents?: InputMaybe<Scalars['Int']['input']>;
  defaultCurrencyCode?: InputMaybe<Scalars['String']['input']>;
  defaultPrice?: InputMaybe<Scalars['Int']['input']>;
  heightMm?: InputMaybe<Scalars['Int']['input']>;
  id: Scalars['ID']['input'];
  lengthMm?: InputMaybe<Scalars['Int']['input']>;
  maxOrderQty?: InputMaybe<Scalars['Float']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  minOrderQty?: InputMaybe<Scalars['Float']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  quantityDecimals?: InputMaybe<Scalars['Int']['input']>;
  quantityStep?: InputMaybe<Scalars['Float']['input']>;
  seoNoindex?: InputMaybe<Scalars['Boolean']['input']>;
  sku?: InputMaybe<Scalars['String']['input']>;
  specialFrom?: InputMaybe<Scalars['Time']['input']>;
  specialPrice?: InputMaybe<Scalars['Int']['input']>;
  specialTo?: InputMaybe<Scalars['Time']['input']>;
  tracksInventoryOverride?: InputMaybe<Scalars['Boolean']['input']>;
  url?: InputMaybe<Scalars['String']['input']>;
  urlAuto?: InputMaybe<Scalars['Boolean']['input']>;
  urlTemplateOverride?: InputMaybe<Scalars['String']['input']>;
  weightGrams?: InputMaybe<Scalars['Int']['input']>;
  widthMm?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateProductMediaInput = {
  altText?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateProductOptionInput = {
  clearFileExtensions?: InputMaybe<Scalars['Boolean']['input']>;
  clearMaxChars?: InputMaybe<Scalars['Boolean']['input']>;
  clearMaxFileBytes?: InputMaybe<Scalars['Boolean']['input']>;
  clearPrice?: InputMaybe<Scalars['Boolean']['input']>;
  clearVisibleIf?: InputMaybe<Scalars['Boolean']['input']>;
  fileExtensions?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  inputType?: InputMaybe<ProductOptionInputType>;
  isRequired?: InputMaybe<Scalars['Boolean']['input']>;
  maxChars?: InputMaybe<Scalars['Int']['input']>;
  maxFileBytes?: InputMaybe<Scalars['Int']['input']>;
  priceType?: InputMaybe<ProductOptionPriceType>;
  priceValue?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  visibleIf?: InputMaybe<Scalars['JSON']['input']>;
};

export type UpdateProductOptionValueInput = {
  forcedLineQty?: InputMaybe<Scalars['Float']['input']>;
  id: Scalars['ID']['input'];
  isDefault?: InputMaybe<Scalars['Boolean']['input']>;
  priceType?: InputMaybe<ProductOptionPriceType>;
  priceValue?: InputMaybe<Scalars['Int']['input']>;
  quantity?: InputMaybe<Scalars['Float']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
  sourceProductId?: InputMaybe<Scalars['ID']['input']>;
};

export type UpdateProductTypeInput = {
  id: Scalars['ID']['input'];
  isDigital?: InputMaybe<Scalars['Boolean']['input']>;
  isShippingRequired?: InputMaybe<Scalars['Boolean']['input']>;
  metadata?: InputMaybe<Array<MetadataInput>>;
  name?: InputMaybe<Scalars['String']['input']>;
  privateMetadata?: InputMaybe<Array<MetadataInput>>;
  tracksInventory?: InputMaybe<Scalars['Boolean']['input']>;
  urlTemplate?: InputMaybe<Scalars['String']['input']>;
};

export type UpdatePromotionCodeInput = {
  code?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  maxUses?: InputMaybe<Scalars['Int']['input']>;
  perCustomerLimit?: InputMaybe<Scalars['Int']['input']>;
  restrictedToEmail?: InputMaybe<Scalars['String']['input']>;
  validFrom?: InputMaybe<Scalars['Time']['input']>;
  validTo?: InputMaybe<Scalars['Time']['input']>;
};

export type UpdatePromotionRuleInput = {
  actions?: InputMaybe<Array<PromotionActionInput>>;
  bannerMediaId?: InputMaybe<Scalars['ID']['input']>;
  channelId?: InputMaybe<Scalars['ID']['input']>;
  conditions?: InputMaybe<Array<PromotionConditionInput>>;
  description?: InputMaybe<Scalars['String']['input']>;
  exclusivityGroup?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  labelMediaId?: InputMaybe<Scalars['ID']['input']>;
  maxUses?: InputMaybe<Scalars['Int']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  perCustomerLimit?: InputMaybe<Scalars['Int']['input']>;
  pillBgColor?: InputMaybe<Scalars['String']['input']>;
  pillText?: InputMaybe<Scalars['String']['input']>;
  pillTextColor?: InputMaybe<Scalars['String']['input']>;
  priority?: InputMaybe<Scalars['Int']['input']>;
  publicLabel?: InputMaybe<Scalars['String']['input']>;
  requiresCode?: InputMaybe<Scalars['Boolean']['input']>;
  scheduleJson?: InputMaybe<Scalars['JSON']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
  stopAfterApply?: InputMaybe<PromotionStopMode>;
  validFrom?: InputMaybe<Scalars['Time']['input']>;
  validTo?: InputMaybe<Scalars['Time']['input']>;
};

export type UpdateRemoteProviderInput = {
  cacheTtlS?: InputMaybe<Scalars['Int']['input']>;
  config?: InputMaybe<Scalars['JSON']['input']>;
  failurePolicy?: InputMaybe<RemoteProviderFailurePolicy>;
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
  targetUrl?: InputMaybe<Scalars['String']['input']>;
  timeoutMs?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateReviewCategoryInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive: Scalars['Boolean']['input'];
  name: Scalars['String']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type UpdateReviewModerationTagInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive: Scalars['Boolean']['input'];
  name: Scalars['String']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type UpdateReviewQuestionInput = {
  allowComment: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
  isRequired: Scalars['Boolean']['input'];
  sortOrder: Scalars['Int']['input'];
};

export type UpdateRoleInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  scopes?: InputMaybe<Array<RolePermissionScopeInput>>;
};

export type UpdateScheduledJobInput = {
  alertAfterFailures?: InputMaybe<Scalars['Int']['input']>;
  enabled: Scalars['Boolean']['input'];
  id: Scalars['ID']['input'];
  maxAttempts?: InputMaybe<Scalars['Int']['input']>;
  retryBackoffSeconds?: InputMaybe<Scalars['Int']['input']>;
  schedule: Scalars['String']['input'];
  timeoutSeconds?: InputMaybe<Scalars['Int']['input']>;
  timezone?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateShippingMethodInput = {
  allowedCountries?: InputMaybe<Array<Scalars['String']['input']>>;
  calcType?: InputMaybe<ShippingCalcType>;
  code?: InputMaybe<Scalars['String']['input']>;
  countryMode?: InputMaybe<Scalars['String']['input']>;
  flatCents?: InputMaybe<Scalars['Int']['input']>;
  freeAboveCents?: InputMaybe<Scalars['Int']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateShippingZoneInput = {
  code?: InputMaybe<Scalars['String']['input']>;
  countries?: InputMaybe<Array<Scalars['String']['input']>>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateTaxRuleInput = {
  conditions?: InputMaybe<Array<TaxConditionInput>>;
  effects?: InputMaybe<Array<TaxEffectInput>>;
  exclusivityGroup?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  isActive?: InputMaybe<Scalars['Boolean']['input']>;
  name?: InputMaybe<Scalars['String']['input']>;
  priority?: InputMaybe<Scalars['Int']['input']>;
  sortOrder?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateUrlRewriteInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  redirectType?: InputMaybe<Scalars['Int']['input']>;
  requestPath?: InputMaybe<Scalars['String']['input']>;
  targetPath?: InputMaybe<Scalars['String']['input']>;
};

export type UpdateWebhookInput = {
  customHeaders?: InputMaybe<Scalars['JSON']['input']>;
  eventTypes?: InputMaybe<Array<Scalars['String']['input']>>;
  failurePolicy?: InputMaybe<WebhookFailurePolicy>;
  id: Scalars['ID']['input'];
  name?: InputMaybe<Scalars['String']['input']>;
  targetUrl?: InputMaybe<Scalars['String']['input']>;
  timeoutMs?: InputMaybe<Scalars['Int']['input']>;
};

export type UpdateWebhookJobInput = {
  body?: InputMaybe<Scalars['String']['input']>;
  description?: InputMaybe<Scalars['String']['input']>;
  headers?: InputMaybe<Array<HttpHeaderInput>>;
  id: Scalars['ID']['input'];
  method?: InputMaybe<Scalars['String']['input']>;
  secret?: InputMaybe<Scalars['String']['input']>;
  url: Scalars['String']['input'];
};

export type UpsertBlogAuthorContentInput = {
  authorId: Scalars['ID']['input'];
  bio?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  name: Scalars['String']['input'];
};

export type UpsertBlogCategoryContentInput = {
  categoryId: Scalars['ID']['input'];
  description?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
};

export type UpsertBlogPostContentInput = {
  body?: InputMaybe<Scalars['JSON']['input']>;
  excerpt?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  postId: Scalars['ID']['input'];
  title: Scalars['String']['input'];
};

export type UpsertBlogTagContentInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  tagId: Scalars['ID']['input'];
};

export type UpsertCmsAttributeContentInput = {
  attributeId: Scalars['ID']['input'];
  helperText?: InputMaybe<Scalars['String']['input']>;
  locale: Scalars['String']['input'];
  name: Scalars['String']['input'];
  placeholder?: InputMaybe<Scalars['String']['input']>;
};

export type UpsertCmsPageContentInput = {
  body?: InputMaybe<Scalars['JSON']['input']>;
  data?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  pageId: Scalars['ID']['input'];
  title: Scalars['String']['input'];
};

export type UpsertCmsPageTypeContentInput = {
  description?: InputMaybe<Scalars['String']['input']>;
  locale: Scalars['String']['input'];
  name: Scalars['String']['input'];
  pageTypeId: Scalars['ID']['input'];
};

export type UpsertCategoryContentInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  locale: Scalars['String']['input'];
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  shortDescription?: InputMaybe<Scalars['JSON']['input']>;
};

export type UpsertCollectionContentInput = {
  description?: InputMaybe<Scalars['JSON']['input']>;
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
};

export type UpsertProductContentInput = {
  descriptionRichText?: InputMaybe<Scalars['JSON']['input']>;
  metaDescription?: InputMaybe<Scalars['String']['input']>;
  metaKeywords?: InputMaybe<Scalars['String']['input']>;
  metaTitle?: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
  shortDescription?: InputMaybe<Scalars['JSON']['input']>;
};

export type UpsertPromotionRuleContentInput = {
  label: Scalars['String']['input'];
  locale: Scalars['String']['input'];
  ruleId: Scalars['ID']['input'];
  terms?: InputMaybe<Scalars['String']['input']>;
};

export type UpsertReviewQuestionContentInput = {
  helpText?: InputMaybe<Scalars['String']['input']>;
  label: Scalars['String']['input'];
  locale: Scalars['String']['input'];
  questionId: Scalars['ID']['input'];
};

export type UrlEntityType =
  | 'BLOG_AUTHOR'
  | 'BLOG_CATEGORY'
  | 'BLOG_POST'
  | 'BLOG_TAG'
  | 'CATEGORY'
  | 'CMS_PAGE'
  | 'COLLECTION'
  | 'PRODUCT';

export type UrlResolveResult = {
  __typename?: 'UrlResolveResult';
  entityId: Scalars['ID']['output'];
  entityType: UrlEntityType;
  redirectTo?: Maybe<Scalars['String']['output']>;
  redirectType: Scalars['Int']['output'];
};

export type UrlRewrite = {
  __typename?: 'UrlRewrite';
  channelId?: Maybe<Scalars['ID']['output']>;
  createdAt: Scalars['Time']['output'];
  description?: Maybe<Scalars['String']['output']>;
  entityId: Scalars['ID']['output'];
  entityType: UrlEntityType;
  id: Scalars['ID']['output'];
  isAutogenerated: Scalars['Boolean']['output'];
  redirectType: Scalars['Int']['output'];
  requestPath: Scalars['String']['output'];
  targetPath?: Maybe<Scalars['String']['output']>;
  updatedAt: Scalars['Time']['output'];
};

export type UrlRewritePage = {
  __typename?: 'UrlRewritePage';
  items: Array<UrlRewrite>;
  totalCount: Scalars['Int']['output'];
};

export type VariantMode =
  | 'ATTRIBUTE'
  | 'LIST';

export type WebForm = {
  __typename?: 'WebForm';
  channelId: Scalars['ID']['output'];
  createdAt: Scalars['Time']['output'];
  deletedAt?: Maybe<Scalars['Time']['output']>;
  fields: Scalars['JSON']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  settings: Scalars['JSON']['output'];
  slug: Scalars['String']['output'];
  status: Scalars['String']['output'];
  submissionCount: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type WebFormSubmission = {
  __typename?: 'WebFormSubmission';
  answers: Scalars['JSON']['output'];
  createdAt: Scalars['Time']['output'];
  customerId?: Maybe<Scalars['ID']['output']>;
  files: Scalars['JSON']['output'];
  formId: Scalars['ID']['output'];
  id: Scalars['ID']['output'];
  meta: Scalars['JSON']['output'];
};

export type Webhook = {
  __typename?: 'Webhook';
  channel?: Maybe<Channel>;
  circuit?: Maybe<WebhookCircuit>;
  createdAt: Scalars['Time']['output'];
  customHeaders?: Maybe<Scalars['JSON']['output']>;
  eventTypes: Array<Scalars['String']['output']>;
  failurePolicy: WebhookFailurePolicy;
  id: Scalars['ID']['output'];
  isActive: Scalars['Boolean']['output'];
  kind: WebhookKind;
  name: Scalars['String']['output'];
  payloadVersion: Scalars['String']['output'];
  targetUrl: Scalars['String']['output'];
  timeoutMs: Scalars['Int']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type WebhookCircuit = {
  __typename?: 'WebhookCircuit';
  consecutiveFailures: Scalars['Int']['output'];
  openedAt?: Maybe<Scalars['Time']['output']>;
  state: WebhookCircuitState;
};

export type WebhookCircuitState =
  | 'CLOSED'
  | 'HALF_OPEN'
  | 'OPEN';

export type WebhookDelivery = {
  __typename?: 'WebhookDelivery';
  attempt: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  durationMs: Scalars['Int']['output'];
  errorMessage?: Maybe<Scalars['String']['output']>;
  eventType: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  requestBody: Scalars['String']['output'];
  requestHeaders?: Maybe<Scalars['JSON']['output']>;
  responseBody?: Maybe<Scalars['String']['output']>;
  responseHeaders?: Maybe<Scalars['JSON']['output']>;
  responseStatus?: Maybe<Scalars['Int']['output']>;
  success: Scalars['Boolean']['output'];
  url: Scalars['String']['output'];
};

export type WebhookDeliveryStats = {
  __typename?: 'WebhookDeliveryStats';
  avgSuccessMs: Scalars['Int']['output'];
  failed: Scalars['Int']['output'];
  lastDeliveryAt?: Maybe<Scalars['Time']['output']>;
  lastFailureAt?: Maybe<Scalars['Time']['output']>;
  lastSuccessAt?: Maybe<Scalars['Time']['output']>;
  succeeded: Scalars['Int']['output'];
  successRate: Scalars['Float']['output'];
  total: Scalars['Int']['output'];
};

export type WebhookEventType = {
  __typename?: 'WebhookEventType';
  description: Scalars['String']['output'];
  kind: WebhookKind;
  name: Scalars['String']['output'];
  type: Scalars['String']['output'];
};

export type WebhookFailurePolicy =
  | 'FAIL_CLOSED'
  | 'FAIL_OPEN';

export type WebhookJobConfig = {
  __typename?: 'WebhookJobConfig';
  body: Scalars['String']['output'];
  hasSecret: Scalars['Boolean']['output'];
  headers: Array<HttpHeader>;
  method: Scalars['String']['output'];
  url: Scalars['String']['output'];
};

export type WebhookKind =
  | 'ASYNC'
  | 'SYNC';

export type WebhookQueueItem = {
  __typename?: 'WebhookQueueItem';
  attempts: Scalars['Int']['output'];
  createdAt: Scalars['Time']['output'];
  eventType: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  lastError?: Maybe<Scalars['String']['output']>;
  nextAttemptAt: Scalars['Time']['output'];
  payload: Scalars['String']['output'];
  status: WebhookQueueStatus;
  updatedAt: Scalars['Time']['output'];
};

export type WebhookQueueStatus =
  | 'DEAD'
  | 'PENDING';
