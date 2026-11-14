import { Container, Box, type ContainerProps } from "@mui/material";
import type { ReactNode } from "react";
import { Navigation } from "./Navigation";

export function PageShell({
  children,
  maxWidth = "lg",
}: {
  children: ReactNode;
  maxWidth?: ContainerProps["maxWidth"];
}) {
  return (
    <Box sx={{ minHeight: "100vh", display: "flex", flexDirection: "column" }}>
      <Navigation />
      <Box component="main" className="page" sx={{ flex: 1 }}>
        <Container maxWidth={maxWidth} disableGutters>
          {children}
        </Container>
      </Box>
    </Box>
  );
}

export function PageHeader({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow?: string;
  title: string;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <Box
      sx={{
        display: "flex",
        flexWrap: "wrap",
        alignItems: "flex-end",
        justifyContent: "space-between",
        gap: 2,
        mb: { xs: 3, md: 4.5 },
      }}
    >
      <Box sx={{ maxWidth: 720 }}>
        {eyebrow && (
          <Box
            component="div"
            sx={{
              fontSize: "0.72rem",
              letterSpacing: "0.2em",
              textTransform: "uppercase",
              opacity: 0.62,
              mb: 0.8,
            }}
          >
            {eyebrow}
          </Box>
        )}
        <Box
          component="h1"
          className="display"
          sx={{
            m: 0,
            fontSize: "clamp(2rem, 5vw, 3.6rem)",
            lineHeight: 1.02,
            fontWeight: 640,
            textShadow: "0 14px 40px rgba(0,0,0,.35)",
          }}
        >
          {title}
        </Box>
        {description && (
          <Box sx={{ mt: 1.2, color: "text.secondary", fontSize: "1.02rem", maxWidth: 620 }}>
            {description}
          </Box>
        )}
      </Box>
      {action && <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>{action}</Box>}
    </Box>
  );
}