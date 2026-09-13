import { Loader2, TimerOff } from "lucide-react";
import { Button, ErrorBanner, Modal } from "./ui";

// CooldownResetModal is the shared confirm dialog for dispatcher-cooldown
// resets (per-account and tenant-wide). It confirms before clearing, reports
// failures inline for retry, and reports cleared counts via the caller's
// success toast.
export function CooldownResetModal({
  open,
  title,
  subtitle,
  confirmLabel,
  pending,
  error,
  onClose,
  onConfirm,
}: Readonly<{
  open: boolean;
  title: string;
  subtitle: string;
  confirmLabel: string;
  pending: boolean;
  error?: string;
  onClose: () => void;
  onConfirm: () => void;
}>) {
  return (
    <Modal
      open={open}
      onClose={() => { if (!pending) onClose(); }}
      title={title}
      subtitle={subtitle}
      maxWidth="max-w-md"
    >
      <div className="space-y-4 px-6 py-5">
        {error && !pending && <ErrorBanner message={error} />}
        <div className="flex items-start gap-3 rounded-xl border border-accent-200 bg-accent-50 px-3.5 py-3 dark:border-accent-800 dark:bg-accent-900/30">
          <TimerOff className="mt-0.5 h-4 w-4 shrink-0 text-accent-700 dark:text-accent-300" strokeWidth={2} />
          <div className="text-sm leading-snug text-accent-800 dark:text-accent-200">
            This only clears dispatcher cooldown state — credentials and settings are untouched.
            <span className="font-semibold"> Safe to repeat: resetting with nothing parked clears nothing.</span>
          </div>
        </div>
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            onClick={onClose}
            disabled={pending}
          >
            Cancel
          </Button>
          <Button
            onClick={onConfirm}
            disabled={pending}
          >
            {pending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <TimerOff className="h-3.5 w-3.5" />
            )}
            {pending ? "Resetting…" : confirmLabel}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
