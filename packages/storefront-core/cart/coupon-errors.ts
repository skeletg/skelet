import { GraphQLRequestError } from "../graphql";

// The backend's typed promotion errors as forms.errors keys; an untyped one keeps its own message
export function formatCouponError(err: unknown): string {
  if (err instanceof GraphQLRequestError) {
    const code = err.errors[0]?.extensions?.code;
    switch (code) {
      case "PROMOTION_CODE_NOT_FOUND":
        return "couponNotFound";
      case "PROMOTION_RULE_INACTIVE":
      case "PROMOTION_CODE_INACTIVE":
        return "couponInactive";
      case "PROMOTION_RULE_MAX_USES":
      case "PROMOTION_CODE_MAX_USES":
        return "couponMaxUses";
      case "PROMOTION_RULE_NOT_YET_VALID":
      case "PROMOTION_CODE_NOT_YET_VALID":
        return "couponNotYetValid";
      case "PROMOTION_RULE_EXPIRED":
      case "PROMOTION_CODE_EXPIRED":
        return "couponExpired";
      case "PROMOTION_CODE_ON_AUTO_APPLY_RULE":
        return "couponAutoApply";
      case "PROMOTION_PER_CUSTOMER_LIMIT_RULE":
      case "PROMOTION_PER_CUSTOMER_LIMIT_CODE":
        return "couponAlreadyUsed";
    }
    return err.errors[0]?.message ?? "couponFailed";
  }
  return err instanceof Error ? err.message : "couponFailed";
}
