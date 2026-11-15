export interface User {
  id: number;
  username: string;
  displayName: string;
  isAdmin: boolean;
  createdAt: string;
  createdBy?: number;
}

export interface ImageItem {
  id: string;
  url: string;
  author: string;
  title: string;
  originalFilename?: string;
  mimeType: string;
  width: number;
  height: number;
  sizeBytes: number;
  uploadedBy?: number;
  createdAt: string;
  deletedAt?: string;
}

export interface VoteMode {
  id: number;
  name: string;
  description: string;
  minValue: number;
  maxValue: number;
  isBinary: boolean;
  createdAt: string;
}

export type VoteWinner = "left" | "right" | "tie";

export interface Vote {
  id: number;
  modeId: number;
  modeName: string;
  leftImageId: string;
  rightImageId: string;
  leftImage?: ImageItem;
  rightImage?: ImageItem;
  winnerImageId?: string;
  loserImageId?: string;
  winnerImage?: ImageItem;
  loserImage?: ImageItem;
  winner: VoteWinner;
  value?: number;
  minValue: number;
  maxValue: number;
  weight: number;
  userId?: number;
  username: string;
  user?: { id: number; username: string; displayName: string };
  metadata: Record<string, unknown>;
  durationMs?: number;
  createdAt: string;
}

export interface RankEntry {
  rank: number;
  id: string;
  url: string;
  author: string;
  title: string;
  width: number;
  height: number;
  wins: number;
  losses: number;
  ties: number;
  votes: number;
  score: number;
  winRate: number;
  /** Win rate expressed as a 0–100 percentage, rounded to one decimal. */
  winPercentage?: number;
  image: ImageItem;
}

export interface AdminRankEntry extends RankEntry {
  /** Rank by raw score, ignoring hide/derank moderation. */
  scoreRank: number;
  hidden: boolean;
  deranked: boolean;
  excluded: boolean;
  /** True when an explicit override exists for this image/window pair. */
  override: boolean;
  note?: string;
  moderationWindow: ModerationWindow;
}

export interface AdminRankResponse {
  time: string;
  window: ModerationWindow;
  from?: string;
  to?: string;
  amount: number;
  offset: number;
  total: number;
  totalVotes: number;
  hiddenCount: number;
  derankedCount: number;
  excludedCount: number;
  winners: AdminRankEntry[];
}

export type ModerationWindow = "all" | "day" | "week" | "month" | "year";

/** Admin-managed regex lists that hide images from public API results. */
export interface RankFilters {
  excludeAuthors: string[];
  excludeTitles: string[];
  includeAuthors: string[];
  includeTitles: string[];
}

export interface ImageModeration {
  imageId: string;
  timeWindow: ModerationWindow;
  hidden: boolean;
  deranked: boolean;
  note: string;
  updatedAt: string;
  updatedBy?: number;
}

export interface RankResponse {
  time: string;
  from?: string;
  to?: string;
  amount: number;
  offset: number;
  total: number;
  totalVotes: number;
  winners: RankEntry[];
}

export interface SessionResponse {
  token: string;
  user: User;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}

/** Server-backed display decisions enforced by the frontend. */
export interface FrontendConfig {
  rankingAnonymized: boolean;
  updatedAt?: string;
}

export type TimeRange = "day" | "week" | "month" | "year" | "all" | "custom";

export interface RankQuery {
  time?: TimeRange;
  amount?: number;
  offset?: number;
  seconds?: number;
  author?: string;
  titleRegex?: string;
  mode?: string;
}

export interface VotePayload {
  mode: string;
  leftImageId: string;
  rightImageId: string;
  winner?: VoteWinner;
  value?: number;
  minValue?: number;
  maxValue?: number;
  metadata?: Record<string, unknown>;
  durationMs?: number;
}

export interface VoteUpdatePayload {
  mode?: string;
  leftImageId?: string;
  rightImageId?: string;
  winner?: VoteWinner;
  value?: number;
  minValue?: number;
  maxValue?: number;
  metadata?: Record<string, unknown>;
  durationMs?: number;
}

export interface AdminFilter {
  time?: TimeRange;
  seconds?: number;
  user?: string;
  author?: string;
  titleRegex?: string;
  mode?: string;
  imageId?: string;
  amount?: number;
  offset?: number;
}