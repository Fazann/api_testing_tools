import { describe, expect, it } from 'vitest';

import { apiVersion, compareVersions, formatExpected, joinKey, jsonPaths, parseExpected, prettyBody } from './model';

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

describe('apiVersion', () => {
  it('finds a version path segment', () => {
    expect(apiVersion('{{base_url}}/v1/orders')).toBe('v1');
    expect(apiVersion('https://api.example.com/api/V2/users/7?x=1')).toBe('v2');
    expect(apiVersion('/api/v2.1/items')).toBe('v2.1');
    expect(apiVersion('/v3')).toBe('v3');
  });

  it('ignores look-alikes in the host, names and other segments', () => {
    expect(apiVersion('https://v1.example.com/orders')).toBe('');
    expect(apiVersion('/videos/v1beta/list')).toBe('');
    expect(apiVersion('/movies/vote')).toBe('');
    expect(apiVersion('{{base_url}}/health')).toBe('');
  });

  it('falls back to a version query parameter, from the URL or the Query table', () => {
    expect(apiVersion('/orders?api-version=2')).toBe('v2');
    expect(apiVersion('/orders?api-version=2024-01-01')).toBe('2024-01-01');
    expect(apiVersion('/orders', [{ key: 'version', value: '3', enabled: true }])).toBe('v3');
    expect(apiVersion('/orders', [{ key: 'version', value: '3', enabled: false }])).toBe('');
    expect(apiVersion('/orders?version={{ver}}')).toBe('');
    expect(apiVersion('/v1/orders?api-version=9')).toBe('v1');
  });

  it('fills placeholders from variables', () => {
    expect(apiVersion('{{base_url}}/orders', [], { base_url: 'https://x.test/api/v2' })).toBe('v2');
    expect(apiVersion('{{base_url}}/{{ver}}/orders', [], { ver: 'v4' })).toBe('v4');
  });

  it('sorts versions naturally', () => {
    expect(['v10', 'v2', 'v1.1', 'v1'].sort(compareVersions)).toEqual(['v1', 'v1.1', 'v2', 'v10']);
  });
});
