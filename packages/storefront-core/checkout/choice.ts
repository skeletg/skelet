// Shared by the server's arrival labels and the client's delivery choices.
export const choiceKey = (c: { optionId: string; carrierSlug: string; methodCode: string }) =>
  `${c.optionId}::${c.carrierSlug}::${c.methodCode}`;

type Labelled = { optionId: string; optionName: string; carrierSlug: string; carrierName: string; methodName: string };

// What tells a choice apart from the rest of its group: the option's name, then the carrier when the option's methods come
// from more than one, then the method when the option has more than one - a single-method option is just its name
export function choiceLabel(c: Labelled, group: readonly Labelled[]): string[] {
  const same = group.filter((o) => o.optionId === c.optionId);
  const parts = [c.optionName];
  if (new Set(same.map((o) => o.carrierSlug)).size > 1) parts.push(c.carrierName);
  if (same.length > 1) parts.push(c.methodName);
  return parts;
}
