import { Box, Typography, alpha, useTheme } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";

interface LogoProps {
  to?: string;
  size?: "sm" | "md" | "lg";
  className?: string;
}

const sizes = {
  sm: { fontSize: "1.15rem", dot: 6 },
  md: { fontSize: "1.6rem", dot: 8 },
  lg: { fontSize: "clamp(2rem, 5vw, 3.4rem)", dot: 12 },
} as const;

export function Logo({ to = "/", size = "md", className }: LogoProps) {
  const theme = useTheme();
  const spec = sizes[size];
  const content = (
    <Typography
      component="span"
      className={className}
      sx={{
        fontFamily: "var(--ad-font-display)",
        fontWeight: 650,
        letterSpacing: "-0.04em",
        lineHeight: 1,
        fontSize: spec.fontSize,
        color: "inherit",
        display: "inline-flex",
        alignItems: "flex-end",
        gap: 0.35,
        textShadow: `0 10px 34px ${alpha(theme.palette.common.black, 0.35)}`,
        "&:hover .logo-dot": {
          transform: "translateY(-5px) scale(1.35)",
          borderRadius: "40% 60% 55% 45%",
        },
      }}
    >
      ArtDream
      <Box
        component="span"
        className="logo-dot"
        sx={{
          width: spec.dot,
          height: spec.dot,
          borderRadius: "50%",
          background: `linear-gradient(135deg, ${theme.palette.primary.main} 15%, ${alpha(theme.palette.text.primary, 0.55)})`,
          marginBottom: spec.dot * 0.6,
          transition: "transform 360ms cubic-bezier(.2,.9,.2,1.4), border-radius 360ms ease",
        }}
      />
    </Typography>
  );

  if (!to) return content;
  return (
    <Box
      component={RouterLink}
      to={to}
      aria-label="ArtDream home"
      sx={{ color: "inherit", textDecoration: "none", display: "inline-flex" }}
    >
      {content}
    </Box>
  );
}