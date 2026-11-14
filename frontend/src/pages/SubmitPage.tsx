import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  TextField,
  Tooltip,
  Typography,
  alpha,
  useTheme,
} from "@mui/material";
import {
  AddPhotoAlternateRounded,
  AutoAwesomeRounded,
  CheckCircleRounded,
  DeleteOutlineRounded,
  ImageSearchRounded,
  LinkRounded,
  SwapHorizRounded,
} from "@mui/icons-material";
import { AnimatePresence, motion } from "framer-motion";
import { Navigate, useNavigate } from "react-router-dom";
import { api, ApiError, imageVariant } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { PageShell, PageHeader } from "../components/PageShell";
import { useToast } from "../components/Feedback";
import { PendingClaimsPrompt } from "../components/PendingClaimsPrompt";
import { useStageImages } from "../theme/ThemeHost";
import {
  PENDING_SUBMISSIONS_EVENT,
  addPendingSubmission,
  emitPendingChange,
  loadPendingSubmissions,
  removePendingSubmission,
  type PendingSubmission,
} from "../utils/pendingSubmissions";
import { relativeTime } from "../utils/format";

const MAX_BYTES = 30 * 1024 * 1024;
const ACCEPTED_TYPES = ["image/jpeg", "image/png", "image/webp", "image/gif", "image/bmp", "image/tiff"];

interface Preview {
  file: File;
  url: string;
  width: number;
  height: number;
}

export function SubmitPage() {
  const { user, status, needsBootstrap } = useAuth();
  const { notify } = useToast();
  const navigate = useNavigate();
  const theme = useTheme();
  const fileInput = useRef<HTMLInputElement | null>(null);

  const [preview, setPreview] = useState<Preview | null>(null);
  const [author, setAuthor] = useState("");
  const [authorTouched, setAuthorTouched] = useState(false);
  const [title, setTitle] = useState("");
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const [uploaded, setUploaded] = useState<{ url: string; title: string; author: string; anonymous: boolean } | null>(null);
  const [pending, setPending] = useState<PendingSubmission[]>(() => loadPendingSubmissions());
  const [showClaims, setShowClaims] = useState(false);

  const stageUrls = useMemo(() => {
    const urls: string[] = [];
    if (preview) urls.push(preview.url);
    if (uploaded) urls.push(uploaded.url);
    urls.push(...pending.slice(0, 4).map((item) => item.url));
    return urls;
  }, [pending, preview, uploaded]);
  useStageImages(stageUrls);

  useEffect(() => {
    if (user && !authorTouched) {
      setAuthor(user.displayName || user.username);
    }
  }, [authorTouched, user]);

  useEffect(() => {
    const refresh = () => setPending(loadPendingSubmissions());
    window.addEventListener(PENDING_SUBMISSIONS_EVENT, refresh);
    window.addEventListener("storage", refresh);
    return () => {
      window.removeEventListener(PENDING_SUBMISSIONS_EVENT, refresh);
      window.removeEventListener("storage", refresh);
    };
  }, []);

  useEffect(() => {
    return () => {
      if (preview) URL.revokeObjectURL(preview.url);
    };
  }, [preview]);

  const validateAndSetFile = useCallback(
    (file: File) => {
      setError("");
      if (!ACCEPTED_TYPES.includes(file.type)) {
        setError("Use a JPEG, PNG, WebP, GIF, BMP or TIFF image.");
        return;
      }
      if (file.size > MAX_BYTES) {
        setError("That image is larger than 30 MiB.");
        return;
      }
      const url = URL.createObjectURL(file);
      const image = new Image();
      image.onload = () => {
        if (image.naturalWidth <= image.naturalHeight) {
          setError("Landscape only for now — the width has to be greater than the height.");
          URL.revokeObjectURL(url);
          return;
        }
        setPreview((previous) => {
          if (previous) URL.revokeObjectURL(previous.url);
          return { file, url, width: image.naturalWidth, height: image.naturalHeight };
        });
        setUploaded(null);
      };
      image.onerror = () => {
        URL.revokeObjectURL(url);
        setError("That image could not be read. Try a different file.");
      };
      image.src = url;
    },
    [],
  );

  const handleDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragging(false);
    const file = event.dataTransfer.files?.[0];
    if (file) validateAndSetFile(file);
  };

  const submit = async () => {
    if (uploading) return;
    if (!preview) {
      setError("Pick a landscape image first.");
      return;
    }
    if (!author.trim()) {
      setError("Tell us the artist name to show.");
      return;
    }
    setError("");
    setUploading(true);
    try {
      const form = new FormData();
      form.append("image", preview.file);
      form.append("author", author.trim());
      if (title.trim()) form.append("title", title.trim());
      const response = await api.uploadImage(form);
      // A claim token is issued for every anonymous upload and also when a
      // stale token silently degrades to an anonymous upload. Remember it so
      // the submission can be attached later.
      const anonymous = Boolean(response.claimToken);
      if (response.claimToken) {
        const next = addPendingSubmission({
          id: response.image.id,
          claimToken: response.claimToken,
          author: response.image.author,
          title: response.image.title,
          url: response.image.url,
          width: response.image.width,
          height: response.image.height,
          createdAt: response.image.createdAt,
        });
        setPending(next);
        emitPendingChange();
      }
      setUploaded({
        url: response.image.url,
        title: response.image.title || "Untitled dream",
        author: response.image.author,
        anonymous,
      });
      if (preview) URL.revokeObjectURL(preview.url);
      setPreview(null);
      setTitle("");
      setAuthorTouched(false);
      if (user) setAuthor(user.displayName || user.username);
      else setAuthor("");
      notify(anonymous ? "Saved on this device — create an account to keep it." : "Submission received.", "success");
    } catch (uploadError) {
      const message =
        uploadError instanceof ApiError ? uploadError.message : "The image could not be uploaded.";
      setError(message);
      notify(message, "error");
    } finally {
      setUploading(false);
    }
  };

  if (status === "ready" && needsBootstrap) {
    return <Navigate to="/" replace />;
  }

  return (
    <PageShell maxWidth="lg">
      <PageHeader
        eyebrow="submit"
        title="Send a dream into the ring."
        description={
          <>
            Landscape images only for now. Pick the artist name freely — it does not have to
            match your account.
          </>
        }
        action={
          user ? (
            <Chip
              avatar={<Avatar>{(user.displayName || user.username).slice(0, 1).toUpperCase()}</Avatar>}
              label={`linked to @${user.username}`}
              sx={{ background: "rgba(255,255,255,.08)" }}
            />
          ) : (
            <Button
              variant="outlined"
              size="small"
              startIcon={<LinkRounded />}
              onClick={() => navigate("/login?next=/submit")}
              sx={{ borderRadius: 99, textTransform: "none" }}
            >
              Sign in to link
            </Button>
          )
        }
      />

      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr", md: "minmax(0, 1.15fr) minmax(0, .85fr)" },
          gap: { xs: 2.5, md: 3.5 },
          alignItems: "start",
        }}
      >
        <Box>
          <Box
            className={`submit-drop ${dragging ? "dragging" : ""}`}
            onDragOver={(event) => {
              event.preventDefault();
              setDragging(true);
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={handleDrop}
            onClick={() => {
              if (!preview) fileInput.current?.click();
            }}
            sx={{
              position: "relative",
              overflow: "hidden",
              minHeight: { xs: 260, md: 380 },
              display: "grid",
              placeItems: "center",
              cursor: preview ? "default" : "pointer",
              background: preview ? "rgba(0,0,0,.3)" : "rgba(255,255,255,.035)",
            }}
          >
            <input
              ref={fileInput}
              type="file"
              accept={ACCEPTED_TYPES.join(",")}
              hidden
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) validateAndSetFile(file);
                event.target.value = "";
              }}
            />
            <AnimatePresence mode="wait">
              {preview ? (
                <motion.div
                  key={preview.url}
                  initial={{ opacity: 0, scale: 1.02 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.45 }}
                  style={{ position: "absolute", inset: 0 }}
                >
                  <img
                    src={preview.url}
                    alt="Your submission preview"
                    style={{ width: "100%", height: "100%", objectFit: "cover" }}
                  />
                  <Box
                    sx={{
                      position: "absolute",
                      inset: 0,
                      background: "linear-gradient(180deg, transparent 55%, rgba(0,0,0,.55))",
                    }}
                  />
                  <Box
                    sx={{
                      position: "absolute",
                      left: 16,
                      bottom: 16,
                      display: "flex",
                      gap: 1,
                      flexWrap: "wrap",
                    }}
                  >
                    <Chip
                      size="small"
                      label={`${preview.width} × ${preview.height} · landscape ✓`}
                      color="success"
                      variant="outlined"
                      sx={{ background: "rgba(0,0,0,.4)" }}
                    />
                    <Button
                      size="small"
                      variant="contained"
                      startIcon={<SwapHorizRounded />}
                      onClick={() => fileInput.current?.click()}
                      sx={{ borderRadius: 99, textTransform: "none" }}
                    >
                      Change
                    </Button>
                  </Box>
                </motion.div>
              ) : (
                <motion.div
                  key="empty"
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  style={{ textAlign: "center", padding: 24 }}
                >
                  <Box
                    sx={{
                      width: 78,
                      height: 78,
                      borderRadius: "36% 64% 55% 45% / 55% 45% 55% 45%",
                      display: "grid",
                      placeItems: "center",
                      mx: "auto",
                      mb: 2,
                      background: `linear-gradient(135deg, ${alpha(theme.palette.primary.main, 0.5)}, ${alpha(theme.palette.secondary.main, 0.45)})`,
                      animation: "bob 5s ease-in-out infinite",
                    }}
                  >
                    <AddPhotoAlternateRounded sx={{ fontSize: 38 }} />
                  </Box>
                  <Typography className="display" sx={{ fontSize: "1.7rem", fontWeight: 620 }}>
                    Drop a landscape image
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.8 }}>
                    or click to browse · JPEG, PNG, WebP · max 30 MiB
                  </Typography>
                </motion.div>
              )}
            </AnimatePresence>
          </Box>

          <AnimatePresence mode="wait">
            {uploaded && (
              <motion.div
                key="uploaded"
                initial={{ opacity: 0, y: 16, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: -10 }}
                transition={{ type: "spring", stiffness: 220, damping: 24 }}
                style={{ marginTop: 20 }}
              >
                <Box
                  className="glass"
                  sx={{
                    borderRadius: 5,
                    p: 2.2,
                    display: "flex",
                    gap: 2,
                    alignItems: "center",
                    flexWrap: "wrap",
                  }}
                >
                  <Box
                    component="img"
                    src={imageVariant(uploaded.url, { w: 360 })}
                    alt={uploaded.title}
                    sx={{ width: 104, height: 74, borderRadius: 2.5, objectFit: "cover" }}
                  />
                  <Box sx={{ flex: 1, minWidth: 180 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                      <CheckCircleRounded fontSize="small" color="success" />
                      <Typography sx={{ fontWeight: 700 }}>
                        {uploaded.anonymous ? "Saved on this device" : "Submission received"}
                      </Typography>
                    </Box>
                    <Typography variant="body2" color="text.secondary">
                      {uploaded.title} · by {uploaded.author}
                    </Typography>
                    {uploaded.anonymous && (
                      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.3, opacity: 0.8 }}>
                        Create an account to link it later — your artist name stays the same.
                      </Typography>
                    )}
                  </Box>
                  <Box sx={{ display: "flex", gap: 1 }}>
                    <Button
                      size="small"
                      variant="contained"
                      onClick={() => navigate("/rank")}
                      endIcon={<AutoAwesomeRounded />}
                      sx={{ borderRadius: 99, textTransform: "none" }}
                    >
                      Rank
                    </Button>
                    <Button
                      size="small"
                      onClick={() => {
                        setUploaded(null);
                        fileInput.current?.click();
                      }}
                      sx={{ textTransform: "none" }}
                    >
                      Another
                    </Button>
                  </Box>
                </Box>
              </motion.div>
            )}
          </AnimatePresence>
        </Box>

        <Box className="glass" sx={{ borderRadius: 5, p: { xs: 2.4, sm: 3 } }}>
          <Typography className="display" sx={{ fontSize: "1.5rem", fontWeight: 620, mb: 0.5 }}>
            The little card
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2.4 }}>
            Only the artist name is required. Title can stay empty.
          </Typography>

          <Box
            component="form"
            onSubmit={(event) => {
              event.preventDefault();
              void submit();
            }}
            sx={{ display: "grid", gap: 1.7 }}
          >
            <TextField
              label="Artist / username"
              value={author}
              onChange={(event) => {
                setAuthorTouched(true);
                setAuthor(event.target.value);
              }}
              required
              size="small"
              helperText={
                user
                  ? `Prefilled from your account, but you can show any name. Account id ${user.id} is stored separately.`
                  : "Shown publicly with the image. Saved locally until you create an account."
              }
              inputProps={{ maxLength: 120 }}
            />
            <TextField
              label="Title (optional)"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              size="small"
              inputProps={{ maxLength: 200 }}
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
              disabled={uploading || !preview}
              disableElevation
              startIcon={
                uploading ? (
                  <CircularProgress size={18} color="inherit" />
                ) : (
                  <ImageSearchRounded />
                )
              }
              sx={{
                borderRadius: 99,
                py: 1.1,
                textTransform: "none",
                fontWeight: 700,
                background: `linear-gradient(120deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                color: theme.palette.getContrastText(theme.palette.primary.main),
              }}
            >
              {uploading ? "Sending…" : "Submit image"}
            </Button>
          </Box>

          <Divider sx={{ my: 2.4, opacity: 0.4 }} />

          <Box sx={{ display: "flex", gap: 1.2 }}>
            <AutoAwesomeRounded sx={{ opacity: 0.6, mt: 0.2 }} />
            <Typography variant="body2" color="text.secondary" sx={{ opacity: 0.86 }}>
              {user
                ? `This submission will be linked to your account id (${user.id}) — separate from the artist name.`
                : "No account? No problem. The image is stored and this device remembers how to attach it later."}
            </Typography>
          </Box>
        </Box>
      </Box>

      {(pending.length > 0 || user) && (
        <Box sx={{ mt: { xs: 4, md: 5 } }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: 2,
              flexWrap: "wrap",
              mb: 2,
            }}
          >
            <Box>
              <Typography className="display" sx={{ fontSize: "1.65rem", fontWeight: 620 }}>
                On this device
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Anonymous submissions remembered here. Connect them whenever you like.
              </Typography>
            </Box>
            {user && pending.length > 0 && (
              <Button
                variant="contained"
                size="small"
                startIcon={<LinkRounded />}
                onClick={() => setShowClaims(true)}
                sx={{ borderRadius: 99, textTransform: "none" }}
              >
                Attach {pending.length} to @{user.username}
              </Button>
            )}
          </Box>

          {pending.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ opacity: 0.7, mb: 4 }}>
              Nothing waiting on this device right now.
            </Typography>
          ) : (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", lg: "repeat(3, 1fr)" },
                gap: 2,
              }}
            >
              {pending.map((item) => (
                <Box
                  key={item.id}
                  className="glass tilt-card"
                  sx={{ borderRadius: 4, p: 1.5, display: "flex", gap: 1.4, alignItems: "center" }}
                >
                  <Box
                    component="img"
                    src={imageVariant(item.url, { w: 280 })}
                    alt={item.title || `By ${item.author}`}
                    sx={{ width: 92, height: 66, borderRadius: 2, objectFit: "cover", flexShrink: 0 }}
                  />
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography noWrap sx={{ fontWeight: 700, fontSize: "0.95rem" }}>
                      {item.title || "Untitled dream"}
                    </Typography>
                    <Typography noWrap variant="body2" color="text.secondary">
                      {item.author} · {relativeTime(item.createdAt)}
                    </Typography>
                    {user && (
                      <Chip
                        size="small"
                        label="not linked"
                        variant="outlined"
                        sx={{ mt: 0.7, height: 20, fontSize: "0.68rem" }}
                      />
                    )}
                  </Box>
                  <Tooltip title="Forget this local claim (image stays online)">
                    <IconButton
                      size="small"
                      onClick={() => {
                        setPending(removePendingSubmission(item.id));
                        emitPendingChange();
                        notify("Removed from this device.", "info");
                      }}
                    >
                      <DeleteOutlineRounded fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </Box>
              ))}
            </Box>
          )}
        </Box>
      )}

      <PendingClaimsPrompt
        open={showClaims && pending.length > 0}
        onClose={() => {
          setShowClaims(false);
          setPending(loadPendingSubmissions());
        }}
        onClaimed={() => {
          setPending(loadPendingSubmissions());
          setShowClaims(false);
        }}
      />
    </PageShell>
  );
}