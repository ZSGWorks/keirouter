import { useRef, useState, type ChangeEvent, type Dispatch, type SetStateAction } from "react";
import { Upload } from "lucide-react";
import { api, type ForeignImportResult, type N9routerAnalyzeResult, type N9routerImportOptions } from "../lib/api";
import { useToast } from "./Toast";
import { Button } from "./ui";

type Props = {
  loading: boolean;
  setLoading: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setResult: Dispatch<SetStateAction<ForeignImportResult | null>>;
};

const sections: Array<{ key: Exclude<keyof N9routerImportOptions, "mode">; label: string; detail: string }> = [
  { key: "usage", label: "Usage records", detail: "token usage, costs, model stats" },
  { key: "providers", label: "Providers & accounts", detail: "connections, custom nodes, credentials re-encrypted" },
  { key: "api_keys", label: "API keys", detail: "re-hashed; same key strings keep working" },
  { key: "proxy_pools", label: "Proxy pools", detail: "Cloudflare / HTTP proxy configs" },
  { key: "chains", label: "Routing chains", detail: "combos → chains (fallback/RR strategies)" },
  { key: "settings", label: "Settings", detail: "token saver (RTK/Caveman/Ponytail), routing strategy" },
  { key: "password", label: "Dashboard password", detail: "import 9router's bcrypt hash (triggers re-login)" },
];

export function N9routerSQLiteImport({ loading, setLoading, setError, setResult }: Props) {
  const toast = useToast();
  const inputRef = useRef<HTMLInputElement>(null);
  const [analysis, setAnalysis] = useState<N9routerAnalyzeResult | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [options, setOptions] = useState<N9routerImportOptions>({ usage: true, providers: true, api_keys: true, proxy_pools: true, chains: true, settings: true, password: true, mode: "merge" });

  const selectFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const selected = event.target.files?.[0];
    if (!selected) return;
    setLoading(true);
    setError(null);
    setResult(null);
    setAnalysis(null);
    try {
      setAnalysis(await api.analyze9routerSQLite(selected));
      setFile(selected);
    } catch (error) {
      setError((error as Error).message || "Analyze failed.");
      toast.error("Analyze failed", (error as Error).message);
    } finally {
      setLoading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const runImport = async () => {
    if (!file) return;
    setLoading(true);
    setError(null);
    setResult(null);
    try {
      const result = await api.import9routerSQLite(file, options);
      setResult(result);
      const parts: string[] = [];
      if (result.accounts) parts.push(`${result.accounts} account${result.accounts === 1 ? "" : "s"}`);
      if (result.custom_providers) parts.push(`${result.custom_providers} provider${result.custom_providers === 1 ? "" : "s"}`);
      if (result.api_keys) parts.push(`${result.api_keys} key${result.api_keys === 1 ? "" : "s"}`);
      if (result.chains) parts.push(`${result.chains} chain${result.chains === 1 ? "" : "s"}`);
      if (result.aliases) parts.push(`${result.aliases} alias${result.aliases === 1 ? "" : "es"}`);
      if (result.proxy_pools) parts.push(`${result.proxy_pools} pool${result.proxy_pools === 1 ? "" : "s"}`);
      if (result.usage_records) parts.push(`${result.usage_records} usage record${result.usage_records === 1 ? "" : "s"}`);
      toast.success("9router SQLite import complete", `${result.imported} record${result.imported === 1 ? "" : "s"} imported (${parts.length ? parts.join(", ") : "nothing"}).${result.skipped ? ` ${result.skipped} skipped.` : ""}`);
      setFile(null);
      setAnalysis(null);
    } catch (error) {
      setError((error as Error).message || "Import failed.");
      toast.error("Import failed", (error as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col gap-3 rounded-xl border border-[var(--border)] bg-[var(--bg)] p-4 sm:col-span-2">
      <div className="flex items-center gap-2"><span className="inline-flex h-7 w-7 items-center justify-center rounded-md bg-accent-100 text-xs font-bold text-accent-700 dark:bg-accent-900/40 dark:text-accent-200">DB</span><div><p className="text-sm font-semibold text-[var(--text)]">9router SQLite database</p><p className="text-[11px] text-[var(--text-muted)]">Full import incl. usage history &amp; settings</p></div></div>
      {!analysis && !file && <><p className="text-xs leading-relaxed text-[var(--text-muted)]">Upload 9router&apos;s <code>data.sqlite</code> directly. Imports everything the JSON backup does, plus usage history, token saver settings, routing strategy, and the dashboard password. Analyzes the file first so you can choose exactly which sections to import.</p><Button variant="ghost" onClick={() => inputRef.current?.click()} disabled={loading} className="w-full"><Upload className="h-4 w-4" />Select 9router data.sqlite</Button><input ref={inputRef} type="file" accept=".sqlite,.db,application/vnd.sqlite3,application/x-sqlite3" className="hidden" onChange={selectFile} /></>}
      {analysis && <div className="space-y-4">
        <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-subtle)] px-4 py-3"><p className="mb-2 text-xs font-semibold uppercase text-[var(--text-muted)]">Detected in file</p><div className="flex flex-wrap gap-x-5 gap-y-1 text-xs text-[var(--text)]">{analysis.providerConnections != null && <span>{analysis.providerConnections} providers/accounts</span>}{analysis.providerNodes != null && <span>{analysis.providerNodes} custom nodes</span>}{analysis.apiKeys != null && <span>{analysis.apiKeys} API keys</span>}{analysis.combos != null && <span>{analysis.combos} chains</span>}{analysis.proxyPools != null && <span>{analysis.proxyPools} proxy pools</span>}{analysis.usageHistory != null && <span>{analysis.usageHistory.toLocaleString()} usage records</span>}</div></div>
        <div className="space-y-2"><p className="text-xs font-semibold uppercase text-[var(--text-muted)]">Sections to import</p>{sections.map(({ key, label, detail }) => <label key={key} className="flex items-start gap-2 text-xs text-[var(--text)]"><input type="checkbox" checked={options[key]} onChange={(event) => setOptions((current) => ({ ...current, [key]: event.target.checked }))} className="mt-0.5" /><span>{label} <span className="text-[var(--text-muted)]">— {detail}</span></span></label>)}</div>
        <div className="space-y-2"><p className="text-xs font-semibold uppercase text-[var(--text-muted)]">Import mode</p><label className="flex items-start gap-2 text-xs text-[var(--text)]"><input type="radio" name="n9mode" value="merge" checked={options.mode === "merge"} onChange={() => setOptions((current) => ({ ...current, mode: "merge" }))} className="mt-0.5" /><span>Merge <span className="text-[var(--text-muted)]">— add new rows, skip existing (safe, repeatable)</span></span></label><label className="flex items-start gap-2 text-xs text-[var(--text)]"><input type="radio" name="n9mode" value="overwrite" checked={options.mode === "overwrite"} onChange={() => setOptions((current) => ({ ...current, mode: "overwrite" }))} className="mt-0.5" /><span>Overwrite <span className="text-[var(--text-muted)]">— remove previous 9router imports, then re-import (clean sync; settings are merged, not replaced)</span></span></label><label className="flex items-start gap-2 text-xs text-[var(--text)]"><input type="radio" name="n9mode" value="wipe" checked={options.mode === "wipe"} onChange={() => setOptions((current) => ({ ...current, mode: "wipe" }))} className="mt-0.5" /><span>Wipe &amp; replace <span className="text-[var(--text-muted)] font-medium text-red-500">— DESTROYS all selected data including KeiRouter-native rows</span></span></label>{options.mode === "wipe" && <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-[11px] text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">⚠️ Wipe mode deletes ALL rows in the selected sections, not just previously imported ones. A safety backup is created automatically before any deletion.</p>}</div>
        <div className="flex gap-2"><Button variant="primary" onClick={runImport} disabled={loading || !sections.some(({ key }) => options[key])} className="w-full">{loading ? "Importing…" : "Run import"}</Button><Button variant="ghost" onClick={() => { setAnalysis(null); setFile(null); }} disabled={loading}>Cancel</Button></div>
      </div>}
    </div>
  );
}
