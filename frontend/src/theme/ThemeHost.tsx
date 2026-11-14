import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Box, CssBaseline, ThemeProvider, alpha, createTheme } from "@mui/material";
import { resolveImageUrl } from "../api/client";
import { CustomThemeImpl, type CustomTheme } from "./customTheme";
import {
  buildPaletteFromSwatches,
  buildThemeFromImageRecord,
  extractImageSwatches,
  type GeneratedThemeRecord,
  type ImageSwatchData,
  type ThemeMode,
} from "./imageTheme";

const SWATCH_CACHE_KEY = "artdream.swatches.v1";
const MAX_CACHE_ENTRIES = 48;

interface ThemeHostValue {
  /** Live theme generated from a displayed artwork. */
  theme: CustomTheme;
  /** Image whose palette produced the current theme. */
  themeImage: string | null;
  /** True while Vibrant is quantizing a new image. */
  busy: boolean;
  setStageImages: (urls: string[]) => void;
  /** Pick the next displayed image and rebuild the palette from it. */
  shuffle: () => void;
}

const ThemeHostContext = createContext<ThemeHostValue | null>(null);

const DEFAULT_THEME = new CustomThemeImpl(
  createTheme({
    palette: {
      mode: "dark",
      primary: { main: "#8fbcbb", light: "#b8d8d8", dark: "#5e81ac" },
      secondary: { main: "#b48ead", light: "#d4b8d0", dark: "#8f6f8a" },
      background: { default: "#14121c", paper: "#211d2c" },
      text: { primary: "#f4eee8", secondary: "#c3b8cc" },
      error: { main: "#e06c75" },
      warning: { main: "#e5c07b" },
      success: { main: "#98c379" },
    },
  }),
  {
    name: "artdream-night",
    longName: "ArtDream Night",
    backgrounds: [],
  },
);

function readCache(): Map<string, ImageSwatchData> {
  const map = new Map<string, ImageSwatchData>();
  try {
    const raw = localStorage.getItem(SWATCH_CACHE_KEY);
    if (!raw) return map;
    const parsed = JSON.parse(raw) as Record<string, ImageSwatchData>;
    for (const [url, swatches] of Object.entries(parsed)) {
      if (
        swatches &&
        typeof swatches === "object" &&
        typeof swatches.dominant === "string" &&
        (swatches.suggestedMode === "light" || swatches.suggestedMode === "dark")
      ) {
        map.set(url, swatches);
      }
    }
  } catch {
    // ignore corrupt cache
  }
  return map;
}

function writeCache(cache: Map<string, ImageSwatchData>): void {
  try {
    const entries = [...cache.entries()].slice(-MAX_CACHE_ENTRIES);
    localStorage.setItem(SWATCH_CACHE_KEY, JSON.stringify(Object.fromEntries(entries)));
  } catch {
    // ignore quota errors
  }
}

const swatchCache = readCache();

function stableKey(input: string): string {
  let hash = 5381;
  for (let i = 0; i < input.length; i += 1) {
    hash = (hash * 33) ^ input.charCodeAt(i);
  }
  return (hash >>> 0).toString(36);
}

function recordFromSwatches(
  swatches: ImageSwatchData,
  image: string,
  mode: ThemeMode,
): GeneratedThemeRecord {
  const key = stableKey(image);
  return {
    id: `live-${key}`,
    groupId: `live-${key}`,
    name: `live-${key}`,
    longName: "Palette of this artwork",
    mode,
    palette: buildPaletteFromSwatches(swatches, mode),
    backgroundImage: image,
    blurredBackgroundImage: image,
    blur: 0,
    createdAt: Date.now(),
  };
}

async function themeForImage(image: string): Promise<CustomTheme> {
  let swatches = swatchCache.get(image);
  if (!swatches) {
    swatches = await extractImageSwatches(resolveImageUrl(image));
    swatchCache.set(image, swatches);
    writeCache(swatchCache);
  }
  const record = recordFromSwatches(swatches, image, swatches.suggestedMode);
  return new CustomThemeImpl(buildThemeFromImageRecord(record));
}

export function ThemeHost({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<CustomTheme>(DEFAULT_THEME);
  const [themeImage, setThemeImage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [stageImages, setStageImagesState] = useState<string[]>([]);

  const stageKey = useMemo(() => stageImages.join("|"), [stageImages]);
  const appliedKey = useRef("");
  const failedKey = useRef("");
  const generation = useRef(0);
  const pickIndex = useRef(0);

  const setStageImages = useCallback((urls: string[]) => {
    const clean = Array.from(new Set(urls.map((url) => url.trim()).filter(Boolean)));
    setStageImagesState((previous) => {
      if (
        previous.length === clean.length &&
        previous.every((value, index) => value === clean[index])
      ) {
        return previous;
      }
      return clean;
    });
  }, []);

  const applyImage = useCallback(async (image: string): Promise<boolean> => {
    const ticket = generation.current + 1;
    generation.current = ticket;
    setBusy(true);
    try {
      const next = await themeForImage(image);
      if (generation.current !== ticket) return true;
      setTheme(next);
      setThemeImage(image);
      return true;
    } catch (error) {
      console.warn("ArtDream theme generation failed", error);
      if (generation.current === ticket) {
        setTheme(DEFAULT_THEME);
        setThemeImage(image);
      }
      return false;
    } finally {
      if (generation.current === ticket) setBusy(false);
    }
  }, []);

  useEffect(() => {
    if (stageImages.length === 0) return;
    if (stageKey === appliedKey.current || stageKey === failedKey.current) return;
    appliedKey.current = stageKey;
    const image = stageImages[Math.floor(Math.random() * stageImages.length)]!;
    void applyImage(image).then((ok) => {
      if (!ok) failedKey.current = stageKey;
    });
  }, [appliedKey, applyImage, failedKey, stageImages, stageKey]);

  const shuffle = useCallback(() => {
    if (stageImages.length === 0) return;
    pickIndex.current = (pickIndex.current + 1) % stageImages.length;
    const image = stageImages[pickIndex.current]!;
    appliedKey.current = stageKey;
    void applyImage(image);
  }, [applyImage, stageImages, stageKey]);

  const value = useMemo<ThemeHostValue>(
    () => ({ theme, themeImage, busy, setStageImages, shuffle }),
    [theme, themeImage, busy, setStageImages, shuffle],
  );

  const primary = theme.palette.primary.main;
  const secondary = theme.palette.secondary.main;

  useEffect(() => {
    const meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", theme.palette.background.default);
  }, [theme]);

  return (
    <ThemeHostContext.Provider value={value}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <Box
          aria-hidden
          className="theme-backdrop"
          sx={{
            backgroundImage: themeImage ? `url("${resolveImageUrl(themeImage)}")` : "none",
            backgroundColor: theme.palette.background.default,
          }}
        />
        <Box
          aria-hidden
          className="theme-wash"
          sx={{
            background: `
              radial-gradient(1100px 700px at 12% -10%, ${alpha(primary, 0.34)} 0%, transparent 65%),
              radial-gradient(900px 700px at 98% 20%, ${alpha(secondary, 0.27)} 0%, transparent 68%),
              radial-gradient(900px 900px at 50% 120%, ${alpha(primary, 0.18)} 0%, transparent 70%)
            `,
          }}
        />
        <Box aria-hidden className="theme-grain" />
        {children}
      </ThemeProvider>
    </ThemeHostContext.Provider>
  );
}

export function useAppTheme(): ThemeHostValue {
  const context = useContext(ThemeHostContext);
  if (!context) throw new Error("useAppTheme must be used within ThemeHost");
  return context;
}

/**
 * Register the images currently visible on a page as palette candidates.
 * The host picks one at random whenever the set changes.
 */
export function useStageImages(urls: Array<string | undefined | null> | undefined): void {
  const { setStageImages } = useAppTheme();
  const key = useMemo(() => (urls ?? []).filter(Boolean).join("|"), [urls]);
  useEffect(() => {
    if (!urls || urls.length === 0) return;
    setStageImages(urls.filter((url): url is string => Boolean(url)));
  }, [key, setStageImages]);
}
