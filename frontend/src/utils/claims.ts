import { api } from "../api/client";
import {
  loadPendingSubmissions,
  replacePendingSubmissions,
  type PendingSubmission,
} from "./pendingSubmissions";

/** Pending count without triggering a network request. */
export function pendingCount(): number {
  return loadPendingSubmissions().length;
}

/**
 * Attach all locally remembered anonymous submissions to the signed-in user.
 * Successful claims are removed from localStorage; failed/unclaimed entries
 * stay so the user can retry later.
 */
export async function claimAllPendingSubmissions(): Promise<{
  claimed: string[];
  count: number;
  remaining: PendingSubmission[];
}> {
  const pending = loadPendingSubmissions();
  if (pending.length === 0) {
    return { claimed: [], count: 0, remaining: [] };
  }
  const response = await api.claimImages(
    pending.map((item) => ({ id: item.id, token: item.claimToken })),
  );
  const claimedSet = new Set(response.claimed);
  const remaining = pending.filter((item) => !claimedSet.has(item.id));
  replacePendingSubmissions(remaining);
  return { claimed: response.claimed, count: response.count, remaining };
}
