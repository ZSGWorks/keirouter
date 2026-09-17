import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { TimerOff } from "lucide-react";
import {
  api,
  formatCooldownReset,
  type CooldownAccountRow,
  type CooldownProviderGroup,
  type ProviderUsage,
} from "../lib/api";
import { CooldownResetModal } from "./CooldownResetModal";
import { useToast } from "./Toast";
import { Badge, Button, Card, EmptyState, ErrorCard, Skeleton } from "./ui";

const REASON_TONE: Record<CooldownAccountRow["reason"], "danger" | "warning"> = {
  credits_exhausted: "danger",
  model_rate_limit: "warning",
  rate_limit: "warning",
};

function fmtRetry(seconds: number): string {
  if (!Number.isFinite(seconds)) return "—";
  if (seconds <= 0) return "<1s";
  if (seconds < 60) return `${Math.ceil(seconds)}s`;
  if (seconds < 3600) return `${Math.ceil(seconds / 60)}m`;
  return `${Math.ceil(seconds / 3600)}h`;
}

// fmtModels clamps a long pending-model list so one row cannot wrap the card.
function fmtModels(models: string[]): string {
  if (models.length === 0) return "All models";
  if (models.length <= 3) return models.join(", ");
  return `${models.slice(0, 3).join(", ")} +${models.length - 3} more`;
}

function CooldownRow({
  row,
  onClear,
  clearing,
  disabled,
}: {
  row: CooldownAccountRow;
  onClear: (id: string) => void;
  clearing: boolean;
  disabled: boolean;
}) {
  return (
    <div className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate text-sm font-semibold" title={row.label}>
            {row.label}
          </span>
          <Badge tone={REASON_TONE[row.reason]}>{row.reason_label}</Badge>
        </div>
        <div className="mt-0.5 text-[10px] text-[var(--text-muted)]" title={row.models.join(", ")}>
          {fmtModels(row.models)} · backoff {row.backoff_level} · retries in {fmtRetry(row.retry_after_seconds)}
        </div>
      </div>
      <Button
        variant="ghost"
        className="min-h-8 px-2.5 py-1 text-xs"
        disabled={disabled || clearing}
        onClick={() => onClear(row.account_id)}
      >
        {clearing ? "Clearing…" : "Clear"}
      </Button>
    </div>
  );
}

function CooldownGroup({
  group,
  providerMeta,
  clearingId,
  clearPending,
  onClear,
}: {
  group: CooldownProviderGroup;
  providerMeta: Map<string, ProviderUsage>;
  clearingId: string | undefined;
  clearPending: boolean;
  onClear: (id: string) => void;
}) {
  const meta = providerMeta.get(group.provider);
  return (
    <div className="py-3">
      <div className="flex items-center gap-2">
        <span
          className="h-2 w-2 shrink-0 rounded-full"
          style={{ backgroundColor: meta?.color || "var(--color-ink-400)" }}
        />
        <span className="text-sm font-semibold">{meta?.display_name || group.provider_name}</span>
        <span className="text-[10px] text-[var(--text-muted)]">{group.accounts.length} parked</span>
      </div>
      <div className="mt-1 divide-y divide-[var(--border)] pl-4">
        {group.accounts.map((row) => (
          <CooldownRow
            key={row.account_id}
            row={row}
            clearing={clearingId === row.account_id}
            disabled={clearPending}
            onClear={onClear}
          />
        ))}
      </div>
    </div>
  );
}

function CooldownBody({
  query,
  groups,
  providerMeta,
  clearingId,
  clearPending,
  onClear,
}: {
  query: { isLoading: boolean; isError: boolean };
  groups: CooldownProviderGroup[];
  providerMeta: Map<string, ProviderUsage>;
  clearingId: string | undefined;
  clearPending: boolean;
  onClear: (id: string) => void;
}) {
  if (query.isLoading) {
    return (
      <div className="px-5 py-4">
        <Skeleton className="h-16 w-full" />
      </div>
    );
  }
  if (query.isError) {
    return (
      <div className="px-5 py-4">
        <ErrorCard message="Failed to load cooldown status." />
      </div>
    );
  }
  if (groups.length === 0) {
    return <EmptyState title="No providers in cooldown." hint="Nothing is parked right now." />;
  }
  return (
    <div className="divide-y divide-[var(--border)] px-5">
      {groups.map((group) => (
        <CooldownGroup
          key={group.provider}
          group={group}
          providerMeta={providerMeta}
          clearingId={clearingId}
          clearPending={clearPending}
          onClear={onClear}
        />
      ))}
    </div>
  );
}

export function CooldownCard({ providers }: { providers: ProviderUsage[] }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [resetOpen, setResetOpen] = useState(false);

  const query = useQuery({
    queryKey: ["cooldowns"],
    queryFn: () => api.cooldowns(),
    staleTime: 15_000,
    refetchInterval: 30_000,
  });

  const invalidate = () => {
    for (const key of ["cooldowns", "quota", "health-overview", "accounts"]) {
      qc.invalidateQueries({ queryKey: [key] });
    }
  };

  const clear = useMutation({
    mutationFn: (target: { accountId?: string }) =>
      target.accountId ? api.resetAccountCooldown(target.accountId) : api.resetAllCooldowns(),
    onSuccess: (res) => {
      const { title, message } = formatCooldownReset(res);
      toast.success(title, message);
      invalidate();
      setResetOpen(false);
    },
    onError: (e: Error) => toast.error("Cooldown reset failed", e.message),
  });

  const groups = query.data?.providers ?? [];
  const total = groups.reduce((sum, group) => sum + group.accounts.length, 0);
  const providerMeta = new Map(providers.map((p) => [p.provider, p]));

  return (
    <Card>
      <div className="flex items-start justify-between gap-3 border-b border-[var(--border)] px-5 py-4">
        <div className="flex items-start gap-2.5">
          <TimerOff className="mt-0.5 h-4 w-4 text-[var(--text-muted)]" />
          <div>
            <h2 className="text-sm font-semibold">Providers in cooldown</h2>
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              Accounts parked on a rate limit or quota pause. Clear to let routing use them again.
            </p>
          </div>
        </div>
        {total > 0 && (
          <Button
            variant="ghost"
            className="min-h-8 px-2.5 py-1 text-xs"
            disabled={clear.isPending}
            onClick={() => setResetOpen(true)}
          >
            Clear all
          </Button>
        )}
      </div>

      <CooldownBody
        query={query}
        groups={groups}
        providerMeta={providerMeta}
        clearingId={clear.isPending ? clear.variables?.accountId : undefined}
        clearPending={clear.isPending}
        onClear={(id) => clear.mutate({ accountId: id })}
      />

      <CooldownResetModal
        open={resetOpen}
        title="Reset all dispatcher cooldowns?"
        subtitle="Every parked account and active model cooldown is released so routing can use them again. Probe history is kept."
        confirmLabel="Reset all cooldowns"
        pending={clear.isPending}
        error={clear.error instanceof Error ? clear.error.message : undefined}
        onClose={() => {
          clear.reset();
          setResetOpen(false);
        }}
        onConfirm={() => clear.mutate({})}
      />
    </Card>
  );
}
