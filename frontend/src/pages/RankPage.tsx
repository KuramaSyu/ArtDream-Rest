import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Slider,
  Tooltip,
  Typography,
  alpha,
  useTheme,
} from "@mui/material";
import {
  ArrowLeftRounded,
  ArrowRightRounded,
  AutoAwesomeRounded,
  CloudUploadOutlined,
  EmojiEventsRounded,
  SwapHorizRounded,
} from "@mui/icons-material";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { api, ApiError, imageVariant } from "../api/client";
import type { ImageItem, VoteMode, VotePayload } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { EmptyState, LoadingView } from "../components/Loading";
import { PageShell } from "../components/PageShell";
import { useToast } from "../components/Feedback";
import { useStageImages } from "../theme/ThemeHost";

type Side = "left" | "right";

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

export function RankPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const { notify } = useToast();
  const theme = useTheme();

  const [modes, setModes] = useState<VoteMode[]>([]);
  const [pair, setPair] = useState<ImageItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [voting, setVoting] = useState(false);
  const [error, setError] = useState("");
  const [slider, setSlider] = useState(50);
  const [picked, setPicked] = useState<Side | null>(null);
  const [castCount, setCastCount] = useState(0);
  const [reloadTick, setReloadTick] = useState(0);

  const recent = useRef<string[]>([]);
  const startedAt = useRef(Date.now());

  const pairUrls = useMemo(() => pair.map((item) => item.url), [pair]);
  useStageImages(pairUrls);

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
      let items = await api.randomImages(2, exclude);
      if (items.length < 2 && exclude.length > 0) {
        items = await api.randomImages(2);
      }
      setPair(items);
      setPicked(null);
      setSlider(50);
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
  }, []);

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
        notify(
          isTie
            ? "A draw. Both keep their dignity."
            : payload.mode === fiveMode.name
              ? `${payload.value === fiveMode.minValue + (fiveMode.maxValue - fiveMode.minValue) * 0.25 ? "Left" : "Right"} got a gentle nod.`
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

  const submitSlider = () => {
    void submitVote({
      mode: hundredMode.name,
      value: slider,
      minValue: 1,
      maxValue: 100,
      metadata: { ui: "long-slider" },
    });
  };

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      // Never steal keys from form controls, the slider, links or buttons.
      if (
        target?.isContentEditable ||
        target?.closest(
          'input, textarea, select, button, a, [role="slider"], [role="button"], .MuiSlider-root',
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
  const leftName = left?.title || left?.author || "left";
  const rightName = right?.title || right?.author || "right";
  const verdict = verdictText(slider, leftName, rightName);

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
          <Chip
            size="small"
            icon={<AutoAwesomeRounded sx={{ fontSize: 16 }} />}
            label="left / right keys"
            sx={{ background: "rgba(255,255,255,.08)", opacity: 0.8 }}
          />
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
                        aria-label={`Vote for ${item.title || item.author}`}
                        onClick={() => pick(side)}
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
                          src={imageVariant(item.url, { w: 1400, quality: 88 })}
                          alt={item.title || `Artwork by ${item.author}`}
                          draggable={false}
                          loading="lazy"
                        />

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
                              background: "rgba(10,9,15,.52)",
                              backdropFilter: "blur(12px)",
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
                          <Box sx={{ minWidth: 0 }}>
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
                              {item.title || "Untitled dream"}
                            </Typography>
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
                          </Box>
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
                  <IconButton size="small" onClick={() => pick("left")} disabled={voting}>
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
                  {slider}/100
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
                  <IconButton size="small" onClick={() => pick("right")} disabled={voting}>
                    <ArrowRightRounded />
                  </IconButton>
                </Tooltip>
              </Box>
            </Box>

            <Box sx={{ px: 0.5, pt: 0.6 }}>
              <Slider
                className="slider-track-gradient"
                value={slider}
                onChange={(_event, value) => setSlider(value as number)}
                min={1}
                max={100}
                step={1}
                disabled={voting}
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
            </Box>

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
              <Typography variant="body2" color="text.secondary" sx={{ opacity: 0.8 }}>
                Tap an image for a soft 2/5 or 4/5. Slide for the whole story.
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
                  background: `linear-gradient(120deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                  color: theme.palette.getContrastText(theme.palette.primary.main),
                  "&:hover": { transform: "translateY(-1px) scale(1.01)" },
                  transition: "transform 200ms ease",
                }}
              >
                Seal the verdict
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

    </PageShell>
  );
}