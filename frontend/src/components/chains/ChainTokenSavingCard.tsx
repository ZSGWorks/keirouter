import { AlertTriangle, Leaf } from "lucide-react";
import { Card, SegmentedControl } from "../ui";
import type { ChainTokenSaving, EndpointSettings } from "../../lib/api";

// Per-chain token-saving override options. "inherit" keeps the global
// Settings value; On/Off force the feature for requests on this chain only.
export type OverrideMode = "inherit" | "on" | "off";
const overrideOptions: { value: OverrideMode; label: string }[] = [
  { value: "inherit", label: "Inherit" },
  { value: "on", label: "On" },
  { value: "off", label: "Off" },
];

const LEVEL_INHERIT = "inherit-level";

interface TokenSavingFeature {
  key: string;
  label: string;
  toggle: "rtk_enabled" | "caveman_enabled" | "terse_enabled" | "headroom_enabled" | "ponytail_enabled";
  level?: { key: "rtk_filter_level" | "caveman_level" | "terse_level" | "ponytail_level"; options: { value: string; label: string }[] };
}

export const tokenSavingFeatures: TokenSavingFeature[] = [
  { key: "rtk", label: "RTK token saver", toggle: "rtk_enabled", level: { key: "rtk_filter_level", options: [{ value: "none", label: "Off" }, { value: "minimal", label: "Minimal" }, { value: "aggressive", label: "Aggressive" }] } },
  { key: "caveman", label: "Caveman output", toggle: "caveman_enabled", level: { key: "caveman_level", options: [{ value: "lite", label: "Gentle" }, { value: "full", label: "Balanced" }, { value: "ultra", label: "Strong" }, { value: "wenyan-lite", label: "Wenyan" }, { value: "wenyan-full", label: "Wenyan Full" }, { value: "wenyan-ultra", label: "Wenyan Ultra" }] } },
  { key: "terse", label: "Terse mode", toggle: "terse_enabled", level: { key: "terse_level", options: [{ value: "light", label: "Gentle" }, { value: "medium", label: "Balanced" }, { value: "aggressive", label: "Strong" }] } },
  { key: "headroom", label: "Headroom proxy", toggle: "headroom_enabled" },
  { key: "ponytail", label: "Ponytail", toggle: "ponytail_enabled", level: { key: "ponytail_level", options: [{ value: "lite", label: "Lite" }, { value: "full", label: "Full" }, { value: "ultra", label: "Ultra" }] } },
];

const globalLevelValue = (settings: EndpointSettings | undefined, key: string): string | undefined =>
  (settings as unknown as Record<string, unknown>)?.[key] as string | undefined;

export const isTokenSavingConflict = (modes: Record<string, OverrideMode>) =>
  modes.caveman === "on" && modes.terse === "on";

export function buildTokenSaving(
  modes: Record<string, OverrideMode>,
  levels: Record<string, string | null>,
): ChainTokenSaving | undefined {
  const out: Record<string, boolean | string | null> = {};
  for (const feature of tokenSavingFeatures) {
    const mode = modes[feature.key] ?? "inherit";
    if (mode === "on") out[feature.toggle] = true;
    if (mode === "off") out[feature.toggle] = false;
    if (feature.level) {
      const lvl = levels[feature.key];
      if (mode === "on" && lvl) out[feature.level.key] = lvl;
    }
  }
  return Object.keys(out).length ? (out as ChainTokenSaving) : undefined;
}

export function savedTokenSavingState(saved: ChainTokenSaving | undefined): {
  modes: Record<string, OverrideMode>;
  levels: Record<string, string | null>;
} {
  const s = saved ?? {};
  const modes: Record<string, OverrideMode> = {};
  const levels: Record<string, string | null> = {};
  for (const feature of tokenSavingFeatures) {
    const toggleValue = s[feature.toggle];
    modes[feature.key] = toggleValue === true ? "on" : toggleValue === false ? "off" : "inherit";
    if (feature.level) levels[feature.key] = s[feature.level.key] ?? null;
  }
  return { modes, levels };
}

export function ChainTokenSavingCard({
  modes,
  levels,
  globalSettings,
  conflictMessage,
  onModeChange,
  onLevelChange,
}: {
  modes: Record<string, OverrideMode>;
  levels: Record<string, string | null>;
  globalSettings?: EndpointSettings;
  conflictMessage?: string;
  onModeChange: (key: string, mode: OverrideMode) => void;
  onLevelChange: (key: string, level: string | null) => void;
}) {
  return (
    <Card className="p-5 sm:p-6">
      <div className="mb-3 flex items-start gap-3">
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-accent-500/10 text-accent-600 dark:text-accent-300"><Leaf className="h-4.5 w-4.5" /></div>
        <div>
          <h2 className="text-base font-semibold">Token saving</h2>
          <p className="mt-1 text-sm text-[var(--text-muted)]">Override the global token-saving settings for requests routed through this chain. Inherit keeps the value configured in Settings; other chains are unaffected.</p>
        </div>
      </div>
      <div className="divide-y divide-[var(--border)]">
        {tokenSavingFeatures.map((feature) => {
          const mode = modes[feature.key] ?? "inherit";
          const globalOn = Boolean((globalSettings as unknown as Record<string, unknown>)?.[feature.toggle]);
          const globalLevel = feature.level ? globalLevelValue(globalSettings, feature.level.key) : undefined;
          const forcedLevel = feature.level && mode === "on" && levels[feature.key] ? ` · ${levels[feature.key]}` : "";
          return (
            <div key={feature.key} className="py-3 first:pt-0 last:pb-0">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-medium">{feature.label}</p>
                  <p className="mt-0.5 text-xs text-[var(--text-muted)]">{mode === "inherit" ? `Global: ${globalOn ? "on" : "off"}${globalLevel ? ` · ${globalLevel}` : ""}` : `Forced ${mode} on this chain${forcedLevel}`}</p>
                </div>
                <SegmentedControl value={mode} onChange={(v) => onModeChange(feature.key, v)} options={overrideOptions} />
              </div>
              {feature.level && mode === "on" && (
                <div className="mt-3 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border)] pt-3">
                  <p className="text-xs font-medium text-[var(--text-muted)]">Level override</p>
                  <SegmentedControl
                    value={levels[feature.key] ?? LEVEL_INHERIT}
                    onChange={(v) => onLevelChange(feature.key, v === LEVEL_INHERIT ? null : v)}
                    options={[{ value: LEVEL_INHERIT, label: `Global (${globalLevel ?? "default"})` }, ...feature.level.options]}
                  />
                </div>
              )}
            </div>
          );
        })}
      </div>
      {conflictMessage && <p className="mt-3 flex gap-2 text-xs leading-5 text-[color:var(--color-warning)]"><AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />{conflictMessage}</p>}
    </Card>
  );
}