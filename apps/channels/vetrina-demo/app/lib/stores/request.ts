import { z } from "zod";

// PostgreSQL and Go accept UUID-shaped identifiers without RFC version bits;
// existing imported identifiers and deterministic demo fixtures do too.
export const storeRequestSchema = z.object({
  action: z.enum(["stores", "search", "product"]).default("stores"),
  productId: z.guid().optional(), preferredStoreId: z.guid().optional(),
  locale: z.string().max(64).optional(),
  city: z.string().max(200).optional(), search: z.string().max(200).optional(),
  pickupOnly: z.enum(["true", "false"]).optional(),
  inStockOnly: z.enum(["true", "false"]).optional(),
  latitude: z.coerce.number().min(-90).max(90).optional(),
  longitude: z.coerce.number().min(-180).max(180).optional(),
  offset: z.coerce.number().int().min(0).max(100000).optional(),
}).refine(p => (p.latitude == null) === (p.longitude == null));
