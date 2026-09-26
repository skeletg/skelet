/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type * as Types from '../../graphql/schema.types';

import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type AddressInput = {
  city: string;
  company?: string | null | undefined;
  countryCode: string;
  firstName: string;
  lastName: string;
  phone?: string | null | undefined;
  postalCode: string;
  state?: string | null | undefined;
  street1: string;
  street2?: string | null | undefined;
};

export type CreateDraftOrderInput = {
  billingAddress: AddressInput;
  channelId: string | number;
  customerId?: string | number | null | undefined;
  customerNote?: string | null | undefined;
  email: string;
  lines: Array<DraftOrderLineInput>;
  manualDiscountCents?: number | null | undefined;
  shippingAddress: AddressInput;
  shippingCarrierSlug?: string | null | undefined;
  shippingMethodCode?: string | null | undefined;
  sourceCartId?: string | number | null | undefined;
};

export type CreateFulfillmentInput = {
  carrier?: string | null | undefined;
  insuredValueCents?: number | null | undefined;
  lines: Array<FulfillmentLineInput>;
  orderId: string | number;
  trackingNumber?: string | null | undefined;
};

export type CreditMemoLineInput = {
  orderLineId: string | number;
  quantity: number;
};

export type DraftOrderLineInput = {
  productSku: string;
  quantity: number;
  unitPriceCents?: number | null | undefined;
};

export type EditAddLineInput = {
  productSku: string;
  quantity: number;
};

export type EditLineQtyInput = {
  orderLineId: string | number;
  quantity: number;
};

export type EditOrderAddressInput = {
  billingAddress?: AddressInput | null | undefined;
  orderId: string | number;
  shippingAddress?: AddressInput | null | undefined;
};

export type EditOrderInput = {
  addLines?: Array<EditAddLineInput> | null | undefined;
  orderId: string | number;
  removeLines?: Array<string | number> | null | undefined;
  updateLines?: Array<EditLineQtyInput> | null | undefined;
};

export type FulfillmentLineInput = {
  orderLineId: string | number;
  quantity: number;
};

export type IssueCreditMemoInput = {
  adjustmentFee?: number | null | undefined;
  adjustmentRefund?: number | null | undefined;
  lines?: Array<CreditMemoLineInput> | null | undefined;
  orderId: string | number;
  reason?: string | null | undefined;
  refundTarget?: string | null | undefined;
  shippingRefund?: number | null | undefined;
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

export type OrderStatus =
  | 'CANCELLED'
  | 'CONFIRMED'
  | 'DRAFT'
  | 'FULFILLED'
  | 'ON_HOLD'
  | 'PARTIALLY_FULFILLED'
  | 'PENDING'
  | 'PROCESSING';

export type PaymentStatus =
  | 'AUTHORIZED'
  | 'PAID'
  | 'PARTIALLY_CAPTURED'
  | 'PARTIALLY_REFUNDED'
  | 'REFUNDED'
  | 'UNPAID'
  | 'VOIDED';

export type RecordOfflinePaymentInput = {
  amount?: number | null | undefined;
  method?: string | null | undefined;
  note?: string | null | undefined;
  orderId: string | number;
  receivedAt?: string | null | undefined;
  reference?: string | null | undefined;
};

export type RequestReturnInput = {
  lines: Array<ReturnLineInput>;
  orderId: string | number;
  reason?: string | null | undefined;
  refundTarget?: string | null | undefined;
  resolution?: string | null | undefined;
  restock?: boolean | null | undefined;
};

export type ReturnLineInput = {
  orderLineId: string | number;
  quantity: number;
  reason?: string | null | undefined;
};

export type OrdersPagedQueryVariables = Exact<{
  search?: string | null | undefined;
  statuses?: Array<string> | string | null | undefined;
  paymentStatuses?: Array<string> | string | null | undefined;
  limit?: number | null | undefined;
  offset?: number | null | undefined;
  channelId?: string | number | null | undefined;
}>;


export type OrdersPagedQuery = { ordersPaged: { totalCount: number, items: Array<{ id: string, number: string, status: Types.OrderStatus, paymentStatus: Types.PaymentStatus, total: number, currencyCode: string, createdAt: string, shippingAddress: { firstName: string, lastName: string } }> } };

export type OrderQueryVariables = Exact<{
  id: string | number;
}>;


export type OrderQuery = { order: { id: string, channelId: string, number: string, status: Types.OrderStatus, paymentStatus: Types.PaymentStatus, paymentProvider: string, paymentReference: string, email: string, customerId: string | null, subtotal: number, shippingAmount: number, discountAmount: number, taxAmount: number, total: number, giftCardAmount: number, storeCreditAmount: number, currencyCode: string, customerNote: string | null, carrier: string, trackingNumber: string, trackingUrl: string, createdAt: string, billingAddress: { firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null }, shippingAddress: { firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null }, lines: Array<{ id: string, productName: string, productSku: string, unitPrice: number, quantity: number, totalPrice: number, isGift: boolean, components: Array<{ id: string, productSku: string, productName: string, unitPrice: number, quantity: number }>, options: Array<{ id: string, optionTitle: string, valueLabel: string, textValue: string, priceDelta: number, quantity: number, fileUrl: string }> }>, events: Array<{ id: string, type: string, message: string, createdAt: string }>, fulfillments: Array<{ id: string, sequence: number, status: string, carrier: string, trackingNumber: string, insuredValueCents: number, shippedAt: string | null, lines: Array<{ id: string, orderLineId: string, quantity: number }>, labels: Array<{ id: string, format: string, trackingNumber: string, dataUrl: string, createdAt: string }> }>, paymentTransactions: Array<{ id: string, kind: string, amount: number, status: string, providerSlug: string, providerReference: string, reason: string, createdAt: string }>, taxLines: Array<{ id: string, ruleId: string | null, name: string, rateBps: number, amount: number }>, appliedPromotions: Array<{ id: string, ruleId: string | null, ruleName: string, ruleSlug: string, code: string, discount: number, appliedAt: string }>, returns: Array<{ id: string, rmaNumber: string, status: string, reason: string, restock: boolean, refundTarget: string, refundAmount: number, createdAt: string, updatedAt: string, lines: Array<{ id: string, orderLineId: string, quantity: number, reason: string, refundUnitCents: number }> }> } };

export type GenerateShippingLabelMutationVariables = Exact<{
  fulfillmentId: string | number;
}>;


export type GenerateShippingLabelMutation = { generateShippingLabel: { id: string, format: string, trackingNumber: string, dataUrl: string } };

export type ResendOrderEmailMutationVariables = Exact<{
  orderId: string | number;
  message: Types.OrderEmailMessage;
  fulfillmentId?: string | number | null | undefined;
}>;


export type ResendOrderEmailMutation = { resendOrderEmail: boolean };

export type ConfirmOrderMutationVariables = Exact<{
  id: string | number;
}>;


export type ConfirmOrderMutation = { confirmOrder: { id: string, status: Types.OrderStatus } };

export type MarkOrderProcessingMutationVariables = Exact<{
  id: string | number;
}>;


export type MarkOrderProcessingMutation = { markProcessing: { id: string, status: Types.OrderStatus } };

export type HoldOrderMutationVariables = Exact<{
  id: string | number;
  reason?: string | null | undefined;
}>;


export type HoldOrderMutation = { holdOrder: { id: string, status: Types.OrderStatus } };

export type ReleaseOrderMutationVariables = Exact<{
  id: string | number;
}>;


export type ReleaseOrderMutation = { releaseOrder: { id: string, status: Types.OrderStatus } };

export type FulfillOrderMutationVariables = Exact<{
  id: string | number;
  carrier?: string | null | undefined;
  trackingNumber?: string | null | undefined;
}>;


export type FulfillOrderMutation = { fulfillOrder: { id: string, status: Types.OrderStatus, carrier: string, trackingNumber: string } };

export type CancelOrderMutationVariables = Exact<{
  id: string | number;
}>;


export type CancelOrderMutation = { cancelOrder: { id: string, status: Types.OrderStatus } };

export type CreateFulfillmentMutationVariables = Exact<{
  input: Types.CreateFulfillmentInput;
}>;


export type CreateFulfillmentMutation = { createFulfillment: { id: string, status: Types.OrderStatus } };

export type EditOrderMutationVariables = Exact<{
  input: Types.EditOrderInput;
}>;


export type EditOrderMutation = { editOrder: { id: string, status: Types.OrderStatus, total: number } };

export type EditOrderAddressMutationVariables = Exact<{
  input: Types.EditOrderAddressInput;
}>;


export type EditOrderAddressMutation = { editOrderAddress: { id: string, status: Types.OrderStatus, subtotal: number, shippingAmount: number, discountAmount: number, taxAmount: number, total: number, currencyCode: string, billingAddress: { firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null }, shippingAddress: { firstName: string, lastName: string, company: string | null, street1: string, street2: string | null, city: string, state: string | null, postalCode: string, countryCode: string, phone: string | null } } };

export type CreateDraftOrderMutationVariables = Exact<{
  input: Types.CreateDraftOrderInput;
}>;


export type CreateDraftOrderMutation = { createDraftOrder: { id: string, status: Types.OrderStatus } };

export type PlaceDraftOrderMutationVariables = Exact<{
  id: string | number;
}>;


export type PlaceDraftOrderMutation = { placeDraftOrder: { id: string, status: Types.OrderStatus } };

export type CapturePaymentMutationVariables = Exact<{
  orderId: string | number;
  amount?: number | null | undefined;
}>;


export type CapturePaymentMutation = { capturePayment: { id: string, paymentStatus: Types.PaymentStatus } };

export type RefundPaymentMutationVariables = Exact<{
  orderId: string | number;
  amount?: number | null | undefined;
  reason?: string | null | undefined;
}>;


export type RefundPaymentMutation = { refundPayment: { id: string, paymentStatus: Types.PaymentStatus } };

export type VoidPaymentMutationVariables = Exact<{
  orderId: string | number;
}>;


export type VoidPaymentMutation = { voidPayment: { id: string, paymentStatus: Types.PaymentStatus } };

export type OfflinePaymentMethodsQueryVariables = Exact<{
  channelId: string | number;
}>;


export type OfflinePaymentMethodsQuery = { paymentMethods: Array<{ providerSlug: string, displayName: string, isOffline: boolean }> };

export type RecordOfflinePaymentMutationVariables = Exact<{
  input: Types.RecordOfflinePaymentInput;
}>;


export type RecordOfflinePaymentMutation = { recordOfflinePayment: { id: string, paymentStatus: Types.PaymentStatus } };

export type RequestReturnMutationVariables = Exact<{
  input: Types.RequestReturnInput;
}>;


export type RequestReturnMutation = { requestReturn: { id: string, rmaNumber: string, status: string } };

export type ApproveReturnMutationVariables = Exact<{
  id: string | number;
}>;


export type ApproveReturnMutation = { approveReturn: { id: string, status: string } };

export type RejectReturnMutationVariables = Exact<{
  id: string | number;
}>;


export type RejectReturnMutation = { rejectReturn: { id: string, status: string } };

export type ReceiveReturnMutationVariables = Exact<{
  id: string | number;
}>;


export type ReceiveReturnMutation = { receiveReturn: { id: string, status: string } };

export type IssueReturnRefundMutationVariables = Exact<{
  id: string | number;
  reason?: string | null | undefined;
}>;


export type IssueReturnRefundMutation = { issueReturnRefund: { id: string, status: string, refundAmount: number } };

export type OrderDocumentPdfQueryVariables = Exact<{
  orderId: string | number;
  kind: Types.OrderDocumentKind;
  creditMemoId?: string | number | null | undefined;
}>;


export type OrderDocumentPdfQuery = { orderDocumentPdf: string };

export type CreditMemosQueryVariables = Exact<{
  orderId: string | number;
}>;


export type CreditMemosQuery = { creditMemos: Array<{ id: string, number: string, returnId: string | null, itemsTotal: number, shippingRefund: number, adjustmentRefund: number, adjustmentFee: number, total: number, refundTarget: string, offline: boolean, reason: string, createdAt: string, lines: Array<{ orderLineId: string, productSku: string, productName: string, quantity: number, unitAmount: number, totalAmount: number }> }> };

export type CreditableQuantitiesQueryVariables = Exact<{
  orderId: string | number;
}>;


export type CreditableQuantitiesQuery = { creditableQuantities: Array<{ orderLineId: string, quantity: number }> };

export type IssueCreditMemoMutationVariables = Exact<{
  input: Types.IssueCreditMemoInput;
}>;


export type IssueCreditMemoMutation = { issueCreditMemo: { id: string, number: string, total: number } };

export type OrderQueueCountsQueryVariables = Exact<{
  channelId?: string | number | null | undefined;
}>;


export type OrderQueueCountsQuery = { pending: { totalCount: number }, toFulfil: { totalCount: number }, onHold: { totalCount: number }, unpaid: { totalCount: number } };


export const OrdersPagedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"OrdersPaged"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"search"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"statuses"}},"type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paymentStatuses"}},"type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"offset"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ordersPaged"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"search"},"value":{"kind":"Variable","name":{"kind":"Name","value":"search"}}},{"kind":"Argument","name":{"kind":"Name","value":"statuses"},"value":{"kind":"Variable","name":{"kind":"Name","value":"statuses"}}},{"kind":"Argument","name":{"kind":"Name","value":"paymentStatuses"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paymentStatuses"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}},{"kind":"Argument","name":{"kind":"Name","value":"offset"},"value":{"kind":"Variable","name":{"kind":"Name","value":"offset"}}},{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"number"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"paymentStatus"}},{"kind":"Field","name":{"kind":"Name","value":"total"}},{"kind":"Field","name":{"kind":"Name","value":"currencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"shippingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}}]}}]}}]}}]}}]} as unknown as DocumentNode<OrdersPagedQuery, OrdersPagedQueryVariables>;
export const OrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Order"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"order"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"channelId"}},{"kind":"Field","name":{"kind":"Name","value":"number"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"paymentStatus"}},{"kind":"Field","name":{"kind":"Name","value":"paymentProvider"}},{"kind":"Field","name":{"kind":"Name","value":"paymentReference"}},{"kind":"Field","name":{"kind":"Name","value":"email"}},{"kind":"Field","name":{"kind":"Name","value":"customerId"}},{"kind":"Field","name":{"kind":"Name","value":"subtotal"}},{"kind":"Field","name":{"kind":"Name","value":"shippingAmount"}},{"kind":"Field","name":{"kind":"Name","value":"discountAmount"}},{"kind":"Field","name":{"kind":"Name","value":"taxAmount"}},{"kind":"Field","name":{"kind":"Name","value":"total"}},{"kind":"Field","name":{"kind":"Name","value":"giftCardAmount"}},{"kind":"Field","name":{"kind":"Name","value":"storeCreditAmount"}},{"kind":"Field","name":{"kind":"Name","value":"currencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"customerNote"}},{"kind":"Field","name":{"kind":"Name","value":"carrier"}},{"kind":"Field","name":{"kind":"Name","value":"trackingNumber"}},{"kind":"Field","name":{"kind":"Name","value":"trackingUrl"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"billingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"company"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}}]}},{"kind":"Field","name":{"kind":"Name","value":"shippingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"company"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}}]}},{"kind":"Field","name":{"kind":"Name","value":"lines"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"productName"}},{"kind":"Field","name":{"kind":"Name","value":"productSku"}},{"kind":"Field","name":{"kind":"Name","value":"unitPrice"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"totalPrice"}},{"kind":"Field","name":{"kind":"Name","value":"isGift"}},{"kind":"Field","name":{"kind":"Name","value":"components"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"productSku"}},{"kind":"Field","name":{"kind":"Name","value":"productName"}},{"kind":"Field","name":{"kind":"Name","value":"unitPrice"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}}]}},{"kind":"Field","name":{"kind":"Name","value":"options"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"optionTitle"}},{"kind":"Field","name":{"kind":"Name","value":"valueLabel"}},{"kind":"Field","name":{"kind":"Name","value":"textValue"}},{"kind":"Field","name":{"kind":"Name","value":"priceDelta"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"fileUrl"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"fulfillments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sequence"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"carrier"}},{"kind":"Field","name":{"kind":"Name","value":"trackingNumber"}},{"kind":"Field","name":{"kind":"Name","value":"insuredValueCents"}},{"kind":"Field","name":{"kind":"Name","value":"shippedAt"}},{"kind":"Field","name":{"kind":"Name","value":"lines"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"orderLineId"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}}]}},{"kind":"Field","name":{"kind":"Name","value":"labels"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"format"}},{"kind":"Field","name":{"kind":"Name","value":"trackingNumber"}},{"kind":"Field","name":{"kind":"Name","value":"dataUrl"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"paymentTransactions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"amount"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"providerSlug"}},{"kind":"Field","name":{"kind":"Name","value":"providerReference"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"taxLines"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"ruleId"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rateBps"}},{"kind":"Field","name":{"kind":"Name","value":"amount"}}]}},{"kind":"Field","name":{"kind":"Name","value":"appliedPromotions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"ruleId"}},{"kind":"Field","name":{"kind":"Name","value":"ruleName"}},{"kind":"Field","name":{"kind":"Name","value":"ruleSlug"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"discount"}},{"kind":"Field","name":{"kind":"Name","value":"appliedAt"}}]}},{"kind":"Field","name":{"kind":"Name","value":"returns"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rmaNumber"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"restock"}},{"kind":"Field","name":{"kind":"Name","value":"refundTarget"}},{"kind":"Field","name":{"kind":"Name","value":"refundAmount"}},{"kind":"Field","name":{"kind":"Name","value":"lines"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"orderLineId"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"refundUnitCents"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}}]}}]}}]}}]} as unknown as DocumentNode<OrderQuery, OrderQueryVariables>;
export const GenerateShippingLabelDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"GenerateShippingLabel"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"fulfillmentId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"generateShippingLabel"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"fulfillmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"fulfillmentId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"format"}},{"kind":"Field","name":{"kind":"Name","value":"trackingNumber"}},{"kind":"Field","name":{"kind":"Name","value":"dataUrl"}}]}}]}}]} as unknown as DocumentNode<GenerateShippingLabelMutation, GenerateShippingLabelMutationVariables>;
export const ResendOrderEmailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ResendOrderEmail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"message"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"OrderEmailMessage"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"fulfillmentId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"resendOrderEmail"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}},{"kind":"Argument","name":{"kind":"Name","value":"message"},"value":{"kind":"Variable","name":{"kind":"Name","value":"message"}}},{"kind":"Argument","name":{"kind":"Name","value":"fulfillmentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"fulfillmentId"}}}]}]}}]} as unknown as DocumentNode<ResendOrderEmailMutation, ResendOrderEmailMutationVariables>;
export const ConfirmOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ConfirmOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"confirmOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<ConfirmOrderMutation, ConfirmOrderMutationVariables>;
export const MarkOrderProcessingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MarkOrderProcessing"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"markProcessing"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<MarkOrderProcessingMutation, MarkOrderProcessingMutationVariables>;
export const HoldOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"HoldOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"holdOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<HoldOrderMutation, HoldOrderMutationVariables>;
export const ReleaseOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReleaseOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"releaseOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<ReleaseOrderMutation, ReleaseOrderMutationVariables>;
export const FulfillOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"FulfillOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"carrier"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"trackingNumber"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"fulfillOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"carrier"},"value":{"kind":"Variable","name":{"kind":"Name","value":"carrier"}}},{"kind":"Argument","name":{"kind":"Name","value":"trackingNumber"},"value":{"kind":"Variable","name":{"kind":"Name","value":"trackingNumber"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"carrier"}},{"kind":"Field","name":{"kind":"Name","value":"trackingNumber"}}]}}]}}]} as unknown as DocumentNode<FulfillOrderMutation, FulfillOrderMutationVariables>;
export const CancelOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<CancelOrderMutation, CancelOrderMutationVariables>;
export const CreateFulfillmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateFulfillment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateFulfillmentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createFulfillment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<CreateFulfillmentMutation, CreateFulfillmentMutationVariables>;
export const EditOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EditOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EditOrderInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"editOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"total"}}]}}]}}]} as unknown as DocumentNode<EditOrderMutation, EditOrderMutationVariables>;
export const EditOrderAddressDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"EditOrderAddress"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EditOrderAddressInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"editOrderAddress"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"subtotal"}},{"kind":"Field","name":{"kind":"Name","value":"shippingAmount"}},{"kind":"Field","name":{"kind":"Name","value":"discountAmount"}},{"kind":"Field","name":{"kind":"Name","value":"taxAmount"}},{"kind":"Field","name":{"kind":"Name","value":"total"}},{"kind":"Field","name":{"kind":"Name","value":"currencyCode"}},{"kind":"Field","name":{"kind":"Name","value":"billingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"company"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}}]}},{"kind":"Field","name":{"kind":"Name","value":"shippingAddress"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"firstName"}},{"kind":"Field","name":{"kind":"Name","value":"lastName"}},{"kind":"Field","name":{"kind":"Name","value":"company"}},{"kind":"Field","name":{"kind":"Name","value":"street1"}},{"kind":"Field","name":{"kind":"Name","value":"street2"}},{"kind":"Field","name":{"kind":"Name","value":"city"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"postalCode"}},{"kind":"Field","name":{"kind":"Name","value":"countryCode"}},{"kind":"Field","name":{"kind":"Name","value":"phone"}}]}}]}}]}}]} as unknown as DocumentNode<EditOrderAddressMutation, EditOrderAddressMutationVariables>;
export const CreateDraftOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateDraftOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateDraftOrderInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createDraftOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<CreateDraftOrderMutation, CreateDraftOrderMutationVariables>;
export const PlaceDraftOrderDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PlaceDraftOrder"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"placeDraftOrder"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<PlaceDraftOrderMutation, PlaceDraftOrderMutationVariables>;
export const CapturePaymentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CapturePayment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"amount"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"capturePayment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}},{"kind":"Argument","name":{"kind":"Name","value":"amount"},"value":{"kind":"Variable","name":{"kind":"Name","value":"amount"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"paymentStatus"}}]}}]}}]} as unknown as DocumentNode<CapturePaymentMutation, CapturePaymentMutationVariables>;
export const RefundPaymentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RefundPayment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"amount"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"refundPayment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}},{"kind":"Argument","name":{"kind":"Name","value":"amount"},"value":{"kind":"Variable","name":{"kind":"Name","value":"amount"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"paymentStatus"}}]}}]}}]} as unknown as DocumentNode<RefundPaymentMutation, RefundPaymentMutationVariables>;
export const VoidPaymentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"VoidPayment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"voidPayment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"paymentStatus"}}]}}]}}]} as unknown as DocumentNode<VoidPaymentMutation, VoidPaymentMutationVariables>;
export const OfflinePaymentMethodsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"OfflinePaymentMethods"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"paymentMethods"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"providerSlug"}},{"kind":"Field","name":{"kind":"Name","value":"displayName"}},{"kind":"Field","name":{"kind":"Name","value":"isOffline"}}]}}]}}]} as unknown as DocumentNode<OfflinePaymentMethodsQuery, OfflinePaymentMethodsQueryVariables>;
export const RecordOfflinePaymentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RecordOfflinePayment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RecordOfflinePaymentInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"recordOfflinePayment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"paymentStatus"}}]}}]}}]} as unknown as DocumentNode<RecordOfflinePaymentMutation, RecordOfflinePaymentMutationVariables>;
export const RequestReturnDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RequestReturn"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RequestReturnInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestReturn"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"rmaNumber"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<RequestReturnMutation, RequestReturnMutationVariables>;
export const ApproveReturnDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ApproveReturn"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approveReturn"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<ApproveReturnMutation, ApproveReturnMutationVariables>;
export const RejectReturnDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RejectReturn"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rejectReturn"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<RejectReturnMutation, RejectReturnMutationVariables>;
export const ReceiveReturnDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReceiveReturn"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"receiveReturn"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<ReceiveReturnMutation, ReceiveReturnMutationVariables>;
export const IssueReturnRefundDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"IssueReturnRefund"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"issueReturnRefund"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"refundAmount"}}]}}]}}]} as unknown as DocumentNode<IssueReturnRefundMutation, IssueReturnRefundMutationVariables>;
export const OrderDocumentPdfDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"OrderDocumentPdf"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"kind"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"OrderDocumentKind"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"creditMemoId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"orderDocumentPdf"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}},{"kind":"Argument","name":{"kind":"Name","value":"kind"},"value":{"kind":"Variable","name":{"kind":"Name","value":"kind"}}},{"kind":"Argument","name":{"kind":"Name","value":"creditMemoId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"creditMemoId"}}}]}]}}]} as unknown as DocumentNode<OrderDocumentPdfQuery, OrderDocumentPdfQueryVariables>;
export const CreditMemosDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CreditMemos"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"creditMemos"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"number"}},{"kind":"Field","name":{"kind":"Name","value":"returnId"}},{"kind":"Field","name":{"kind":"Name","value":"itemsTotal"}},{"kind":"Field","name":{"kind":"Name","value":"shippingRefund"}},{"kind":"Field","name":{"kind":"Name","value":"adjustmentRefund"}},{"kind":"Field","name":{"kind":"Name","value":"adjustmentFee"}},{"kind":"Field","name":{"kind":"Name","value":"total"}},{"kind":"Field","name":{"kind":"Name","value":"refundTarget"}},{"kind":"Field","name":{"kind":"Name","value":"offline"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lines"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"orderLineId"}},{"kind":"Field","name":{"kind":"Name","value":"productSku"}},{"kind":"Field","name":{"kind":"Name","value":"productName"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}},{"kind":"Field","name":{"kind":"Name","value":"unitAmount"}},{"kind":"Field","name":{"kind":"Name","value":"totalAmount"}}]}}]}}]}}]} as unknown as DocumentNode<CreditMemosQuery, CreditMemosQueryVariables>;
export const CreditableQuantitiesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"CreditableQuantities"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"creditableQuantities"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"orderId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"orderLineId"}},{"kind":"Field","name":{"kind":"Name","value":"quantity"}}]}}]}}]} as unknown as DocumentNode<CreditableQuantitiesQuery, CreditableQuantitiesQueryVariables>;
export const IssueCreditMemoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"IssueCreditMemo"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"IssueCreditMemoInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"issueCreditMemo"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"number"}},{"kind":"Field","name":{"kind":"Name","value":"total"}}]}}]}}]} as unknown as DocumentNode<IssueCreditMemoMutation, IssueCreditMemoMutationVariables>;
export const OrderQueueCountsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"OrderQueueCounts"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","alias":{"kind":"Name","value":"pending"},"name":{"kind":"Name","value":"ordersPaged"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"statuses"},"value":{"kind":"ListValue","values":[{"kind":"StringValue","value":"PENDING","block":false}]}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"1"}},{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"toFulfil"},"name":{"kind":"Name","value":"ordersPaged"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"statuses"},"value":{"kind":"ListValue","values":[{"kind":"StringValue","value":"CONFIRMED","block":false},{"kind":"StringValue","value":"PROCESSING","block":false},{"kind":"StringValue","value":"PARTIALLY_FULFILLED","block":false}]}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"1"}},{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"onHold"},"name":{"kind":"Name","value":"ordersPaged"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"statuses"},"value":{"kind":"ListValue","values":[{"kind":"StringValue","value":"ON_HOLD","block":false}]}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"1"}},{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}},{"kind":"Field","alias":{"kind":"Name","value":"unpaid"},"name":{"kind":"Name","value":"ordersPaged"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"paymentStatuses"},"value":{"kind":"ListValue","values":[{"kind":"StringValue","value":"UNPAID","block":false}]}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"1"}},{"kind":"Argument","name":{"kind":"Name","value":"channelId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"channelId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<OrderQueueCountsQuery, OrderQueueCountsQueryVariables>;