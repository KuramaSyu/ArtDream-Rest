import { api } from "../api/client";
import type { VotePayload } from "../api/types";

/**
 * Anonymous rankings are kept on this device even though every vote is also
 * sent to the server immediately. The local copy is the "claim ticket" that
 * lets a later sign-in/registration move those votes under the new account.
 */
export interface LocalRanking {
  localId: string;
  /** Device-local secret the server stored with this vote. */
  anonymousId: string;
  payload: VotePayload;
  createdAt: string;
  /** The vote reached the server. */
  synced: boolean;
  /** The vote is owned by an account (or was claimed by one). */
  claimed: boolean;
}

const ANONYMOUS_ID_KEY = "artdream.anonymousVoterId.v1";
const RANKINGS_KEY = "artdream.localRankings.v1";
const MAX_ENTRIES = 500;

let anonymousIdCache: string | null = null;

function isValidAnonymousId(value: string): boolean {
  return /^[A-Za-z0-9_-]{16,128}$/.test(value);
}

function randomHex(bytes = 16): string {
  try {
    const buffer = new Uint8Array(bytes);
    if (globalThis.crypto?.getRandomValues) {
      globalThis.crypto.getRandomValues(buffer);
      return Array.from(buffer, (byte) => byte.toString(16).padStart(2, "0")).join("");
    }
  } catch {
    // Fall through to the Math.random fallback below.
  }
  let out = "";
  for (let i = 0; i < bytes * 2; i += 1) {
    out += Math.floor(Math.random() * 16).toString(16);
  }
  return out;
}

function readStorage(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeStorage(key: string, value: string | null): void {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // Private mode / quota: the in-memory value keeps this tab working.
  }
}

/** The stable anonymous identity used for votes without an account. */
export function getAnonymousVoterId(): string {
  if (anonymousIdCache) return anonymousIdCache;
  const stored = readStorage(ANONYMOUS_ID_KEY)?.trim();
  if (stored && isValidAnonymousId(stored)) {
    anonymousIdCache = stored;
    return stored;
  }
  const created = randomHex();
  anonymousIdCache = created;
  writeStorage(ANONYMOUS_ID_KEY, created);
  return created;
}

/** Start a fresh anonymous identity after the old one was claimed. */
function rotateAnonymousVoterId(): string {
  const created = randomHex();
  anonymousIdCache = created;
  writeStorage(ANONYMOUS_ID_KEY, created);
  return created;
}

function isLocalRanking(value: unknown): value is LocalRanking {
  if (value === null || typeof value !== "object") return false;
  const item = value as Partial<LocalRanking>;
  return (
    typeof item.localId === "string" &&
    typeof item.anonymousId === "string" &&
    typeof item.createdAt === "string" &&
    Boolean(item.payload) &&
    typeof item.payload === "object"
  );
}

export function loadLocalRankings(): LocalRanking[] {
  const raw = readStorage(RANKINGS_KEY);
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isLocalRanking).map((item) => ({
      ...item,
      synced: Boolean(item.synced),
      claimed: Boolean(item.claimed),
    }));
  } catch {
    return [];
  }
}

function saveLocalRankings(items: LocalRanking[]): void {
  const sorted = [...items].sort(
    (a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt),
  );
  // Never lose an unsynced/unclaimed vote just because the history grew.
  const pending = sorted.filter((item) => !item.synced || !item.claimed);
  const settled = sorted.filter((item) => item.synced && item.claimed);
  const kept = [...pending, ...settled.slice(0, Math.max(0, MAX_ENTRIES - pending.length))];
  writeStorage(RANKINGS_KEY, JSON.stringify(kept));
}

function updateRanking(localId: string, patch: Partial<LocalRanking>): LocalRanking | null {
  const items = loadLocalRankings();
  let updated: LocalRanking | null = null;
  const next = items.map((item) => {
    if (item.localId !== localId) return item;
    updated = { ...item, ...patch };
    return updated;
  });
  saveLocalRankings(next);
  return updated;
}

/**
 * Persist a ranking locally before/while it is uploaded. `claimed` is true for
 * signed-in voters because the server already links the vote to their account.
 */
export function recordLocalRanking(
  payload: VotePayload,
  options: { claimed?: boolean } = {},
): LocalRanking {
  const entry: LocalRanking = {
    localId: randomHex(12),
    anonymousId:
      payload.anonymousId && isValidAnonymousId(payload.anonymousId)
        ? payload.anonymousId
        : getAnonymousVoterId(),
    payload,
    createdAt: new Date().toISOString(),
    synced: false,
    claimed: Boolean(options.claimed),
  };
  saveLocalRankings([entry, ...loadLocalRankings()]);
  return entry;
}

export function markLocalRankingSynced(localId: string, claimed?: boolean): void {
  const patch: Partial<LocalRanking> = { synced: true };
  if (claimed !== undefined) patch.claimed = claimed;
  updateRanking(localId, patch);
}

export function pendingLocalRankings(): LocalRanking[] {
  return loadLocalRankings().filter((item) => !item.synced);
}

function unclaimedAnonymousIds(items = loadLocalRankings()): string[] {
  const ids = new Set<string>();
  for (const item of items) {
    if (!item.claimed && item.synced && item.anonymousId) ids.add(item.anonymousId);
  }
  return [...ids];
}

let syncPromise: Promise<{ uploaded: number; claimed: number }> | null = null;

async function runSync(): Promise<{ uploaded: number; claimed: number }> {
  // A failed upload while logged out may still be pending here. Submit it with
  // the now-valid session, which links it to the account directly.
  let uploaded = 0;
  for (const item of pendingLocalRankings()) {
    try {
      await api.vote(item.payload);
      uploaded += 1;
      markLocalRankingSynced(item.localId, true);
    } catch {
      // Keep it pending; the next sign-in or retry can upload it.
    }
  }

  const ids = unclaimedAnonymousIds();
  if (ids.length === 0) return { uploaded, claimed: 0 };

  let claimed = 0;
  try {
    const response = await api.claimVotes(ids);
    claimed = response.claim ?? response.count ?? 0;
    const claimedSet = new Set(ids);
    const next = loadLocalRankings().map((item) =>
      item.synced && !item.claimed && claimedSet.has(item.anonymousId)
        ? { ...item, claimed: true }
        : item,
    );
    saveLocalRankings(next);
    // Do not attach future anonymous votes from this browser to the same
    // account unless the visitor explicitly signs in again.
    rotateAnonymousVoterId();
  } catch {
    // The local list stays unclaimed so a later login can retry.
  }
  return { uploaded, claimed };
}

/**
 * Upload pending local votes and claim every anonymous vote this device still
 * remembers for the signed-in account. Safe to call repeatedly; concurrent
 * calls share one promise.
 */
export function syncLocalRankingsWithAccount(): Promise<{
  uploaded: number;
  claimed: number;
}> {
  if (!syncPromise) {
    syncPromise = runSync().finally(() => {
      syncPromise = null;
    });
  }
  return syncPromise;
}

/** Number of rankings saved only on this device (not yet uploaded). */
export function pendingRankingsCount(): number {
  return pendingLocalRankings().length;
}
