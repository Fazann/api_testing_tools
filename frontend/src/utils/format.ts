// Display formatting shared by views and components.

export function formatBytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

export function formatDuration(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(2)} s` : `${ms} ms`;
}

export function formatTime(iso?: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  return isNaN(d.getTime()) ? '—' : d.toLocaleString();
}

/** Pretty-prints JSON bodies; returns other text unchanged. */
export function prettyBody(body: string, contentType = ''): string {
  const t = body.trim();
  if (!t || !(contentType.includes('json') || t.startsWith('{') || t.startsWith('['))) return body;
  try {
    return JSON.stringify(JSON.parse(t), null, 2);
  } catch {
    return body;
  }
}

/** "+42%", "−8%", "±0%"; '—' without a value. */
export function formatChange(pct: number | null | undefined): string {
  if (pct === null || pct === undefined) return '—';
  const r = Math.round(pct);
  if (r === 0) return '±0%';
  return r > 0 ? `+${r}%` : `−${-r}%`;
}
