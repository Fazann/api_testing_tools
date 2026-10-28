import type { Environment, Variable } from '../types';

// Must match referencePattern in internal/environments/resolver.go.
const REFERENCE = /\{\{\s*(\$?[A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}/g;

export interface Segment {
  text: string;
  /** Variable name when this segment is a {{reference}}. */
  name?: string;
  start: number;
  end: number;
}

/** Splits text into plain segments and {{variable}} references. */
export function splitVariables(text: string): Segment[] {
  const out: Segment[] = [];
  let last = 0;
  for (const m of text.matchAll(REFERENCE)) {
    const start = m.index ?? 0;
    if (start > last) out.push({ text: text.slice(last, start), start: last, end: start });
    out.push({ text: m[0], name: m[1], start, end: start + m[0].length });
    last = start + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last), start: last, end: text.length });
  return out;
}

export function hasVariables(text: string | undefined): boolean {
  return !!text && splitVariables(text).some((s) => s.name);
}

export const BUILTINS: Record<string, string> = {
  $uuid: 'A new random UUID for every request',
  $timestamp: 'Current Unix time in seconds',
  $isoTimestamp: 'Current UTC time in ISO 8601 format',
};

export type VariableState = 'builtin' | 'defined' | 'empty' | 'disabled' | 'missing';

export interface VariableInfo {
  name: string;
  state: VariableState;
  variable?: Variable;
  /** Where the variable is defined: the selected environment or the request's collection. */
  source?: 'environment' | 'collection';
}

/**
 * Describes how a reference resolves, mirroring the Go resolver: an enabled environment variable wins, then an
 * enabled collection variable.
 */
export function describeVariable(name: string, env: Environment | null, collectionVars: Variable[] = []): VariableInfo {
  if (name in BUILTINS) return { name, state: 'builtin' };
  const fromEnv = env?.variables.find((v) => v.key === name);
  const fromCollection = collectionVars.find((v) => v.key === name);
  const found = (variable: Variable, source: VariableInfo['source']): VariableInfo => ({
    name,
    state: !variable.enabled ? 'disabled' : variable.value === '' ? 'empty' : 'defined',
    variable,
    source,
  });
  if (fromEnv?.enabled) return found(fromEnv, 'environment');
  if (fromCollection?.enabled) return found(fromCollection, 'collection');
  if (fromEnv) return found(fromEnv, 'environment');
  if (fromCollection) return found(fromCollection, 'collection');
  return { name, state: 'missing' };
}
