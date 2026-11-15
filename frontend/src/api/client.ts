import { getToken, notifyUnauthorized } from "./session";
import type {
  AdminFilter,
  AdminRankResponse,
  FrontendConfig,
  ImageItem,
  ImageModeration,
  ImageRequestFilters,
  ListResponse,
  ModerationWindow,
  RankFilters,
  RankQuery,
  RankResponse,
  SessionResponse,
  TimeRange,
  User,
  Vote,
  VoteClaimResponse,
  VoteMode,
  VotePayload,
  VoteUpdatePayload,
} from "./types";

export const API_BASE =
  (import.meta.env.VITE_API_BASE as string | undefined)?.replace(/\/$/, "") ||
  "/api/v1";

export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

type QueryValue = string | number | boolean | undefined | null;

interface RequestOptions {
  method?: string;
  body?: unknown;
  query?: Record<string, QueryValue>;
  signal?: AbortSignal;
  skipAuthHandler?: boolean;
}

function buildUrl(path: string, query?: Record<string, QueryValue>): string {
  const url = `${API_BASE}${path}`;
  if (!query) return url;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === "") continue;
    params.set(key, String(value));
  }
  const qs = params.toString();
  return qs ? `${url}?${qs}` : url;
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;

  let body: BodyInit | undefined;
  if (options.body instanceof FormData) {
    body = options.body;
  } else if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(options.body);
  }

  const res = await fetch(buildUrl(path, options.query), {
    method: options.method ?? "GET",
    headers,
    body,
    signal: options.signal,
  });

  if (res.status === 401 && !options.skipAuthHandler) {
    notifyUnauthorized();
  }

  if (!res.ok) {
    let code = "request_failed";
    let message = `Request failed (${res.status}).`;
    try {
      const payload = (await res.json()) as {
        error?: { code?: string; message?: string };
      };
      if (payload.error?.message) message = payload.error.message;
      if (payload.error?.code) code = payload.error.code;
    } catch {
      // Non-JSON error responses keep the generic message.
    }
    throw new ApiError(res.status, code, message);
  }

  if (res.status === 204) return undefined as T;
  const contentType = res.headers.get("content-type") ?? "";
  if (!contentType.includes("application/json")) return undefined as T;
  return (await res.json()) as T;
}

export interface UploadResponse {
  image: ImageItem;
  claimToken?: string;
}

export interface ImageClaim {
  id: string;
  token: string;
}

export interface ListWithAuthor<T> extends ListResponse<T> {
  author: string;
}

export const api = {
  needsBootstrap: () =>
    request<{ needsBootstrap: boolean; userCount: number }>("/users/needs-bootstrap"),

  bootstrap: (body: {
    username: string;
    password: string;
    displayName?: string;
  }) => request<SessionResponse>("/users/bootstrap", { method: "POST", body }),

  frontendConfig: () => request<FrontendConfig>("/frontend-config"),

  register: (body: {
    username: string;
    password: string;
    displayName?: string;
  }) => request<SessionResponse>("/auth/register", { method: "POST", body }),

  login: (body: { username: string; password: string }) =>
    request<SessionResponse>("/auth/login", { method: "POST", body }),

  logout: () =>
    request<{ ok: boolean }>("/auth/logout", { method: "POST", skipAuthHandler: true }),

  me: () => request<{ user: User }>("/auth/me"),

  updateMe: (body: {
    displayName?: string;
    password?: string;
    currentPassword?: string;
  }) => request<{ user: User }>("/auth/me", { method: "PATCH", body }),

  randomImages: (
    amount = 2,
    exclude: string[] = [],
    author?: string,
    filters: ImageRequestFilters = {},
  ) =>
    request<ImageItem[]>("/images/random", {
      query: { amount, exclude: exclude.join(","), author, ...filters },
    }),

  imagesByAuthor: (
    author: string,
    amount = 100,
    offset = 0,
    filters: ImageRequestFilters = {},
  ) =>
    request<ListWithAuthor<ImageItem>>("/images/author", {
      query: { author, amount, offset, ...filters },
    }),

  rank: (query: RankQuery = {}) =>
    request<RankResponse>("/images/rank", {
      query: {
        time: query.time,
        amount: query.amount,
        offset: query.offset,
        seconds: query.seconds,
        author: query.author,
        titleRegex: query.titleRegex,
        mode: query.mode,
        strategy: query.strategy,
        visibility: query.visibility,
        service: query.service,
        uploadedBy: query.uploadedBy,
        mine: query.mine,
        includeAuthors: query.includeAuthors,
        excludeAuthors: query.excludeAuthors,
        includeTitles: query.includeTitles,
        excludeTitles: query.excludeTitles,
        rotation: query.rotation,
        rotationPercent: query.rotationPercent,
        exclude: query.exclude,
      },
    }),

  smartImages: (query: RankQuery = {}) =>
    request<RankResponse>("/images/smart", {
      query: {
        amount: query.amount,
        offset: query.offset,
        author: query.author,
        titleRegex: query.titleRegex,
        mode: query.mode,
        strategy: query.strategy,
        visibility: query.visibility,
        service: query.service,
        uploadedBy: query.uploadedBy,
        mine: query.mine,
        includeAuthors: query.includeAuthors,
        excludeAuthors: query.excludeAuthors,
        includeTitles: query.includeTitles,
        excludeTitles: query.excludeTitles,
        exclude: query.exclude,
      },
    }),

  modes: () => request<VoteMode[]>("/rank/modes"),

  vote: (body: VotePayload) =>
    request<Vote>("/rank/vote", { method: "POST", body, skipAuthHandler: true }),

  claimVotes: (anonymousIds: string[]) =>
    request<VoteClaimResponse>("/rank/claim", {
      method: "POST",
      body: { anonymousIds },
    }),

  myVotes: (query: { time?: TimeRange; amount?: number; offset?: number } = {}) =>
    request<ListResponse<Vote>>("/rank/me", { query }),

  uploadImage: (formData: FormData) =>
    request<UploadResponse>("/images", { method: "POST", body: formData }),

  claimImages: (claims: ImageClaim[]) =>
    request<{ claimed: string[]; count: number }>("/images/claim", {
      method: "POST",
      body: { claims },
    }),

  admin: {
    listVotes: (filter: AdminFilter = {}) =>
      request<{ items: Vote[]; total: number; time: string }>("/admin/votes", {
        query: filter as Record<string, QueryValue>,
      }),

    rank: (filter: AdminFilter & { includeHidden?: boolean } = {}) =>
      request<AdminRankResponse>("/admin/rank", {
        query: filter as Record<string, QueryValue>,
      }),

    rankFilters: () => request<RankFilters>("/admin/rank/filters"),

    updateRankFilters: (body: Partial<RankFilters>) =>
      request<RankFilters>("/admin/rank/filters", { method: "PATCH", body }),

    setModeration: (
      id: string,
      body: {
        timeWindow: ModerationWindow;
        hidden?: boolean;
        deranked?: boolean;
        note?: string;
      },
    ) => request<ImageModeration>(`/admin/images/${id}/moderation`, { method: "PATCH", body }),

    clearModeration: (id: string, timeWindow: ModerationWindow) =>
      request<{ deleted: string; timeWindow: ModerationWindow }>(
        `/admin/images/${id}/moderation`,
        { method: "DELETE", query: { timeWindow } },
      ),

    updateVote: (id: number, body: VoteUpdatePayload) =>
      request<Vote>(`/admin/votes/${id}`, { method: "PATCH", body }),

    deleteVote: (id: number) =>
      request<{ deleted: number }>(`/admin/votes/${id}`, { method: "DELETE" }),

    bulkDeleteVotes: (filter: AdminFilter & { ids?: string }) =>
      request<{ deleted: number }>("/admin/votes", {
        method: "DELETE",
        query: filter as Record<string, QueryValue>,
      }),

    listImages: (filter: AdminFilter = {}) =>
      request<ListResponse<ImageItem>>("/admin/images", {
        query: filter as Record<string, QueryValue>,
      }),

    updateImage: (
      id: string,
      body: {
        author?: string;
        title?: string;
        fromPublicService?: boolean;
        serviceName?: string;
      },
    ) => request<ImageItem>(`/admin/images/${id}`, { method: "PATCH", body }),

    deleteImage: (id: string, keepVotes = false) =>
      request<{ deleted: string }>(`/admin/images/${id}`, {
        method: "DELETE",
        query: { keepVotes },
      }),

    bulkDeleteImages: (filter: AdminFilter & { ids?: string; keepVotes?: boolean }) =>
      request<{ deleted: number }>("/admin/images", {
        method: "DELETE",
        query: filter as Record<string, QueryValue>,
      }),

    listUsers: () => request<ListResponse<User>>("/users"),

    updateFrontendConfig: (
      body: Partial<
        Pick<
          FrontendConfig,
          "rankingAnonymized" | "defaultImageVisibility" | "rankRotationEnabled" | "rankRotationPercent"
        >
      >,
    ) => request<FrontendConfig>("/admin/frontend-config", { method: "PATCH", body }),

    createUser: (body: {
      username: string;
      password: string;
      displayName?: string;
      isAdmin?: boolean;
    }) => request<{ user: User }>("/users", { method: "POST", body }),

    updateUser: (
      id: number,
      body: { displayName?: string; password?: string; isAdmin?: boolean },
    ) => request<{ user: User }>(`/users/${id}`, { method: "PATCH", body }),

    deleteUser: (id: number) =>
      request<{ deleted: number }>(`/users/${id}`, { method: "DELETE" }),

    createMode: (body: Omit<VoteMode, "id" | "createdAt">) =>
      request<VoteMode>("/admin/rank/modes", { method: "POST", body }),

    updateMode: (id: number, body: Partial<Omit<VoteMode, "id" | "createdAt">>) =>
      request<VoteMode>(`/admin/rank/modes/${id}`, { method: "PATCH", body }),

    deleteMode: (id: number) =>
      request<{ deleted: number }>(`/admin/rank/modes/${id}`, { method: "DELETE" }),
  },
};

/**
 * Resolve an API-provided relative image URL against a possible absolute API
 * base. Backend responses use `/api/v1/images/...`, which works unchanged in
 * dev (Vite proxy) and when the SPA is served by the Go backend.
 */
export function resolveImageUrl(url: string | undefined | null): string {
  if (!url) return "";
  if (/^(https?:)?\/\//.test(url) || url.startsWith("blob:") || url.startsWith("data:")) {
    return url;
  }
  if (API_BASE.startsWith("http")) {
    return new URL(API_BASE).origin + url;
  }
  return url;
}

/** Build an image URL with render overrides (format, width, height). */
export function imageVariant(
  url: string,
  options: {
    format?: "webp" | "png" | "jpg";
    w?: number;
    h?: number;
    quality?: number;
    /** Explicit opt-in to the pristine upload (used only by "Original"). */
    original?: boolean;
    download?: boolean;
  } = {},
): string {
  const base = resolveImageUrl(url);
  if (!base) return base;
  const params = new URLSearchParams();
  if (options.original) {
    params.set("original", "true");
  } else {
    if (options.format) params.set("format", options.format);
    if (options.w) params.set("w", String(options.w));
    if (options.h) params.set("h", String(options.h));
    if (options.quality) params.set("quality", String(options.quality));
  }
  if (options.download) params.set("download", "true");
  const qs = params.toString();
  if (!qs) return base;
  return `${base}${base.includes("?") ? "&" : "?"}${qs}`;
}