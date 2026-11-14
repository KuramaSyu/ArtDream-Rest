const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Short, human relative time. Falls back to a date for old entries. */
export function relativeTime(iso: string, now = Date.now()): string {
  const value = new Date(iso).getTime();
  if (!Number.isFinite(value)) return "";
  const diff = now - value;
  if (diff < MINUTE) return "just now";
  if (diff < HOUR) return `${Math.floor(diff / MINUTE)}m ago`;
  if (diff < DAY) return `${Math.floor(diff / HOUR)}h ago`;
  if (diff < 7 * DAY) return `${Math.floor(diff / DAY)}d ago`;
  return new Date(iso).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, exponent);
  return `${value >= 10 || exponent === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[exponent]}`;
}

export function periodLabel(period: string): string {
  switch (period) {
    case "day":
      return "today";
    case "week":
      return "this week";
    case "month":
      return "this month";
    case "year":
      return "this year";
    case "all":
      return "all time";
    default:
      return period;
  }
}