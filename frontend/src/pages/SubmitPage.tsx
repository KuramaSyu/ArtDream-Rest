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
  LinearProgress,
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
const MAX_FILES = 12;
const MAX_TOTAL_BYTES = 200 * 1024 * 1024;
const UPLOAD_CONCURRENCY = 3;
const ACCEPTED_TYPES = [
  "image/jpeg",
  "image/png",
  "image/webp",
  "image/gif",
  "image/bmp",
  "image/tiff",
];

interface Preview {
  key: string;
  file: File;
  url: string;
  width: number;
  height: number;
  title: string;
}

interface UploadedItem {
  id: string;
  url: string;
  title: string;
  author: string;
  anonymous: boolean;
  localUrl?: string;
}

interface UploadProgress {
  done: number;
  total: number;
}

function newPreviewKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function readImageFile(file: File): Promise<{ url: string; width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const image = new Image();
    image.onload = () => {
      if (image.naturalWidth <= image.naturalHeight) {
        URL.revokeObjectURL(url);
        reject(new Error("landscape"));
        return;
      }
      resolve({ url, width: image.naturalWidth, height: image.naturalHeight });
    };
    image.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("unreadable"));
    };
    image.src = url;
  });
}

export function SubmitPage() {
  const { user, status, needsBootstrap } = useAuth();
  const { notify } = useToast();
  const navigate = useNavigate();
  const theme = useTheme();
  const fileInput = useRef<HTMLInputElement | null>(null);

  const [previews, setPreviews] = useState<Preview[]>([]);
  const [author, setAuthor] = useState("");
  const [authorTouched, setAuthorTouched] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [adding, setAdding] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [progress, setProgress] = useState<UploadProgress>({ done: 0, total: 0 });
  const [error, setError] = useState("");
  const [uploaded, setUploaded] = useState<UploadedItem[]>([]);
  const [pending, setPending] = useState<PendingSubmission[]>(() => loadPendingSubmissions());
  const [showClaims, setShowClaims] = useState(false);

  const previewsRef = useRef<Preview[]>(previews);
  useEffect(() => {
    previewsRef.current = previews;
  }, [previews]);

  const uploadedRef = useRef<UploadedItem[]>(uploaded);
  useEffect(() => {
    uploadedRef.current = uploaded;
  }, [uploaded]);

  const stageUrls = useMemo(() => {
    const urls: string[] = previews.map((preview) => preview.url);
    urls.push(...uploaded.slice(0, 6).map((item) => item.localUrl ?? item.url));
    urls.push(...pending.slice(0, 4).map((item) => item.url));
    return urls;
  }, [pending, previews, uploaded]);
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

  // Revoke object URLs when previews leave the page; individual removals revoke
  // explicitly, so this effect only handles unmount.
  useEffect(() => {
    return () => {
      previewsRef.current.forEach((preview) => URL.revokeObjectURL(preview.url));
      uploadedRef.current.forEach((item) => {
        if (item.localUrl) URL.revokeObjectURL(item.localUrl);
      });
    };
  }, []);

  const clearUploaded = useCallback(() => {
    setUploaded((previous) => {
      previous.forEach((item) => {
        if (item.localUrl) URL.revokeObjectURL(item.localUrl);
      });
      return [];
    });
  }, []);

  const updateTitle = (key: string, title: string) => {
    setPreviews((previous) =>
      previous.map((preview) => (preview.key === key ? { ...preview, title } : preview)),
    );
  };

  const removePreview = (key: string) => {
    setPreviews((previous) => {
      const target = previous.find((preview) => preview.key === key);
      if (target) URL.revokeObjectURL(target.url);
      return previous.filter((preview) => preview.key !== key);
    });
  };

  const addFiles = useCallback(
    async (fileList: FileList | File[]) => {
      if (adding || uploading) return;
      const files = Array.from(fileList);
      if (files.length === 0) return;

      const current = previewsRef.current;
      const slots = Math.max(0, MAX_FILES - current.length);
      if (slots === 0) {
        notify(`You can upload up to ${MAX_FILES} images at once.`, "warning");
        return;
      }

      const existing = new Set(
        current.map((preview) => `${preview.file.name}|${preview.file.size}|${preview.file.lastModified}`),
      );
      let totalBytes = current.reduce((sum, preview) => sum + preview.file.size, 0);
      const accepted: File[] = [];
      let rejectedType = 0;
      let rejectedSize = 0;
      let duplicate = 0;

      for (const file of files) {
        if (accepted.length >= slots) {
          notify(`Only ${slots} more image${slots === 1 ? "" : "s"} fit in this batch.`, "info");
          break;
        }
        const fingerprint = `${file.name}|${file.size}|${file.lastModified}`;
        if (existing.has(fingerprint)) {
          duplicate += 1;
          continue;
        }
        if (!ACCEPTED_TYPES.includes(file.type)) {
          rejectedType += 1;
          continue;
        }
        if (file.size > MAX_BYTES) {
          rejectedSize += 1;
          continue;
        }
        if (totalBytes + file.size > MAX_TOTAL_BYTES) {
          rejectedSize += 1;
          continue;
        }
        existing.add(fingerprint);
        totalBytes += file.size;
        accepted.push(file);
      }

      if (accepted.length === 0) {
        const reason =
          rejectedType > 0
            ? "Use JPEG, PNG, WebP, GIF, BMP or TIFF images."
            : rejectedSize > 0
              ? "Each image must be under 30 MiB, and the batch under 200 MiB."
              : duplicate > 0
                ? "Those files are already in the batch."
                : "";
        if (reason) setError(reason);
        return;
      }

      setAdding(true);
      setError("");
      const additions: Preview[] = [];
      let portrait = 0;
      for (const file of accepted) {
        try {
          const meta = await readImageFile(file);
          additions.push({
            key: newPreviewKey(),
            file,
            url: meta.url,
            width: meta.width,
            height: meta.height,
            title: "",
          });
        } catch (readError) {
          if (readError instanceof Error && readError.message === "landscape") {
            portrait += 1;
          } else {
            rejectedType += 1;
          }
        }
      }

      if (additions.length > 0) {
        clearUploaded();
        setPreviews((previous) => [...previous, ...additions]);
      }
      setAdding(false);

      const problems: string[] = [];
      if (portrait > 0) {
        problems.push(
          `${portrait} portrait image${portrait === 1 ? " was" : "s were"} skipped — landscape only.`,
        );
      }
      if (rejectedType > 0) problems.push(`${rejectedType} file(s) were not supported images.`);
      if (rejectedSize > 0) problems.push(`${rejectedSize} file(s) were too large.`);
      if (duplicate > 0) problems.push(`${duplicate} duplicate(s) ignored.`);
      if (problems.length > 0) {
        const message = problems.join(" ");
        setError(message);
        notify(message, additions.length > 0 ? "info" : "warning");
      }
    },
    [adding, clearUploaded, notify, uploading],
  );

  const handleDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragging(false);
    void addFiles(event.dataTransfer.files);
  };

  const submit = async () => {
    if (uploading || adding) return;
    if (previews.length === 0) {
      setError("Pick at least one landscape image first.");
      return;
    }
    if (!author.trim()) {
      setError("Tell us the artist name to show.");
      return;
    }

    setError("");
    setUploading(true);
    clearUploaded();
    const batch = [...previews];
    const results: Array<{
      preview: Preview;
      response?: Awaited<ReturnType<typeof api.uploadImage>>;
      error?: string;
    }> = [];
    let cursor = 0;
    let completed = 0;
    setProgress({ done: 0, total: batch.length });

    const worker = async () => {
      while (true) {
        const index = cursor;
        cursor += 1;
        if (index >= batch.length) return;
        const preview = batch[index]!;
        try {
          const form = new FormData();
          form.append("image", preview.file);
          form.append("author", author.trim());
          if (preview.title.trim()) form.append("title", preview.title.trim());
          const response = await api.uploadImage(form);
          results.push({ preview, response });
        } catch (uploadError) {
          results.push({
            preview,
            error:
              uploadError instanceof ApiError
                ? uploadError.message
                : "The image could not be uploaded.",
          });
        } finally {
          completed += 1;
          setProgress({ done: completed, total: batch.length });
        }
      }
    };

    await Promise.all(
      Array.from({ length: Math.min(UPLOAD_CONCURRENCY, batch.length) }, () => worker()),
    );

    let pendingList = loadPendingSubmissions();
    const successes: UploadedItem[] = [];
    const successfulKeys = new Set<string>();
    let failures = 0;
    let anonymousCount = 0;

    for (const result of results) {
      if (!result.response) {
        failures += 1;
        continue;
      }
      const response = result.response;
      const anonymous = Boolean(response.claimToken);
      if (anonymous) anonymousCount += 1;
      if (response.claimToken) {
        pendingList = addPendingSubmission({
          id: response.image.id,
          claimToken: response.claimToken,
          author: response.image.author,
          title: response.image.title,
          url: response.image.url,
          width: response.image.width,
          height: response.image.height,
          createdAt: response.image.createdAt,
        });
      }
      successes.push({
        id: response.image.id,
        url: response.image.url,
        title: response.image.title,
        author: response.image.author,
        anonymous,
        localUrl: result.preview.url,
      });
      successfulKeys.add(result.preview.key);
    }

    if (successes.length > 0) {
      // Successful preview object URLs are transferred to the success cards,
      // so they are revoked when the success panel is dismissed, not here.
      setPreviews((previous) =>
        previous.filter((preview) => !successfulKeys.has(preview.key)),
      );
      setUploaded(successes);
      const linked = successes.length - anonymousCount;
      notify(
        [
          `${successes.length} image${successes.length === 1 ? "" : "s"} uploaded`,
          linked > 0 ? `${linked} linked to your account` : "",
          anonymousCount > 0 ? `${anonymousCount} saved on this device` : "",
        ]
          .filter(Boolean)
          .join(" · "),
        "success",
      );
    }
    if (failures > 0) {
      notify(
        `${failures} image${failures === 1 ? "" : "s"} could not be uploaded. The previews remain so you can retry.`,
        "error",
      );
    }

    setPending(pendingList);
    emitPendingChange();
    if (successes.length > 0) {
      setAuthorTouched(false);
      if (user) setAuthor(user.displayName || user.username);
    }
    setUploading(false);
    setProgress({ done: 0, total: 0 });
  };

  if (status === "ready" && needsBootstrap) {
    return <Navigate to="/" replace />;
  }

  const anonymousSuccesses = uploaded.filter((item) => item.anonymous).length;
  const linkedSuccesses = uploaded.length - anonymousSuccesses;

  return (
    <PageShell maxWidth="lg">
      <PageHeader
        eyebrow="submit"
        title="Send dreams into the ring."
        description={
          <>
            Landscape images only for now. Pick one artist name for the batch; every image can
            still carry its own optional title.
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
            onClick={() => fileInput.current?.click()}
            sx={{
              position: "relative",
              overflow: "hidden",
              minHeight: previews.length > 0 ? 118 : { xs: 260, md: 360 },
              display: "grid",
              placeItems: "center",
              cursor: adding ? "progress" : "pointer",
              background: previews.length > 0 ? "rgba(255,255,255,.03)" : "rgba(255,255,255,.035)",
              transition: "min-height 260ms ease",
            }}
          >
            <input
              ref={fileInput}
              type="file"
              accept={ACCEPTED_TYPES.join(",")}
              multiple
              hidden
              onChange={(event) => {
                if (event.target.files) void addFiles(event.target.files);
                event.target.value = "";
              }}
            />
            <AnimatePresence mode="wait">
              {previews.length === 0 ? (
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
                    {adding ? <CircularProgress size={34} /> : <AddPhotoAlternateRounded sx={{ fontSize: 38 }} />}
                  </Box>
                  <Typography className="display" sx={{ fontSize: "1.7rem", fontWeight: 620 }}>
                    Drop landscape images
                  </Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.8 }}>
                    or click to browse · up to {MAX_FILES} at once · JPEG, PNG, WebP · max 30 MiB each
                  </Typography>
                </motion.div>
              ) : (
                <motion.div
                  key="more"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  style={{ display: "grid", placeItems: "center", gap: 8, padding: 20, textAlign: "center" }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", justifyContent: "center" }}>
                    {adding ? <CircularProgress size={18} /> : <AddPhotoAlternateRounded />}
                    <Typography sx={{ fontWeight: 650 }}>
                      {adding ? "Reading images…" : "Add more images"}
                    </Typography>
                    <Chip
                      size="small"
                      label={`${previews.length}/${MAX_FILES}`}
                      sx={{ background: "rgba(255,255,255,.08)" }}
                    />
                  </Box>
                  <Typography variant="body2" color="text.secondary">
                    Drop more files here, or click to browse.
                  </Typography>
                </motion.div>
              )}
            </AnimatePresence>
          </Box>

          <AnimatePresence>
            {uploaded.length > 0 && (
              <motion.div
                key="uploaded"
                initial={{ opacity: 0, y: 16, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: -10 }}
                transition={{ type: "spring", stiffness: 220, damping: 24 }}
                style={{ marginTop: 20 }}
              >
                <Box className="glass" sx={{ borderRadius: 5, p: 2.2 }}>
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1.2,
                      flexWrap: "wrap",
                      justifyContent: "space-between",
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <CheckCircleRounded color="success" />
                      <Box>
                        <Typography sx={{ fontWeight: 700 }}>
                          {uploaded.length} submission{uploaded.length === 1 ? "" : "s"} received
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                          {linkedSuccesses > 0 ? `${linkedSuccesses} linked to your account` : ""}
                          {linkedSuccesses > 0 && anonymousSuccesses > 0 ? " · " : ""}
                          {anonymousSuccesses > 0
                            ? `${anonymousSuccesses} saved on this device`
                            : ""}
                        </Typography>
                      </Box>
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
                          clearUploaded();
                          fileInput.current?.click();
                        }}
                        sx={{ textTransform: "none" }}
                      >
                        Submit more
                      </Button>
                    </Box>
                  </Box>
                  <Box
                    sx={{
                      mt: 1.8,
                      display: "grid",
                      gridTemplateColumns: {
                        xs: "repeat(2, minmax(0, 1fr))",
                        sm: "repeat(3, minmax(0, 1fr))",
                        md: "repeat(4, minmax(0, 1fr))",
                      },
                      gap: 1.2,
                    }}
                  >
                    {uploaded.map((item) => (
                      <Box key={item.id} sx={{ minWidth: 0 }}>
                        <Box
                          component="img"
                          src={
                            item.localUrl ??
                            imageVariant(item.url, { w: 360, quality: 78 })
                          }
                          alt={item.title || `By ${item.author}`}
                          sx={{
                            width: "100%",
                            aspectRatio: "4 / 3",
                            objectFit: "cover",
                            borderRadius: 2,
                          }}
                        />
                        {item.title ? (
                          <>
                            <Typography noWrap variant="body2" sx={{ fontWeight: 650, mt: 0.4 }}>
                              {item.title}
                            </Typography>
                            <Typography noWrap variant="caption" color="text.secondary">
                              {item.author}
                            </Typography>
                          </>
                        ) : (
                          <Typography noWrap variant="body2" sx={{ fontWeight: 650, mt: 0.4 }}>
                            {item.author}
                          </Typography>
                        )}
                      </Box>
                    ))}
                  </Box>
                  {anonymousSuccesses > 0 && (
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 1.2, opacity: 0.8 }}>
                      Create an account to link anonymous submissions later — the artist names
                      stay exactly as submitted.
                    </Typography>
                  )}
                </Box>
              </motion.div>
            )}
          </AnimatePresence>

          {previews.length > 0 && (
            <Box
              sx={{
                mt: 2,
                display: "grid",
                gridTemplateColumns: {
                  xs: "1fr",
                  sm: "repeat(2, minmax(0, 1fr))",
                },
                gap: 2,
              }}
            >
              {previews.map((preview, index) => (
                <motion.div
                  key={preview.key}
                  layout
                  initial={{ opacity: 0, y: 12 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.97 }}
                >
                  <Box className="glass" sx={{ borderRadius: 4, overflow: "hidden" }}>
                    <Box sx={{ position: "relative", aspectRatio: "4 / 3", bgcolor: "rgba(0,0,0,.25)" }}>
                      <Box
                        component="img"
                        src={preview.url}
                        alt={`Preview ${index + 1}`}
                        sx={{ width: "100%", height: "100%", objectFit: "cover" }}
                      />
                      <Chip
                        size="small"
                        label={index + 1}
                        sx={{
                          position: "absolute",
                          top: 8,
                          left: 8,
                          background: "rgba(10,9,15,.72)",
                          color: "#fff",
                          height: 22,
                          minWidth: 26,
                          fontSize: "0.7rem",
                        }}
                      />
                      <Tooltip title="Remove from batch">
                        <IconButton
                          size="small"
                          aria-label={`Remove image ${index + 1}`}
                          onClick={() => removePreview(preview.key)}
                          disabled={uploading}
                          sx={{
                            position: "absolute",
                            top: 6,
                            right: 6,
                            color: "#fff",
                            background: "rgba(10,9,15,.66)",
                            "&:hover": { background: "rgba(10,9,15,.9)" },
                          }}
                        >
                          <DeleteOutlineRounded fontSize="small" />
                        </IconButton>
                      </Tooltip>
                      <Chip
                        size="small"
                        label={`${preview.width} × ${preview.height} · landscape ✓`}
                        sx={{
                          position: "absolute",
                          left: 8,
                          bottom: 8,
                          background: "rgba(10,9,15,.66)",
                          color: "rgba(255,255,255,.82)",
                          height: 22,
                          fontSize: "0.66rem",
                        }}
                      />
                    </Box>
                    <Box sx={{ p: 1.4 }}>
                      <TextField
                        label="Title (optional)"
                        size="small"
                        fullWidth
                        value={preview.title}
                        onChange={(event) => updateTitle(preview.key, event.target.value)}
                        inputProps={{ maxLength: 200 }}
                        disabled={uploading}
                      />
                    </Box>
                  </Box>
                </motion.div>
              ))}
            </Box>
          )}
        </Box>

        <Box className="glass" sx={{ borderRadius: 5, p: { xs: 2.4, sm: 3 } }}>
          <Typography className="display" sx={{ fontSize: "1.5rem", fontWeight: 620, mb: 0.5 }}>
            The little card
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2.4 }}>
            One artist name for every image in this batch. Titles are optional and can be set on
            the previews.
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
                  : "Shown publicly with every image. Saved locally until you create an account."
              }
              inputProps={{ maxLength: 120 }}
              disabled={uploading}
            />

            {previews.length > 0 && (
              <Typography variant="body2" color="text.secondary">
                {previews.length} image{previews.length === 1 ? "" : "s"} ready
                {previews.some((preview) => preview.title.trim()) ? " · titles set" : ""}.
              </Typography>
            )}

            {uploading && progress.total > 0 && (
              <Box>
                <LinearProgress
                  variant="determinate"
                  value={(progress.done / progress.total) * 100}
                  sx={{ borderRadius: 99, height: 6 }}
                />
                <Typography variant="caption" color="text.secondary" sx={{ mt: 0.6, display: "block" }}>
                  Uploading {progress.done}/{progress.total}…
                </Typography>
              </Box>
            )}

            {error && (
              <Alert severity="warning" sx={{ borderRadius: 3 }}>
                {error}
              </Alert>
            )}

            <Button
              type="submit"
              variant="contained"
              size="large"
              disabled={uploading || adding || previews.length === 0}
              disableElevation
              startIcon={
                uploading ? <CircularProgress size={18} color="inherit" /> : <ImageSearchRounded />
              }
              sx={{
                borderRadius: 99,
                py: 1.1,
                textTransform: "none",
                fontWeight: 700,
              }}
            >
              {uploading
                ? `Uploading ${progress.done}/${progress.total}…`
                : previews.length > 1
                  ? `Submit ${previews.length} images`
                  : "Submit image"}
            </Button>
          </Box>

          <Divider sx={{ my: 2.4, opacity: 0.4 }} />

          <Box sx={{ display: "flex", gap: 1.2 }}>
            <AutoAwesomeRounded sx={{ opacity: 0.6, mt: 0.2 }} />
            <Typography variant="body2" color="text.secondary" sx={{ opacity: 0.86 }}>
              {user
                ? `Submissions are linked to your account id (${user.id}) — separate from the artist name.`
                : "No account? No problem. Images are stored and this device remembers how to attach them later."}
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
                    src={imageVariant(item.url, { w: 360, quality: 78 })}
                    alt={item.title || `By ${item.author}`}
                    sx={{ width: 92, height: 66, borderRadius: 2, objectFit: "cover", flexShrink: 0 }}
                  />
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    {item.title && (
                      <Typography noWrap sx={{ fontWeight: 700, fontSize: "0.95rem" }}>
                        {item.title}
                      </Typography>
                    )}
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