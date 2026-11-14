import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Snackbar,
  type AlertColor,
} from "@mui/material";

interface Toast {
  id: number;
  message: string;
  severity: AlertColor;
}

interface FeedbackValue {
  notify: (message: string, severity?: AlertColor) => void;
}

const FeedbackContext = createContext<FeedbackValue | null>(null);

export function FeedbackProvider({ children }: { children: ReactNode }) {
  const [queue, setQueue] = useState<Toast[]>([]);

  const notify = useCallback((message: string, severity: AlertColor = "info") => {
    setQueue((previous) => [...previous, { id: Date.now() + Math.random(), message, severity }]);
  }, []);

  const close = useCallback(() => {
    setQueue((previous) => previous.slice(1));
  }, []);

  const current = queue[0];
  const value = useMemo(() => ({ notify }), [notify]);

  return (
    <FeedbackContext.Provider value={value}>
      {children}
      <Snackbar
        key={current?.id ?? "empty"}
        open={Boolean(current)}
        autoHideDuration={current?.severity === "error" ? 6000 : 3800}
        onClose={close}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
        className="toast-stack"
      >
        {current ? (
          <Alert
            severity={current.severity}
            variant="filled"
            onClose={close}
            sx={{ borderRadius: 3, fontWeight: 500 }}
          >
            {current.message}
          </Alert>
        ) : undefined}
      </Snackbar>
    </FeedbackContext.Provider>
  );
}

export function useToast(): FeedbackValue {
  const context = useContext(FeedbackContext);
  if (!context) throw new Error("useToast must be used within FeedbackProvider");
  return context;
}

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  loading = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onClose={loading ? undefined : onCancel} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ fontFamily: "var(--ad-font-display)", fontWeight: 600 }}>{title}</DialogTitle>
      <DialogContent>
        <DialogContentText component="div">{description}</DialogContentText>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button onClick={onCancel} disabled={loading} color="inherit">
          {cancelLabel}
        </Button>
        <Button
          onClick={onConfirm}
          disabled={loading}
          variant="contained"
          color={danger ? "error" : "primary"}
          disableElevation
        >
          {loading ? "Working…" : confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}