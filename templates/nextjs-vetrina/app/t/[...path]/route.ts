import { backendRoute } from "@skelet/storefront-next/backend-proxy";

export const dynamic = "force-dynamic";

export {
  backendRoute as GET,
  backendRoute as HEAD,
  backendRoute as POST,
  backendRoute as PUT,
  backendRoute as PATCH,
  backendRoute as DELETE,
  backendRoute as OPTIONS,
};
