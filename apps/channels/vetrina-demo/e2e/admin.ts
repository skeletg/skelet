import { request, type APIRequestContext } from "@playwright/test";

// The admin API for specs that set the store up - bin/test-storefront makes a superuser for the run and passes its
// credentials, the admin endpoint and the storefront's channel in the environment
export type Admin = { gql: <T = Record<string, unknown>>(query: string, variables?: Record<string, unknown>) => Promise<T>; channelId: string; dispose: () => Promise<void> };

export async function adminSession(): Promise<Admin> {
  const url = process.env.E2E_ADMIN_GRAPHQL;
  const email = process.env.E2E_ADMIN_EMAIL;
  const password = process.env.E2E_ADMIN_PASSWORD;
  const channelId = process.env.E2E_CHANNEL_ID;
  if (!url || !email || !password || !channelId) {
    throw new Error("E2E_ADMIN_* / E2E_CHANNEL_ID are unset - run the browser tests through ./skelet test storefront");
  }
  const ctx: APIRequestContext = await request.newContext();
  let token = "";
  const gql = async <T,>(query: string, variables: Record<string, unknown> = {}): Promise<T> => {
    const res = await ctx.post(url, {
      data: { query, variables },
      headers: token ? { authorization: `Bearer ${token}` } : {},
    });
    const body = (await res.json()) as { data?: T; errors?: { message: string }[] };
    if (!res.ok() || body.errors?.length) throw new Error(`admin ${query.slice(0, 60)}: ${res.status()} ${JSON.stringify(body.errors)}`);
    return body.data as T;
  };
  const login = await gql<{ adminLogin: { accessToken: string } }>(
    "mutation ($input: AdminLoginInput!) { adminLogin(input: $input) { accessToken } }",
    { input: { email, password } },
  );
  token = login.adminLogin.accessToken;
  return { gql, channelId, dispose: () => ctx.dispose() };
}
