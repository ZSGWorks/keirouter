import { type Provider, type ProviderModel } from "../lib/api";
import { Badge, Button, Modal } from "./ui";

const CAPABILITY_LABELS = [
  ["vision", "Vision input"],
  ["pdf", "PDF input"],
  ["audio_input", "Audio input"],
  ["video_input", "Video input"],
  ["image_output", "Image output"],
  ["audio_output", "Audio output"],
  ["search", "Web search"],
  ["tools", "Tool use"],
  ["reasoning", "Reasoning"],
  ["structured_output", "Structured output"],
] as const;

const rate = (value: number) =>
  `$${value.toLocaleString(undefined, { maximumFractionDigits: 6 })} / M tokens`;

function CapabilityDetails({ model }: { model: ProviderModel }) {
  const supported = model.capabilities
    ? CAPABILITY_LABELS.filter(([key]) => model.capabilities?.[key])
    : [];

  return (
    <section>
      <h3 className="text-sm font-semibold">Capabilities</h3>
      {supported.length ? (
        <ul className="mt-2 flex flex-wrap gap-2">
          {supported.map(([, label]) => (
            <li key={label}>
              <Badge tone="accent">{label}</Badge>
            </li>
          ))}
        </ul>
      ) : (
        <p className="mt-2 text-sm text-[var(--text-muted)]">
          No supported capabilities are cataloged.
        </p>
      )}
    </section>
  );
}

function RateList({ rates }: { rates: Array<[string, number]> }) {
  return rates.map(([label, value]) => (
    <div key={label} className="flex justify-between gap-4 text-sm">
      <span className="text-[var(--text-muted)]">{label}</span>
      <span className="font-medium tabular-nums">{rate(value)}</span>
    </div>
  ));
}

function PricingDetails({ model }: { model: ProviderModel }) {
  const pricing = model.pricing;
  if (!pricing) {
    return (
      <section>
        <h3 className="text-sm font-semibold">Pricing</h3>
        <p className="mt-2 text-sm text-[var(--text-muted)]">
          Pricing unavailable from provider catalog and models.dev.
        </p>
      </section>
    );
  }
  if (pricing.explicit_free) {
    return (
      <section>
        <h3 className="text-sm font-semibold">Pricing</h3>
        <p className="mt-2 text-sm text-[var(--text-muted)]">Free model. No per-token cost.</p>
      </section>
    );
  }

  const rates: Array<[string, number]> = [
    ["Input", pricing.input_per_m],
    ["Output", pricing.output_per_m],
    ["Cached input", pricing.cached_input_per_m],
    ["Cache write", pricing.cache_write_per_m],
    ["Reasoning", pricing.reasoning_per_m],
  ];
  const availableRates = rates.filter(([, value]) => value > 0);
  const longRates: Array<[string, number]> = [
    ["Input", pricing.long_input_per_m],
    ["Output", pricing.long_output_per_m],
    ["Cached input", pricing.long_cached_input_per_m],
    ["Cache write", pricing.long_cache_write_per_m],
  ];
  const availableLongRates = longRates.filter(([, value]) => value > 0);
  const source = pricing.source.replaceAll("_", " ");

  return (
    <section>
      <h3 className="text-sm font-semibold">Pricing</h3>
      {availableRates.length > 0 ? (
        <div className="mt-2 space-y-2">
          <RateList rates={availableRates} />
          {pricing.long_context_threshold > 0 && availableLongRates.length > 0 && (
            <div className="border-t border-[var(--border)] pt-2">
              <p className="mb-2 text-xs text-[var(--text-muted)]">
                Over {pricing.long_context_threshold.toLocaleString()} tokens
              </p>
              <RateList rates={availableLongRates} />
            </div>
          )}
        </div>
      ) : (
        <p className="mt-2 text-sm text-[var(--text-muted)]">No per-token rates are cataloged.</p>
      )}
      {pricing.source_url ? (
        <a
          className="mt-3 inline-block text-xs text-accent-600 hover:underline dark:text-accent-300"
          href={pricing.source_url}
          target="_blank"
          rel="noreferrer"
        >
          Pricing source: {source || "catalog"}
        </a>
      ) : source ? (
        <p className="mt-3 text-xs text-[var(--text-muted)]">Pricing source: {source}</p>
      ) : null}
    </section>
  );
}

export function ModelDetailsModal({
  open,
  model,
  provider,
  onClose,
}: {
  open: boolean;
  model: ProviderModel;
  provider: Provider;
  onClose: () => void;
}) {
  return (
    <Modal open={open} onClose={onClose} title={model.name || model.id} subtitle="Read-only model metadata and pricing.">
      <div className="space-y-5 px-6 py-5">
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wide text-[var(--text-muted)]">Model</p>
          <code className="block rounded-lg bg-[var(--bg-subtle)] px-3 py-2 font-mono text-xs text-[var(--text-muted)]">
            {provider.alias || provider.id}/{model.id}
          </code>
          <div className="flex flex-wrap gap-2">
            <Badge tone="neutral">{model.kind || "Model"}</Badge>
            {model.discovered && <Badge tone="accent">Discovered</Badge>}
            {model.pricing?.estimated && <Badge tone="warning">Estimated pricing</Badge>}
          </div>
        </div>
        <div className="grid grid-cols-2 gap-3 text-sm">
          <Metric label="Context window" value={model.capabilities?.context_window} />
          <Metric label="Maximum output" value={model.capabilities?.max_output} />
        </div>
        <CapabilityDetails model={model} />
        <PricingDetails model={model} />
      </div>
      <div className="flex justify-end border-t border-[var(--border)] px-6 py-4">
        <Button variant="ghost" onClick={onClose}>Close</Button>
      </div>
    </Modal>
  );
}

function Metric({ label, value }: { label: string; value: number | undefined }) {
  return (
    <div className="rounded-lg bg-[var(--bg-subtle)] p-3">
      <p className="text-xs text-[var(--text-muted)]">{label}</p>
      <p className="mt-1 font-medium">{value ? value.toLocaleString() : "Unavailable"}</p>
    </div>
  );
}
