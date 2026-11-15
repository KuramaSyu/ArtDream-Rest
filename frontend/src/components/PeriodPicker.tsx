import { Box, ToggleButton, ToggleButtonGroup, Tooltip, alpha, useTheme } from "@mui/material";
import type { ReactNode } from "react";
import type { TimeRange } from "../api/types";

const options: Array<{ value: TimeRange; label: string; title: string }> = [
  { value: "day", label: "Day", title: "Today's winners" },
  { value: "week", label: "Week", title: "This week's winners" },
  { value: "month", label: "Month", title: "This month's winners" },
  { value: "year", label: "Year", title: "This year's winners" },
  { value: "all", label: "All", title: "All-time winners" },
];

export function PeriodPicker({
  value,
  onChange,
  leading,
}: {
  value: TimeRange;
  onChange: (value: TimeRange) => void;
  leading?: ReactNode;
}) {
  const theme = useTheme();
  const text = theme.palette.text.primary;
  return (
    <Box
      className="period-dock"
      sx={{ display: "flex", alignItems: "center", gap: 1 }}
    >
      <ToggleButtonGroup
        value={value}
        exclusive
        size="small"
        onChange={(_event, next: TimeRange | null) => {
          if (next) onChange(next);
        }}
        aria-label="Choose ranking period"
        sx={{
          background: alpha(theme.palette.background.paper, 0.55),
          backdropFilter: "blur(14px)",
          borderRadius: 99,
          p: 0.4,
          border: `1px solid ${alpha(text, 0.14)}`,
          "& .MuiToggleButton-root": {
            border: 0,
            borderRadius: 99,
            px: 1.3,
            py: 0.25,
            color: alpha(text, 0.75),
            fontSize: "0.7rem",
            letterSpacing: "0.05em",
            textTransform: "none",
            transition: "all 200ms ease",
            "&:hover": { background: alpha(text, 0.1), color: text },
            "&.Mui-selected": {
              background: alpha(theme.palette.primary.main, 0.75),
              color: theme.palette.getContrastText(theme.palette.primary.main),
              "&:hover": { background: alpha(theme.palette.primary.main, 0.88) },
            },
          },
        }}
      >
        {options.map((option) => (
          <ToggleButton key={option.value} value={option.value} aria-label={option.title}>
            <Tooltip title={option.title} enterDelay={400}>
              <span>{option.label}</span>
            </Tooltip>
          </ToggleButton>
        ))}
      </ToggleButtonGroup>
      {leading}
    </Box>
  );
}