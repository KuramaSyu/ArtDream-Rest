import { Box, CircularProgress, Typography, alpha, useTheme } from "@mui/material";
import type { ReactNode } from "react";

export function LoadingView({ label = "Mixing colors…" }: { label?: string }) {
  return (
    <Box
      sx={{
        minHeight: "45vh",
        display: "grid",
        placeItems: "center",
        gap: 2,
        textAlign: "center",
      }}
    >
      <Box>
        <Box sx={{ position: "relative", display: "inline-flex", mb: 1.4 }}>
          <CircularProgress size={46} thickness={3.2} />
          <Box
            sx={{
              position: "absolute",
              inset: 0,
              display: "grid",
              placeItems: "center",
              fontSize: "0.9rem",
              animation: "bob 2.6s ease-in-out infinite",
            }}
          >
            ✦
          </Box>
        </Box>
        <Typography variant="body2" color="text.secondary" sx={{ letterSpacing: "0.04em" }}>
          {label}
        </Typography>
      </Box>
    </Box>
  );
}

export function EmptyState({
  emoji = "🫧",
  title,
  description,
  action,
}: {
  emoji?: string;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
}) {
  const theme = useTheme();
  return (
    <Box
      sx={{
        borderRadius: 6,
        p: { xs: 4, sm: 6 },
        textAlign: "center",
        display: "grid",
        placeItems: "center",
        gap: 1.2,
        background: alpha(theme.palette.background.paper, 0.72),
        border: `1px solid ${alpha(theme.palette.text.primary, 0.12)}`,
        boxShadow: `0 16px 44px ${alpha(theme.palette.common.black, 0.3)}, inset 0 1px 0 ${alpha(theme.palette.text.primary, 0.05)}`,
      }}
    >
      <Box sx={{ fontSize: "2.6rem", animation: "bob 5s ease-in-out infinite" }}>{emoji}</Box>
      <Typography
        className="display"
        sx={{ fontSize: "1.7rem", fontWeight: 600, lineHeight: 1.15 }}
      >
        {title}
      </Typography>
      {description && (
        <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 460 }}>
          {description}
        </Typography>
      )}
      {action && <Box sx={{ mt: 1.4 }}>{action}</Box>}
    </Box>
  );
}