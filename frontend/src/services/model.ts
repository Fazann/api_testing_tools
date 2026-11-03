import type { Assertion, AssertionKind, KeyValue, Operator, Request } from '../types';

export function uid(): string {
  return crypto.randomUUID();
}

/** Deep copy that also strips Vue reactivity proxies. */
export function clone<T>(v: T): T {
  return JSON.parse(JSON.stringify(v)) as T;
}

export function emptyRow(): KeyValue {
  return { key: '', value: '', enabled: true };
}

export function newRequest(collectionId: string, parentId = ''): Request {
  return {
    id: '',
    collectionId,
    parentId,
    name: 'New request',
    method: 'GET',
    url: '{{base_url}}/',
    headers: [],
    query: [],
    body: '',
    bodyType: 'none',
    auth: { type: 'inherit' },
    timeoutMs: 30000,
    followRedirects: true,
    sortOrder: 0,
    assertions: [newAssertion('status')],
    preRequestScript: '',
    postResponseScript: '',
  };
}

/** Operators allowed per kind; must match internal/domain/assertion.go. */
export const OPERATORS: Record<AssertionKind, Operator[]> = {
  status: ['eq', 'neq', 'gt', 'gte', 'lt', 'lte'],
  header: ['exists', 'not_exists', 'eq', 'neq', 'contains', 'not_contains', 'matches'],
  json_path: [
    'exists',
    'not_exists',
    'eq',
    'neq',
    'gt',
    'gte',
    'lt',
    'lte',
    'contains',
    'not_contains',
    'matches',
    'type',
  ],
  response_time: ['lt', 'lte'],
  valid_json: [''],
  required_fields: [''],
  body: ['contains', 'not_contains', 'matches', 'eq'],
  aggregate: ['eq', 'neq', 'gt', 'gte', 'lt', 'lte'],
};

export const KIND_LABELS: Record<AssertionKind, string> = {
  status: 'Status code',
  header: 'Header',
  json_path: 'JSON field',
  response_time: 'Response time (ms)',
  valid_json: 'Body is valid JSON',
  required_fields: 'Required fields',
  body: 'Body text',
  aggregate: 'Aggregate (count/sum/min/max)',
};

export const OPERATOR_LABELS: Record<Operator, string> = {
  '': '',
  exists: 'exists',
  not_exists: 'does not exist',
  eq: 'equals',
  neq: 'not equals',
  gt: '>',
  gte: '≥',
  lt: '<',
  lte: '≤',
  contains: 'contains',
  not_contains: 'does not contain',
  matches: 'matches regex',
  type: 'has type',
};

export function newAssertion(kind: AssertionKind): Assertion {
  const defaults: Record<AssertionKind, Partial<Assertion>> = {
    status: { operator: 'eq', expected: 200 },
    header: { operator: 'exists', path: 'Content-Type', expected: null },
    json_path: { operator: 'exists', path: 'data', expected: null },
    response_time: { operator: 'lt', expected: 1000 },
    valid_json: { operator: '', expected: null },
    required_fields: { operator: '', expected: ['data'] },
    body: { operator: 'contains', expected: '' },
    aggregate: { operator: 'eq', path: 'items', function: 'count', expected: 0 },
  };
  return {
    id: uid(),
    kind,
    path: '',
    tolerance: 0,
    enabled: true,
    expected: null,
    operator: 'eq',
    ...defaults[kind],
  } as Assertion;
}

export function needsExpected(a: Assertion): boolean {
  return a.operator !== 'exists' && a.operator !== 'not_exists' && a.kind !== 'valid_json';
}

export function needsPath(kind: AssertionKind): boolean {
  return kind === 'header' || kind === 'json_path' || kind === 'aggregate';
}

/**
 * Converts the text a user typed into a JSON value. Valid JSON is used as-is
 * (numbers, booleans, objects, quoted strings); anything else is a string.
 */
export function parseExpected(text: string): unknown {
  const t = text.trim();
  if (t === '') return '';
  try {
    return JSON.parse(t);
  } catch {
    return text;
  }
}

/** Inverse of parseExpected for display in a text input. */
export function formatExpected(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') {
    // Show strings unquoted unless they would otherwise parse as another type.
    try {
      JSON.parse(v);
      return JSON.stringify(v);
    } catch {
      return v;
    }
  }
  return JSON.stringify(v);
}

export function isDestructive(method: string): boolean {
  return ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
}

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

/** Appends an object key to a path using the same syntax as the Go engine. */
export function joinKey(path: string, key: string): string {
  if (/^[A-Za-z0-9_-]+$/.test(key)) return path ? `${path}.${key}` : key;
  return `${path}[${JSON.stringify(key)}]`;
}

/** Lists JSON leaf paths of a document, used to pick baseline paths. */
export function jsonPaths(doc: unknown, max = 500): string[] {
  const out: string[] = [];
  const walk = (v: unknown, path: string) => {
    if (out.length >= max) return;
    if (Array.isArray(v)) {
      if (v.length === 0 && path) out.push(path);
      v.forEach((item, i) => walk(item, `${path}[${i}]`));
    } else if (v && typeof v === 'object') {
      const entries = Object.entries(v as Record<string, unknown>);
      if (entries.length === 0 && path) out.push(path);
      for (const [k, child] of entries) walk(child, joinKey(path, k));
    } else if (path) {
      out.push(path);
    }
  };
  walk(doc, '');
  return out;
}
