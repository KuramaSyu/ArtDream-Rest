import { useEffect, useState } from "react";
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Typography,
} from "@mui/material";
import { ApiError } from "../api/client";
import {
  PENDING_SUBMISSIONS_EVENT,
  loadPendingSubmissions,
} from "../utils/pendingSubmissions";
import { claimAllPendingSubmissions } from "../utils/claims";
import { useToast } from "./Feedback";
import { useAuth } from "../auth/AuthContext";

export function usePendingCount(): number {
  const [count, setCount] = useState(() => loadPendingSubmissions().length);
  useEffect(() => {
    const update = () => setCount(loadPendingSubmissions().length);
    window.addEventListener(PENDING_SUBMISSIONS_EVENT, update);
    window.addEventListener("storage", update);
    return () => {
      window.removeEventListener(PENDING_SUBMISSIONS_EVENT, update);
      window.removeEventListener("storage", update);
    };
  }, []);
  return count;
}

interface PendingClaimsPromptProps {
  open: boolean;
  onClose: () => void;
  onClaimed?: (count: number) => void;
}

export function PendingClaimsPrompt({ open, onClose, onClaimed }: PendingClaimsPromptProps) {
  const { user } = useAuth();
  const { notify } = useToast();
  const [count, setCount] = useState(0);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (open) setCount(loadPendingSubmissions().length);
  }, [open]);

  const claim = async () => {
    setLoading(true);
    try {
      const result = await claimAllPendingSubmissions();
      if (result.count > 0) {
        notify(
          result.count === 1
            ? "Submission attached to your account."
            : `${result.count} submissions attached to your account.`,
          "success",
        );
        onClaimed?.(result.count);
      } else {
        notify("Those submissions could not be attached anymore.", "warning");
      }
    } catch (error) {
      notify(error instanceof ApiError ? error.message : "Could not attach submissions.", "error");
    } finally {
      setLoading(false);
      onClose();
    }
  };

  return (
    <Dialog open={open} onClose={loading ? undefined : onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 650, fontSize: "1.65rem" }}>
        Keep them together?
      </DialogTitle>
      <DialogContent>
        <Typography variant="body1" color="text.secondary">
          You have {count} anonymous {count === 1 ? "submission" : "submissions"} saved on
          this device. Attach {count === 1 ? "it" : "them"} to{" "}
          <strong>@{user?.username}</strong>?
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1.2, opacity: 0.75 }}>
          Your chosen artist names stay exactly as submitted. Only the account link is added.
        </Typography>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.4 }}>
        <Button onClick={onClose} disabled={loading} color="inherit">
          Not now
        </Button>
        <Button onClick={claim} disabled={loading} variant="contained" disableElevation>
          {loading ? "Attaching…" : "Attach them"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}