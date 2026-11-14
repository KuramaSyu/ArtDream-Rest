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
  image: ImageItem;
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