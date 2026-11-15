export interface PendingSubmission {
  id: string;
  claimToken: string;
  author: string;
  title: string;
  url: string;
  width: number;
  height: number;
  fromPublicService?: boolean;
  serviceName?: string;
  createdAt: string;
}

const STORAGE_KEY = "artdream.pendingSubmissions.v1";
const MAX_ENTRIES = 60;

function isPendingSubmission(value: unknown): value is PendingSubmission {
  if (value === null || typeof value !== "object") return false;
  const item = value as Partial<PendingSubmission>;
  return (
    typeof item.id === "string" &&
    typeof item.claimToken === "string" &&
    typeof item.author === "string" &&
    typeof item.url === "string" &&
    typeof item.createdAt === "string"
  );
}

export function loadPendingSubmissions(): PendingSubmission[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isPendingSubmission).slice(0, MAX_ENTRIES);
  } catch {
    return [];
  }
}

export function replacePendingSubmissions(items: PendingSubmission[]): void {
  save(items);
  emitPendingChange();
}

function save(items: PendingSubmission[]): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items.slice(0, MAX_ENTRIES)));
  } catch {
    // Quota or privacy mode: the submission still exists on the server, only
    // the local claim list is lost.
  }
}

export function addPendingSubmission(item: PendingSubmission): PendingSubmission[] {
  const items = loadPendingSubmissions().filter((entry) => entry.id !== item.id);
  const next = [item, ...items].slice(0, MAX_ENTRIES);
  save(next);
  return next;
}

export function removePendingSubmission(id: string): PendingSubmission[] {
  const next = loadPendingSubmissions().filter((entry) => entry.id !== id);
  save(next);
  return next;
}

export function clearPendingSubmissions(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

export const PENDING_SUBMISSIONS_EVENT = "artdream:pending-submissions";

/** Notify same-tab listeners (the `storage` event only fires across tabs). */
export function emitPendingChange(): void {
  window.dispatchEvent(new Event(PENDING_SUBMISSIONS_EVENT));
}