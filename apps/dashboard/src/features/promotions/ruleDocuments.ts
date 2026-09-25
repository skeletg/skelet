import { gql } from '@apollo/client'

// The promotion rule editor's documents - hand-written rather than codegen'd, kept apart from the
// component so tests can mock them

export const PROMOTION_RULE_QUERY = gql`
  query PromotionRuleDetail($id: ID!) {
    promotionRule(id: $id) {
      id
      slug
      name
      description
      publicLabel
      channelId
      requiresCode
      priority
      stopAfterApply
      exclusivityGroup
      validFrom
      validTo
      maxUses
      perCustomerLimit
      usedCount
      isActive
      labelMediaId
      bannerMediaId
      pillText
      pillTextColor
      pillBgColor
      contents {
        locale
        label
        terms
        updatedAt
      }
      conditions {
        id
        type
        params
        paramsVersion
        sortOrder
      }
      actions {
        id
        type
        params
        paramsVersion
        sortOrder
      }
      codes(limit: 1000) {
        items {
          id
          code
          maxUses
          perCustomerLimit
          usedCount
          isActive
        }
        totalCount
      }
    }
  }
`

export const CONDITION_TYPES_QUERY = gql`
  query PromotionConditionTypes {
    promotionConditionTypes {
      slug
      label
      description
      paramsSchema
    }
  }
`

export const ACTION_TYPES_QUERY = gql`
  query PromotionActionTypes {
    promotionActionTypes {
      slug
      label
      description
      paramsSchema
    }
  }
`

export const CREATE_PROMOTION_RULE = gql`
  mutation CreatePromotionRule($input: CreatePromotionRuleInput!) {
    createPromotionRule(input: $input) {
      id
    }
  }
`

export const UPDATE_PROMOTION_RULE = gql`
  mutation UpdatePromotionRule($input: UpdatePromotionRuleInput!) {
    updatePromotionRule(input: $input) {
      id
    }
  }
`

export const UPDATE_PROMOTION_CODE = gql`
  mutation UpdatePromotionCodeInline($input: UpdatePromotionCodeInput!) {
    updatePromotionCode(input: $input) {
      id
      isActive
    }
  }
`

export const DELETE_PROMOTION_CODE = gql`
  mutation DeletePromotionCode($id: ID!) {
    deletePromotionCode(id: $id)
  }
`

