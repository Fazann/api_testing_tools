import { describe, expect, it } from 'vitest';

import type { Environment, Request } from '@/types';

import { CurlParseError, looksLikeCurl, parseCurl, shellQuote, shellSplit, toCurl } from './curl';
import { newRequest } from './model';

describe('shellSplit', () => {
  it('handles quotes, escapes and line continuations', () => {
    expect(shellSplit(`curl 'a b' "c \\"d\\"" e\\ f \\\n  -H x`)).toEqual(['curl', 'a b', 'c "d"', 'e f', '-H', 'x']);
    expect(shellSplit(`curl 'it'\\''s'`)).toEqual(['curl', "it's"]);
    expect(shellSplit(`curl \\\r\n -s`)).toEqual(['curl', '-s']);
  });

  it("decodes bash $'…' quoting as emitted by browsers", () => {
    expect(shellSplit(`curl $'{"a":"it\\'s\\n"}'`)).toEqual(['curl', `{"a":"it's\n"}`]);
    expect(shellSplit(`curl $'caf\\xc3\\xa9 \\u00e9'`)).toEqual(['curl', 'café é']);
  });

  it('rejects unterminated quotes', () => {
    expect(() => shellSplit(`curl 'abc`)).toThrow(CurlParseError);
    expect(() => shellSplit(`curl "abc`)).toThrow(CurlParseError);
  });
});

describe('parseCurl', () => {
  it('parses a simple GET', () => {
    const { request, warnings } = parseCurl('curl https://api.example.com/v1/users?page=2');
    expect(request.method).toBe('GET');
    expect(request.url).toBe('https://api.example.com/v1/users?page=2');
    expect(request.name).toBe('GET /v1/users');
    expect(request.bodyType).toBe('none');
    expect(request.auth.type).toBe('inherit');
    expect(warnings).toEqual([]);
  });

  it('parses a browser "Copy as cURL" POST with JSON', () => {
    const cmd = `curl 'https://api.example.com/orders' \\
  -H 'accept: application/json' \\
  -H 'content-type: application/json' \\
  -H 'authorization: Bearer abc.def' \\
  --data-raw '{"amount":10}' \\
  --compressed`;
    const { request, warnings } = parseCurl(cmd);
    expect(request.method).toBe('POST');
    expect(request.bodyType).toBe('json');
    expect(request.body).toBe('{"amount":10}');
    expect(request.auth).toEqual({ type: 'bearer', token: 'abc.def' });
    expect(request.headers.map((h) => h.key)).toEqual(['accept', 'content-type']);
    expect(warnings).toEqual([]);
  });

  it('supports combined and attached short options', () => {
    const { request } = parseCurl(`curl -sSL -XPUT -H'X-A: 1' -d 'x=1' -d y=2 example.com/a`);
    expect(request.method).toBe('PUT');
    expect(request.url).toBe('http://example.com/a');
    expect(request.body).toBe('x=1&y=2');
    expect(request.bodyType).toBe('text');
    expect(request.headers).toContainEqual({ key: 'X-A', value: '1', enabled: true });
    expect(request.headers).toContainEqual({
      key: 'Content-Type',
      value: 'application/x-www-form-urlencoded',
      enabled: true,
    });
  });

  it('maps -u to basic auth and decodes Basic headers', () => {
    expect(parseCurl('curl -u alice:s3:cret https://x.io').request.auth).toEqual({
      type: 'basic',
      username: 'alice',
      password: 's3:cret',
    });
    const header = `Authorization: Basic ${btoa('bob:pw')}`;
    expect(parseCurl(`curl -H '${header}' https://x.io`).request.auth).toEqual({
      type: 'basic',
      username: 'bob',
      password: 'pw',
    });
  });

  it('keeps unknown Authorization schemes as a header', () => {
    const { request } = parseCurl(`curl -H 'Authorization: Digest abc' https://x.io`);
    expect(request.auth.type).toBe('none');
    expect(request.headers).toEqual([{ key: 'Authorization', value: 'Digest abc', enabled: true }]);
  });

  it('handles --json, -G, -I, --data-urlencode and -m', () => {
    const j = parseCurl(`curl --json '{"a":1}' https://x.io`).request;
    expect(j.method).toBe('POST');
    expect(j.bodyType).toBe('json');
    expect(j.headers.map((h) => h.key)).toEqual(['Content-Type', 'Accept']);

    const g = parseCurl(`curl -G --data-urlencode 'q=a b&c' -d n=1 https://x.io/s`).request;
    expect(g.method).toBe('GET');
    expect(g.url).toBe('https://x.io/s?q=a%20b%26c&n=1');
    expect(g.bodyType).toBe('none');

    expect(parseCurl('curl -I https://x.io').request.method).toBe('HEAD');
    expect(parseCurl('curl -m 2.5 https://x.io').request.timeoutMs).toBe(2500);
  });

  it('turns cookies, user agent and referer into headers', () => {
    const { request } = parseCurl(`curl -b 'sid=1; x=2' -A agent/1 -e https://ref https://x.io`);
    expect(request.headers).toEqual([
      { key: 'Cookie', value: 'sid=1; x=2', enabled: true },
      { key: 'User-Agent', value: 'agent/1', enabled: true },
      { key: 'Referer', value: 'https://ref', enabled: true },
    ]);
  });

  it('warns about what it cannot represent', () => {
    const { warnings } = parseCurl(`curl -k -F file=@a.png --frobnicate https://x.io`);
    expect(warnings).toHaveLength(3);
    expect(warnings.join(' ')).toMatch(/insecure/);
    expect(warnings.join(' ')).toMatch(/choose the files again/);
    expect(warnings.join(' ')).toMatch(/--frobnicate/);
  });

  it('reads -F and --form-string as a form-data body', () => {
    const { request } = parseCurl(
      `curl https://x.io/upload -H 'Content-Type: multipart/form-data' ` +
        `-F 'avatar=@"/tmp/my photo.png";type=image/png' -F name=Ann --form-string 'note=@literal'`,
    );
    expect(request.method).toBe('POST');
    expect(request.bodyType).toBe('form-data');
    expect(request.headers).toEqual([]);
    expect(request.form).toEqual([
      { key: 'avatar', value: '', type: 'file', file: '/tmp/my photo.png', contentType: 'image/png', enabled: true },
      { key: 'name', value: 'Ann', type: 'text', enabled: true },
      { key: 'note', value: '@literal', type: 'text', enabled: true },
    ]);
  });

  it('reads --data-binary @file and -T as a file body', () => {
    const bin = parseCurl(`curl https://x.io --data-binary @/tmp/a.bin`).request;
    expect(bin).toMatchObject({ method: 'POST', bodyType: 'binary', bodyFile: '/tmp/a.bin', body: '' });
    const put = parseCurl(`curl -T /tmp/a.bin https://x.io/a.bin`).request;
    expect(put).toMatchObject({ method: 'PUT', bodyType: 'binary', bodyFile: '/tmp/a.bin' });
    // -d @file still keeps the old behavior (curl strips newlines there, so it is not a raw upload).
    expect(parseCurl(`curl -d @a.json https://x.io`).request.bodyType).not.toBe('binary');
  });

  it('writes form-data, URL-encoded and file bodies', () => {
    const base = {
      method: 'POST' as const,
      url: 'https://x.io',
      headers: [{ key: 'Content-Type', value: 'multipart/form-data', enabled: true }],
      query: [],
      body: 'ignored',
      auth: { type: 'none' as const },
      timeoutMs: 30000,
      followRedirects: false,
    };
    const form = [
      { key: 'name', value: '@Ann', type: 'text' as const, enabled: true },
      { key: 'avatar', value: '', type: 'file' as const, file: '/tmp/a "b".png', enabled: true },
      { key: 'off', value: 'x', type: 'text' as const, enabled: false },
    ];
    expect(toCurl({ ...base, bodyType: 'form-data', form })).toBe(
      [`curl https://x.io`, `  --form-string name=@Ann`, `  -F 'avatar=@"/tmp/a \\"b\\".png";type=image/png'`].join(
        ' \\\n',
      ),
    );
    expect(toCurl({ ...base, headers: [], bodyType: 'urlencoded', form })).toBe(
      [`curl https://x.io`, `  -H 'Content-Type: application/x-www-form-urlencoded'`, `  --data-raw name=%40Ann`].join(
        ' \\\n',
      ),
    );
    expect(toCurl({ ...base, headers: [], method: 'PUT', bodyType: 'binary', bodyFile: '/tmp/a.pdf' })).toBe(
      [`curl -X PUT`, `  https://x.io`, `  -H 'Content-Type: application/pdf'`, `  --data-binary @/tmp/a.pdf`].join(
        ' \\\n',
      ),
    );
  });

  it('rejects non-curl input and missing URLs', () => {
    expect(() => parseCurl('wget https://x.io')).toThrow(CurlParseError);
    expect(() => parseCurl('curl -s')).toThrow(/No URL/);
    expect(() => parseCurl('curl -H')).toThrow(/needs a value/);
  });

  it('keeps {{variables}} in URLs', () => {
    expect(parseCurl('curl {{base_url}}/users').request.url).toBe('{{base_url}}/users');
  });

  it('accepts a shell prompt prefix', () => {
    expect(looksLikeCurl('$ curl https://x.io')).toBe(true);
    expect(looksLikeCurl('curling')).toBe(false);
    expect(parseCurl('$ curl https://x.io').request.url).toBe('https://x.io');
  });
});

function req(over: Partial<Request>): Request {
  return { ...newRequest('c'), auth: { type: 'none' }, ...over };
}

describe('toCurl', () => {
  it('quotes for the shell', () => {
    expect(shellQuote('simple/path')).toBe('simple/path');
    expect(shellQuote("it's here")).toBe(`'it'\\''s here'`);
    expect(shellQuote('')).toBe(`''`);
  });

  it('mirrors what the runner sends', () => {
    const out = toCurl(
      req({
        method: 'POST',
        url: '{{base_url}}/orders',
        headers: [
          { key: 'X-Trace', value: '1', enabled: true },
          { key: 'X-Off', value: '1', enabled: false },
        ],
        query: [{ key: 'dry run', value: '{{flag}}', enabled: true }],
        bodyType: 'json',
        body: '{"amount": 10}',
        auth: { type: 'inherit' },
      }),
      { collectionAuth: { type: 'bearer', token: '{{token}}' } },
    );
    expect(out).toBe(
      [
        `curl '{{base_url}}/orders?dry%20run={{flag}}'`,
        `-H 'X-Trace: 1'`,
        `-H 'Content-Type: application/json'`,
        `-H 'Authorization: Bearer {{token}}'`,
        `--data-raw '{"amount": 10}'`,
        `-L`,
      ].join(' \\\n  '),
    );
  });

  it('resolves variables but keeps secrets unless asked', () => {
    const env: Environment = {
      id: 'e',
      name: 'dev',
      protected: false,
      variables: [
        { key: 'base_url', value: 'https://dev.x.io', secret: false, enabled: true },
        { key: 'token', value: 'SECRET', secret: true, enabled: true },
      ],
    };
    const r = req({ url: '{{base_url}}/me', auth: { type: 'bearer', token: '{{token}}' } });
    const resolve = { environment: env, collectionVariables: [], includeSecrets: false };
    const masked = toCurl(r, { resolve });
    expect(masked).toContain(`curl https://dev.x.io/me`);
    expect(masked).toContain('Bearer {{token}}');
    expect(toCurl(r, { resolve: { ...resolve, includeSecrets: true } })).toContain('Bearer SECRET');
  });

  it('uses the right method flags', () => {
    expect(toCurl(req({ method: 'DELETE' }))).toMatch(/^curl -X DELETE /);
    expect(toCurl(req({ method: 'HEAD' }))).toMatch(/^curl --head /);
    expect(toCurl(req({ method: 'GET', bodyType: 'json', body: '{}' }))).not.toContain('--data');
    expect(toCurl(req({ method: 'PUT', bodyType: 'text', body: 'x' }))).toMatch(/^curl -X PUT /);
    expect(toCurl(req({ timeoutMs: 1500, followRedirects: false }))).toMatch(/--max-time 1\.5$/);
  });

  it('puts api keys in the query or a header', () => {
    const q = toCurl(
      req({ url: 'https://x.io/a?b=1', auth: { type: 'apikey', keyIn: 'query', keyName: 'k', keyValue: 'v' } }),
    );
    expect(q).toContain(`'https://x.io/a?b=1&k=v'`);
    const h = toCurl(req({ auth: { type: 'apikey', keyIn: 'header', keyName: 'X-Key', keyValue: 'v' } }));
    expect(h).toContain(`-H 'X-Key: v'`);
  });

  it('round-trips through parseCurl', () => {
    const original = req({
      method: 'PATCH',
      url: 'https://x.io/items/1',
      headers: [{ key: 'X-Note', value: "it's", enabled: true }],
      bodyType: 'json',
      body: '{\n  "name": "a \'b\'"\n}',
      auth: { type: 'basic', username: 'u', password: 'p w' },
      timeoutMs: 5000,
    });
    const { request, warnings } = parseCurl(toCurl(original));
    expect(warnings).toEqual([]);
    expect(request.method).toBe('PATCH');
    expect(request.url).toBe(original.url);
    expect(request.body).toBe(original.body);
    expect(request.bodyType).toBe('json');
    expect(request.auth).toEqual(original.auth);
    expect(request.timeoutMs).toBe(5000);
    expect(request.headers).toEqual([
      { key: 'X-Note', value: "it's", enabled: true },
      { key: 'Content-Type', value: 'application/json', enabled: true },
    ]);
  });
});
