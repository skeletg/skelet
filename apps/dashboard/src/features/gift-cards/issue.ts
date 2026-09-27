export const ALL_STORES = '__all__'

export function issueChannelId(choice: string): string | null {
  return choice === ALL_STORES ? null : choice
}

export function currencyForChoice(channels: { id: string; currencyCode: string }[], choice: string, current: string): string {
  return channels.find((c) => c.id === choice)?.currencyCode ?? current
}
