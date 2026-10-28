import { describe, expect, it } from 'vitest';

import { formatExpected, joinKey, jsonPaths, parseExpected, prettyBody } from './model';

describe('expected values', () => {
  it('parses JSON and falls back to strings', () => {
    expect(parseExpected('200')).toBe(200);
    expect(parseExpected('true')).toBe(true);
    expect(parseExpected('{"$path": "total"}')).toEqual({ $path: 'total' });
    expect(parseExpected('USD')).toBe('USD');
    expect(parseExpected('"200"')).toBe('200');
  });

  it('round-trips through formatExpected', () => {
    for (const v of [200, 'USD', '200', { $path: 'a' }, true]) {
      expect(parseExpected(formatExpected(v))).toEqual(v);
    }
  });
});

describe('json paths', () => {
  it('uses the same syntax as the Go engine', () => {
    expect(joinKey('', 'data')).toBe('data');
    expect(joinKey('data', 'request-id')).toBe('data.request-id');
    expect(joinKey('meta', 'a.b')).toBe('meta["a.b"]');
  });

  it('lists leaf paths', () => {
    expect(jsonPaths({ data: { items: [{ id: 1 }], empty: [] }, ok: true })).toEqual([
      'data.items[0].id',
      'data.empty',
      'ok',
    ]);
  });
});

describe('prettyBody', () => {
  it('formats JSON and leaves other text alone', () => {
    expect(prettyBody('{"a":1}')).toBe('{\n  "a": 1\n}');
    expect(prettyBody('<html>')).toBe('<html>');
  });
});
