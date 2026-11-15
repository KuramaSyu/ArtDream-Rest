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
  Slider,
  TextField,
  Tooltip,
  Typography,
  alpha,
  useTheme,
} from "@mui/material";
import {
  ArrowLeftRounded,
  ArrowRightRounded,
  AutoAwesomeRounded,
  CloseRounded,
  CloudUploadOutlined,
  EmojiEventsRounded,
  FullscreenRounded,
  SwapHorizRounded,
  TuneRounded,
  VisibilityOffRounded,
} from "@mui/icons-material";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
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
import { useStageImages } from "../theme/ThemeHost";
import { useFrontendConfig } from "../config/FrontendConfig";

type Side = "left" | "right";

type SliderHover = { value: number; id: number };

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

function verdictText(value: number, left: string, right: string): string {
  if (value >= 49 && value <= 52) return "a perfect draw";
  if (value < 49) {
    const strength = value <= 12 ? "clearly" : value <= 32 ? "softly" : "barely";
    return `${strength} ${left}`;
  }
  const strength = value >= 88 ? "clearly" : value >= 68 ? "softly" : "barely";
  return `${strength} ${right}`;
}

const FIVE_RATE_HINTS = [
  { key: "1", label: "left" },
  { key: "2", label: "lean left" },
  { key: "3", label: "even" },
  { key: "4", label: "lean right" },
  { key: "5", label: "right" },
] as const;

/** Long-slider value previewed while the matching five-point area is hovered. */
const FIVE_AREA_VALUES = [1, 26, 50, 75, 100] as const;

function Keycap({ children }: { children: string }) {
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
        border: "1px solid rgba(255,255,255,.2)",
        background: "rgba(255,255,255,.06)",
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

type DuelSliderProps = {
  leftName: string;
  rightName: string;
  disabled: boolean;
  hover: SliderHover | null;
  resetKey: string;
  onValue: (value: number) => void;
  onPick: (side: Side) => void;
  onRate: (rating: number) => void;
};

/**
 * Long slider plus its verdict label. Kept as its own component so the
 * requestAnimationFrame animation only re-renders this small subtree instead of
 * the whole duel page (artwork cards included) on every frame.
 *
 * Values are fractional while animating: the thumb glides by sub-pixel amounts
 * instead of stepping across 1% chunks of the track, and exponential smoothing
 * keeps the velocity continuous when the pointer hops from one image to the other.
 */
function DuelSlider({
  leftName,
  rightName,
  disabled,
  hover,
  resetKey,
  onValue,
  onPick,
  onRate,
}: DuelSliderProps) {
  const theme = useTheme();
  const [value, setValue] = useState(50);
  const [zone, setZone] = useState<number | null>(null);
  const valueRef = useRef(50);
  const goalRef = useRef(50);
  const animation = useRef<number | null>(null);
  const onValueRef = useRef(onValue);
  onValueRef.current = onValue;

  const stop = useCallback(() => {
    if (animation.current !== null) {
      cancelAnimationFrame(animation.current);
      animation.current = null;
    }
  }, []);

  const commit = useCallback((next: number) => {
    valueRef.current = next;
    onValueRef.current(next);
    setValue(next);
  }, []);

  /** Glide toward a goal, keeping velocity when the goal changes mid-flight. */
  const glideTo = useCallback(
    (goal: number) => {
      goalRef.current = goal;
      if (animation.current !== null) return;
      let last = performance.now();
      const step = (now: number) => {
        const dt = Math.min(64, Math.max(0, now - last));
        last = now;
        const current = valueRef.current;
        const target = goalRef.current;
        const next = current + (target - current) * (1 - Math.exp(-dt / 70));
        if (Math.abs(target - next) < 0.4) {
          commit(target);
          animation.current = null;
          return;
        }
        commit(next);
        animation.current = requestAnimationFrame(step);
      };
      animation.current = requestAnimationFrame(step);
    },
    [commit],
  );

  useEffect(() => () => stop(), [stop]);

  // Every new pair starts from the middle.
  useEffect(() => {
    stop();
    goalRef.current = 50;
    valueRef.current = 50;
    onValueRef.current(50);
    setValue(50);
    setZone(null);
  }, [resetKey, stop]);

  useEffect(() => {
    if (!hover) return;
    setZone(null);
    glideTo(hover.value);
  }, [hover, glideTo]);

  const verdict = verdictText(value, leftName, rightName);
  const activeZone = zone;

  return (
    <>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 2,
          mb: 0.4,
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
            {verdict}
          </Typography>
          <Typography sx={{ fontSize: "0.68rem", letterSpacing: "0.14em", opacity: 0.55 }}>
            {activeZone !== null
              ? `${activeZone}/5 — ${FIVE_RATE_HINTS[activeZone - 1].label}`
              : `${Math.round(value)}/100`}
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
          gridTemplateColumns: { xs: "minmax(0, 1fr)", md: "repeat(5, minmax(0, 1fr))" },
          gap: 1,
          mt: 1.4,
          mb: 1.6,
          opacity: disabled ? 0.55 : 1,
          transition: "opacity 180ms ease",
        }}
        onMouseLeave={() => setZone(null)}
      >
        {FIVE_RATE_HINTS.map((hint, index) => {
          const rating = index + 1;
          const active = activeZone === rating;
          const tint =
            rating < 3
              ? theme.palette.primary.main
              : rating > 3
                ? theme.palette.secondary.main
                : theme.palette.text.primary;
          return (
            <Box
              key={hint.key}
              role="button"
              tabIndex={disabled ? -1 : 0}
              aria-disabled={disabled}
              aria-label={`Rate ${rating} of 5, ${hint.label}`}
              onMouseEnter={() => {
                if (disabled) return;
                setZone(rating);
                glideTo(FIVE_AREA_VALUES[index]);
              }}
              onFocus={() => {
                if (disabled) return;
                setZone(rating);
                glideTo(FIVE_AREA_VALUES[index]);
              }}
              onBlur={() => setZone((current) => (current === rating ? null : current))}
              onClick={() => {
                if (!disabled) onRate(rating);
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  event.preventDefault();
                  if (!disabled) onRate(rating);
                }
              }}
              sx={{
                position: "relative",
                display: "flex",
                flexDirection: { xs: "row", md: "column" },
                alignItems: "center",
                justifyContent: "center",
                gap: { xs: 1.2, md: 0.5 },
                minHeight: { xs: 52, md: 86 },
                px: 1.5,
                borderRadius: 3,
                cursor: disabled ? "default" : "pointer",
                outline: "none",
                background: active
                  ? `linear-gradient(135deg, ${alpha(tint, 0.3)}, ${alpha(tint, 0.1)})`
                  : "rgba(255,255,255,.035)",
                border: active
                  ? `1.5px solid ${alpha(tint, 0.85)}`
                  : "1.5px solid rgba(255,255,255,.09)",
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
                  fontSize: { xs: "1.2rem", md: "1.7rem" },
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
        })}
      </Box>

      <Box sx={{ px: 0.5, pt: 0.6 }}>
        <Slider
          className="slider-track-gradient"
          value={value}
          onChange={(_event, next) => {
            stop();
            setZone(null);
            const numeric = next as number;
            goalRef.current = numeric;
            commit(numeric);
          }}
          min={1}
          max={100}
          step={1}
          disabled={disabled}
          aria-label="How much better is the right image than the left image?"
          sx={{
            "--ad-slider-gradient": `linear-gradient(90deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
          }}
        />
        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            fontSize: "0.66rem",
            letterSpacing: "0.1em",
            textTransform: "uppercase",
            color: "text.secondary",
            opacity: 0.7,
            mt: -0.4,
            px: 0.5,
          }}
        >
          <span>left feels better</span>
          <span>even</span>
          <span>right feels better</span>
        </Box>

        <Box
          aria-label="Keyboard shortcuts"
          sx={{
            display: { xs: "none", md: "flex" },
            flexWrap: "wrap",
            alignItems: "center",
            columnGap: 1.4,
            rowGap: 1,
            mt: 1.7,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "flex-end", gap: 0.9 }}>
            {FIVE_RATE_HINTS.map((hint) => (
              <Box
                key={hint.key}
                sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 0.35 }}
              >
                <Keycap>{hint.key}</Keycap>
                <Box
                  sx={{
                    fontSize: "0.6rem",
                    letterSpacing: "0.05em",
                    textTransform: "uppercase",
                    color: "text.secondary",
                    opacity: 0.65,
                    whiteSpace: "nowrap",
                  }}
                >
                  {hint.label}
                </Box>
              </Box>
            ))}
          </Box>

          <Divider orientation="vertical" flexItem sx={{ opacity: 0.2, alignSelf: "stretch" }} />

          <Box sx={{ display: "flex", alignItems: "center", gap: 0.6 }}>
            <Keycap>←</Keycap>
            <Keycap>→</Keycap>
            <Typography sx={{ fontSize: "0.7rem", color: "text.secondary", opacity: 0.8 }}>
              soft pick
            </Typography>
          </Box>

          <Divider orientation="vertical" flexItem sx={{ opacity: 0.2, alignSelf: "stretch" }} />

          <Box sx={{ display: "flex", alignItems: "center", gap: 0.6 }}>
            <Keycap>Enter</Keycap>
            <Typography sx={{ fontSize: "0.7rem", color: "text.secondary", opacity: 0.8 }}>
              send slider
            </Typography>
          </Box>
        </Box>
      </Box>
    </>
  );
}

export function RankPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const { notify } = useToast();
  const theme = useTheme();
  const { config: frontendConfig, loading: frontendConfigLoading } = useFrontendConfig();
  const rankingAnonymized = frontendConfig.rankingAnonymized;
  const hideAuthors = frontendConfigLoading || rankingAnonymized;

  const [modes, setModes] = useState<VoteMode[]>([]);
  const [pair, setPair] = useState<ImageItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [voting, setVoting] = useState(false);
  const [error, setError] = useState("");
  const [picked, setPicked] = useState<Side | null>(null);
  const [hover, setHover] = useState<SliderHover | null>(null);
  const [castCount, setCastCount] = useState(0);
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
  const sliderRef = useRef(50);
  const hoverId = useRef(0);
  const handleSliderValue = useCallback((value: number) => {
    sliderRef.current = value;
  }, []);

  const pairUrls = useMemo(() => pair.map((item) => item.url), [pair]);
  useStageImages(pairUrls);

  const handleHover = (side: Side) => {
    if (voting || pair.length < 2) return;
    if (!window.matchMedia("(hover: hover)").matches) return;
    // Hand a fresh target to the slider; it glides there from wherever it is.
    hoverId.current += 1;
    setHover({ value: side === "left" ? 25 : 75, id: hoverId.current });
  };

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

  const hundredMode = useMemo(() => {
    return (
      modes.find((mode) => !mode.isBinary && mode.maxValue - mode.minValue >= 90) ??
      modes.find((mode) => !mode.isBinary && mode.maxValue - mode.minValue > 10) ??
      {
        id: 0,
        name: "ahp100",
        description: "",
        minValue: 1,
        maxValue: 100,
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
      setPair(items);
      setPicked(null);
      hoverId.current += 1;
      setHover(null);
      sliderRef.current = 50;
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

  const submitVote = useCallback(
    async (payload: Omit<VotePayload, "leftImageId" | "rightImageId">) => {
      if (pair.length < 2 || voting) return;
      const [left, right] = pair;
      if (!left || !right) return;
      setVoting(true);
      try {
        await api.vote({
          ...payload,
          leftImageId: left.id,
          rightImageId: right.id,
          durationMs: Date.now() - startedAt.current,
        });
        recent.current = [left.id, right.id, ...recent.current.filter((id) => id !== left.id && id !== right.id)].slice(0, 12);
        setCastCount((count) => count + 1);
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
            ? "A draw. Both keep their dignity."
            : fiveRelative !== null
              ? fiveRelative < 0.01
                ? "Left, unmistakably."
                : fiveRelative < 0.5
                  ? "Left got a gentle nod."
                  : fiveRelative > 0.99
                    ? "Right, unmistakably."
                    : "Right got a gentle nod."
              : "Verdict sealed.",
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
    [loadPair, notify, pair, voting],
  );

  const pick = (side: Side) => {
    if (voting || pair.length < 2) return;
    setPicked(side);
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
      metadata: { ui: "image-click", side },
    });
  };

  const rateFive = (rating: number) => {
    if (voting || pair.length < 2) return;
    const span = fiveMode.maxValue - fiveMode.minValue;
    const value = Math.round(fiveMode.minValue + ((rating - 1) / 4) * span);
    setPicked(rating === 3 ? null : rating < 3 ? "left" : "right");
    void submitVote({
      mode: fiveMode.name,
      value,
      minValue: fiveMode.minValue,
      maxValue: fiveMode.maxValue,
      metadata: { ui: "keyboard-five", rating },
    });
  };

  const submitSlider = () => {
    void submitVote({
      mode: hundredMode.name,
      value: Math.round(sliderRef.current),
      minValue: 1,
      maxValue: 100,
      metadata: { ui: "long-slider" },
    });
  };

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      if (rankFiltersOpen) return;
      if (event.metaKey || event.ctrlKey || event.altKey || event.repeat) return;
      const target = event.target as HTMLElement | null;
      // Never steal keys from text controls, selects, or the slider itself.
      if (
        target?.isContentEditable ||
        target?.closest(
          'input, textarea, select, [role="combobox"], [role="listbox"], [role="slider"], .MuiSlider-root',
        )
      ) {
        return;
      }
      // Let a focused button/link handle Enter itself.
      if (
        event.key === "Enter" &&
        target?.closest('button, a, [role="button"]')
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
        rateFive(Number(event.key));
      } else if (event.key === "Enter") {
        event.preventDefault();
        submitSlider();
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
          <Box
            sx={{
              fontSize: "0.72rem",
              letterSpacing: "0.22em",
              textTransform: "uppercase",
              opacity: 0.6,
              mb: 0.7,
            }}
          >
            {castCount > 0 ? `${castCount} verdict${castCount === 1 ? "" : "s"} cast` : "a duel of dreams"}
          </Box>
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
              sx={{ background: "rgba(255,255,255,.08)", opacity: 0.8 }}
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
                  return (
                    <motion.div
                      key={item.id}
                      whileHover={{ rotate: side === "left" ? -0.35 : 0.35 }}
                      whileTap={{ scale: 0.985 }}
                      animate={isPicked ? { scale: 1.018 } : { scale: 1 }}
                      transition={{ type: "spring", stiffness: 260, damping: 22 }}
                      style={{ position: "relative" }}
                    >
                      <Box
                        className="image-frame rank-image-card"
                        role="button"
                        tabIndex={0}
                        aria-label={`Vote for ${cardLabel}`}
                        onClick={() => pick(side)}
                        onMouseEnter={() => handleHover(side)}
                        onKeyDown={(event) => {
                          if (event.key === "Enter" || event.key === " ") {
                            event.preventDefault();
                            pick(side);
                          }
                        }}
                        sx={{
                          height: { xs: "clamp(240px, 38vh, 380px)", md: "clamp(340px, 50vh, 560px)" },
                          border: isPicked
                            ? `2px solid ${theme.palette.primary.main}`
                            : "2px solid rgba(255,255,255,.12)",
                          transition: "border-color 220ms ease, box-shadow 220ms ease",
                          boxShadow: isPicked
                            ? `0 30px 90px ${alpha(theme.palette.primary.main, 0.35)}`
                            : undefined,
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
                              color: "#fff",
                              background: "rgba(10,9,15,.62)",
                              "&:hover": { background: "rgba(10,9,15,.9)" },
                            }}
                          >
                            <FullscreenRounded fontSize="small" />
                          </IconButton>
                        </Tooltip>

                        <Box
                          className="pick-hint"
                          sx={{
                            opacity: isPicked ? 1 : undefined,
                            transform: isPicked ? "none" : undefined,
                          }}
                        >
                          <Box
                            sx={{
                              px: 2.4,
                              py: 1.2,
                              borderRadius: 99,
                              background: "rgba(10,9,15,.78)",
                              border: "1px solid rgba(255,255,255,.24)",
                              display: "flex",
                              alignItems: "center",
                              gap: 1,
                              fontWeight: 700,
                              letterSpacing: "0.01em",
                            }}
                          >
                            {isPicked ? (
                              <>
                                <AutoAwesomeRounded fontSize="small" />
                                {side === "left" ? "left" : "right"} it is
                              </>
                            ) : (
                              <>
                                {side === "left" ? <ArrowLeftRounded /> : <ArrowRightRounded />}
                                Pick this one
                              </>
                            )}
                          </Box>
                        </Box>

                        <Box
                          sx={{
                            position: "absolute",
                            left: 0,
                            right: 0,
                            bottom: 0,
                            zIndex: 3,
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
                                    color: "#fff",
                                    textShadow: "0 2px 18px rgba(0,0,0,.6)",
                                  }}
                                  noWrap
                                >
                                  {item.title}
                                </Typography>
                              )}
                              {!hideAuthor && item.author && (
                                <Typography
                                  sx={{
                                    color: "rgba(255,255,255,.72)",
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
                              color: "rgba(255,255,255,.55)",
                              border: "1px solid rgba(255,255,255,.2)",
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
            </motion.div>
          </AnimatePresence>

          <Box
            className="glass"
            sx={{
              mt: { xs: 2.5, md: 3 },
              borderRadius: 5,
              p: { xs: 2.2, sm: 3 },
              opacity: voting ? 0.75 : 1,
              transition: "opacity 200ms ease",
            }}
          >
            <DuelSlider
              leftName={leftName}
              rightName={rightName}
              disabled={voting}
              hover={hover}
              resetKey={`${left?.id ?? ""}-${right?.id ?? ""}`}
              onValue={handleSliderValue}
              onPick={pick}
              onRate={rateFive}
            />

            <Divider sx={{ my: 2, opacity: 0.4 }} />

            <Box
              sx={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 1.5,
              }}
            >
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ display: { xs: "block", md: "none" }, opacity: 0.8 }}
              >
                Tap an image for a soft 2/5 or 4/5, tap an area for 1–5, or slide for the whole story.
              </Typography>
              <Typography
                variant="body2"
                color="text.secondary"
                sx={{ display: { xs: "none", md: "block" }, opacity: 0.8 }}
              >
                Hover an image to nudge the slider, pick one of the five areas, or slide for the whole story.
              </Typography>
              <Button
                variant="contained"
                size="large"
                onClick={submitSlider}
                disabled={voting}
                disableElevation
                endIcon={
                  voting ? (
                    <CircularProgress size={18} color="inherit" />
                  ) : (
                    <EmojiEventsRounded />
                  )
                }
                sx={{
                  borderRadius: 99,
                  px: 3,
                  py: 1.1,
                  textTransform: "none",
                  fontWeight: 700,
                  "&:hover": { transform: "translateY(-1px) scale(1.01)" },
                  transition: "transform 200ms ease",
                }}
              >
                send
              </Button>
            </Box>
          </Box>
        </>
      )}

      {!user && (
        <Box sx={{ mt: 2.5, textAlign: "center" }}>
          <Typography variant="body2" color="text.secondary">
            Votes are stored per account. <Button onClick={() => navigate("/login?next=/rank")}>Sign in</Button>{" "}
            to keep yours.
          </Typography>
        </Box>
      )}

      <Dialog
        fullScreen
        open={Boolean(fullscreen)}
        onClose={() => setFullscreen(null)}
        PaperProps={{ sx: { background: "rgba(8,7,12,.97)" } }}
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
              color: "#fff",
              background: "rgba(10,9,15,.65)",
              "&:hover": { background: "rgba(10,9,15,.92)" },
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
                      background: "linear-gradient(0deg, rgba(0,0,0,.72), transparent)",
                      pointerEvents: "none",
                    }}
                  >
                    {fullscreen.title && (
                      <Typography
                        className="display"
                        sx={{ color: "#fff", fontSize: { xs: "1.4rem", sm: "2rem" }, fontWeight: 620 }}
                      >
                        {fullscreen.title}
                      </Typography>
                    )}
                    {!fullscreenAuthorHidden && fullscreen.author && (
                      <Typography sx={{ color: "rgba(255,255,255,.7)" }}>
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