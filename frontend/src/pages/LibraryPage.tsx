import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  IconButton,
  InputAdornment,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  CloudUploadOutlined,
  CloseRounded,
  DownloadRounded,
  SearchRounded,
} from "@mui/icons-material";
import { motion } from "framer-motion";
import { useNavigate } from "react-router-dom";
import { api, ApiError, imageVariant } from "../api/client";
import type { ImageItem } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { EmptyState, LoadingView } from "../components/Loading";
import { PageHeader, PageShell } from "../components/PageShell";
import { useToast } from "../components/Feedback";
import { useStageImages } from "../theme/ThemeHost";
import { relativeTime } from "../utils/format";

export function LibraryPage() {
  const { user } = useAuth();
  const { notify } = useToast();
  const navigate = useNavigate();

  const defaultAuthor = user?.displayName || user?.username || "";
  const [query, setQuery] = useState(defaultAuthor);
  const [author, setAuthor] = useState(defaultAuthor);
  const [images, setImages] = useState<ImageItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<ImageItem | null>(null);

  const urls = useMemo(() => images.map((item) => item.url), [images]);
  useStageImages(urls);

  const load = useCallback(
    async (name: string) => {
      if (!name.trim()) {
        setImages([]);
        setTotal(0);
        setLoading(false);
        return;
      }
      setLoading(true);
      try {
        const result = await api.imagesByAuthor(name.trim(), 200, 0);
        setImages(result.items);
        setTotal(result.total);
      } catch (error) {
        notify(error instanceof ApiError ? error.message : "Could not load the library.", "error");
        setImages([]);
      } finally {
        setLoading(false);
      }
    },
    [notify],
  );

  useEffect(() => {
    void load(author);
  }, [author, load]);

  useEffect(() => {
    if (user) {
      const name = user.displayName || user.username;
      setQuery(name);
      setAuthor(name);
    }
    // Only prefill when the account changes, not on every keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [user?.id]);

  const search = () => setAuthor(query.trim());

  return (
    <PageShell maxWidth="xl">
      <PageHeader
        eyebrow="library"
        title="Dreams by name."
        description="Browse every artwork submitted under an artist name. Names are independent from accounts, so try different spellings."
        action={
          <Button
            variant="contained"
            startIcon={<CloudUploadOutlined />}
            onClick={() => navigate("/submit")}
            sx={{ borderRadius: 99, textTransform: "none", fontWeight: 700 }}
          >
            Submit art
          </Button>
        }
      />

      <Box
        className="glass"
        sx={{
          borderRadius: 4,
          p: 1.4,
          mb: 3,
          display: "flex",
          gap: 1,
          alignItems: "center",
          flexWrap: "wrap",
        }}
      >
        <TextField
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") search();
          }}
          placeholder="Artist name"
          size="small"
          sx={{ flex: 1, minWidth: 220 }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchRounded fontSize="small" sx={{ opacity: 0.6 }} />
              </InputAdornment>
            ),
          }}
        />
        <Button onClick={search} variant="outlined" sx={{ borderRadius: 99, textTransform: "none" }}>
          Find
        </Button>
        {author && (
          <Chip
            label={`${total} ${total === 1 ? "piece" : "pieces"} by ${author}`}
            size="small"
            sx={{ background: "rgba(255,255,255,.07)" }}
          />
        )}
      </Box>

      {loading ? (
        <LoadingView label="Opening the sketchbook…" />
      ) : images.length === 0 ? (
        <EmptyState
          emoji="🎐"
          title={author ? "Nothing under that name" : "Who are we looking for?"}
          description={
            author
              ? "Try a different artist name, or be the first to submit under it."
              : "Type an artist name above. Account names and artist names can differ."
          }
          action={
            <Button variant="contained" onClick={() => navigate("/submit")} sx={{ borderRadius: 99 }}>
              Submit the first one
            </Button>
          }
        />
      ) : (
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: {
              xs: "1fr",
              sm: "repeat(2, minmax(0, 1fr))",
              md: "repeat(3, minmax(0, 1fr))",
              xl: "repeat(4, minmax(0, 1fr))",
            },
            gap: 2,
          }}
        >
          {images.map((image, index) => (
            <motion.div
              key={image.id}
              initial={{ opacity: 0, y: 16 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: Math.min(index * 0.04, 0.5), duration: 0.45 }}
            >
              <Box
                className="glass tilt-card"
                role="button"
                tabIndex={0}
                onClick={() => setSelected(image)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") setSelected(image);
                }}
                sx={{
                  borderRadius: 4,
                  overflow: "hidden",
                  cursor: "pointer",
                  "&:hover img": { transform: "scale(1.04)" },
                }}
              >
                <Box sx={{ position: "relative", aspectRatio: "4 / 3", overflow: "hidden" }}>
                  <Box
                    component="img"
                    src={imageVariant(image.url, { w: 640, quality: 78 })}
                    alt={image.title || `Artwork by ${image.author}`}
                    loading="lazy"
                    sx={{
                      width: "100%",
                      height: "100%",
                      objectFit: "cover",
                      transition: "transform 500ms ease",
                    }}
                  />
                  <Box
                    sx={{
                      position: "absolute",
                      inset: 0,
                      background: "linear-gradient(180deg, transparent 55%, rgba(0,0,0,.6))",
                    }}
                  />
                  <Box sx={{ position: "absolute", left: 12, bottom: 10, right: 12 }}>
                    {image.title && (
                      <Typography
                        className="display"
                        noWrap
                        sx={{ fontWeight: 620, fontSize: "1.15rem", color: "#fff" }}
                      >
                        {image.title}
                      </Typography>
                    )}
                    <Typography
                      noWrap
                      sx={{ color: "rgba(255,255,255,.68)", fontSize: "0.76rem", letterSpacing: "0.04em" }}
                    >
                      {image.author} · {relativeTime(image.createdAt)}
                    </Typography>
                  </Box>
                </Box>
              </Box>
            </motion.div>
          ))}
        </Box>
      )}

      <Dialog
        open={Boolean(selected)}
        onClose={() => setSelected(null)}
        maxWidth="md"
        fullWidth
      >
        {selected && (
          <>
            <DialogTitle
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1,
                pr: 6,
                fontFamily: "var(--ad-font-display)",
                fontWeight: 650,
              }}
            >
              {selected.title || selected.author}
              <IconButton
                onClick={() => setSelected(null)}
                sx={{ position: "absolute", right: 12, top: 12 }}
                aria-label="Close"
              >
                <CloseRounded />
              </IconButton>
            </DialogTitle>
            <DialogContent>
              <Box
                component="img"
                src={imageVariant(selected.url, { h: 1080, quality: 85 })}
                alt={selected.title || selected.author}
                sx={{ width: "100%", borderRadius: 3, display: "block", mb: 2 }}
              />
              <Box
                sx={{
                  display: "flex",
                  flexWrap: "wrap",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 1.5,
                }}
              >
                <Box>
                  <Typography sx={{ fontWeight: 700 }}>{selected.author}</Typography>
                  <Typography variant="body2" color="text.secondary">
                    {selected.width} × {selected.height} · {selected.mimeType} ·{" "}
                    {relativeTime(selected.createdAt)}
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", gap: 1 }}>
                  {(["webp", "png", "jpg"] as const).map((format) => (
                    <Tooltip key={format} title={`Download as ${format.toUpperCase()}`}>
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<DownloadRounded />}
                        href={imageVariant(selected.url, { format, download: true })}
                        download
                        sx={{ borderRadius: 99, textTransform: "none" }}
                      >
                        {format}
                      </Button>
                    </Tooltip>
                  ))}
                  <Tooltip title="Download the pristine upload (large)">
                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={<DownloadRounded />}
                      href={imageVariant(selected.url, { original: true, download: true })}
                      download
                      sx={{ borderRadius: 99, textTransform: "none" }}
                    >
                      original
                    </Button>
                  </Tooltip>
                </Box>
              </Box>
            </DialogContent>
          </>
        )}
      </Dialog>
    </PageShell>
  );
}