export function uid(): string {
  return crypto.randomUUID();
}

/** Deep copy that also strips Vue reactivity proxies. */
export function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

/** Splits text into trimmed, non-empty lines. */
export function lines(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean);
}

/** Appends lines that are not in text yet, keeping the existing order. */
export function mergeLines(text: string, add: readonly string[]): string {
  const out = lines(text);
  for (const l of add) if (!out.includes(l)) out.push(l);
  return out.join('\n');
}
