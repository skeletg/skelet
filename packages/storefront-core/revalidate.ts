import { request } from "./request";

export async function revalidatePublicPath(path: string): Promise<void> {
  await request().revalidatePublic(path);
}

export function revalidateAccount(): void {
  request().revalidateAccount();
}
