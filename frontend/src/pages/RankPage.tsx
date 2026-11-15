import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Button,
  Checkbox,
  Chip,
  CircularProgress,
  Dialog,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  IconButton,
  MenuItem,
  TextField,
  ThemeProvider,
  Tooltip,
  Typography,
  alpha,
  useMediaQuery,
  useTheme,
} from "@mui/material";
import {
  ArrowLeftRounded,
  ArrowRightRounded,
  AutoAwesomeRounded,
  CloseRounded,
  CloudUploadOutlined,
  FullscreenRounded,
  SwapHorizRounded,
  TuneRounded,
  VisibilityOffRounded,
} from "@mui/icons-material";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import type { Theme } from "@mui/material/styles";
import { api, ApiError, imageVariant } from "../api/client";
import type {
  ImageItem,
  ImageRequestFilters,
  ImageVisibility,
  VoteMode,
  VotePayload,
} from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { EmptyState, LoadingView } from "../components/Loading";
import { PageShell } from "../components/PageShell";
import { useToast } from "../components/Feedback";
import { useImageTheme, useStageImage } from "../theme/ThemeHost";
import { useFrontendConfig } from "../config/FrontendConfig";
import {
  getAnonymousVoterId,
  markLocalRankingSynced,
  recordLocalRanking,
} from "../utils/localRankings";

type Side = "left" | "right";

interface RankFiltersDraft {
  visibility: "" | ImageVisibility;
  onlyMine: boolean;
  friends: string;
  service: string;
  includeAuthors: string;
  excludeAuthors: string;
  includeTitles: string;
  excludeTitles: string;
}

function emptyRankFilters(): RankFiltersDraft {
  return {
    visibility: "",
    onlyMine: false,
    friends: "",
    service: "",
    includeAuthors: "",
    excludeAuthors: "",
    includeTitles: "",
    excludeTitles: "",
  };
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

const FIVE_RATE_HINTS = [
  { key: "1", label: "left" },
  { key: "2", label: "lean left" },
  { key: "3", label: "even" },
  { key: "4", label: "lean right" },
  { key: "5", label: "right" },
] as const;

function Keycap({ children }: { children: string }) {
  const theme = useTheme();
  return (
    <Box
      component="kbd"
      sx={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        minWidth: 22,
        height: 22,
        px: 0.6,
        borderRadius: 0.8,
        border: `1px solid ${alpha(theme.palette.text.primary, 0.2)}`,
        background: alpha(theme.palette.text.primary, 0.06),
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        fontSize: "0.68rem",
        lineHeight: 1,
        color: "text.primary",
        opacity: 0.82,
      }}
    >
      {children}
    </Box>
  );
}

type ZoneButtonProps = {
  hint: (typeof FIVE_RATE_HINTS)[number];
  rating: number;
  active: boolean;
  disabled: boolean;
  onHover: () => void;
  onLeave: () => void;
  onPick: () => void;
};

/**
 * One of the five rating columns. It renders inside a per-button
 * ThemeProvider, so its palette is the artwork it favours and stays stable
 * while the pointer moves across the page.
 */
function ZoneButton({
  hint,
  rating,
  active,
  disabled,
  onHover,
  onLeave,
  onPick,
}: ZoneButtonProps) {
  const theme = useTheme();
  const tint =
    rating < 3
      ? theme.palette.primary.main
      : rating > 3
        ? theme.palette.secondary.main
        : theme.palette.text.primary;
  return (
    <Box
      role="button"
      tabIndex={disabled ? -1 : 0}
      aria-disabled={disabled}
      aria-label={`Rate ${rating} of 5, ${hint.label}`}
      onMouseEnter={() => {
        if (!disabled) onHover();
      }}
      onFocus={() => {
        if (!disabled) onHover();
      }}
      onBlur={onLeave}
      onClick={() => {
        if (!disabled) onPick();
      }}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          if (!disabled) onPick();
        }
      }}
      sx={{
        position: "relative",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: { xs: 0.35, md: 0.5 },
        minHeight: { xs: 40, md: 86 },
        px: { xs: 0.4, md: 1.5 },
        borderRadius: 3,
        cursor: disabled ? "default" : "pointer",
        outline: "none",
        background: active
          ? `linear-gradient(135deg, ${alpha(tint, 0.3)}, ${alpha(tint, 0.1)})`
          : alpha(theme.palette.text.primary, 0.035),
        border: active
          ? `1.5px solid ${alpha(tint, 0.85)}`
          : `1.5px solid ${alpha(theme.palette.text.primary, 0.09)}`,
        boxShadow: active ? `0 14px 38px ${alpha(tint, 0.28)}` : "none",
        transform: active ? "translateY(-2px)" : "none",
        transition:
          "background 180ms ease, border-color 180ms ease, transform 180ms ease, box-shadow 180ms ease",
        "&:focus-visible": {
          boxShadow: `0 0 0 2px ${alpha(tint, 0.9)}`,
        },
      }}
    >
      <Box
        component="span"
        sx={{
          fontSize: { xs: "1.15rem", md: "1.7rem" },
          fontWeight: 700,
          lineHeight: 1,
          color: active ? tint : "text.primary",
          opacity: active ? 1 : 0.9,
        }}
      >
        {rating}
      </Box>
      <Box
        component="span"
        sx={{
          display: { xs: "none", md: "block" },
          fontSize: "0.6rem",
          letterSpacing: "0.12em",
          textTransform: "uppercase",
          color: "text.secondary",
          opacity: active ? 0.95 : 0.6,
          whiteSpace: "nowrap",
        }}
      >
        {hint.label}
      </Box>
    </Box>
  );
}

type DuelPanelProps = {
  leftName: string;
  rightName: string;
  disabled: boolean;
  zone: number | null;
  onZoneChange: (zone: number | null) => void;
  onPick: (side: Side) => void;
  onRate: (rating: number) => void;
  leftTheme: Theme | null;
  rightTheme: Theme | null;
  tieTheme: Theme | null;
};

function DuelPanel({
  leftName,
  rightName,
  disabled,
  zone,
  onZoneChange,
  onPick,
  onRate,
  leftTheme,
  rightTheme,
  tieTheme,
}: DuelPanelProps) {
  const theme = useTheme();
  const activeZone = zone;
  const activeHint = activeZone !== null ? FIVE_RATE_HINTS[activeZone - 1] : null;

  return (
    <>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          mb: 1.4,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
          <Tooltip title="A tiny nod for the left image">
            <IconButton size="small" onClick={() => onPick("left")} disabled={disabled}>
              <ArrowLeftRounded />
            </IconButton>
          </Tooltip>
          <Typography noWrap sx={{ fontSize: "0.85rem", fontWeight: 600, opacity: 0.8 }}>
            {leftName}
          </Typography>
        </Box>
        <Box sx={{ flexShrink: 0, textAlign: "center" }}>
          <Typography
            className="display"
            sx={{ fontSize: { xs: "1rem", sm: "1.2rem" }, fontWeight: 620 }}
          >
            {activeHint ? activeHint.label : "choose a verdict"}
          </Typography>
          <Typography sx={{ fontSize: "0.68rem", letterSpacing: "0.14em", opacity: 0.55 }}>
            {activeZone !== null ? `${activeZone}/5` : "1 — 5"}
          </Typography>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
          <Typography
            noWrap
            sx={{ fontSize: "0.85rem", fontWeight: 600, opacity: 0.8, textAlign: "right" }}
          >
            {rightName}
          </Typography>
          <Tooltip title="A tiny nod for the right image">
            <IconButton size="small" onClick={() => onPick("right")} disabled={disabled}>
              <ArrowRightRounded />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      <Box
        aria-label="Five quick verdict areas"
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "repeat(5, minmax(0, 1fr))" },
          gap: { xs: 0.55, md: 1 },
          mb: 0.4,
          opacity: disabled ? 0.55 : 1,
          transition: "opacity 180ms ease",
        }}
        onMouseLeave={() => onZoneChange(null)}
      >
        {FIVE_RATE_HINTS.map((hint, index) => {
          const rating = index + 1;
          const themeForButton =
            rating < 3 ? leftTheme : rating > 3 ? rightTheme : tieTheme;
          return (
            <ThemeProvider key={hint.key} theme={themeForButton ?? theme}>
              <ZoneButton
                hint={hint}
                rating={rating}
                active={activeZone === rating}
                disabled={disabled}
                onHover={() => onZoneChange(rating)}
                onLeave={() => onZoneChange(null)}
                onPick={() => onRate(rating)}
              />
            </ThemeProvider>
          );
        })}
      </Box>

      <Box
        aria-label="Keyboard shortcuts"
        sx={{
          display: { xs: "none", md: "flex" },
          flexWrap: "wrap",
          alignItems: "center",
          columnGap: 1.4,
          rowGap: 1,
          mt: 1.2,
        }}
      >
        <Typography sx={{ fontSize: "0.7rem", color: "text.secondary", opacity: 0.65 }}>
          Click a column for 1–5
        </Typography>
        <Divider orientation="vertical" flexItem sx={{ opacity: 0.2, alignSelf: "stretch" }} />
        <Typography sx={{ fontSize: "0.7rem", color: "text.secondary", opacity: 0.65 }}>
          Keys 1–5 rate
        </Typography>
        <Divider orientation="vertical" flexItem sx={{ opacity: 0.2, alignSelf: "stretch" }} />
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.6 }}>
          <Keycap>←</Keycap>
          <Keycap>→</Keycap>
          <Typography sx={{ fontSize: "0.7rem", color: "text.secondary", opacity: 0.8 }}>
            soft pick
          </Typography>
        </Box>
      </Box>
      <Typography
        sx={{
          display: { xs: "block", md: "none" },
          mt: 1.2,
          fontSize: "0.76rem",
          color: "text.secondary",
          opacity: 0.72,
        }}
      >
        Tap a row in the artwork or a number here.
      </Typography>
    </>
  );
}

export function RankPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const { notify } = useToast();
  const theme = useTheme();
  const text = theme.palette.text.primary;
  const black = theme.palette.common.black;
  const isWideLayout = useMediaQuery(theme.breakpoints.up("md"));
  const { config: frontendConfig, loading: frontendConfigLoading } = useFrontendConfig();
  const rankingAnonymized = frontendConfig.rankingAnonymized;
  const hideAuthors = frontendConfigLoading || rankingAnonymized;

  const [modes, setModes] = useState<VoteMode[]>([]);
  const [pair, setPair] = useState<ImageItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [voting, setVoting] = useState(false);
  const [error, setError] = useState("");
  const [picked, setPicked] = useState<Side | null>(null);
  const [activeZone, setActiveZone] = useState<number | null>(null);
  const [zonePreviewOpen, setZonePreviewOpen] = useState(false);
  const [reloadTick, setReloadTick] = useState(0);
  const [fullscreen, setFullscreen] = useState<ImageItem | null>(null);
  const [fullscreenLoaded, setFullscreenLoaded] = useState(false);
  const [rankFilters, setRankFilters] = useState<RankFiltersDraft>(emptyRankFilters);
  const [rankFiltersDraft, setRankFiltersDraft] = useState<RankFiltersDraft>(emptyRankFilters);
  const [rankFiltersOpen, setRankFiltersOpen] = useState(false);

  const requestFilters = useMemo<ImageRequestFilters>(
    () => ({
      visibility: rankFilters.visibility || undefined,
      mine: rankFilters.onlyMine && Boolean(user) || undefined,
      uploadedBy: rankFilters.friends.trim() || undefined,
      service: rankFilters.service.trim() || undefined,
      includeAuthors: rankFilters.includeAuthors.trim() || undefined,
      excludeAuthors: rankFilters.excludeAuthors.trim() || undefined,
      includeTitles: rankFilters.includeTitles.trim() || undefined,
      excludeTitles: rankFilters.excludeTitles.trim() || undefined,
    }),
    [rankFilters, user],
  );
  const filtersActive = useMemo(
    () =>
      Boolean(
        rankFilters.visibility ||
          rankFilters.onlyMine ||
          rankFilters.friends.trim() ||
          rankFilters.service.trim() ||
          rankFilters.includeAuthors.trim() ||
          rankFilters.excludeAuthors.trim() ||
          rankFilters.includeTitles.trim() ||
          rankFilters.excludeTitles.trim(),
      ),
    [rankFilters],
  );

  const recent = useRef<string[]>([]);
  const startedAt = useRef(Date.now());
  const lastPickedSide = useRef<Side>("left");
  const [themeSide, setThemeSide] = useState<Side>("left");
  const [baseSide, setBaseSide] = useState<Side>("left");
  const ratingFromPoint = useCallback(
    (clientX: number, clientY: number, element: HTMLElement) => {
      const rect = element.getBoundingClientRect();
      // Desktop results are side-by-side, so columns map to 1–5.
      // Mobile results are stacked, so rows map to 1–5.
      const ratio = isWideLayout
        ? (clientX - rect.left) / rect.width
        : (clientY - rect.top) / rect.height;
      return Math.min(5, Math.max(1, Math.floor(ratio * 5) + 1));
    },
    [isWideLayout],
  );
  // Hovering 1/2 previews the left artwork, 4/5 the right one. 3 (a draw)
  // deliberately keeps the current palette.
  const previewZone = useCallback((zone: number | null) => {
    setActiveZone(zone);
    if (zone === null || zone === 3) return;
    setThemeSide(zone < 3 ? "left" : "right");
  }, []);
  const activeSide: Side | null =
    activeZone === null || activeZone === 3 ? null : activeZone < 3 ? "left" : "right";
  const pairUrls = useMemo(() => pair.map((item) => item.url), [pair]);
  // Build both palettes once per pair. The five rating buttons then use their
  // own ThemeProvider, so hovering does not regenerate their colours.
  const leftTheme = useImageTheme(pair[0]?.url);
  const rightTheme = useImageTheme(pair[1]?.url);
  const tieTheme = baseSide === "left" ? leftTheme : rightTheme;
  // The palette follows the image the user is hovering/clicking. On a fresh
  // pair it follows whichever side was clicked last.
  const themeUrl = themeSide === "left" ? pair[0]?.url : pair[1]?.url;
  useStageImage(themeUrl ?? null);

  const fiveMode = useMemo(() => {
    return (
      modes.find((mode) => !mode.isBinary && mode.maxValue - mode.minValue === 4) ??
      modes.find((mode) => !mode.isBinary) ?? {
        id: 0,
        name: "ahp5",
        description: "",
        minValue: 1,
        maxValue: 5,
        isBinary: false,
        createdAt: "",
      }
    );
  }, [modes]);

  useEffect(() => {
    let cancelled = false;
    api
      .modes()
      .then((result) => {
        if (!cancelled) setModes(result);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  const loadPair = useCallback(async (exclude: string[]) => {
    setLoading(true);
    setError("");
    try {
      let items = await api.randomImages(2, exclude, undefined, requestFilters);
      if (items.length < 2 && exclude.length > 0) {
        items = await api.randomImages(2, [], undefined, requestFilters);
      }
      setThemeSide(lastPickedSide.current);
      setBaseSide(lastPickedSide.current);
      setPair(items);
      setPicked(null);
      setActiveZone(null);
      startedAt.current = Date.now();
      if (items.length < 2) {
        setError("ArtDream needs at least two artworks to start a duel.");
      }
    } catch (loadError) {
      setError(
        loadError instanceof ApiError
          ? loadError.message
          : "Could not find two artworks. Try again in a moment.",
      );
      setPair([]);
    } finally {
      setLoading(false);
    }
  }, [requestFilters]);

  useEffect(() => {
    void loadPair(recent.current);
  }, [loadPair, reloadTick]);

  const previewShown = useRef(false);
  useEffect(() => {
    if (previewShown.current || loading || pair.length < 2) return;
    previewShown.current = true;
    setZonePreviewOpen(true);
    setActiveZone(null);
  }, [loading, pair.length]);

  const submitVote = useCallback(
    async (payload: Omit<VotePayload, "leftImageId" | "rightImageId">) => {
      if (pair.length < 2 || voting) return;
      const [left, right] = pair;
      if (!left || !right) return;
      // Every ranking is uploaded right away, even without an account. The
      // local copy carries the anonymous identity so it can be claimed later.
      const votePayload: VotePayload = {
        ...payload,
        leftImageId: left.id,
        rightImageId: right.id,
        durationMs: Date.now() - startedAt.current,
        anonymousId: getAnonymousVoterId(),
      };
      const localEntry = recordLocalRanking(votePayload, { claimed: Boolean(user) });
      setVoting(true);
      try {
        await api.vote(votePayload);
        markLocalRankingSynced(localEntry.localId, Boolean(user));
        recent.current = [left.id, right.id, ...recent.current.filter((id) => id !== left.id && id !== right.id)].slice(0, 12);
        const mid = ((payload.minValue ?? 0) + (payload.maxValue ?? 0)) / 2;
        const isTie =
          payload.value === undefined || Math.abs(payload.value - mid) < 1;
        const fiveSpan = fiveMode.maxValue - fiveMode.minValue;
        const fiveRelative =
          payload.mode === fiveMode.name && payload.value !== undefined && fiveSpan > 0
            ? (payload.value - fiveMode.minValue) / fiveSpan
            : null;
        notify(
          isTie
            ? "Draw."
            : fiveRelative !== null
              ? fiveRelative < 0.01
                ? "Left wins."
                : fiveRelative < 0.5
                  ? "Left, barely."
                  : fiveRelative > 0.99
                    ? "Right wins."
                    : "Right, barely."
              : "Saved.",
          "success",
        );
        await sleep(430);
        await loadPair(recent.current);
      } catch (voteError) {
        setPicked(null);
        notify(
          voteError instanceof ApiError ? voteError.message : "Your vote did not reach the server.",
          "error",
        );
      } finally {
        setVoting(false);
      }
    },
    [loadPair, notify, pair, user, voting],
  );

  const pick = (side: Side) => {
    if (voting || pair.length < 2) return;
    lastPickedSide.current = side;
    setThemeSide(side);
    setPicked(side);
    setActiveZone(side === "left" ? 2 : 4);
    const range = fiveMode.maxValue - fiveMode.minValue;
    const value =
      side === "left"
        ? Math.round(fiveMode.minValue + range * 0.25)
        : Math.round(fiveMode.minValue + range * 0.75);
    void submitVote({
      mode: fiveMode.name,
      value,
      minValue: fiveMode.minValue,
      maxValue: fiveMode.maxValue,
      metadata: { ui: "soft-pick", side },
    });
  };

  const rateFive = (
    rating: number,
    ui: "keyboard-five" | "five-area" | "five-button" = "five-button",
  ) => {
    if (voting || pair.length < 2) return;
    if (rating !== 3) {
      const side: Side = rating < 3 ? "left" : "right";
      lastPickedSide.current = side;
      setThemeSide(side);
    }
    const span = fiveMode.maxValue - fiveMode.minValue;
    const value = Math.round(fiveMode.minValue + ((rating - 1) / 4) * span);
    setPicked(rating === 3 ? null : rating < 3 ? "left" : "right");
    setActiveZone(rating);
    void submitVote({
      mode: fiveMode.name,
      value,
      minValue: fiveMode.minValue,
      maxValue: fiveMode.maxValue,
      metadata: { ui, rating },
    });
  };

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (rankFiltersOpen || zonePreviewOpen) return;
      if (event.metaKey || event.ctrlKey || event.altKey || event.repeat) return;
      const target = event.target as HTMLElement | null;
      // Never steal keys from text and select controls.
      if (
        target?.isContentEditable ||
        target?.closest(
          'input, textarea, select, [role="combobox"], [role="listbox"]',
        )
      ) {
        return;
      }
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        pick("left");
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        pick("right");
      } else if (event.key >= "1" && event.key <= "5") {
        event.preventDefault();
        rateFive(Number(event.key), "keyboard-five");
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  });

  const left = pair[0];
  const right = pair[1];
  const leftAuthorHidden = hideAuthors;
  const rightAuthorHidden = hideAuthors;
  const leftName =
    left?.title || (!leftAuthorHidden ? left?.author : "") || "left image";
  const rightName =
    right?.title || (!rightAuthorHidden ? right?.author : "") || "right image";
  const fullscreenAuthorHidden = hideAuthors;

  return (
    <PageShell maxWidth="xl">
      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "flex-end",
          justifyContent: "space-between",
          gap: 2,
          mb: { xs: 2.5, md: 3.5 },
        }}
      >
        <Box>
          <Typography
            className="display"
            sx={{
              fontSize: "clamp(2rem, 4.6vw, 3.4rem)",
              lineHeight: 1,
              fontWeight: 640,
            }}
          >
            Which one moves you?
          </Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          {rankingAnonymized && (
            <Chip
              size="small"
              icon={<VisibilityOffRounded sx={{ fontSize: 16 }} />}
              label="authors anonymized"
              sx={{ background: alpha(text, 0.08), opacity: 0.8 }}
            />
          )}
          <Button
            size="small"
            startIcon={<TuneRounded />}
            onClick={() => {
              setRankFiltersDraft({ ...rankFilters });
              setRankFiltersOpen(true);
            }}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Source{filtersActive ? " ✓" : ""}
          </Button>
          <Button
            size="small"
            startIcon={<SwapHorizRounded />}
            onClick={() => {
              recent.current = [...recent.current, ...pairUrls].slice(-12);
              setReloadTick((tick) => tick + 1);
            }}
            disabled={loading || voting}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Skip pair
          </Button>
        </Box>
      </Box>

      {loading ? (
        <LoadingView label="Summoning two artworks…" />
      ) : error ? (
        <EmptyState
          emoji="🌱"
          title="No duel yet"
          description={error}
          action={
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", justifyContent: "center" }}>
              <Button variant="contained" onClick={() => setReloadTick((tick) => tick + 1)}>
                Try again
              </Button>
              <Button
                variant="outlined"
                startIcon={<CloudUploadOutlined />}
                onClick={() => navigate("/submit")}
              >
                Submit art
              </Button>
            </Box>
          }
        />
      ) : (
        <>
          <AnimatePresence mode="wait">
            <motion.div
              key={pair.map((item) => item.id).join("-")}
              initial={{ opacity: 0, y: 18 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -16, scale: 0.985 }}
              transition={{ duration: 0.42, ease: [0.2, 0.9, 0.2, 1] }}
            >
              <Box
                onClick={(event) => {
                  if (voting || zonePreviewOpen) return;
                  if (
                    (event.target as HTMLElement).closest(
                      'button, a, [role="button"]',
                    )
                  ) {
                    return;
                  }
                  rateFive(
                    ratingFromPoint(event.clientX, event.clientY, event.currentTarget),
                    "five-area",
                  );
                }}
                onMouseMove={(event) => {
                  if (voting || zonePreviewOpen) return;
                  previewZone(ratingFromPoint(event.clientX, event.clientY, event.currentTarget));
                }}
                onMouseLeave={() => previewZone(null)}
                onPointerDown={(event) => {
                  if (voting || zonePreviewOpen || event.pointerType === "mouse") return;
                  previewZone(
                    ratingFromPoint(event.clientX, event.clientY, event.currentTarget),
                  );
                }}
                sx={{
                  position: "relative",
                  cursor: voting ? "default" : "pointer",
                  touchAction: "pan-y",
                }}
              >
                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" },
                    gap: { xs: 2, md: 3 },
                  }}
                >
                {[left, right].map((item, index) => {
                  if (!item) return null;
                  const side: Side = index === 0 ? "left" : "right";
                  const isPicked = picked === side;
                  const hideAuthor = hideAuthors;
                  const cardLabel =
                    item.title || (hideAuthor ? "this artwork" : item.author);
                  const isActive = activeSide === side;
                  // Tilt the artwork the palette follows: 1/2 leans left,
                  // 4/5 leans right, 3 stays upright. Deliberately gentle.
                  const tilt =
                    isActive && activeZone !== null
                      ? (activeZone - 3) * 0.8
                      : isPicked
                        ? side === "left"
                          ? -1.2
                          : 1.2
                        : 0;
                  return (
                    <motion.div
                      key={item.id}
                      animate={{ rotate: tilt }}
                      transition={{ type: "spring", stiffness: 240, damping: 20 }}
                      style={{ position: "relative" }}
                    >
                      <Box
                        className="image-frame rank-image-card"
                        sx={{
                          height: { xs: "clamp(240px, 38vh, 380px)", md: "clamp(340px, 50vh, 560px)" },
                          border: `2px solid ${alpha(text, isActive ? 0.3 : 0.12)}`,
                          // The artwork being judged keeps a clean edge: no
                          // drop shadow and no darkening overlay.
                          boxShadow: isActive ? "none" : `0 24px 70px ${alpha(black, 0.45)}`,
                          "&::after": {
                            background: `linear-gradient(180deg, transparent 30%, ${alpha(black, 0.72)})`,
                            opacity: isActive ? 0 : 0.5,
                          },
                          "&:hover::after": { opacity: isActive ? 0 : 1 },
                          "&:focus-visible": {
                            boxShadow: `0 0 0 3px ${alpha(text, 0.75)}, 0 24px 70px ${alpha(black, 0.5)}`,
                          },
                        }}
                      >
                        <img
                          src={imageVariant(item.url, { h: 720, quality: 82 })}
                          alt={
                            item.title ||
                            (hideAuthor ? "Artwork" : `Artwork by ${item.author}`)
                          }
                          draggable={false}
                          decoding="async"
                          fetchPriority="high"
                        />

                        <Tooltip title="View fullscreen">
                          <IconButton
                            size="small"
                            aria-label={`View ${cardLabel} fullscreen`}
                            onClick={(event) => {
                              event.stopPropagation();
                              setFullscreenLoaded(false);
                              setFullscreen(item);
                            }}
                            onKeyDown={(event) => event.stopPropagation()}
                            sx={{
                              position: "absolute",
                              top: 12,
                              right: 12,
                              zIndex: 4,
                              color: text,
                              background: alpha(theme.palette.background.default, 0.62),
                              "&:hover": { background: alpha(theme.palette.background.default, 0.9) },
                            }}
                          >
                            <FullscreenRounded fontSize="small" />
                          </IconButton>
                        </Tooltip>

                        {isPicked && (
                          <Box className="pick-hint" sx={{ opacity: 1, transform: "none" }}>
                            <Box
                              sx={{
                                px: 2.4,
                                py: 1.2,
                                borderRadius: 99,
                                background: alpha(theme.palette.background.default, 0.78),
                                border: `1px solid ${alpha(text, 0.24)}`,
                                display: "flex",
                                alignItems: "center",
                                gap: 1,
                                fontWeight: 700,
                                letterSpacing: "0.01em",
                              }}
                            >
                              <AutoAwesomeRounded fontSize="small" />
                              {side === "left" ? "left" : "right"} it is
                            </Box>
                          </Box>
                        )}

                        <Box
                          sx={{
                            position: "absolute",
                            left: 0,
                            right: 0,
                            bottom: 0,
                            zIndex: 3,
                            pointerEvents: "none",
                            p: { xs: 1.8, md: 2.4 },
                            display: "flex",
                            alignItems: "flex-end",
                            justifyContent: "space-between",
                            gap: 1.5,
                          }}
                        >
                          {(item.title || (!hideAuthor && item.author)) && (
                            <Box sx={{ minWidth: 0 }}>
                              {item.title && (
                                <Typography
                                  className="display"
                                  sx={{
                                    fontSize: { xs: "1.25rem", md: "1.7rem" },
                                    fontWeight: 600,
                                    lineHeight: 1.1,
                                    color: text,
                                    textShadow: `0 2px 18px ${alpha(black, 0.6)}`,
                                  }}
                                  noWrap
                                >
                                  {item.title}
                                </Typography>
                              )}
                              {!hideAuthor && item.author && (
                                <Typography
                                  sx={{
                                    color: alpha(text, 0.72),
                                    fontSize: "0.82rem",
                                    letterSpacing: "0.05em",
                                    textTransform: "uppercase",
                                  }}
                                  noWrap
                                >
                                  {item.author}
                                </Typography>
                              )}
                            </Box>
                          )}
                          <Box
                            sx={{
                              fontSize: "0.7rem",
                              color: alpha(text, 0.55),
                              border: `1px solid ${alpha(text, 0.2)}`,
                              borderRadius: 99,
                              px: 1,
                              py: 0.35,
                              flexShrink: 0,
                            }}
                          >
                            {side}
                          </Box>
                        </Box>
                      </Box>
                    </motion.div>
                  );
                })}
                </Box>

                {zonePreviewOpen && (
                  <Box
                    role="dialog"
                    aria-modal="true"
                    aria-label="How five-zone ranking works"
                    sx={{
                      position: "absolute",
                      inset: 0,
                      zIndex: 6,
                      display: "grid",
                      gridTemplateColumns: {
                        xs: "1fr",
                        md: "repeat(5, minmax(0, 1fr))",
                      },
                      gridTemplateRows: {
                        xs: "repeat(5, minmax(0, 1fr))",
                        md: "1fr",
                      },
                      borderRadius: "22px",
                      overflow: "hidden",
                      background: alpha(theme.palette.background.default, 0.42),
                    }}
                  >
                    {FIVE_RATE_HINTS.map((hint, index) => (
                      <Box
                        key={hint.key}
                        sx={{
                          display: "flex",
                          flexDirection: { xs: "row", md: "column" },
                          alignItems: "center",
                          justifyContent: "center",
                          gap: { xs: 1.2, md: 0.6 },
                          border: `1px solid ${alpha(text, 0.14)}`,
                          background:
                            index % 2 === 0
                              ? alpha(text, 0.035)
                              : alpha(text, 0.015),
                        }}
                      >
                        <Typography
                          sx={{
                            fontSize: { xs: "1.5rem", md: "2rem" },
                            fontWeight: 750,
                            lineHeight: 1,
                          }}
                        >
                          {index + 1}
                        </Typography>
                        <Typography
                          sx={{
                            fontSize: "0.68rem",
                            letterSpacing: "0.12em",
                            textTransform: "uppercase",
                            color: alpha(text, 0.72),
                            whiteSpace: "nowrap",
                          }}
                        >
                          {hint.label}
                        </Typography>
                      </Box>
                    ))}

                    <Box
                      sx={{
                        position: "absolute",
                        left: "50%",
                        bottom: 14,
                        transform: "translateX(-50%)",
                        width: { xs: "calc(100% - 28px)", sm: "auto" },
                        maxWidth: 430,
                        p: 1.8,
                        borderRadius: 3,
                        textAlign: "center",
                        background: alpha(theme.palette.background.default, 0.92),
                        border: `1px solid ${alpha(text, 0.16)}`,
                        boxShadow: `0 20px 50px ${alpha(black, 0.45)}`,
                      }}
                    >
                      <Typography sx={{ fontWeight: 700, fontSize: "0.92rem" }}>
                        {isWideLayout ? "Click a column" : "Tap a row"}
                      </Typography>
                      <Typography
                        sx={{
                          mt: 0.25,
                          mb: 1.2,
                          fontSize: "0.76rem",
                          color: "text.secondary",
                        }}
                      >
                        {isWideLayout
                          ? "1 = far left · 5 = far right"
                          : "1 = top · 5 = bottom"}
                      </Typography>
                      <Button
                        size="small"
                        variant="contained"
                        disableElevation
                        onClick={() => {
                          setZonePreviewOpen(false);
                          setActiveZone(null);
                        }}
                        sx={{ borderRadius: 99, px: 2.4, textTransform: "none" }}
                      >
                        Got it
                      </Button>
                    </Box>
                  </Box>
                )}

              </Box>
            </motion.div>
          </AnimatePresence>

          <Box
            sx={{
              mt: { xs: 2.5, md: 3 },
              borderRadius: 5,
              p: { xs: 2.2, sm: 3 },
              opacity: voting ? 0.75 : 1,
              transition: "opacity 200ms ease",
              background: alpha(theme.palette.background.paper, 0.72),
              border: `1px solid ${alpha(text, 0.12)}`,
              boxShadow: `0 16px 44px ${alpha(black, 0.3)}, inset 0 1px 0 ${alpha(text, 0.05)}`,
            }}
          >
            <DuelPanel
              leftName={leftName}
              rightName={rightName}
              disabled={voting || zonePreviewOpen}
              zone={activeZone}
              onZoneChange={previewZone}
              onPick={pick}
              onRate={rateFive}
              leftTheme={leftTheme}
              rightTheme={rightTheme}
              tieTheme={tieTheme}
            />
          </Box>
        </>
      )}

      {!user && (
        <Box sx={{ mt: 2.5, textAlign: "center" }}>
          <Typography variant="body2" color="text.secondary">
            Rankings are saved on this device and uploaded right away.{" "}
            <Button onClick={() => navigate("/login?next=/rank")}>Sign in</Button>{" "}
            to link them to your account.
          </Typography>
        </Box>
      )}

      <Dialog
        fullScreen
        open={Boolean(fullscreen)}
        onClose={() => setFullscreen(null)}
        PaperProps={{ sx: { background: alpha(theme.palette.background.default, 0.97) } }}
      >
        <Box
          sx={{
            position: "relative",
            height: "100vh",
            width: "100vw",
            display: "grid",
            placeItems: "center",
            p: { xs: 0, sm: 3 },
            overflow: "hidden",
          }}
        >
          <IconButton
            aria-label="Close fullscreen"
            onClick={() => setFullscreen(null)}
            sx={{
              position: "fixed",
              top: { xs: 12, sm: 20 },
              right: { xs: 12, sm: 20 },
              zIndex: 5,
              color: text,
              background: alpha(theme.palette.background.default, 0.65),
              "&:hover": { background: alpha(theme.palette.background.default, 0.92) },
            }}
          >
            <CloseRounded />
          </IconButton>

          {!fullscreenLoaded && (
            <CircularProgress sx={{ position: "absolute", zIndex: 1 }} />
          )}

          {fullscreen && (
            <>
              <Box
                component="img"
                src={imageVariant(fullscreen.url, { w: 2560, quality: 90 })}
                alt={
                  fullscreen.title ||
                  (fullscreenAuthorHidden ? "Artwork" : `Artwork by ${fullscreen.author}`)
                }
                draggable={false}
                onLoad={() => setFullscreenLoaded(true)}
                onError={() => setFullscreenLoaded(true)}
                sx={{
                  maxWidth: "100%",
                  maxHeight: "100%",
                  objectFit: "contain",
                  opacity: fullscreenLoaded ? 1 : 0,
                  transition: "opacity 200ms ease",
                }}
              />
                {(fullscreen.title || (!fullscreenAuthorHidden && fullscreen.author)) && (
                  <Box
                    sx={{
                      position: "absolute",
                      left: 0,
                      right: 0,
                      bottom: 0,
                      p: { xs: 2, sm: 3 },
                      background: `linear-gradient(0deg, ${alpha(black, 0.72)}, transparent)`,
                      pointerEvents: "none",
                    }}
                  >
                    {fullscreen.title && (
                      <Typography
                        className="display"
                        sx={{ color: text, fontSize: { xs: "1.4rem", sm: "2rem" }, fontWeight: 620 }}
                      >
                        {fullscreen.title}
                      </Typography>
                    )}
                    {!fullscreenAuthorHidden && fullscreen.author && (
                      <Typography sx={{ color: alpha(text, 0.7) }}>
                        {fullscreen.author}
                      </Typography>
                    )}
                  </Box>
                )}
            </>
          )}
        </Box>
      </Dialog>
      <Dialog
        open={rankFiltersOpen}
        onClose={() => setRankFiltersOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle
          sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 650, fontSize: "1.6rem" }}
        >
          Choose your pair source
        </DialogTitle>
        <Divider />
        <DialogContent sx={{ pt: 2 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Leave everything on default to follow the admin setting. Pick private and “only mine”
            for a duel of your own work, or add friend usernames/ids for a private circle.
          </Typography>
          <Box sx={{ display: "grid", gap: 1.6, gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" } }}>
            <TextField
              select
              size="small"
              label="Visibility"
              value={rankFiltersDraft.visibility}
              onChange={(event) =>
                setRankFiltersDraft({
                  ...rankFiltersDraft,
                  visibility: event.target.value as "" | ImageVisibility,
                })
              }
            >
              <MenuItem value="">Admin default{frontendConfig.defaultImageVisibility === "all" ? "" : ` (${frontendConfig.defaultImageVisibility})`}</MenuItem>
              <MenuItem value="all">All images</MenuItem>
              <MenuItem value="public">Public service only</MenuItem>
              <MenuItem value="private">Private only</MenuItem>
            </TextField>
            <TextField
              size="small"
              label="Service contains"
              value={rankFiltersDraft.service}
              onChange={(event) =>
                setRankFiltersDraft({ ...rankFiltersDraft, service: event.target.value })
              }
              placeholder="Instagram, DeviantArt…"
            />
            <TextField
              size="small"
              label="Friends / uploaders"
              value={rankFiltersDraft.friends}
              onChange={(event) =>
                setRankFiltersDraft({ ...rankFiltersDraft, friends: event.target.value })
              }
              placeholder="@names or account ids, comma separated"
              helperText="Account usernames or numeric ids. Combine with “only mine”."
              sx={{ gridColumn: { sm: "1 / -1" } }}
            />
            <TextField
              size="small"
              label="Include authors (regex)"
              value={rankFiltersDraft.includeAuthors}
              onChange={(event) =>
                setRankFiltersDraft({ ...rankFiltersDraft, includeAuthors: event.target.value })
              }
              placeholder="ada|bob"
              helperText="Comma separated. Images must match at least one."
            />
            <TextField
              size="small"
              label="Exclude authors (regex)"
              value={rankFiltersDraft.excludeAuthors}
              onChange={(event) =>
                setRankFiltersDraft({ ...rankFiltersDraft, excludeAuthors: event.target.value })
              }
              placeholder="spam.*"
              helperText="Comma separated blacklist."
            />
            <TextField
              size="small"
              label="Include titles (regex)"
              value={rankFiltersDraft.includeTitles}
              onChange={(event) =>
                setRankFiltersDraft({ ...rankFiltersDraft, includeTitles: event.target.value })
              }
              placeholder="sunset|sea"
            />
            <TextField
              size="small"
              label="Exclude titles (regex)"
              value={rankFiltersDraft.excludeTitles}
              onChange={(event) =>
                setRankFiltersDraft({ ...rankFiltersDraft, excludeTitles: event.target.value })
              }
              placeholder="advert"
            />
          </Box>
          <FormControlLabel
            sx={{ mt: 1.4 }}
            control={
              <Checkbox
                checked={rankFiltersDraft.onlyMine}
                onChange={(event) =>
                  setRankFiltersDraft({ ...rankFiltersDraft, onlyMine: event.target.checked })
                }
                disabled={!user}
              />
            }
            label={user ? `Only my uploads (@${user.username})` : "Only my uploads (sign in first)"}
          />
        </DialogContent>
        <Divider />
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, p: 2 }}>
          <Button
            onClick={() => {
              setRankFiltersDraft(emptyRankFilters());
              setRankFilters(emptyRankFilters());
              setRankFiltersOpen(false);
              setReloadTick((tick) => tick + 1);
            }}
            sx={{ textTransform: "none" }}
          >
            Reset
          </Button>
          <Button
            variant="contained"
            onClick={() => {
              setRankFilters(rankFiltersDraft);
              setRankFiltersOpen(false);
              setReloadTick((tick) => tick + 1);
            }}
            sx={{ borderRadius: 99, textTransform: "none" }}
          >
            Apply source
          </Button>
        </Box>
      </Dialog>

    </PageShell>
  );
}