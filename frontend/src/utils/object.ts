export function uid(): string {
  return crypto.randomUUID();
}

/** Deep copy that also strips Vue reactivity proxies. */
export function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}
