import { useEffect, useMemo, useState, type FormEvent } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Divider,
  IconButton,
  InputAdornment,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  ArrowForwardRounded,
  AutoAwesome,
  Visibility,
  VisibilityOff,
} from "@mui/icons-material";
import { motion } from "framer-motion";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { api, ApiError, resolveImageUrl } from "../api/client";
import type { ImageItem } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { Navigation } from "../components/Navigation";
import { useToast } from "../components/Feedback";
import { PendingClaimsPrompt, usePendingCount } from "../components/PendingClaimsPrompt";
import { useStageImages } from "../theme/ThemeHost";
import { Logo } from "../components/Logo";

type Mode = "login" | "register";

export function LoginPage() {
  const { user, status, needsBootstrap, login, register } = useAuth();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const { notify } = useToast();
  const pending = usePendingCount();

  const nextParam = params.get("next");
  const next = nextParam && nextParam.startsWith("/") ? nextParam : "/rank";

  const [mode, setMode] = useState<Mode>("login");
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [showClaims, setShowClaims] = useState(false);
  const [gallery, setGallery] = useState<ImageItem[]>([]);

  const galleryUrls = useMemo(() => gallery.map((item) => item.url), [gallery]);
  useStageImages(galleryUrls);

  useEffect(() => {
    let cancelled = false;
    api
      .randomImages(6)
      .then((items) => {
        if (!cancelled) setGallery(items);
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (user && status === "ready" && !showClaims && pending === 0) {
      navigate(next, { replace: true });
    }
  }, [next, navigate, pending, showClaims, status, user]);

  if (status === "loading") {
    return (
      <Box sx={{ minHeight: "100vh", display: "grid", placeItems: "center" }}>
        <CircularProgress />
      </Box>
    );
  }

  if (needsBootstrap) {
    return <Navigate to="/" replace />;
  }

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (loading) return;
    setError("");
    if (!username.trim() || !password) {
      setError("Tell us who you are and the password.");
      return;
    }
    if (mode === "register" && password.length < 8) {
      setError("Use at least 8 characters for the password.");
      return;
    }
    setLoading(true);
    try {
      if (mode === "login") {
        await login(username.trim(), password);
        notify("Welcome back.", "success");
      } else {
        await register({
          username: username.trim(),
          password,
          displayName: displayName.trim() || undefined,
        });
        notify("Your account is ready.", "success");
      }
      if (pending > 0) setShowClaims(true);
      else navigate(next, { replace: true });
    } catch (submitError) {
      const message =
        submitError instanceof ApiError ? submitError.message : "Something went wrong.";
      setError(message);
      notify(message, "error");
    } finally {
      setLoading(false);
    }
  };

  const thumbs = gallery.slice(1, 5);

  return (
    <Box sx={{ position: "relative", minHeight: "100vh", overflow: "hidden" }}>
      <Navigation floating />

      <Box
        aria-hidden
        sx={{
          position: "fixed",
          inset: 0,
          zIndex: -1,
          overflow: "hidden",
          background: "#0d0b12",
        }}
      >
        {gallery[0] && (
          <Box
            component="img"
            src={resolveImageUrl(gallery[0].url)}
            alt=""
            sx={{
              width: "100%",
              height: "100%",
              objectFit: "cover",
              filter: "blur(2px) brightness(.45) saturate(1.1)",
              transform: "scale(1.06)",
            }}
          />
        )}
        <Box
          sx={{
            position: "absolute",
            inset: 0,
            background:
              "linear-gradient(120deg, rgba(8,7,12,.78), rgba(8,7,12,.36) 55%, rgba(8,7,12,.7))",
          }}
        />
      </Box>

      <Box
        sx={{
          minHeight: "100vh",
          display: "grid",
          placeItems: "center",
          px: 2,
          py: 12,
        }}
      >
        {thumbs.map((item, index) => (
          <motion.img
            key={item.id}
            src={resolveImageUrl(item.url)}
            alt=""
            initial={{ opacity: 0, y: 30, rotate: 0 }}
            animate={{ opacity: 0.9, y: 0, rotate: index % 2 === 0 ? -4 : 5 }}
            transition={{ delay: 0.3 + index * 0.12, duration: 0.9 }}
            className="float-slow"
            draggable={false}
            style={{
              position: "fixed",
              width: index % 2 === 0 ? 190 : 150,
              zIndex: 2,
              borderRadius: 18,
              boxShadow: "0 22px 55px rgba(0,0,0,.45)",
              display: window.innerWidth < 900 ? "none" : "block",
              top: ["16%", "62%", "14%", "68%"][index],
              left: ["6%", "4%", "82%", "86%"][index],
              animationDelay: `${index * 1.7}s`,
            }}
          />
        ))}

        <motion.div
          initial={{ opacity: 0, y: 22, scale: 0.985 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ duration: 0.7, ease: [0.2, 0.9, 0.2, 1] }}
          style={{ width: "min(460px, 100%)", position: "relative", zIndex: 5 }}
        >
          <Box className="glass-strong" sx={{ borderRadius: 6, p: { xs: 3, sm: 4.2 } }}>
            <Logo size="lg" className="display" />
            <Typography
              className="display"
              sx={{ mt: 2, fontSize: "2rem", fontWeight: 620, lineHeight: 1.05 }}
            >
              {mode === "login" ? "Welcome back." : "Join the dreaming."}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.8, mb: 2.6 }}>
              {mode === "login"
                ? "Your votes are waiting where you left them."
                : "A name, a password, and you are in."}
            </Typography>

            <ToggleButtonGroup
              exclusive
              fullWidth
              size="small"
              value={mode}
              onChange={(_event, value: Mode | null) => {
                if (value) {
                  setMode(value);
                  setError("");
                }
              }}
              sx={{
                mb: 2.6,
                "& .MuiToggleButton-root": {
                  textTransform: "none",
                  borderRadius: 99,
                  py: 0.7,
                  fontWeight: 600,
                  border: "1px solid rgba(255,255,255,.18)",
                },
              }}
            >
              <ToggleButton value="login">Sign in</ToggleButton>
              <ToggleButton value="register">Create account</ToggleButton>
            </ToggleButtonGroup>

            <Box component="form" onSubmit={submit} sx={{ display: "grid", gap: 1.6 }}>
              <TextField
                label="Username"
                size="small"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                autoFocus
                inputProps={{ spellCheck: false }}
              />
              {mode === "register" && (
                <TextField
                  label="Display name (optional)"
                  size="small"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  autoComplete="nickname"
                />
              )}
              <TextField
                label="Password"
                size="small"
                type={showPassword ? "text" : "password"}
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete={mode === "login" ? "current-password" : "new-password"}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={() => setShowPassword((value) => !value)}
                        size="small"
                        edge="end"
                        aria-label={showPassword ? "Hide password" : "Show password"}
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
                endIcon={
                  loading ? <CircularProgress size={18} color="inherit" /> : <ArrowForwardRounded />
                }
                sx={{ borderRadius: 99, py: 1.15, textTransform: "none", fontWeight: 700, mt: 0.4 }}
              >
                {loading
                  ? "One moment…"
                  : mode === "login"
                    ? "Enter"
                    : "Create my account"}
              </Button>
            </Box>

            {pending > 0 && (
              <>
                <Divider sx={{ my: 2.2 }} />
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1.2,
                    p: 1.4,
                    borderRadius: 3,
                    border: "1px solid rgba(255,255,255,.12)",
                    background: "rgba(255,255,255,.04)",
                  }}
                >
                  <AutoAwesome sx={{ opacity: 0.7 }} />
                  <Typography variant="body2" sx={{ flex: 1, opacity: 0.8 }}>
                    {pending} anonymous {pending === 1 ? "submission is" : "submissions are"} saved
                    on this device.
                  </Typography>
                  <Tooltip title="Only after signing in">
                    <span>
                      <Button size="small" disabled={!user} onClick={() => setShowClaims(true)}>
                        Attach
                      </Button>
                    </span>
                  </Tooltip>
                </Box>
              </>
            )}

            <Box sx={{ mt: 3, textAlign: "center" }}>
              <Button
                size="small"
                onClick={() => navigate("/")}
                sx={{ color: "text.secondary", textTransform: "none" }}
              >
                ← back to the dream
              </Button>
            </Box>
          </Box>
        </motion.div>
      </Box>

      <PendingClaimsPrompt
        open={showClaims && pending > 0}
        onClose={() => {
          setShowClaims(false);
          navigate(next, { replace: true });
        }}
        onClaimed={() => setShowClaims(false)}
      />
    </Box>
  );
}
