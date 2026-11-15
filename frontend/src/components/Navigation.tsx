import { useState } from "react";
import { alpha } from "@mui/material/styles";
import {
  Avatar,
  Box,
  Button,
  Divider,
  IconButton,
  ListItemIcon,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
  useTheme,
} from "@mui/material";
import {
  AdminPanelSettings,
  AutoAwesome,
  Collections,
  Leaderboard,
  Login,
  Logout,
  PersonOutline,
  UploadFile,
} from "@mui/icons-material";
import { Link as RouterLink, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useAppTheme } from "../theme/ThemeHost";
import { Logo } from "./Logo";

interface NavigationProps {
  floating?: boolean;
}

interface NavItem {
  to: string;
  label: string;
  icon: typeof Leaderboard;
  auth?: boolean;
  admin?: boolean;
}

const links: NavItem[] = [
  { to: "/rank", label: "Rank", icon: Leaderboard },
  { to: "/submit", label: "Submit", icon: UploadFile },
  { to: "/library", label: "Library", icon: Collections, auth: true },
  { to: "/admin", label: "Admin", icon: AdminPanelSettings, auth: true, admin: true },
];

export function Navigation({ floating = false }: NavigationProps) {
  const { user, isAdmin, logout } = useAuth();
  const { shuffle, busy, themeImage } = useAppTheme();
  const location = useLocation();
  const navigate = useNavigate();
  const theme = useTheme();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [mobileAnchor, setMobileAnchor] = useState<HTMLElement | null>(null);

  const visibleLinks = links.filter((link) => {
    if (link.admin && !isAdmin) return false;
    if (link.auth && !user) return false;
    return true;
  });

  const active = (to: string) => location.pathname === to;

  const handleLogout = async () => {
    setAnchor(null);
    await logout();
    navigate("/");
  };

  return (
    <Box
      component="header"
      sx={{
        position: floating ? "fixed" : "sticky",
        top: 0,
        left: 0,
        right: 0,
        zIndex: 40,
        px: floating ? { xs: 2, sm: 3.5 } : { xs: 1.5, sm: 3 },
        py: floating ? 2 : 1.2,
        display: "flex",
        alignItems: "center",
        gap: 2,
        background: floating
          ? `linear-gradient(180deg, ${alpha(theme.palette.background.default, 0.6)}, ${alpha(theme.palette.background.default, 0)})`
          : alpha(theme.palette.background.paper, 0.92),
        borderBottom: floating ? "none" : `1px solid ${theme.palette.divider}`,
      }}
    >
      <Logo size={floating ? "md" : "sm"} />

      <Box sx={{ flex: 1 }} />

      <Box sx={{ display: { xs: "none", sm: "flex" }, alignItems: "center", gap: 0.6 }}>
        {visibleLinks.map((link) => (
          <Button
            key={link.to}
            component={RouterLink}
            to={link.to}
            size="small"
            startIcon={<link.icon sx={{ fontSize: 18 }} />}
            sx={{
              textTransform: "none",
              fontWeight: 600,
              px: 1.6,
              borderRadius: 99,
              color: active(link.to) ? "text.primary" : "text.secondary",
              background: active(link.to)
                ? alpha(theme.palette.text.primary, 0.12)
                : "transparent",
              "&:hover": {
                background: alpha(theme.palette.text.primary, 0.08),
                transform: "translateY(-1px)",
              },
              transition: "all 200ms ease",
            }}
          >
            {link.label}
          </Button>
        ))}
      </Box>

      <IconButton
        onClick={(event) =>
          setMobileAnchor((previous) => (previous ? null : event.currentTarget))
        }
        sx={{ display: { xs: "inline-flex", sm: "none" }, color: "text.primary" }}
        aria-label="Open navigation"
      >
        <Box component="span" sx={{ fontSize: "1.2rem", lineHeight: 1 }}>
          ☰
        </Box>
      </IconButton>
      <Menu
        anchorEl={mobileAnchor}
        open={Boolean(mobileAnchor)}
        onClose={() => setMobileAnchor(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
      >
        {visibleLinks.map((link) => (
          <MenuItem
            key={link.to}
            component={RouterLink}
            to={link.to}
            selected={active(link.to)}
            onClick={() => setMobileAnchor(null)}
          >
            <ListItemIcon>
              <link.icon fontSize="small" />
            </ListItemIcon>
            {link.label}
          </MenuItem>
        ))}
        {!user && (
          <MenuItem
            component={RouterLink}
            to="/login"
            onClick={() => setMobileAnchor(null)}
          >
            <ListItemIcon>
              <Login fontSize="small" />
            </ListItemIcon>
            Sign in
          </MenuItem>
        )}
      </Menu>

      <Tooltip
        title={
          themeImage
            ? busy
              ? "Mixing colors…"
              : "Repaint from another artwork"
            : "No artwork on stage yet"
        }
      >
        <span>
          <IconButton
            onClick={shuffle}
            disabled={!themeImage || busy}
            size="small"
            sx={{
              color: "text.primary",
              background: alpha(theme.palette.text.primary, 0.07),
              transition: "transform 300ms ease, background 300ms ease",
              "&:hover": {
                background: alpha(theme.palette.text.primary, 0.16),
                transform: "rotate(18deg) scale(1.08)",
              },
            }}
            aria-label="Generate a new theme from another image"
          >
            <AutoAwesome
              sx={{
                fontSize: 19,
                animation: busy ? "ad-spin 1.6s linear infinite" : undefined,
              }}
            />
          </IconButton>
        </span>
      </Tooltip>

      {user ? (
        <>
          <Tooltip title={`${user.displayName} (@${user.username})`}>
            <IconButton onClick={(event) => setAnchor(event.currentTarget)} size="small" sx={{ p: 0.4 }}>
              <Avatar
                sx={{
                  width: 34,
                  height: 34,
                  fontSize: "0.95rem",
                  fontWeight: 700,
                  background: `linear-gradient(135deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                  color: theme.palette.getContrastText(theme.palette.primary.main),
                }}
              >
                {(user.displayName || user.username).slice(0, 1).toUpperCase()}
              </Avatar>
            </IconButton>
          </Tooltip>
          <Menu
            anchorEl={anchor}
            open={Boolean(anchor)}
            onClose={() => setAnchor(null)}
            anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
            transformOrigin={{ vertical: "top", horizontal: "right" }}
          >
            <Box sx={{ px: 2, py: 1.2, minWidth: 210 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                {user.displayName}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                @{user.username}
                {user.isAdmin ? " · admin" : ""}
              </Typography>
            </Box>
            <Divider />
            <MenuItem
              component={RouterLink}
              to="/library"
              onClick={() => setAnchor(null)}
            >
              <ListItemIcon>
                <Collections fontSize="small" />
              </ListItemIcon>
              My submissions
            </MenuItem>
            <MenuItem onClick={() => setAnchor(null)} disabled>
              <ListItemIcon>
                <PersonOutline fontSize="small" />
              </ListItemIcon>
              Signed in
            </MenuItem>
            <Divider />
            <MenuItem onClick={handleLogout}>
              <ListItemIcon>
                <Logout fontSize="small" />
              </ListItemIcon>
              Sign out
            </MenuItem>
          </Menu>
        </>
      ) : (
        <Button
          component={RouterLink}
          to="/login"
          size="small"
          variant="outlined"
          startIcon={<Login sx={{ fontSize: 18 }} />}
          sx={{ borderRadius: 99, textTransform: "none", fontWeight: 700, ml: 0.5 }}
        >
          Sign in
        </Button>
      )}
    </Box>
  );
}