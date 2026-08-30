// Split from SchedulerWidgets so that file exports only components - fast refresh needs the
// separation, and these are plain option tables

export const SERVER_TZ = '__server__'

export const CRON_PRESETS: { value: string; label: string }[] = [
  { value: '@every 5m', label: 'Every 5 minutes' },
  { value: '@every 15m', label: 'Every 15 minutes' },
  { value: '@every 30m', label: 'Every 30 minutes' },
  { value: '@hourly', label: 'Hourly' },
  { value: '@every 6h', label: 'Every 6 hours' },
  { value: '@every 12h', label: 'Every 12 hours' },
  { value: '@daily', label: 'Daily (midnight)' },
  { value: '0 2 * * *', label: 'Daily at 02:00' },
  { value: '@weekly', label: 'Weekly (Sun 00:00)' },
  { value: '@monthly', label: 'Monthly (1st 00:00)' },
]

// Curated common IANA zones. "" (server time) is offered separately
export const TIMEZONES: string[] = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Madrid',
  'Europe/Moscow',
  'Africa/Cairo',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Australia/Sydney',
  'Pacific/Auckland',
]
