import { z } from "zod";

import { requiredText } from "../forms/schema";

// Shared by the store forms (client validation) and their server actions, so no server imports here
//
// z.guid, not z.uuid: PostgreSQL and Go accept UUID-shaped ids without RFC version bits, and imported
// identifiers and the deterministic demo fixtures use them
export const storeFiltersSchema = z
  .object({
    city: z.string().trim().max(200, "tooLong").optional(),
    search: z.string().trim().max(200, "tooLong").optional(),
    pickupOnly: z.boolean().optional(),
    inStockOnly: z.boolean().optional(),
    productId: z.guid().optional(),
    preferredStoreId: z.guid().optional(),
    latitude: z.number().min(-90).max(90).optional(),
    longitude: z.number().min(-180).max(180).optional(),
    offset: z.number().int().min(0).max(100000).optional(),
  })
  .refine((p) => (p.latitude == null) === (p.longitude == null));
export type StoreFilters = z.infer<typeof storeFiltersSchema>;

export const productSearchSchema = z.object({
  q: requiredText().max(200, "tooLong"),
});

export const productPickSchema = z.object({
  productId: z.guid(),
});
