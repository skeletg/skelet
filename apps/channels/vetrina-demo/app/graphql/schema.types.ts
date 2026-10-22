export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  Int64: { input: unknown; output: unknown; }
  JSON: { input: unknown; output: unknown; }
  Time: { input: string; output: string; }
};

export type AddBundleToCartInput = {
  cartId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
  selections: Array<AddBundleToCartSelectionInput>;
  sku: Scalars['String']['input'];
};

export type AddBundleToCartSelectionInput = {
  optionId: Scalars['ID']['input'];
  selectionIds: Array<Scalars['ID']['input']>;
};

export type AddConfiguredToCartInput = {
  cartId: Scalars['ID']['input'];
  options: Array<ProductOptionSelectionInput>;
  quantity: Scalars['Float']['input'];
  sku: Scalars['String']['input'];
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

export type AddressInput2 = {
  __typename?: 'AddressInput2';
  city: Scalars['String']['output'];
  countryCode: Scalars['String']['output'];
  postalCode: Scalars['String']['output'];
  state: Scalars['String']['output'];
  street1: Scalars['String']['output'];
  street2: Scalars['String']['output'];
};

export type AddressValidationResult = {
  __typename?: 'AddressValidationResult';
  messages: Array<Scalars['String']['output']>;
  normalized?: Maybe<AddressInput2>;
  strict: Scalars['Boolean']['output'];
  valid: Scalars['Boolean']['output'];
};

export type AppliedPromotionCode = {
  __typename?: 'AppliedPromotionCode';
  code: Scalars['String']['output'];
  freeGifts: Array<PromotionFreeGift>;
  freeShipping: Scalars['Boolean']['output'];
  invalidatedReason: Scalars['String']['output'];
  publicLabel: Scalars['String']['output'];
  requiresCode: Scalars['Boolean']['output'];
  ruleName: Scalars['String']['output'];
  subtotalDiscount: Scalars['Int']['output'];
};

export type Attribute = {
  __typename?: 'Attribute';
  filterable: Scalars['Boolean']['output'];
  id: Scalars['ID']['output'];
  inputType: AttributeInputType;
  isPublic: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  relationshipTarget?: Maybe<AttributeReferenceTarget>;
  searchable: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  synapseTrack: Scalars['Boolean']['output'];
  unit: Scalars['String']['output'];
  values: Array<AttributeValue>;
};

export type AttributeFilterInput = {
  slug: Scalars['String']['input'];
  values: Array<Scalars['String']['input']>;
};

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

export type AttributeReferenceTarget =
  | 'CATEGORY'
  | 'COLLECTION'
  | 'PRODUCT';

export type AttributeValue = {
  __typename?: 'AttributeValue';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  value: Scalars['String']['output'];
};

export type BlogAuthor = {
  __typename?: 'BlogAuthor';
  content?: Maybe<BlogAuthorContent>;
  displayName: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  publicEmail?: Maybe<Scalars['String']['output']>;
  slug: Scalars['String']['output'];
};

export type BlogAuthorContent = {
  __typename?: 'BlogAuthorContent';
  bio?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  name: Scalars['String']['output'];
};

export type BlogCategory = {
  __typename?: 'BlogCategory';
  children: Array<BlogCategory>;
  content?: Maybe<BlogCategoryContent>;
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  parent?: Maybe<BlogCategory>;
  position: Scalars['Int']['output'];
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
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
};

export type BlogPost = {
  __typename?: 'BlogPost';
  author?: Maybe<BlogAuthor>;
  categories: Array<BlogCategory>;
  content?: Maybe<BlogPostContent>;
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  publishedAt?: Maybe<Scalars['Time']['output']>;
  readingTimeMinutes?: Maybe<Scalars['Int']['output']>;
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  status: BlogPostStatus;
  tags: Array<BlogTag>;
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
};

export type BlogPostFilter = {
  authorSlug?: InputMaybe<Scalars['String']['input']>;
  categorySlug?: InputMaybe<Scalars['String']['input']>;
  tagSlug?: InputMaybe<Scalars['String']['input']>;
};

export type BlogPostList = {
  __typename?: 'BlogPostList';
  items: Array<BlogPost>;
  totalCount?: Maybe<Scalars['Int']['output']>;
};

export type BlogPostSearchHit = {
  __typename?: 'BlogPostSearchHit';
  authorName?: Maybe<Scalars['String']['output']>;
  excerpt?: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  publishedAt?: Maybe<Scalars['Time']['output']>;
  slug: Scalars['String']['output'];
  title?: Maybe<Scalars['String']['output']>;
};

export type BlogPostSearchResult = {
  __typename?: 'BlogPostSearchResult';
  items: Array<BlogPostSearchHit>;
  totalCount: Scalars['Int']['output'];
};

export type BlogPostStatus =
  | 'ARCHIVED'
  | 'DRAFT'
  | 'PUBLISHED'
  | 'SCHEDULED';

export type BlogTag = {
  __typename?: 'BlogTag';
  content?: Maybe<BlogTagContent>;
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
};

export type BlogTagContent = {
  __typename?: 'BlogTagContent';
  description?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
};

export type BrowseFacet = {
  __typename?: 'BrowseFacet';
  buckets: Array<BrowseFacetBucket>;
  defaultCollapsed: Scalars['Boolean']['output'];
  description: Scalars['String']['output'];
  h1Position: BrowseFacetH1Position;
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type BrowseFacetBucket = {
  __typename?: 'BrowseFacetBucket';
  count: Scalars['Int']['output'];
  description: Scalars['String']['output'];
  isIndexable: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type BrowseFacetH1Position =
  | 'AFTER'
  | 'BEFORE';

export type BrowseProductsFilter = {
  attributes?: InputMaybe<Array<AttributeFilterInput>>;
  categoryId?: InputMaybe<Scalars['ID']['input']>;
  collectionId?: InputMaybe<Scalars['ID']['input']>;
  kinds?: InputMaybe<Array<ProductKind>>;
  priceMaxCents?: InputMaybe<Scalars['Int']['input']>;
  priceMinCents?: InputMaybe<Scalars['Int']['input']>;
};

export type BrowseProductsResult = {
  __typename?: 'BrowseProductsResult';
  facets: Array<BrowseFacet>;
  items: Array<Product>;
  totalCount: Scalars['Int']['output'];
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
  id: Scalars['ID']['output'];
  inputType: BundleInputType;
  isRequired: Scalars['Boolean']['output'];
  selections: Array<BundleSelection>;
  sortOrder: Scalars['Int']['output'];
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

export type BusinessIdField = {
  __typename?: 'BusinessIdField';
  hint: Scalars['String']['output'];
  key: Scalars['String']['output'];
  label: Scalars['String']['output'];
  pattern: Scalars['String']['output'];
  required: Scalars['Boolean']['output'];
};

export type BusinessIdValue = {
  __typename?: 'BusinessIdValue';
  key: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type BusinessIdValueInput = {
  key: Scalars['String']['input'];
  value: Scalars['String']['input'];
};

export type CmsPage = {
  __typename?: 'CMSPage';
  canonicalUrl?: Maybe<Scalars['String']['output']>;
  content?: Maybe<CmsPageContent>;
  data?: Maybe<Scalars['JSON']['output']>;
  experimentKey: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  ogImage?: Maybe<Scalars['String']['output']>;
  pageTypeId: Scalars['ID']['output'];
  parentId?: Maybe<Scalars['ID']['output']>;
  position: Scalars['Int']['output'];
  publishedAt?: Maybe<Scalars['Time']['output']>;
  relationships: Array<CmsPageRelationship>;
  selections: Array<CmsPageSelection>;
  seoNofollow: Scalars['Boolean']['output'];
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
  structuredData?: Maybe<Scalars['String']['output']>;
  structuredDataSpec?: Maybe<Scalars['String']['output']>;
  tags: Array<Scalars['String']['output']>;
  urlPath: Scalars['String']['output'];
  variants: Array<CmsPageVariant>;
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
};

export type CmsPageFilter = {
  onlyPublished?: InputMaybe<Scalars['Boolean']['input']>;
  pageTypeId?: InputMaybe<Scalars['ID']['input']>;
  parentId?: InputMaybe<Scalars['ID']['input']>;
};

export type CmsPageRelationship = {
  __typename?: 'CMSPageRelationship';
  attributeId: Scalars['ID']['output'];
  position: Scalars['Int']['output'];
  targetId: Scalars['ID']['output'];
  targetType: CmsRelationshipTarget;
};

export type CmsPageSelection = {
  __typename?: 'CMSPageSelection';
  attributeId: Scalars['ID']['output'];
  valueId: Scalars['ID']['output'];
};

export type CmsPageVariant = {
  __typename?: 'CMSPageVariant';
  body?: Maybe<Scalars['JSON']['output']>;
  variantKey: Scalars['String']['output'];
};

export type CmsRelationshipTarget =
  | 'BLOG_POST'
  | 'CATEGORY'
  | 'CMS_PAGE'
  | 'COLLECTION'
  | 'PRODUCT';

export type Cart = {
  __typename?: 'Cart';
  appliedCodes: Array<AppliedPromotionCode>;
  checkout: CartCheckout;
  id: Scalars['ID']['output'];
  lines: Array<CartLine>;
  note: Scalars['String']['output'];
  offer?: Maybe<CartOffer>;
  savedItems: Array<SavedItem>;
  subtotal: Scalars['Int']['output'];
  totalDiscount: Scalars['Int']['output'];
};

export type CartAddress = {
  __typename?: 'CartAddress';
  city: Scalars['String']['output'];
  company?: Maybe<Scalars['String']['output']>;
  countryCode: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  lastName: Scalars['String']['output'];
  phone?: Maybe<Scalars['String']['output']>;
  postalCode: Scalars['String']['output'];
  sourceAddressId?: Maybe<Scalars['ID']['output']>;
  state?: Maybe<Scalars['String']['output']>;
  street1: Scalars['String']['output'];
  street2?: Maybe<Scalars['String']['output']>;
};

export type CartCheckout = {
  __typename?: 'CartCheckout';
  billingAddress?: Maybe<CartAddress>;
  billingSameAsShipping: Scalars['Boolean']['output'];
  businessIds: Array<BusinessIdValue>;
  canSplit: Scalars['Boolean']['output'];
  company: Scalars['String']['output'];
  deliveryGroups: Array<DeliveryGroup>;
  destination?: Maybe<CheckoutDestination>;
  digitalLineIds: Array<Scalars['ID']['output']>;
  email: Scalars['String']['output'];
  fulfillment: FulfillmentNeed;
  giftCards: Array<CartGiftCard>;
  isPickup: Scalars['Boolean']['output'];
  paymentProviderSlug?: Maybe<Scalars['String']['output']>;
  phone: Scalars['String']['output'];
  pickupContact?: Maybe<PickupContact>;
  poNumber: Scalars['String']['output'];
  purchaseType: PurchaseType;
  ready: Scalars['Boolean']['output'];
  requirements: Array<CheckoutRequirement>;
  shippingAddress?: Maybe<CartAddress>;
  shippingMethod?: Maybe<ShippingQuote>;
  splitDelivery: Scalars['Boolean']['output'];
  totals: CheckoutTotals;
  useStoreCredit: Scalars['Boolean']['output'];
};

export type CartEstimate = {
  __typename?: 'CartEstimate';
  cheapestShippingCents: Scalars['Int']['output'];
  estimatedTotalCents: Scalars['Int']['output'];
  shippingOptions: Array<ShippingQuote>;
  subtotalCents: Scalars['Int']['output'];
  taxCents: Scalars['Int']['output'];
};

export type CartGiftCard = {
  __typename?: 'CartGiftCard';
  appliedCents: Scalars['Int']['output'];
  balanceCents: Scalars['Int']['output'];
  code: Scalars['String']['output'];
};

export type CartLine = {
  __typename?: 'CartLine';
  availableQuantity?: Maybe<Scalars['Float']['output']>;
  components: Array<CartLineComponent>;
  currentUnitPrice?: Maybe<Scalars['Int']['output']>;
  id: Scalars['ID']['output'];
  imageUrl?: Maybe<Scalars['String']['output']>;
  isCustom: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  options: Array<CartLineOption>;
  quantity: Scalars['Float']['output'];
  sku: Scalars['String']['output'];
  totalPrice: Scalars['Int']['output'];
  unitPrice: Scalars['Int']['output'];
};

export type CartLineComponent = {
  __typename?: 'CartLineComponent';
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  optionId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
  selectionId: Scalars['ID']['output'];
  sku: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  unitPrice: Scalars['Int']['output'];
};

export type CartLineOption = {
  __typename?: 'CartLineOption';
  fileUrl: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  inputType: Scalars['String']['output'];
  optionTitle: Scalars['String']['output'];
  optionValueId?: Maybe<Scalars['ID']['output']>;
  priceDelta: Scalars['Int']['output'];
  productOptionId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
  sortOrder: Scalars['Int']['output'];
  textValue: Scalars['String']['output'];
  valueLabel: Scalars['String']['output'];
};

export type CartOffer = {
  __typename?: 'CartOffer';
  expiresAt: Scalars['Time']['output'];
  isGuest: Scalars['Boolean']['output'];
  locks: Array<CartOfferLock>;
  message: Scalars['String']['output'];
  paymentMethods: Array<Scalars['String']['output']>;
  state: CartOfferState;
};

export type CartOfferLock =
  | 'BILLING'
  | 'COUPONS'
  | 'DELIVERY'
  | 'LINES'
  | 'SHIPPING';

export type CartOfferOpening = {
  __typename?: 'CartOfferOpening';
  cartId: Scalars['ID']['output'];
  requiresLogin: Scalars['Boolean']['output'];
  state: CartOfferState;
};

export type CartOfferState =
  | 'COMPLETED'
  | 'DRAFT'
  | 'EXPIRED'
  | 'OPENED'
  | 'REVOKED'
  | 'SENT';

export type Category = {
  __typename?: 'Category';
  ancestors: Array<Category>;
  children: Array<Category>;
  coverImage?: Maybe<CollectionCover>;
  description?: Maybe<Scalars['JSON']['output']>;
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  level: Scalars['Int']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name?: Maybe<Scalars['String']['output']>;
  parent?: Maybe<Category>;
  position: Scalars['Int']['output'];
  products: Array<Product>;
  seoNoindex: Scalars['Boolean']['output'];
  shortDescription?: Maybe<Scalars['JSON']['output']>;
  slug: Scalars['String']['output'];
  urlPath: Scalars['String']['output'];
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
};

export type Channel = {
  __typename?: 'Channel';
  currency: Currency;
  currencyCode: Scalars['String']['output'];
  defaultLocale: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  locales: Array<Locale>;
  name: Scalars['String']['output'];
  publicUrl: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type CheckoutDestination = {
  __typename?: 'CheckoutDestination';
  countryCode: Scalars['String']['output'];
  postalCode: Scalars['String']['output'];
  regionCode?: Maybe<Scalars['String']['output']>;
};

export type CheckoutDetailsInput = {
  billingAddress?: InputMaybe<AddressInput>;
  billingAddressId?: InputMaybe<Scalars['ID']['input']>;
  billingSameAsShipping: Scalars['Boolean']['input'];
  businessIds?: InputMaybe<Array<BusinessIdValueInput>>;
  company?: InputMaybe<Scalars['String']['input']>;
  email?: InputMaybe<Scalars['String']['input']>;
  phone?: InputMaybe<Scalars['String']['input']>;
  pickupContact?: InputMaybe<PickupContactInput>;
  poNumber?: InputMaybe<Scalars['String']['input']>;
  purchaseType?: PurchaseType;
  shippingAddress?: InputMaybe<AddressInput>;
  shippingAddressId?: InputMaybe<Scalars['ID']['input']>;
};

export type CheckoutRequirement = {
  __typename?: 'CheckoutRequirement';
  code: Scalars['String']['output'];
  label: Scalars['String']['output'];
  message: Scalars['String']['output'];
  mode: RequirementMode;
  scope: Scalars['String']['output'];
  status: RequirementStatus;
  type: Scalars['String']['output'];
};

export type CheckoutTotals = {
  __typename?: 'CheckoutTotals';
  amountDueCents: Scalars['Int']['output'];
  discountCents: Scalars['Int']['output'];
  giftCardCents: Scalars['Int']['output'];
  shippingCents: Scalars['Int']['output'];
  storeCreditCents: Scalars['Int']['output'];
  subtotalCents: Scalars['Int']['output'];
  taxCents: Scalars['Int']['output'];
  totalCents: Scalars['Int']['output'];
};

export type Collection = {
  __typename?: 'Collection';
  coverImage?: Maybe<CollectionCover>;
  description?: Maybe<Scalars['JSON']['output']>;
  id: Scalars['ID']['output'];
  isPublished: Scalars['Boolean']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name?: Maybe<Scalars['String']['output']>;
  products: Array<Product>;
  seoNoindex: Scalars['Boolean']['output'];
  slug: Scalars['String']['output'];
};

export type CollectionContent = {
  __typename?: 'CollectionContent';
  description?: Maybe<Scalars['JSON']['output']>;
  locale: Scalars['String']['output'];
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
};

export type CollectionCover = {
  __typename?: 'CollectionCover';
  altText: Scalars['String']['output'];
  height: Scalars['Int']['output'];
  mimeType: Scalars['String']['output'];
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

export type ConfigScopeKind =
  | 'CHANNEL'
  | 'DEFAULT';

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
  | 'SELECT'
  | 'STRING'
  | 'TEXTAREA'
  | 'WORKING_HOURS';

export type ConfigValue = {
  __typename?: 'ConfigValue';
  field: Scalars['String']['output'];
  group: Scalars['String']['output'];
  isDefault: Scalars['Boolean']['output'];
  label: Scalars['String']['output'];
  options?: Maybe<Array<ConfigOption>>;
  path: Scalars['String']['output'];
  public: Scalars['Boolean']['output'];
  scope: ConfigScope;
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
  businessIdFields: Array<BusinessIdField>;
  code: Scalars['String']['output'];
  defaultCurrency?: Maybe<Currency>;
  defaultCurrencyCode: Scalars['String']['output'];
  emojiFlag: Scalars['String']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  nativeName: Scalars['String']['output'];
  phoneCode: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type CountrySettings = {
  __typename?: 'CountrySettings';
  allowOptionalState: Scalars['Boolean']['output'];
  defaultCountryCode: Scalars['String']['output'];
  euCountryCodes: Array<Scalars['String']['output']>;
  requiredRegionCountryCodes: Array<Scalars['String']['output']>;
  topDestinationCountryCodes: Array<Scalars['String']['output']>;
};

export type Currency = {
  __typename?: 'Currency';
  code: Scalars['String']['output'];
  decimals: Scalars['Int']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  symbol: Scalars['String']['output'];
  symbolPosition: Scalars['String']['output'];
};

export type Customer = {
  __typename?: 'Customer';
  addresses: Array<Address>;
  consents: Array<CustomerConsent>;
  defaultBillingAddress?: Maybe<Address>;
  defaultShippingAddress?: Maybe<Address>;
  email: Scalars['String']['output'];
  firstName: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  isEmailVerified: Scalars['Boolean']['output'];
  lastName: Scalars['String']['output'];
  phone?: Maybe<Scalars['String']['output']>;
  storeCreditBalance: Scalars['Int']['output'];
};

export type CustomerAuthPayload = {
  __typename?: 'CustomerAuthPayload';
  accessToken?: Maybe<Scalars['String']['output']>;
  customer?: Maybe<Customer>;
  refreshToken?: Maybe<Scalars['String']['output']>;
  verificationRequired: Scalars['Boolean']['output'];
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

export type CustomerLoginInput = {
  email: Scalars['String']['input'];
  password: Scalars['String']['input'];
};

export type CustomerRegisterInput = {
  email: Scalars['String']['input'];
  firstName: Scalars['String']['input'];
  lastName: Scalars['String']['input'];
  marketingOptIn?: InputMaybe<Scalars['Boolean']['input']>;
  password: Scalars['String']['input'];
  referralCode?: InputMaybe<Scalars['String']['input']>;
};

export type CustomerSubscription = {
  __typename?: 'CustomerSubscription';
  canceledAt?: Maybe<Scalars['Time']['output']>;
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  currentPeriodEnd: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  intervalCount: Scalars['Int']['output'];
  intervalUnit: SubscriptionInterval;
  nextBillingAt: Scalars['Time']['output'];
  paymentMethodId?: Maybe<Scalars['ID']['output']>;
  price: Scalars['Int']['output'];
  productId: Scalars['ID']['output'];
  status: SubscriptionStatus;
  title: Scalars['String']['output'];
};

export type DeliveryChoice = {
  __typename?: 'DeliveryChoice';
  carrierName: Scalars['String']['output'];
  carrierSlug: Scalars['String']['output'];
  description: Scalars['String']['output'];
  estimatedArrival?: Maybe<Scalars['String']['output']>;
  estimatedDays?: Maybe<Scalars['Int']['output']>;
  kind: Scalars['String']['output'];
  kindLabel: Scalars['String']['output'];
  methodCode: Scalars['String']['output'];
  methodName: Scalars['String']['output'];
  optionId: Scalars['ID']['output'];
  optionName: Scalars['String']['output'];
  pointLabel?: Maybe<Scalars['String']['output']>;
  priceCents: Scalars['Int']['output'];
  shipTo: DeliveryShipTo;
};

export type DeliveryGroup = {
  __typename?: 'DeliveryGroup';
  choices: Array<DeliveryChoice>;
  key: Scalars['String']['output'];
  kinds: Array<Scalars['String']['output']>;
  lineIds: Array<Scalars['ID']['output']>;
  selection?: Maybe<DeliverySelection>;
  shipments: Array<DeliveryShipment>;
};

export type DeliveryPoint = {
  __typename?: 'DeliveryPoint';
  city: Scalars['String']['output'];
  countryCode: Scalars['String']['output'];
  distanceKm?: Maybe<Scalars['Float']['output']>;
  hours?: Maybe<Scalars['String']['output']>;
  id: Scalars['String']['output'];
  instructions?: Maybe<Scalars['String']['output']>;
  lat?: Maybe<Scalars['Float']['output']>;
  lng?: Maybe<Scalars['Float']['output']>;
  name: Scalars['String']['output'];
  phone?: Maybe<Scalars['String']['output']>;
  postalCode: Scalars['String']['output'];
  region?: Maybe<Scalars['String']['output']>;
  street1: Scalars['String']['output'];
  street2?: Maybe<Scalars['String']['output']>;
};

export type DeliverySelection = {
  __typename?: 'DeliverySelection';
  carrierSlug: Scalars['String']['output'];
  estimatedArrival?: Maybe<Scalars['String']['output']>;
  estimatedDays?: Maybe<Scalars['Int']['output']>;
  kind: Scalars['String']['output'];
  methodCode: Scalars['String']['output'];
  methodName: Scalars['String']['output'];
  optionId: Scalars['ID']['output'];
  optionName: Scalars['String']['output'];
  point?: Maybe<DeliveryPoint>;
  pointLabel?: Maybe<Scalars['String']['output']>;
  priceCents: Scalars['Int']['output'];
  shipTo: DeliveryShipTo;
};

export type DeliveryShipTo =
  | 'CUSTOMER'
  | 'POINT';

export type DeliveryShipment = {
  __typename?: 'DeliveryShipment';
  estimatedArrival?: Maybe<Scalars['String']['output']>;
  estimatedDays?: Maybe<Scalars['Int']['output']>;
  lineIds: Array<Scalars['ID']['output']>;
  originCity?: Maybe<Scalars['String']['output']>;
  originName?: Maybe<Scalars['String']['output']>;
  priceCents?: Maybe<Scalars['Int']['output']>;
};

export type DownloadGrant = {
  __typename?: 'DownloadGrant';
  createdAt: Scalars['Time']['output'];
  downloadUrl: Scalars['String']['output'];
  downloadsUsed: Scalars['Int']['output'];
  expiresAt?: Maybe<Scalars['Time']['output']>;
  id: Scalars['ID']['output'];
  maxDownloads?: Maybe<Scalars['Int']['output']>;
  productId: Scalars['ID']['output'];
  title: Scalars['String']['output'];
};

export type FulfillmentNeed =
  | 'MIXED'
  | 'NONE'
  | 'PHYSICAL';

export type GiftCardBalance = {
  __typename?: 'GiftCardBalance';
  active: Scalars['Boolean']['output'];
  balanceCents: Scalars['Int']['output'];
  currencyCode: Scalars['String']['output'];
};

export type GroupedMember = {
  __typename?: 'GroupedMember';
  defaultQuantity: Scalars['Float']['output'];
  id: Scalars['ID']['output'];
  member: Product;
  sortOrder: Scalars['Int']['output'];
};

export type GroupedMembership = {
  __typename?: 'GroupedMembership';
  defaultQuantity: Scalars['Float']['output'];
  groupedProduct: Product;
  siblings: Array<GroupedMember>;
  sortOrder: Scalars['Int']['output'];
};

export type Locale = {
  __typename?: 'Locale';
  code: Scalars['String']['output'];
  dateFormat: Scalars['String']['output'];
  decimalSeparator: Scalars['String']['output'];
  defaultCountryCode: Scalars['String']['output'];
  firstDayOfWeek: Scalars['Int']['output'];
  flagEmoji: Scalars['String']['output'];
  hreflangValue: Scalars['String']['output'];
  isActive: Scalars['Boolean']['output'];
  isRTL: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  nativeName: Scalars['String']['output'];
  pluralRule: Scalars['String']['output'];
  script: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
  thousandsSeparator: Scalars['String']['output'];
  timeFormat: Scalars['String']['output'];
};

export type LoyaltyRedeemResult = {
  __typename?: 'LoyaltyRedeemResult';
  balancePoints: Scalars['Int']['output'];
  grantedCents: Scalars['Int']['output'];
  storeCreditBalance: Scalars['Int']['output'];
};

export type LoyaltyStatus = {
  __typename?: 'LoyaltyStatus';
  balancePoints: Scalars['Int']['output'];
  earnPointsPerUnit: Scalars['Int']['output'];
  enabled: Scalars['Boolean']['output'];
  redeemCentsPerPoint: Scalars['Int']['output'];
};

export type Menu = {
  __typename?: 'Menu';
  id: Scalars['ID']['output'];
  items: Array<MenuItem>;
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type MenuItem = {
  __typename?: 'MenuItem';
  children: Array<MenuItem>;
  id: Scalars['ID']['output'];
  label: Scalars['String']['output'];
  parentId?: Maybe<Scalars['ID']['output']>;
  sortOrder: Scalars['Int']['output'];
  targetId?: Maybe<Scalars['ID']['output']>;
  targetType?: Maybe<MenuItemTargetType>;
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

export type Mutation = {
  __typename?: 'Mutation';
  addAddress: Address;
  addBundleToCart: Cart;
  addConfiguredToCart: Cart;
  addToCart: Cart;
  addToWishlist: Scalars['Boolean']['output'];
  applyCoupon: Cart;
  applyGiftCard: Cart;
  cancelSubscription: CustomerSubscription;
  changeCustomerPassword: Scalars['Boolean']['output'];
  clearCart: Cart;
  createGuestCart: Cart;
  customerLogin: CustomerAuthPayload;
  customerLogout: Scalars['Boolean']['output'];
  customerRefreshToken: CustomerAuthPayload;
  customerRegister: CustomerAuthPayload;
  deleteAddress: Scalars['Boolean']['output'];
  deleteMyPaymentMethod: Scalars['Boolean']['output'];
  finalizeOrderPayment: Scalars['Boolean']['output'];
  markCartOfferOpened: Scalars['Boolean']['output'];
  mergeGuestCart: Cart;
  moveSavedToCart: Cart;
  moveToCart: Cart;
  openCartOffer: CartOfferOpening;
  placeOrder: Order;
  presignWebFormUpload: WebFormUploadTicket;
  redeemLoyaltyPoints: LoyaltyRedeemResult;
  removeCoupon: Cart;
  removeFromCart: Cart;
  removeFromWishlist: Scalars['Boolean']['output'];
  removeGiftCard: Cart;
  removeSavedItem: Cart;
  reorder: ReorderResult;
  requestPasswordReset: Scalars['Boolean']['output'];
  requestProductOptionFileUpload: ProductOptionFileUpload;
  requestReturn: OrderReturn;
  requestReviewMediaUpload: ReviewMediaUpload;
  resendVerification: Scalars['Boolean']['output'];
  resendVerificationEmail: Scalars['Boolean']['output'];
  resetPassword: Scalars['Boolean']['output'];
  revokeWishlistShare: Scalars['Boolean']['output'];
  saveForLater: Cart;
  savePaymentMethod: SavedPaymentMethod;
  setCartDelivery: Cart;
  setCartDeliveryPoint: Cart;
  setCartDestination: Cart;
  setCartDetails: Cart;
  setCartEmail: Cart;
  setCartNote: Cart;
  setCartPaymentMethod: Cart;
  setCartShippingMethod: Cart;
  setCartSplitDelivery: Cart;
  setCartStoreCredit: Cart;
  setConsent: CustomerConsent;
  setDefaultBillingAddress: Customer;
  setDefaultShippingAddress: Customer;
  setMySubscriptionPaymentMethod: Scalars['Boolean']['output'];
  submitProductQuestion: Scalars['Boolean']['output'];
  submitProductReview: Review;
  submitWebForm: SubmitWebFormResult;
  updateAddress: Address;
  updateCartLine: Cart;
  updateProfile: Customer;
  verifyEmail: Scalars['Boolean']['output'];
  voteReview: Review;
};


export type MutationAddAddressArgs = {
  input: AddressInput;
};


export type MutationAddBundleToCartArgs = {
  input: AddBundleToCartInput;
};


export type MutationAddConfiguredToCartArgs = {
  input: AddConfiguredToCartInput;
};


export type MutationAddToCartArgs = {
  cartId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
  sku: Scalars['String']['input'];
};


export type MutationAddToWishlistArgs = {
  productId: Scalars['ID']['input'];
};


export type MutationApplyCouponArgs = {
  cartId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
};


export type MutationApplyGiftCardArgs = {
  cartId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
};


export type MutationCancelSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationChangeCustomerPasswordArgs = {
  currentPassword: Scalars['String']['input'];
  newPassword: Scalars['String']['input'];
};


export type MutationClearCartArgs = {
  cartId: Scalars['ID']['input'];
};


export type MutationCustomerLoginArgs = {
  input: CustomerLoginInput;
};


export type MutationCustomerLogoutArgs = {
  token: Scalars['String']['input'];
};


export type MutationCustomerRefreshTokenArgs = {
  token: Scalars['String']['input'];
};


export type MutationCustomerRegisterArgs = {
  input: CustomerRegisterInput;
};


export type MutationDeleteAddressArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteMyPaymentMethodArgs = {
  id: Scalars['ID']['input'];
};


export type MutationFinalizeOrderPaymentArgs = {
  orderId: Scalars['ID']['input'];
};


export type MutationMarkCartOfferOpenedArgs = {
  cartId: Scalars['ID']['input'];
};


export type MutationMergeGuestCartArgs = {
  guestCartId: Scalars['ID']['input'];
};


export type MutationMoveSavedToCartArgs = {
  cartId: Scalars['ID']['input'];
  savedItemId: Scalars['ID']['input'];
};


export type MutationMoveToCartArgs = {
  productId: Scalars['ID']['input'];
};


export type MutationOpenCartOfferArgs = {
  token: Scalars['String']['input'];
};


export type MutationPlaceOrderArgs = {
  acceptTerms?: InputMaybe<Scalars['Boolean']['input']>;
  cartId?: InputMaybe<Scalars['ID']['input']>;
  paymentToken?: InputMaybe<Scalars['String']['input']>;
  savedPaymentMethodId?: InputMaybe<Scalars['ID']['input']>;
};


export type MutationPresignWebFormUploadArgs = {
  contentType: Scalars['String']['input'];
  fieldKey: Scalars['String']['input'];
  filename: Scalars['String']['input'];
  formId: Scalars['ID']['input'];
  sizeBytes: Scalars['Int']['input'];
};


export type MutationRedeemLoyaltyPointsArgs = {
  points: Scalars['Int']['input'];
};


export type MutationRemoveCouponArgs = {
  cartId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
};


export type MutationRemoveFromCartArgs = {
  cartId: Scalars['ID']['input'];
  lineId: Scalars['ID']['input'];
};


export type MutationRemoveFromWishlistArgs = {
  productId: Scalars['ID']['input'];
};


export type MutationRemoveGiftCardArgs = {
  cartId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
};


export type MutationRemoveSavedItemArgs = {
  cartId: Scalars['ID']['input'];
  savedItemId: Scalars['ID']['input'];
};


export type MutationReorderArgs = {
  orderId: Scalars['ID']['input'];
};


export type MutationRequestPasswordResetArgs = {
  email: Scalars['String']['input'];
};


export type MutationRequestProductOptionFileUploadArgs = {
  filename: Scalars['String']['input'];
  mimeType: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
  sizeBytes: Scalars['Int']['input'];
  sku: Scalars['String']['input'];
};


export type MutationRequestReturnArgs = {
  input: RequestReturnInput;
};


export type MutationRequestReviewMediaUploadArgs = {
  filename: Scalars['String']['input'];
  mimeType: Scalars['String']['input'];
  productId: Scalars['ID']['input'];
  sizeBytes: Scalars['Int']['input'];
};


export type MutationResendVerificationArgs = {
  email: Scalars['String']['input'];
};


export type MutationResetPasswordArgs = {
  newPassword: Scalars['String']['input'];
  token: Scalars['String']['input'];
};


export type MutationSaveForLaterArgs = {
  cartId: Scalars['ID']['input'];
  lineId: Scalars['ID']['input'];
};


export type MutationSavePaymentMethodArgs = {
  providerSlug: Scalars['String']['input'];
  token: Scalars['String']['input'];
};


export type MutationSetCartDeliveryArgs = {
  carrierSlug: Scalars['String']['input'];
  cartId: Scalars['ID']['input'];
  groupKey: Scalars['String']['input'];
  methodCode: Scalars['String']['input'];
  optionId: Scalars['ID']['input'];
};


export type MutationSetCartDeliveryPointArgs = {
  cartId: Scalars['ID']['input'];
  groupKey: Scalars['String']['input'];
  pointId: Scalars['String']['input'];
};


export type MutationSetCartDestinationArgs = {
  cartId: Scalars['ID']['input'];
  countryCode: Scalars['String']['input'];
  postalCode: Scalars['String']['input'];
  regionCode?: InputMaybe<Scalars['String']['input']>;
};


export type MutationSetCartDetailsArgs = {
  cartId: Scalars['ID']['input'];
  input: CheckoutDetailsInput;
};


export type MutationSetCartEmailArgs = {
  cartId: Scalars['ID']['input'];
  email: Scalars['String']['input'];
};


export type MutationSetCartNoteArgs = {
  cartId: Scalars['ID']['input'];
  note: Scalars['String']['input'];
};


export type MutationSetCartPaymentMethodArgs = {
  cartId: Scalars['ID']['input'];
  providerSlug: Scalars['String']['input'];
};


export type MutationSetCartShippingMethodArgs = {
  carrierSlug: Scalars['String']['input'];
  cartId: Scalars['ID']['input'];
  code: Scalars['String']['input'];
};


export type MutationSetCartSplitDeliveryArgs = {
  cartId: Scalars['ID']['input'];
  split: Scalars['Boolean']['input'];
};


export type MutationSetCartStoreCreditArgs = {
  cartId: Scalars['ID']['input'];
  use: Scalars['Boolean']['input'];
};


export type MutationSetConsentArgs = {
  kind: Scalars['String']['input'];
  optedIn: Scalars['Boolean']['input'];
};


export type MutationSetDefaultBillingAddressArgs = {
  id: Scalars['ID']['input'];
};


export type MutationSetDefaultShippingAddressArgs = {
  id: Scalars['ID']['input'];
};


export type MutationSetMySubscriptionPaymentMethodArgs = {
  paymentMethodId?: InputMaybe<Scalars['ID']['input']>;
  subscriptionId: Scalars['ID']['input'];
};


export type MutationSubmitProductQuestionArgs = {
  authorEmail?: InputMaybe<Scalars['String']['input']>;
  authorName?: InputMaybe<Scalars['String']['input']>;
  productId: Scalars['ID']['input'];
  question: Scalars['String']['input'];
};


export type MutationSubmitProductReviewArgs = {
  input: SubmitReviewInput;
};


export type MutationSubmitWebFormArgs = {
  input: SubmitWebFormInput;
};


export type MutationUpdateAddressArgs = {
  id: Scalars['ID']['input'];
  input: AddressInput;
};


export type MutationUpdateCartLineArgs = {
  cartId: Scalars['ID']['input'];
  lineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
};


export type MutationUpdateProfileArgs = {
  firstName?: InputMaybe<Scalars['String']['input']>;
  lastName?: InputMaybe<Scalars['String']['input']>;
  phone?: InputMaybe<Scalars['String']['input']>;
};


export type MutationVerifyEmailArgs = {
  token: Scalars['String']['input'];
};


export type MutationVoteReviewArgs = {
  reviewId: Scalars['ID']['input'];
  value: Scalars['Int']['input'];
};

export type Order = {
  __typename?: 'Order';
  amountDue: Scalars['Int']['output'];
  billingAddress: OrderAddress;
  carrier: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  currencyCode: Scalars['String']['output'];
  customerNote?: Maybe<Scalars['String']['output']>;
  discountAmount: Scalars['Int']['output'];
  events: Array<OrderEvent>;
  giftCardAmount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  lines: Array<OrderLine>;
  number: Scalars['String']['output'];
  paymentClientSecret?: Maybe<Scalars['String']['output']>;
  paymentInstructions?: Maybe<Scalars['String']['output']>;
  paymentProvider: Scalars['String']['output'];
  paymentStatus: PaymentStatus;
  returnableLines: Array<ReturnableLine>;
  returns: Array<OrderReturn>;
  shippingAddress: OrderAddress;
  shippingAmount: Scalars['Int']['output'];
  status: OrderStatus;
  storeCreditAmount: Scalars['Int']['output'];
  subtotal: Scalars['Int']['output'];
  taxAmount: Scalars['Int']['output'];
  total: Scalars['Int']['output'];
  trackingNumber: Scalars['String']['output'];
  trackingUrl?: Maybe<Scalars['String']['output']>;
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

export type OrderEvent = {
  __typename?: 'OrderEvent';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  message: Scalars['String']['output'];
  type: Scalars['String']['output'];
};

export type OrderLine = {
  __typename?: 'OrderLine';
  id: Scalars['ID']['output'];
  isGift: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  options: Array<OrderLineOption>;
  quantity: Scalars['Float']['output'];
  sku: Scalars['String']['output'];
  totalPrice: Scalars['Int']['output'];
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

export type OrderReturn = {
  __typename?: 'OrderReturn';
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  lines: Array<OrderReturnLine>;
  reason: Scalars['String']['output'];
  refundAmount: Scalars['Int']['output'];
  refundTarget: Scalars['String']['output'];
  resolution: Scalars['String']['output'];
  rmaNumber: Scalars['String']['output'];
  status: Scalars['String']['output'];
};

export type OrderReturnLine = {
  __typename?: 'OrderReturnLine';
  id: Scalars['ID']['output'];
  orderLineId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
  reason: Scalars['String']['output'];
};

export type OrderStatus =
  | 'CANCELLED'
  | 'CONFIRMED'
  | 'FULFILLED'
  | 'ON_HOLD'
  | 'PARTIALLY_FULFILLED'
  | 'PENDING'
  | 'PROCESSING';

export type PaymentMethodOption = {
  __typename?: 'PaymentMethodOption';
  displayName: Scalars['String']['output'];
  instructions: Scalars['String']['output'];
  isOffline: Scalars['Boolean']['output'];
  providerSlug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type PaymentStatus =
  | 'AUTHORIZED'
  | 'PAID'
  | 'REFUNDED'
  | 'UNPAID'
  | 'VOIDED';

export type PhoneCountryFormat = {
  __typename?: 'PhoneCountryFormat';
  callingCode: Scalars['String']['output'];
  countryCode: Scalars['String']['output'];
  example: Scalars['String']['output'];
  nationalExample: Scalars['String']['output'];
  template: Scalars['String']['output'];
};

export type PhoneSettings = {
  __typename?: 'PhoneSettings';
  defaultCountryCode: Scalars['String']['output'];
  formats: Array<PhoneCountryFormat>;
  lockedCountryCode?: Maybe<Scalars['String']['output']>;
  restrictedToStoreCountries: Scalars['Boolean']['output'];
};

export type PickupContact = {
  __typename?: 'PickupContact';
  firstName: Scalars['String']['output'];
  lastName: Scalars['String']['output'];
  someoneElse: Scalars['Boolean']['output'];
};

export type PickupContactInput = {
  firstName: Scalars['String']['input'];
  lastName: Scalars['String']['input'];
  someoneElse: Scalars['Boolean']['input'];
};

export type PickupLocation = {
  __typename?: 'PickupLocation';
  city: Scalars['String']['output'];
  countryCode: Scalars['String']['output'];
  instructions: Scalars['String']['output'];
  line1: Scalars['String']['output'];
  name: Scalars['String']['output'];
  postalCode: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type PresentmentCurrency = {
  __typename?: 'PresentmentCurrency';
  code: Scalars['String']['output'];
  decimals: Scalars['Int']['output'];
  factorFromBase: Scalars['Float']['output'];
  name: Scalars['String']['output'];
  rateUpdatedAt: Scalars['Time']['output'];
  symbol: Scalars['String']['output'];
  symbolPosition: Scalars['String']['output'];
};

export type Product = {
  __typename?: 'Product';
  alsoBought: Array<Product>;
  attributeValues: Array<ProductAttributeValue>;
  backorderable: Scalars['Boolean']['output'];
  bundleOptions: Array<BundleOption>;
  canonicalUrl?: Maybe<Scalars['String']['output']>;
  categories: Array<Category>;
  children: Array<Product>;
  collections: Array<Collection>;
  compareAtPrice?: Maybe<Scalars['Int']['output']>;
  configurableOptions: Array<Attribute>;
  customerCanReview: ReviewEligibility;
  defaultCurrencyCode: Scalars['String']['output'];
  defaultPrice: Scalars['Int']['output'];
  description?: Maybe<Scalars['JSON']['output']>;
  descriptionRichText?: Maybe<Scalars['JSON']['output']>;
  groupedMembers: Array<GroupedMember>;
  id: Scalars['ID']['output'];
  inStock: Scalars['Boolean']['output'];
  kind: ProductKind;
  linkedProducts: Array<Product>;
  mainImage?: Maybe<ProductMedia>;
  maxOrderQty?: Maybe<Scalars['Float']['output']>;
  media: Array<ProductMedia>;
  metaDescription?: Maybe<Scalars['String']['output']>;
  metaKeywords?: Maybe<Scalars['String']['output']>;
  metaTitle?: Maybe<Scalars['String']['output']>;
  minOrderQty?: Maybe<Scalars['Float']['output']>;
  name?: Maybe<Scalars['String']['output']>;
  ogImage?: Maybe<ProductMedia>;
  onSale: Scalars['Boolean']['output'];
  options: Array<ProductCustomOption>;
  partOf: ProductMemberships;
  price: Scalars['Int']['output'];
  primaryCategory?: Maybe<Category>;
  productType: ProductType;
  promotionalDisplays: Array<PromotionalDisplay>;
  quantityDecimals: Scalars['Int']['output'];
  quantityStep: Scalars['Float']['output'];
  reviewAggregates: ReviewAggregates;
  reviewQuestions: Array<ReviewQuestion>;
  reviewSummary?: Maybe<Scalars['String']['output']>;
  reviews: Array<Review>;
  salableQuantity?: Maybe<Scalars['Float']['output']>;
  seoNoindex: Scalars['Boolean']['output'];
  shortDescription?: Maybe<Scalars['JSON']['output']>;
  sku: Scalars['String']['output'];
  subscriptionTerms?: Maybe<SubscriptionTerms>;
  thumbnail?: Maybe<ProductMedia>;
  tierPrices: Array<TierPrice>;
  url: Scalars['String']['output'];
  variantMode?: Maybe<VariantMode>;
};


export type ProductAlsoBoughtArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
};


export type ProductInStockArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};


export type ProductLinkedProductsArgs = {
  kind: ProductLinkKind;
};


export type ProductReviewsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  sort?: InputMaybe<ReviewSort>;
  withMediaOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type ProductSalableQuantityArgs = {
  channelId?: InputMaybe<Scalars['ID']['input']>;
};

export type ProductAttributeReference = {
  __typename?: 'ProductAttributeReference';
  label: Scalars['String']['output'];
  targetId: Scalars['ID']['output'];
  targetType: AttributeReferenceTarget;
  url: Scalars['String']['output'];
};

export type ProductAttributeValue = {
  __typename?: 'ProductAttributeValue';
  attribute: Attribute;
  references: Array<ProductAttributeReference>;
  scalarValue?: Maybe<Scalars['JSON']['output']>;
  selectedValues: Array<AttributeValue>;
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
};

export type ProductCustomOption = {
  __typename?: 'ProductCustomOption';
  fileExtensions?: Maybe<Scalars['String']['output']>;
  helpText: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  inputType: Scalars['String']['output'];
  isRequired: Scalars['Boolean']['output'];
  maxChars?: Maybe<Scalars['Int']['output']>;
  maxFileBytes?: Maybe<Scalars['Int']['output']>;
  priceType: Scalars['String']['output'];
  priceValue?: Maybe<Scalars['Int']['output']>;
  sortOrder: Scalars['Int']['output'];
  title: Scalars['String']['output'];
  values: Array<ProductCustomOptionValue>;
  visibleIf?: Maybe<Scalars['JSON']['output']>;
};

export type ProductCustomOptionValue = {
  __typename?: 'ProductCustomOptionValue';
  forcedLineQty?: Maybe<Scalars['Float']['output']>;
  id: Scalars['ID']['output'];
  isDefault: Scalars['Boolean']['output'];
  priceType: Scalars['String']['output'];
  priceValue: Scalars['Int']['output'];
  quantity: Scalars['Float']['output'];
  sourceProduct?: Maybe<Product>;
  title: Scalars['String']['output'];
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
  height: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  mimeType: Scalars['String']['output'];
  sizeBytes: Scalars['Int']['output'];
  sortOrder: Scalars['Int']['output'];
  transformUrl?: Maybe<Scalars['String']['output']>;
  url: Scalars['String']['output'];
  width: Scalars['Int']['output'];
};


export type ProductMediaTransformUrlArgs = {
  height?: InputMaybe<Scalars['Int']['input']>;
  width: Scalars['Int']['input'];
};

export type ProductMediaRole =
  | 'MAIN'
  | 'OG_IMAGE'
  | 'THUMBNAIL';

export type ProductMediaStatus =
  | 'PENDING'
  | 'READY';

export type ProductMemberships = {
  __typename?: 'ProductMemberships';
  bundles: Array<BundleMembership>;
  configurable?: Maybe<ConfigurableMembership>;
  groupedProducts: Array<GroupedMembership>;
};

export type ProductOptionFileUpload = {
  __typename?: 'ProductOptionFileUpload';
  assetId: Scalars['ID']['output'];
  expiresAt: Scalars['Time']['output'];
  uploadUrl: Scalars['String']['output'];
};

export type ProductOptionSelectionInput = {
  fileAssetId?: InputMaybe<Scalars['ID']['input']>;
  optionId: Scalars['ID']['input'];
  textValue?: InputMaybe<Scalars['String']['input']>;
  valueIds?: InputMaybe<Array<Scalars['ID']['input']>>;
};

export type ProductQuestion = {
  __typename?: 'ProductQuestion';
  answer: Scalars['String']['output'];
  answeredAt?: Maybe<Scalars['Time']['output']>;
  authorName: Scalars['String']['output'];
  createdAt: Scalars['Time']['output'];
  id: Scalars['ID']['output'];
  question: Scalars['String']['output'];
};

export type ProductSearchHit = {
  __typename?: 'ProductSearchHit';
  id: Scalars['ID']['output'];
  name?: Maybe<Scalars['String']['output']>;
  sku: Scalars['String']['output'];
  url: Scalars['String']['output'];
};

export type ProductSearchResult = {
  __typename?: 'ProductSearchResult';
  items: Array<ProductSearchHit>;
  redirectUrl?: Maybe<Scalars['String']['output']>;
  totalCount: Scalars['Int']['output'];
};

export type ProductSortOption = {
  __typename?: 'ProductSortOption';
  isIndexable: Scalars['Boolean']['output'];
  label: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type ProductType = {
  __typename?: 'ProductType';
  id: Scalars['ID']['output'];
  isDigital: Scalars['Boolean']['output'];
  isShippingRequired: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  slug: Scalars['String']['output'];
};

export type PromotionFreeGift = {
  __typename?: 'PromotionFreeGift';
  href?: Maybe<Scalars['String']['output']>;
  inCart: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  productId?: Maybe<Scalars['ID']['output']>;
  quantity: Scalars['Float']['output'];
  sku: Scalars['String']['output'];
};

export type PromotionalDisplay = {
  __typename?: 'PromotionalDisplay';
  bannerMedia?: Maybe<ProductMedia>;
  labelMedia?: Maybe<ProductMedia>;
  pillBgColor: Scalars['String']['output'];
  pillText: Scalars['String']['output'];
  pillTextColor: Scalars['String']['output'];
  publicLabel: Scalars['String']['output'];
  ruleId: Scalars['ID']['output'];
  ruleSlug: Scalars['String']['output'];
};

export type PublicStore = {
  __typename?: 'PublicStore';
  city: Scalars['String']['output'];
  countryCode: Scalars['String']['output'];
  distanceKm?: Maybe<Scalars['Float']['output']>;
  hours: Scalars['JSON']['output'];
  id: Scalars['ID']['output'];
  latitude?: Maybe<Scalars['Float']['output']>;
  line1: Scalars['String']['output'];
  line2: Scalars['String']['output'];
  longitude?: Maybe<Scalars['Float']['output']>;
  name: Scalars['String']['output'];
  phone: Scalars['String']['output'];
  pickupEnabled: Scalars['Boolean']['output'];
  postalCode: Scalars['String']['output'];
  region: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  stockStatus?: Maybe<StoreStockStatus>;
  timezone: Scalars['String']['output'];
};

export type PublicStoresInput = {
  city?: InputMaybe<Scalars['String']['input']>;
  inStockOnly?: InputMaybe<Scalars['Boolean']['input']>;
  latitude?: InputMaybe<Scalars['Float']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  longitude?: InputMaybe<Scalars['Float']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  pickupOnly?: InputMaybe<Scalars['Boolean']['input']>;
  preferredStoreId?: InputMaybe<Scalars['ID']['input']>;
  productId?: InputMaybe<Scalars['ID']['input']>;
  search?: InputMaybe<Scalars['String']['input']>;
};

export type PublicStoresResult = {
  __typename?: 'PublicStoresResult';
  cities: Array<Scalars['String']['output']>;
  enabled: Scalars['Boolean']['output'];
  hasPickupStores: Scalars['Boolean']['output'];
  items: Array<PublicStore>;
  preferredStore?: Maybe<PublicStore>;
  productEligible: Scalars['Boolean']['output'];
  stockCheckingAvailable: Scalars['Boolean']['output'];
  totalCount: Scalars['Int']['output'];
};

export type PurchaseType =
  | 'BUSINESS'
  | 'PERSONAL';

export type Query = {
  __typename?: 'Query';
  attribute: Attribute;
  attributes: Array<Attribute>;
  blogAuthor?: Maybe<BlogAuthor>;
  blogAuthors: Array<BlogAuthor>;
  blogCategories: Array<BlogCategory>;
  blogCategory?: Maybe<BlogCategory>;
  blogPost?: Maybe<BlogPost>;
  blogPosts: BlogPostList;
  blogTag?: Maybe<BlogTag>;
  blogTags: Array<BlogTag>;
  browseProducts: BrowseProductsResult;
  cart?: Maybe<Cart>;
  cartEstimate: CartEstimate;
  cartRecommendations: Array<Product>;
  category: Category;
  categoryChildren: Array<Category>;
  cmsPage?: Maybe<CmsPage>;
  cmsPages: Array<CmsPage>;
  collection: Collection;
  collections: Array<Collection>;
  consentDefinitions: Array<ConsentDefinition>;
  countries: Array<Country>;
  country: Country;
  countrySettings: CountrySettings;
  currencies: Array<Currency>;
  currency: Currency;
  currentChannel?: Maybe<Channel>;
  deliveryPoints: Array<DeliveryPoint>;
  features: StoreFeatures;
  freeShippingThreshold: Scalars['Int']['output'];
  giftCardBalance?: Maybe<GiftCardBalance>;
  guestOrder?: Maybe<Order>;
  homePage?: Maybe<CmsPage>;
  locale: Locale;
  locales: Array<Locale>;
  loyaltyStatus: LoyaltyStatus;
  menu?: Maybe<Menu>;
  menus: Array<Menu>;
  myCart: Cart;
  myCartOffers: Array<Cart>;
  myDataExport: Scalars['String']['output'];
  myDownloads: Array<DownloadGrant>;
  myOrder: Order;
  myOrders: Array<Order>;
  myPaymentMethods: Array<SavedPaymentMethod>;
  myProfile: Customer;
  myReferral: ReferralInfo;
  mySubscriptions: Array<CustomerSubscription>;
  myWishlist: Array<Product>;
  myWishlistShareToken: Scalars['String']['output'];
  paymentMethods: Array<PaymentMethodOption>;
  phoneSettings: PhoneSettings;
  pickupLocations: Array<PickupLocation>;
  presentmentCurrencies: Array<PresentmentCurrency>;
  previewBlogPost?: Maybe<BlogPost>;
  previewCMSPage?: Maybe<CmsPage>;
  product: Product;
  productQuestions: Array<ProductQuestion>;
  productSortOptions: Array<ProductSortOption>;
  productsByIds: Array<Product>;
  productsBySkus: Array<Product>;
  publicConfig: Array<ConfigValue>;
  publicConfigSections: Array<ConfigSection>;
  publicStores: PublicStoresResult;
  regions: Array<Region>;
  rootCategories: Array<Category>;
  route?: Maybe<RouteResolution>;
  searchBlogPosts: BlogPostSearchResult;
  searchProducts: ProductSearchResult;
  sharedWishlist: Array<Product>;
  shippingQuotes: Array<ShippingQuote>;
  sitemapCounts: SitemapCounts;
  sitemapEntries: Array<SitemapEntry>;
  trackOrder?: Maybe<TrackOrderResult>;
  validateAddress: AddressValidationResult;
  wishlistProductIds: Array<Scalars['ID']['output']>;
};


export type QueryAttributeArgs = {
  id: Scalars['ID']['input'];
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


export type QueryBrowseProductsArgs = {
  filter?: InputMaybe<BrowseProductsFilter>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  query?: InputMaybe<Scalars['String']['input']>;
  sort?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCartArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCartEstimateArgs = {
  cartId: Scalars['ID']['input'];
  countryCode: Scalars['String']['input'];
  regionCode?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCartRecommendationsArgs = {
  cartId: Scalars['ID']['input'];
};


export type QueryCategoryArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCategoryChildrenArgs = {
  parentId: Scalars['ID']['input'];
};


export type QueryCmsPageArgs = {
  id?: InputMaybe<Scalars['ID']['input']>;
  slug?: InputMaybe<Scalars['String']['input']>;
  urlPath?: InputMaybe<Scalars['String']['input']>;
};


export type QueryCmsPagesArgs = {
  filter?: InputMaybe<CmsPageFilter>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryCollectionArgs = {
  id: Scalars['ID']['input'];
};


export type QueryCountriesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryCountryArgs = {
  code: Scalars['String']['input'];
};


export type QueryCurrenciesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryCurrencyArgs = {
  code: Scalars['String']['input'];
};


export type QueryDeliveryPointsArgs = {
  cartId: Scalars['ID']['input'];
  groupKey: Scalars['String']['input'];
  search?: InputMaybe<Scalars['String']['input']>;
};


export type QueryFreeShippingThresholdArgs = {
  countryCode?: InputMaybe<Scalars['String']['input']>;
};


export type QueryGiftCardBalanceArgs = {
  code: Scalars['String']['input'];
};


export type QueryGuestOrderArgs = {
  email: Scalars['String']['input'];
  id: Scalars['ID']['input'];
};


export type QueryLocaleArgs = {
  code: Scalars['String']['input'];
};


export type QueryLocalesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryMenuArgs = {
  slug: Scalars['String']['input'];
};


export type QueryMenusArgs = {
  slugs: Array<Scalars['String']['input']>;
};


export type QueryMyOrderArgs = {
  id: Scalars['ID']['input'];
};


export type QueryPaymentMethodsArgs = {
  cartId?: InputMaybe<Scalars['ID']['input']>;
  countryCode: Scalars['String']['input'];
};


export type QueryPreviewBlogPostArgs = {
  token: Scalars['String']['input'];
};


export type QueryPreviewCmsPageArgs = {
  token: Scalars['String']['input'];
};


export type QueryProductArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProductQuestionsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  productId: Scalars['ID']['input'];
};


export type QueryProductsByIdsArgs = {
  ids: Array<Scalars['ID']['input']>;
};


export type QueryProductsBySkusArgs = {
  skus: Array<Scalars['String']['input']>;
};


export type QueryPublicStoresArgs = {
  input?: InputMaybe<PublicStoresInput>;
};


export type QueryRegionsArgs = {
  countryCode: Scalars['String']['input'];
};


export type QueryRouteArgs = {
  path: Scalars['String']['input'];
};


export type QuerySearchBlogPostsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  query: Scalars['String']['input'];
};


export type QuerySearchProductsArgs = {
  limit?: InputMaybe<Scalars['Int']['input']>;
  offset?: InputMaybe<Scalars['Int']['input']>;
  query: Scalars['String']['input'];
};


export type QuerySharedWishlistArgs = {
  token: Scalars['String']['input'];
};


export type QueryShippingQuotesArgs = {
  cartId?: InputMaybe<Scalars['ID']['input']>;
  countryCode: Scalars['String']['input'];
  postalCode?: InputMaybe<Scalars['String']['input']>;
  regionCode?: InputMaybe<Scalars['String']['input']>;
};


export type QuerySitemapEntriesArgs = {
  entityType: SitemapEntityType;
  limit: Scalars['Int']['input'];
  offset: Scalars['Int']['input'];
};


export type QueryTrackOrderArgs = {
  email: Scalars['String']['input'];
  number: Scalars['String']['input'];
  postalCode: Scalars['String']['input'];
};


export type QueryValidateAddressArgs = {
  city: Scalars['String']['input'];
  countryCode: Scalars['String']['input'];
  postalCode: Scalars['String']['input'];
  state?: InputMaybe<Scalars['String']['input']>;
  street1: Scalars['String']['input'];
  street2?: InputMaybe<Scalars['String']['input']>;
};

export type ReferralInfo = {
  __typename?: 'ReferralInfo';
  code: Scalars['String']['output'];
  earnedCents: Scalars['Int']['output'];
  enabled: Scalars['Boolean']['output'];
  refereeRewardCents: Scalars['Int']['output'];
  referredCount: Scalars['Int']['output'];
  referrerRewardCents: Scalars['Int']['output'];
  rewardedCount: Scalars['Int']['output'];
};

export type Region = {
  __typename?: 'Region';
  code: Scalars['String']['output'];
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type ReorderResult = {
  __typename?: 'ReorderResult';
  added: Scalars['Int']['output'];
  cart: Cart;
  skipped: Array<ReorderSkippedLine>;
};

export type ReorderSkippedLine = {
  __typename?: 'ReorderSkippedLine';
  name: Scalars['String']['output'];
  reason: Scalars['String']['output'];
  sku: Scalars['String']['output'];
};

export type RequestReturnInput = {
  lines: Array<ReturnLineInput>;
  orderId: Scalars['ID']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
  resolution?: InputMaybe<Scalars['String']['input']>;
};

export type RequirementMode =
  | 'OPTIONAL'
  | 'REQUIRED';

export type RequirementStatus =
  | 'INVALID'
  | 'MISSING'
  | 'OPTIONAL'
  | 'SATISFIED';

export type ReturnLineInput = {
  orderLineId: Scalars['ID']['input'];
  quantity: Scalars['Float']['input'];
  reason?: InputMaybe<Scalars['String']['input']>;
};

export type ReturnableLine = {
  __typename?: 'ReturnableLine';
  orderLineId: Scalars['ID']['output'];
  quantity: Scalars['Float']['output'];
};

export type Review = {
  __typename?: 'Review';
  answers: Array<ReviewAnswer>;
  body: Scalars['String']['output'];
  customerDisplayName: Scalars['String']['output'];
  helpfulCount: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  isVerifiedPurchase: Scalars['Boolean']['output'];
  media: Array<ReviewMedia>;
  merchantReply: Scalars['String']['output'];
  merchantReplyAt?: Maybe<Scalars['Time']['output']>;
  myVote: Scalars['Int']['output'];
  notHelpfulCount: Scalars['Int']['output'];
  overallRating?: Maybe<Scalars['Float']['output']>;
  submittedAt: Scalars['Time']['output'];
  title: Scalars['String']['output'];
};

export type ReviewAggregates = {
  __typename?: 'ReviewAggregates';
  count: Scalars['Int']['output'];
  distribution: Array<Scalars['Int']['output']>;
  overallAverage?: Maybe<Scalars['Float']['output']>;
  perQuestion: Array<ReviewQuestionAverage>;
  votingPolicy: ReviewVotingPolicy;
};

export type ReviewAnswer = {
  __typename?: 'ReviewAnswer';
  boolValue?: Maybe<Scalars['Boolean']['output']>;
  question: ReviewQuestion;
  rating?: Maybe<Scalars['Int']['output']>;
  text: Scalars['String']['output'];
};

export type ReviewAnswerInput = {
  boolValue?: InputMaybe<Scalars['Boolean']['input']>;
  questionId: Scalars['ID']['input'];
  rating?: InputMaybe<Scalars['Int']['input']>;
  text?: InputMaybe<Scalars['String']['input']>;
};

export type ReviewEligibility = {
  __typename?: 'ReviewEligibility';
  canReview: Scalars['Boolean']['output'];
  existingReviewId?: Maybe<Scalars['ID']['output']>;
  reason?: Maybe<ReviewIneligibilityReason>;
};

export type ReviewIneligibilityReason =
  | 'ALREADY_REVIEWED'
  | 'NOT_AUTHENTICATED'
  | 'NOT_PURCHASED';

export type ReviewMedia = {
  __typename?: 'ReviewMedia';
  height?: Maybe<Scalars['Int']['output']>;
  id: Scalars['ID']['output'];
  mimeType: Scalars['String']['output'];
  url: Scalars['String']['output'];
  width?: Maybe<Scalars['Int']['output']>;
};

export type ReviewMediaUpload = {
  __typename?: 'ReviewMediaUpload';
  assetId: Scalars['ID']['output'];
  expiresAt: Scalars['Time']['output'];
  uploadUrl: Scalars['String']['output'];
};

export type ReviewQuestion = {
  __typename?: 'ReviewQuestion';
  allowComment: Scalars['Boolean']['output'];
  helpText: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  inputType: ReviewQuestionInputType;
  isRequired: Scalars['Boolean']['output'];
  label: Scalars['String']['output'];
  slug: Scalars['String']['output'];
  sortOrder: Scalars['Int']['output'];
};

export type ReviewQuestionAverage = {
  __typename?: 'ReviewQuestionAverage';
  average: Scalars['Float']['output'];
  count: Scalars['Int']['output'];
  question: ReviewQuestion;
};

export type ReviewQuestionInputType =
  | 'FREE_TEXT'
  | 'RATING_1_5'
  | 'YES_NO';

export type ReviewSort =
  | 'HIGHEST'
  | 'LOWEST'
  | 'NEWEST'
  | 'OLDEST';

export type ReviewVotingPolicy = {
  __typename?: 'ReviewVotingPolicy';
  requireAuthentication: Scalars['Boolean']['output'];
};

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

export type RouteResolution = {
  __typename?: 'RouteResolution';
  entityId: Scalars['ID']['output'];
  kind: RouteKind;
  pluginKind?: Maybe<Scalars['String']['output']>;
  redirectStatusCode?: Maybe<Scalars['Int']['output']>;
  redirectTo?: Maybe<Scalars['String']['output']>;
};

export type SavedItem = {
  __typename?: 'SavedItem';
  id: Scalars['ID']['output'];
  imageUrl?: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  quantity: Scalars['Float']['output'];
  sku: Scalars['String']['output'];
};

export type SavedPaymentMethod = {
  __typename?: 'SavedPaymentMethod';
  brand: Scalars['String']['output'];
  expMonth: Scalars['Int']['output'];
  expYear: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  last4: Scalars['String']['output'];
  providerSlug: Scalars['String']['output'];
};

export type ShippingQuote = {
  __typename?: 'ShippingQuote';
  carrierName: Scalars['String']['output'];
  carrierSlug: Scalars['String']['output'];
  code: Scalars['String']['output'];
  estimatedArrival?: Maybe<Scalars['String']['output']>;
  estimatedDays?: Maybe<Scalars['Int']['output']>;
  name: Scalars['String']['output'];
  priceCents: Scalars['Int']['output'];
};

export type SitemapCounts = {
  __typename?: 'SitemapCounts';
  categories: Scalars['Int']['output'];
  collections: Scalars['Int']['output'];
  products: Scalars['Int']['output'];
};

export type SitemapEntityType =
  | 'CATEGORY'
  | 'COLLECTION'
  | 'PRODUCT';

export type SitemapEntry = {
  __typename?: 'SitemapEntry';
  path: Scalars['String']['output'];
  updatedAt: Scalars['Time']['output'];
};

export type StoreFeatures = {
  __typename?: 'StoreFeatures';
  signals: Scalars['Boolean']['output'];
};

export type StoreStockStatus =
  | 'IN_STOCK'
  | 'LOW_STOCK'
  | 'OUT_OF_STOCK'
  | 'UNKNOWN';

export type SubmitReviewInput = {
  answers: Array<ReviewAnswerInput>;
  body?: InputMaybe<Scalars['String']['input']>;
  guestEmail?: InputMaybe<Scalars['String']['input']>;
  guestName?: InputMaybe<Scalars['String']['input']>;
  mediaAssetIds?: InputMaybe<Array<Scalars['ID']['input']>>;
  productId: Scalars['ID']['input'];
  title?: InputMaybe<Scalars['String']['input']>;
};

export type SubmitWebFormInput = {
  captchaToken?: InputMaybe<Scalars['String']['input']>;
  files?: InputMaybe<Array<WebFormFileInput>>;
  formId: Scalars['ID']['input'];
  honeypot?: InputMaybe<Scalars['String']['input']>;
  values: Scalars['JSON']['input'];
};

export type SubmitWebFormResult = {
  __typename?: 'SubmitWebFormResult';
  errors: Array<WebFormFieldError>;
  ok: Scalars['Boolean']['output'];
  redirectUrl?: Maybe<Scalars['String']['output']>;
  successMessage?: Maybe<Scalars['String']['output']>;
};

export type SubscriptionInterval =
  | 'DAY'
  | 'MONTH'
  | 'WEEK'
  | 'YEAR';

export type SubscriptionStatus =
  | 'ACTIVE'
  | 'CANCELED'
  | 'EXPIRED'
  | 'PAUSED';

export type SubscriptionTerms = {
  __typename?: 'SubscriptionTerms';
  intervalCount: Scalars['Int']['output'];
  intervalUnit: SubscriptionInterval;
  trialDays: Scalars['Int']['output'];
};

export type TierPrice = {
  __typename?: 'TierPrice';
  minQty: Scalars['Float']['output'];
  price: Scalars['Int']['output'];
};

export type TrackOrderResult = {
  __typename?: 'TrackOrderResult';
  email: Scalars['String']['output'];
  orderId: Scalars['ID']['output'];
  requiresLogin: Scalars['Boolean']['output'];
};

export type VariantMode =
  | 'ATTRIBUTE'
  | 'LIST';

export type WebFormFieldError = {
  __typename?: 'WebFormFieldError';
  field: Scalars['String']['output'];
  message: Scalars['String']['output'];
};

export type WebFormFileInput = {
  fieldKey: Scalars['String']['input'];
  name: Scalars['String']['input'];
  objectKey: Scalars['String']['input'];
};

export type WebFormUploadTicket = {
  __typename?: 'WebFormUploadTicket';
  objectKey: Scalars['String']['output'];
  url: Scalars['String']['output'];
};
