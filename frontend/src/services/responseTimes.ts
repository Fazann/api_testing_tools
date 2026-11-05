// Geometry and formatting for response-time charts. Kept free of Vue so it can
// be unit-tested.

export interface Box {
  width: number;
  height: number;
  /** Space for the y-axis labels on the left. */
  left: number;
  right: number;
  top: number;
  bottom: number;
}

export interface Tick {
  value: number;
  y: number;
}

export interface Geometry {
  xs: number[];
  ys: number[];
  /** SVG path through every point. */
  path: string;
  ticks: Tick[];
  y: (ms: number) => number;
}

/** Rounds up to 1, 2 or 5 × 10ⁿ so the axis has friendly labels. */
export function niceCeil(v: number): number {
  if (v <= 0) return 1;
  const exp = Math.pow(10, Math.floor(Math.log10(v)));
  for (const m of [1, 2, 5, 10]) if (v <= m * exp) return m * exp;
  return 10 * exp;
}

/**
 * Places durations left to right (oldest first) on a y-axis from 0 to a nice
 * maximum that also fits `extra` values such as the median and limit lines.
 */
export function geometry(values: number[], box: Box, extra: number[] = []): Geometry {
  const plotW = Math.max(1, box.width - box.left - box.right);
  const plotH = Math.max(1, box.height - box.top - box.bottom);
  const top = niceCeil(Math.max(1, ...values, ...extra.filter((v) => v > 0)));
  const y = (ms: number) => box.top + plotH - (Math.min(ms, top) / top) * plotH;
  const step = values.length > 1 ? plotW / (values.length - 1) : 0;
  const xs = values.map((_, i) => box.left + (values.length > 1 ? i * step : plotW / 2));
  const ys = values.map(y);
  const path = xs.map((x, i) => `${i ? 'L' : 'M'}${round(x)},${round(ys[i])}`).join('');
  const ticks = [0, 0.5, 1].map((f) => ({ value: top * f, y: y(top * f) }));
  return { xs, ys, path, ticks, y };
}

/** Index of the point closest to an x coordinate. */
export function nearestIndex(xs: number[], x: number): number {
  let best = 0;
  for (let i = 1; i < xs.length; i++) if (Math.abs(xs[i] - x) < Math.abs(xs[best] - x)) best = i;
  return best;
}

/** A small line through values (oldest first) for a table cell. */
export function sparkPath(values: number[], width: number, height: number): string {
  if (values.length < 2) return '';
  const max = Math.max(...values);
  const min = Math.min(...values);
  const span = max - min || 1;
  const pad = 2;
  return values
    .map((v, i) => {
      const x = (i / (values.length - 1)) * width;
      const yy = pad + (1 - (v - min) / span) * (height - 2 * pad);
      return `${i ? 'L' : 'M'}${round(x)},${round(yy)}`;
    })
    .join('');
}

/** "+42%", "−8%", "±0%"; '—' without a value. */
export function formatChange(pct: number | null | undefined): string {
  if (pct === null || pct === undefined) return '—';
  const r = Math.round(pct);
  if (r === 0) return '±0%';
  return r > 0 ? `+${r}%` : `−${-r}%`;
}

/** Short axis label: "850", "1.2k"… milliseconds. */
export function axisLabel(ms: number): string {
  if (ms >= 1000) return `${+(ms / 1000).toFixed(1)} s`;
  return `${Math.round(ms)} ms`;
}

function round(n: number): number {
  return Math.round(n * 10) / 10;
}
