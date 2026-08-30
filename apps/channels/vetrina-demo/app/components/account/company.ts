import { cache } from "react";

import { getMyCompany } from "@/app/lib/b2b/actions";

// b2b-actions is a "use server" module, so nothing in it can be wrapped in cache()
// there (every export has to stay an async function)
export const getCachedCompany = cache(getMyCompany);
