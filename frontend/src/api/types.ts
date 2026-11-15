export interface User {
  id: number;
  username: string;
  displayName: string;
  isAdmin: boolean;
  createdAt: string;
  createdBy?: number;
}

export type ImageVisibility = "all" | "public" | "private";

/** Filters accepted by the public image listing, pair and ranking requests. */
export interface ImageRequestFilters {
  /** Public-service provenance filter. Omit to use the admin default. */
  visibility?: ImageVisibility;
  /** Case-insensitive substring match against the optional service name. */
  service?: string;
  /** Comma-separated account ids/usernames, or "me". */
  uploadedBy?: string;
  /** Shorthand for uploadedBy=me; requires a signed-in session. */
  mine?: boolean;
  includeAuthors?: string;
  excludeAuthors?: string;
  includeTitles?: string;
  excludeTitles?: string;
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
  /** True when the uploader marked the image as coming from a public service. */
  fromPublicService: boolean;
  /** Optional free-text name of that public service (e.g. Instagram). */
  serviceName?: string;
  /** Derived convenience field: "public" or "private". */
  visibility: Exclude<ImageVisibility, "all">;
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
  /** True when rank rotation added this deterministic fallback image. */
  rotated?: boolean;
  /** True when the smart endpoint used its random fallback. */
  random?: boolean;
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

export interface RankRotationInfo {
  enabled: boolean;
  percent: number;
  previousFrom?: string;
  previousTo?: string;
  required: number;
  changed: number;
  satisfied: boolean;
}

export type RankSource = "day" | "week" | "month" | "year" | "random";

export interface RankResponse {
  time: string;
  /** Set by the smart endpoint to the ladder step that produced the page. */
  source?: RankSource;
  from?: string;
  to?: string;
  amount: number;
  offset: number;
  total: number;
  totalVotes: number;
  winners: RankEntry[];
  rotation?: RankRotationInfo;
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
  /** Fallback public/private mode for image requests that omit visibility. */
  defaultImageVisibility: ImageVisibility;
  /** When enabled, quiet ranking periods are diversified against the previous period. */
  rankRotationEnabled: boolean;
  /** Minimum share (0-100) of a rank page that should differ from the previous period. */
  rankRotationPercent: number;
  updatedAt?: string;
}

export type TimeRange = "day" | "week" | "month" | "year" | "all" | "custom";

export interface RankQuery extends ImageRequestFilters {
  time?: TimeRange;
  amount?: number;
  offset?: number;
  seconds?: number;
  author?: string;
  titleRegex?: string;
  mode?: string;
  /** Per-request override of the admin rank-rotation switch. */
  rotation?: boolean;
  /** Per-request override of the minimum changed share (0-100). */
  rotationPercent?: number;
  /** Comma-separated image ids to omit from both periods and fallbacks. */
  exclude?: string;
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

export interface AdminFilter extends ImageRequestFilters {
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