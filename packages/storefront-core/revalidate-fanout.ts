export const WARM_CONCURRENCY = 4;
export const LOCALE_CONCURRENCY = 8;

export async function mapLimit<T, R>(
  items: readonly T[],
  limit: number,
  fn: (item: T) => Promise<R>,
): Promise<PromiseSettledResult<R>[]> {
  const out: PromiseSettledResult<R>[] = new Array(items.length);
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      try {
        out[i] = { status: "fulfilled", value: await fn(items[i]) };
      } catch (reason) {
        out[i] = { status: "rejected", reason };
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(Math.max(1, limit), items.length) }, worker));
  return out;
}

export type WarmFailure = { channel: string; path: string };

export async function warmAcrossChannels(
  channels: readonly string[],
  paths: readonly string[],
  warmOne: (channel: string, path: string) => Promise<boolean>,
  limit = WARM_CONCURRENCY,
): Promise<{ warmed: number; failed: WarmFailure[] }> {
  const jobs = channels.flatMap((channel) => paths.map((path) => ({ channel, path })));
  const results = await mapLimit(jobs, limit, (j) => warmOne(j.channel, j.path));
  const failed = jobs.filter((_, i) => {
    const r = results[i];
    return r.status === "rejected" || !r.value;
  });
  return { warmed: jobs.length - failed.length, failed };
}

export async function localesByChannel(
  channels: readonly string[],
  load: (channel: string) => Promise<{ codes: string[] }>,
  limit = LOCALE_CONCURRENCY,
): Promise<Map<string, string[]>> {
  const results = await mapLimit(channels, limit, load);
  return new Map(channels.map((ch, i) => {
    const r = results[i];
    return [ch, r.status === "fulfilled" ? r.value.codes : []];
  }));
}
