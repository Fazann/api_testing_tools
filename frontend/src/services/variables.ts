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
}

/** Describes how a reference resolves in env, mirroring the Go resolver. */
export function describeVariable(name: string, env: Environment | null): VariableInfo {
  if (name in BUILTINS) return { name, state: 'builtin' };
  const variable = env?.variables.find((v) => v.key === name);
  if (!variable) return { name, state: 'missing' };
  if (!variable.enabled) return { name, state: 'disabled', variable };
  return { name, state: variable.value === '' ? 'empty' : 'defined', variable };
}
