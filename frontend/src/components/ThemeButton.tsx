import { useMemo } from "react";
import {
  Button,
  alpha,
  useTheme,
  type ButtonProps,
} from "@mui/material";
import { keyframes } from "@emotion/react";
import type { SxProps, Theme } from "@mui/material/styles";

/**
 * Theme-aware button used on the landing page. Every color, shadow and
 * transition comes from the live image-generated MUI theme, so the controls
 * repaint together with the artwork palette.
 *
 * Tones:
 *  - hero:  primary/secondary gradient call to action with an optional glow
 *  - glass: translucent outlined secondary action
 *  - ghost: low-emphasis text action
 *  - pill:  compact toolbar toggle
 */
export type ThemeButtonTone = "hero" | "glass" | "ghost" | "pill";

export interface ThemeButtonProps extends ButtonProps {
  tone?: ThemeButtonTone;
  glow?: boolean;
  active?: boolean;
}

export function ThemeButton({
  tone = "glass",
  glow = false,
  active = false,
  variant,
  sx,
  ...props
}: ThemeButtonProps) {
  const theme = useTheme();
  const text = theme.palette.text.primary;
  const primary = theme.palette.primary.main;
  const secondary = theme.palette.secondary.main;
  const primaryContrast = theme.palette.getContrastText(primary);

  const pulse = useMemo(
    () => keyframes`
      0%, 100% {
        box-shadow: 0 18px 52px ${alpha(theme.palette.primary.dark, 0.55)};
      }
      50% {
        box-shadow:
          0 18px 52px ${alpha(theme.palette.primary.dark, 0.4)},
          0 0 34px 6px ${alpha(theme.palette.secondary.main, 0.38)};
      }
    `,
    [theme.palette.primary.dark, theme.palette.secondary.main],
  );

  const hero: SxProps<Theme> = {
    position: "relative",
    overflow: "hidden",
    px: 4.5,
    py: 1.6,
    borderRadius: 99,
    fontSize: "1.02rem",
    fontWeight: 750,
    textTransform: "none",
    color: primaryContrast,
    background: `linear-gradient(120deg, ${primary}, ${secondary})`,
    boxShadow: `0 22px 60px ${alpha(theme.palette.primary.dark, 0.6)}`,
    animation: glow ? `${pulse} 4s ease-in-out infinite` : undefined,
    "&::after": {
      content: '""',
      position: "absolute",
      inset: 0,
      pointerEvents: "none",
      background: `linear-gradient(110deg, transparent 20%, ${alpha(text, 0.2)} 48%, transparent 72%)`,
      transform: "translateX(-130%)",
      transition: `transform ${theme.transitions.duration.complex}ms ease`,
    },
    "&:hover": {
      transform: "translateY(-2px) scale(1.015)",
      boxShadow: `0 28px 70px ${alpha(primary, 0.5)}`,
    },
    "&:hover::after": { transform: "translateX(130%)" },
  };

  const glass: SxProps<Theme> = {
    px: 3,
    py: 1.6,
    borderRadius: 99,
    textTransform: "none",
    fontWeight: 600,
    color: text,
    borderColor: alpha(text, 0.32),
    backgroundColor: alpha(theme.palette.background.paper, 0.3),
    "&:hover": {
      borderColor: alpha(text, 0.7),
      backgroundColor: alpha(text, 0.09),
    },
  };

  const ghost: SxProps<Theme> = {
    color: alpha(text, 0.68),
    textTransform: "none",
    "&:hover": {
      color: text,
      backgroundColor: alpha(text, 0.08),
    },
  };

  const pill: SxProps<Theme> = {
    minWidth: 0,
    px: 1.2,
    py: 0.25,
    borderRadius: 99,
    textTransform: "none",
    fontSize: "0.7rem",
    fontWeight: 600,
    color: active ? primaryContrast : alpha(text, 0.82),
    background: active ? primary : alpha(theme.palette.background.paper, 0.55),
    border: `1px solid ${alpha(text, 0.16)}`,
    "& .MuiButton-startIcon": { mr: 0.5 },
    "&:hover": {
      background: active ? alpha(primary, 0.88) : alpha(text, 0.1),
      color: active ? primaryContrast : text,
    },
  };

  const tones: Record<ThemeButtonTone, SxProps<Theme>> = {
    hero,
    glass,
    ghost,
    pill,
  };
  const resolvedVariant =
    variant ?? (tone === "hero" ? "contained" : tone === "ghost" ? "text" : "outlined");

  return (
    <Button
      {...props}
      variant={resolvedVariant}
      sx={[tones[tone], sx] as SxProps<Theme>}
    />
  );
}