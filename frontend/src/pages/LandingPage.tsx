import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  InputAdornment,
  Menu,
  MenuItem,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  ArrowForwardRounded,
  ChevronLeftRounded,
  ChevronRightRounded,
  CloudUploadOutlined,
  HighQualityRounded,
  LoginRounded,
  Visibility,
  VisibilityOff,
} from "@mui/icons-material";
import { AnimatePresence, motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { api, ApiError, imageVariant } from "../api/client";
import type { ImageItem, TimeRange } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { Logo } from "../components/Logo";
import { Navigation } from "../components/Navigation";
import { PeriodPicker } from "../components/PeriodPicker";
import { useToast } from "../components/Feedback";
import { useStageImages } from "../theme/ThemeHost";
import { periodLabel } from "../utils/format";
import { useFrontendConfig } from "../config/FrontendConfig";

const SLIDE_INTERVAL_MS = 6500;

type HomeQuality = "720" | "1080" | "1440" | "original";
const HOME_QUALITY_KEY = "artdream.homeQuality";
const HOME_QUALITIES: Array<{ value: HomeQuality; label: string }> = [
  { value: "720", label: "720p · light" },
  { value: "1080", label: "1080p · default" },
  { value: "1440", label: "1440p · sharper" },
  { value: "original", label: "Original · largest" },
];

function readHomeQuality(): HomeQuality {
  try {
    const stored = localStorage.getItem(HOME_QUALITY_KEY);
    if (stored === "720" || stored === "1080" || stored === "1440" || stored === "original") {
      return stored;
    }
  } catch {
    // ignore storage restrictions
  }
  return "1080";
}

export function LandingPage() {
  const { user, needsBootstrap, status, bootstrap } = useAuth();
  const navigate = useNavigate();
  const { notify } = useToast();
  const { config: frontendConfig, loading: frontendConfigLoading } = useFrontendConfig();
  const rankingAnonymized = frontendConfig.rankingAnonymized;
  const [period, setPeriod] = useState<TimeRange>("day");
  const [slides, setSlides] = useState<ImageItem[]>([]);
  const [index, setIndex] = useState(0);
  const [loadingSlides, setLoadingSlides] = useState(true);
  const [reloadToken, setReloadToken] = useState(0);
  const [quality, setQuality] = useState<HomeQuality>(readHomeQuality);
  const [qualityMenu, setQualityMenu] = useState<HTMLElement | null>(null);

  const needsSetup = status === "ready" && needsBootstrap;
  const slideUrls = useMemo(() => slides.map((slide) => slide.url), [slides]);
  useStageImages(slideUrls);

  useEffect(() => {
    let cancelled = false;
    setLoadingSlides(true);
    (async () => {
      try {
        const ranking = await api.rank({ time: period, amount: 8 });
        let images = ranking.winners.map((winner) => winner.image);
        if (images.length === 0) {
          images = await api.randomImages(8);
        }
        if (!cancelled) {
          setSlides(images);
          setIndex(0);
        }
      } catch {
        if (!cancelled) setSlides([]);
      } finally {
        if (!cancelled) setLoadingSlides(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [period, reloadToken]);

  useEffect(() => {
    if (slides.length < 2) return;
    const timer = window.setTimeout(() => {
      setIndex((previous) => (previous + 1) % slides.length);
    }, SLIDE_INTERVAL_MS);
    return () => window.clearTimeout(timer);
  }, [index, slides]);

  const goToSlide = useCallback(
    (delta: number) => {
      if (slides.length < 2) return;
      setIndex((previous) => (previous + delta + slides.length) % slides.length);
    },
    [slides.length],
  );

  useEffect(() => {
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
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
        goToSlide(-1);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        goToSlide(1);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [goToSlide]);

  const current = slides[index];
  const currentAuthorHidden = frontendConfigLoading || rankingAnonymized;
  const backgroundSrc = current
    ? quality === "original"
      ? imageVariant(current.url, { original: true })
      : imageVariant(current.url, {
          h: Number(quality),
          quality: quality === "720" ? 82 : 85,
        })
    : "";
  const chooseQuality = (next: HomeQuality) => {
    setQuality(next);
    setQualityMenu(null);
    try {
      localStorage.setItem(HOME_QUALITY_KEY, next);
    } catch {
      // ignore storage restrictions
    }
  };

  const start = () => {
    navigate("/rank");
  };

  return (
    <Box sx={{ position: "relative", minHeight: "100vh", overflow: "hidden" }}>
      <Navigation floating />

      <Box className="landing-background" aria-hidden>
        <AnimatePresence initial={false}>
          {current && (
            <motion.div
              key={`${current.id}-${index}`}
              className="slide"
              initial={{ opacity: 0, scale: 1.04 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: 1.5, ease: [0.22, 0.9, 0.24, 1] }}
            >
              <img
                src={backgroundSrc}
                alt=""
                className="ken-burns"
                draggable={false}
                decoding="async"
                fetchPriority="high"
              />
            </motion.div>
          )}
        </AnimatePresence>
        {!current && (
          <Box
            sx={{
              position: "absolute",
              inset: 0,
              background:
                "radial-gradient(900px 600px at 20% 20%, rgba(143,188,187,.25), transparent 60%), radial-gradient(800px 700px at 85% 30%, rgba(180,142,173,.22), transparent 65%), #0d0b12",
            }}
          />
        )}
      </Box>

      {needsSetup ? (
        <BootstrapPanel
          onSubmit={async (values) => {
            try {
              await bootstrap(values);
              notify("The first light is yours. You are the administrator.", "success");
            } catch (error) {
              notify(
                error instanceof ApiError ? error.message : "Could not create the account.",
                "error",
              );
              throw error;
            }
          }}
        />
      ) : (
        <Box
          sx={{
            position: "relative",
            zIndex: 10,
            minHeight: "100vh",
            display: "grid",
            placeItems: "center",
            px: 3,
            pb: { xs: 12, md: 10 },
          }}
        >
          <Box sx={{ textAlign: "center", maxWidth: 860 }}>
            <motion.div
              initial={{ y: 26 }}
              animate={{ y: 0 }}
              transition={{ duration: 1, ease: [0.2, 0.9, 0.2, 1] }}
            >
              <Box
                className="display brand-glow"
                sx={{
                  fontSize: "clamp(3.4rem, 13vw, 10.5rem)",
                  lineHeight: 0.88,
                  fontWeight: 600,
                  letterSpacing: "-0.055em",
                  mb: 2,
                }}
              >
                ArtDream
              </Box>
              <Typography
                sx={{
                  fontSize: "clamp(1rem, 2.1vw, 1.35rem)",
                  color: "rgba(255,255,255,.76)",
                  letterSpacing: "0.02em",
                  mb: 1,
                }}
              >
                Art for TV-Dreams.
              </Typography>
              <Typography
                sx={{
                  fontSize: "0.8rem",
                  color: "rgba(255,255,255,.5)",
                  letterSpacing: "0.16em",
                  textTransform: "uppercase",
                  mb: { xs: 4, md: 6 },
                }}
              >
                {slides.length > 0
                  ? `${periodLabel(period)}'s favourites`
                  : "every picture has a rival"}
              </Typography>

              <Box
                sx={{
                  display: "flex",
                  gap: 1.4,
                  justifyContent: "center",
                  flexWrap: "wrap",
                }}
              >
                <Button
                  onClick={start}
                  variant="contained"
                  size="large"
                  className="hero-cta pulse-glow"
                  endIcon={<ArrowForwardRounded />}
                  sx={{
                    px: 4.5,
                    py: 1.6,
                    borderRadius: 99,
                    fontSize: "1.02rem",
                    fontWeight: 750,
                    textTransform: "none",
                    boxShadow: "0 22px 60px rgba(0,0,0,.45)",
                    "&:hover": {
                      transform: "translateY(-2px) scale(1.015)",
                      boxShadow: "0 28px 70px rgba(0,0,0,.5)",
                    },
                  }}
                >
                  Rank images
                </Button>
                <Button
                  onClick={() => navigate("/submit")}
                  size="large"
                  startIcon={<CloudUploadOutlined />}
                  sx={{
                    px: 3,
                    py: 1.6,
                    borderRadius: 99,
                    textTransform: "none",
                    fontWeight: 600,
                    color: "#fff",
                    borderColor: "rgba(255,255,255,.32)",
                    "&:hover": {
                      borderColor: "rgba(255,255,255,.7)",
                      background: "rgba(255,255,255,.09)",
                    },
                  }}
                  variant="outlined"
                >
                  Submit art
                </Button>
              </Box>

              {!user && !needsSetup && (
                <Box sx={{ mt: 2.6, display: "flex", gap: 2, justifyContent: "center" }}>
                  <Button
                    onClick={() => navigate("/login?next=/rank")}
                    size="small"
                    startIcon={<LoginRounded sx={{ fontSize: 16 }} />}
                    sx={{
                      color: "rgba(255,255,255,.65)",
                      textTransform: "none",
                      "&:hover": { color: "#fff" },
                    }}
                  >
                    Sign in to keep your votes
                  </Button>
                </Box>
              )}
            </motion.div>
          </Box>
        </Box>
      )}

      {current && (
        <Box
          sx={{
            position: "fixed",
            left: { xs: 16, md: 32 },
            bottom: { xs: 88, md: 32 },
            zIndex: 20,
            display: "flex",
            alignItems: "center",
            gap: 1.4,
            width: { xs: "calc(100vw - 32px)", sm: "auto" },
            minWidth: { sm: 300 },
            maxWidth: { xs: "calc(100vw - 32px)", sm: "min(560px, 62vw)" },
            px: 1.8,
            py: 1.1,
            borderRadius: 99,
            background: "rgba(10,9,15,.62)",
            border: "1px solid rgba(255,255,255,.12)",
          }}
        >
          <Box
            sx={{
              flexShrink: 0,
              fontSize: "0.68rem",
              letterSpacing: "0.08em",
              textTransform: "uppercase",
              color: "rgba(255,255,255,.52)",
            }}
          >
            {index + 1}/{slides.length}
          </Box>
          <Box
            sx={{
              width: "1px",
              flexShrink: 0,
              height: 16,
              background: "rgba(255,255,255,.18)",
            }}
          />
          {(current.title || (!currentAuthorHidden && current.author)) && (
            <Box sx={{ minWidth: 0, flex: 1 }}>
              {current.title && (
                <Typography
                  noWrap
                  sx={{ fontSize: "0.94rem", fontWeight: 650, lineHeight: 1.25 }}
                >
                  {current.title}
                </Typography>
              )}
              {!currentAuthorHidden && current.author && (
                <Typography noWrap sx={{ fontSize: "0.78rem", color: "rgba(255,255,255,.62)" }}>
                  by {current.author}
                </Typography>
              )}
            </Box>
          )}
          {slides.length > 1 && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.1, flexShrink: 0 }}>
              <Tooltip title="Previous artwork">
                <IconButton
                  size="small"
                  aria-label="Previous artwork"
                  onClick={() => goToSlide(-1)}
                  sx={{
                    color: "rgba(255,255,255,.74)",
                    "&:hover": { background: "rgba(255,255,255,.1)", color: "#fff" },
                  }}
                >
                  <ChevronLeftRounded fontSize="small" />
                </IconButton>
              </Tooltip>
              <Tooltip title="Next artwork">
                <IconButton
                  size="small"
                  aria-label="Next artwork"
                  onClick={() => goToSlide(1)}
                  sx={{
                    color: "rgba(255,255,255,.74)",
                    "&:hover": { background: "rgba(255,255,255,.1)", color: "#fff" },
                  }}
                >
                  <ChevronRightRounded fontSize="small" />
                </IconButton>
              </Tooltip>
            </Box>
          )}
          {!loadingSlides && slides.length === 0 && (
            <Button size="small" onClick={() => setReloadToken((n) => n + 1)}>
              retry
            </Button>
          )}
        </Box>
      )}

      {slides.length > 1 && (
        <Box
          aria-hidden
          sx={{
            position: "fixed",
            left: 0,
            right: 0,
            bottom: 0,
            height: 3,
            zIndex: 35,
            pointerEvents: "none",
            background: "rgba(255,255,255,.16)",
          }}
        >
          <Box
            key={`${period}-${index}-${current?.id ?? ""}`}
            className="slide-progress-fill"
            sx={{ animationDuration: `${SLIDE_INTERVAL_MS}ms` }}
          />
        </Box>
      )}

      <PeriodPicker
        value={period}
        onChange={setPeriod}
        leading={
          <>
            <Tooltip
              title={`Backdrop resolution — ${quality === "original" ? "original" : `${quality}p`}`}
              enterDelay={400}
            >
              <Button
                size="small"
                onClick={(event) => setQualityMenu(event.currentTarget)}
                startIcon={<HighQualityRounded sx={{ fontSize: 16 }} />}
                sx={{
                  minWidth: 0,
                  px: 1.2,
                  py: 0.25,
                  borderRadius: 99,
                  textTransform: "none",
                  fontSize: "0.7rem",
                  fontWeight: 600,
                  color: "rgba(255,255,255,.8)",
                  background: "rgba(10,9,15,.42)",
                  border: "1px solid rgba(255,255,255,.14)",
                  "& .MuiButton-startIcon": { mr: 0.5 },
                  "&:hover": { background: "rgba(255,255,255,.1)", color: "#fff" },
                }}
              >
                {quality === "original" ? "Orig" : `${quality}p`}
              </Button>
            </Tooltip>
            <Menu
              anchorEl={qualityMenu}
              open={Boolean(qualityMenu)}
              onClose={() => setQualityMenu(null)}
              anchorOrigin={{ vertical: "top", horizontal: "center" }}
              transformOrigin={{ vertical: "bottom", horizontal: "center" }}
            >
              {HOME_QUALITIES.map((option) => (
                <MenuItem
                  key={option.value}
                  selected={quality === option.value}
                  onClick={() => chooseQuality(option.value)}
                >
                  {option.label}
                </MenuItem>
              ))}
            </Menu>
          </>
        }
      />

      {loadingSlides && (
        <Box
          sx={{
            position: "fixed",
            bottom: 24,
            left: "50%",
            transform: "translateX(-50%)",
            zIndex: 25,
            opacity: 0.6,
          }}
        >
          <CircularProgress size={18} thickness={4} />
        </Box>
      )}
    </Box>
  );
}

function BootstrapPanel({
  onSubmit,
}: {
  onSubmit: (values: { username: string; password: string; displayName?: string }) => Promise<void>;
}) {
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const { notify } = useToast();

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (loading) return;
    if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$/.test(username)) {
      setError("3-32 characters, letters, digits, _ . -");
      return;
    }
    if (password.length < 8) {
      setError("Give the password at least 8 characters.");
      return;
    }
    setError("");
    setLoading(true);
    try {
      await onSubmit({ username, password, displayName: displayName || undefined });
    } catch (submitError) {
      if (!(submitError instanceof ApiError)) {
        notify("Could not create the account.", "error");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box
      sx={{
        position: "relative",
        zIndex: 10,
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        px: 2,
        py: 12,
      }}
    >
      <motion.div
        initial={{ opacity: 0, y: 24, scale: 0.98 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.8, ease: [0.2, 0.9, 0.2, 1] }}
        style={{ width: "min(480px, 100%)" }}
      >
        <Box className="glass-strong" sx={{ borderRadius: 6, p: { xs: 3, sm: 4.5 } }}>
          <Logo size="lg" to={undefined} className="display" />
          <Typography
            className="display"
            sx={{ mt: 2, fontSize: "1.9rem", fontWeight: 600, lineHeight: 1.1 }}
          >
            The first light.
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.8, mb: 3 }}>
            Create the first account. It becomes the administrator of this ArtDream.
          </Typography>

          <Box component="form" onSubmit={submit} sx={{ display: "grid", gap: 1.7 }}>
            <TextField
              label="Username"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoFocus
              autoComplete="username"
              size="small"
              inputProps={{ spellCheck: false }}
            />
            <TextField
              label="Display name (optional)"
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              size="small"
              autoComplete="nickname"
            />
            <TextField
              label="Password"
              type={showPassword ? "text" : "password"}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="new-password"
              size="small"
              helperText="At least 8 characters."
              InputProps={{
                endAdornment: (
                  <InputAdornment position="end">
                    <IconButton
                      aria-label={showPassword ? "Hide password" : "Show password"}
                      onClick={() => setShowPassword((value) => !value)}
                      edge="end"
                      size="small"
                    >
                      {showPassword ? <VisibilityOff fontSize="small" /> : <Visibility fontSize="small" />}
                    </IconButton>
                  </InputAdornment>
                ),
              }}
            />
            {error && (
              <Alert severity="warning" sx={{ borderRadius: 3 }}>
                {error}
              </Alert>
            )}
            <Button
              type="submit"
              variant="contained"
              size="large"
              disabled={loading}
              disableElevation
              endIcon={loading ? <CircularProgress size={18} color="inherit" /> : <ArrowForwardRounded />}
              sx={{ mt: 0.6, borderRadius: 99, py: 1.2, textTransform: "none", fontWeight: 700 }}
            >
              {loading ? "Lighting up…" : "Begin"}
            </Button>
          </Box>

          <Box sx={{ mt: 2.4, display: "flex", justifyContent: "center" }}>
            <Tooltip title="Only the very first account gets admin rights.">
              <Typography className="helper-text">Only the first account is admin.</Typography>
            </Tooltip>
          </Box>
        </Box>
      </motion.div>
    </Box>
  );
}