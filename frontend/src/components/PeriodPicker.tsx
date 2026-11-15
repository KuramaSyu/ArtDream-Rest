import { Box, ToggleButton, ToggleButtonGroup, Tooltip } from "@mui/material";
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
          background: "rgba(10,9,15,.42)",
          backdropFilter: "blur(14px)",
          borderRadius: 99,
          p: 0.4,
          border: "1px solid rgba(255,255,255,.14)",
          "& .MuiToggleButton-root": {
            border: 0,
            borderRadius: 99,
            px: 1.3,
            py: 0.25,
            color: "rgba(255,255,255,.75)",
            fontSize: "0.7rem",
            letterSpacing: "0.05em",
            textTransform: "none",
            transition: "all 200ms ease",
            "&:hover": { background: "rgba(255,255,255,.1)", color: "#fff" },
            "&.Mui-selected": {
              background: "rgba(255,255,255,.2)",
              color: "#fff",
              "&:hover": { background: "rgba(255,255,255,.24)" },
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