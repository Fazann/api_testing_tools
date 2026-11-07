import { describe, expect, it } from 'vitest';

import type { Request } from '@/types';

import { CODE_LANGUAGES, generateCode } from './codegen';
import { newRequest } from './model';

const req = (o: Partial<Request>): Request => ({ ...newRequest('c', ''), ...o });

const post = req({
  method: 'POST',
  url: '{{base_url}}/orders',
  headers: [{ key: 'X-Trace', value: 'a"b$c', enabled: true }],
  query: [{ key: 'dry run', value: '1', enabled: true }],
  bodyType: 'json',
  body: '{"amount": 10, "ok": true, "note": null}',
  auth: { type: 'inherit' },
  timeoutMs: 2500,
});
const opts = { collectionAuth: { type: 'bearer' as const, token: '{{token}}' } };

describe('generateCode', () => {
  it('produces code for every language with the same URL, method, headers and auth', () => {
    for (const { value } of CODE_LANGUAGES) {
      const code = generateCode(value, post, opts);
      expect(code, value).toContain('{{base_url}}/orders?dry%20run=1');
      // curl leaves out -X POST because --data-raw implies it; axios writes the method in lower case.
      if (value !== 'curl') expect(code.toUpperCase(), value).toContain('POST');
      expect(code, value).toContain('Bearer {{token}}');
      expect(code, value).toContain('X-Trace');
    }
  });

  it('writes JSON bodies idiomatically', () => {
    expect(generateCode('fetch', post, opts)).toContain('body: JSON.stringify({\n    "amount": 10,');
    expect(generateCode('axios', post, opts)).toContain('data: {\n    "amount": 10,');
    const py = generateCode('python', post, opts);
    expect(py).toContain('payload = {\n    "amount": 10,\n    "ok": True,\n    "note": None,\n}');
    expect(py).toContain('json=payload,');
    expect(py).not.toContain('"Content-Type": "application/json"'); // requests sets it for json=
    expect(py).toContain('timeout=2.5,');
    expect(generateCode('go', post, opts)).toContain('strings.NewReader(`{\n  "amount": 10,');
  });

  it('keeps bodies with unresolved variables as text', () => {
    const r = { ...post, body: '{"amount": {{amount}}}' };
    expect(generateCode('fetch', r, opts)).toContain('body: "{\\"amount\\": {{amount}}}",');
    expect(generateCode('python', r, opts)).toContain('data="{\\"amount\\": {{amount}}}".encode("utf-8"),');
  });

  it('escapes strings for each language', () => {
    const r = req({ method: 'PUT', url: 'http://x/a', bodyType: 'text', body: 'a$b\n\tc\u0007"d\'\\e??=' });
    expect(generateCode('php', req({ ...r, body: "it's $x \\ ok" }))).toContain(
      `CURLOPT_POSTFIELDS => 'it\\'s $x \\\\ ok',`,
    );
    expect(generateCode('php', r)).toContain(`CURLOPT_POSTFIELDS => "a\\$b\\n\\tc\\x07\\"d'\\\\e??=",`);
    expect(generateCode('dart', r)).toContain(`request.body = "a\\$b\\n\\tc\\u0007\\"d'\\\\e??=";`);
    expect(generateCode('swift', r)).toContain(`Data("a$b\\n\\tc\\u{7}\\"d'\\\\e??=".utf8)`);
    expect(generateCode('rust', r)).toContain(`.body("a$b\\n\\tc\\u{7}\\"d'\\\\e??=")`);
    expect(generateCode('cpp', r)).toContain(`std::string body = "a$b\\n\\tc\\007\\"d'\\\\e?\\?=";`);
    expect(generateCode('objc', r)).toContain(
      `[@"a$b\\n\\tc\\007\\"d'\\\\e?\\?=" dataUsingEncoding:NSUTF8StringEncoding]`,
    );
    expect(generateCode('java', r)).toContain(`String body = "a$b\\n\\tc\\u0007\\"d'\\\\e??=";`);
    expect(generateCode('csharp', r)).toContain(`new StringContent("a$b\\n\\tc\\u0007\\"d'\\\\e??=", Encoding.UTF8)`);
  });

  it('turns off redirects and handles basic auth natively where the language can', () => {
    const r = req({
      method: 'GET',
      url: 'http://x/a',
      followRedirects: false,
      auth: { type: 'basic', username: 'alice', password: 'pw' },
    });
    expect(generateCode('fetch', r)).toContain(`redirect: 'manual'`);
    expect(generateCode('fetch', r)).toContain('"Authorization": "Basic YWxpY2U6cHc="');
    expect(generateCode('axios', r)).toContain('maxRedirects: 0,');
    expect(generateCode('axios', r)).toContain('auth: { username: "alice", password: "pw" },');
    expect(generateCode('python', r)).toContain('allow_redirects=False,');
    expect(generateCode('python', r)).toContain('auth=("alice", "pw"),');
    expect(generateCode('go', r)).toContain('http.ErrUseLastResponse');
    expect(generateCode('go', r)).toContain('req.SetBasicAuth("alice", "pw")');
    expect(generateCode('php', r)).toContain('CURLOPT_FOLLOWLOCATION => false,');
    expect(generateCode('php', r)).toContain(`CURLOPT_USERPWD => 'alice:pw',`);
    expect(generateCode('java', r)).toContain('HttpClient.Redirect.NEVER');
    expect(generateCode('csharp', r)).toContain('AllowAutoRedirect = false');
    expect(generateCode('dart', r)).toContain('..followRedirects = false;');
    expect(generateCode('swift', r)).toContain('willPerformHTTPRedirection');
    expect(generateCode('objc', r)).toContain('completionHandler(nil);');
    expect(generateCode('cpp', r)).toContain('CURLOPT_FOLLOWLOCATION, 0L');
    expect(generateCode('rust', r)).toContain('reqwest::redirect::Policy::none()');
    expect(generateCode('rust', r)).toContain('.basic_auth("alice", Some("pw"))');
  });

  it('leaves out headers Java HttpClient refuses and moves .NET content headers', () => {
    const r = req({
      method: 'POST',
      url: 'http://x/a',
      bodyType: 'text',
      body: 'x',
      headers: [
        { key: 'Host', value: 'api', enabled: true },
        { key: 'Content-Language', value: 'en', enabled: true },
      ],
    });
    const java = generateCode('java', r);
    expect(java).not.toContain('.header("Host"');
    expect(java).toContain('// Set by HttpClient itself: Host');
    const cs = generateCode('csharp', r);
    expect(cs).toContain('request.Headers.TryAddWithoutValidation("Host", "api");');
    expect(cs).toContain('request.Content.Headers.TryAddWithoutValidation("Content-Language", "en");');
  });

  it('uses HEAD-specific options', () => {
    const r = req({ method: 'HEAD', url: 'http://x/a' });
    expect(generateCode('php', r)).toContain('CURLOPT_NOBODY => true,');
    expect(generateCode('cpp', r)).toContain('CURLOPT_NOBODY, 1L');
    expect(generateCode('rust', r)).toContain('reqwest::Method::HEAD');
    expect(generateCode('rust', req({ method: 'PROPFIND' as never, url: 'http://x' }))).toContain(
      'reqwest::Method::from_bytes("PROPFIND".as_bytes())?',
    );
  });
});

describe('file uploads', () => {
  const upload = req({
    method: 'POST',
    url: 'https://x.io/upload',
    headers: [{ key: 'Content-Type', value: 'multipart/form-data', enabled: true }],
    bodyType: 'form-data',
    form: [
      { key: 'title', value: 'Holiday', type: 'text', enabled: true },
      { key: 'photo', value: '', type: 'file', file: '/data/beach.png', enabled: true },
      { key: 'doc', value: '', type: 'file', file: '/data/notes.bin', contentType: 'text/markdown', enabled: true },
    ],
    auth: { type: 'none' },
  });
  const binary = req({
    method: 'PUT',
    url: 'https://x.io/blob',
    bodyType: 'binary',
    bodyFile: '/data/archive.zip',
    auth: { type: 'none' },
  });

  it('sends every form field and file in every language, with the library writing the boundary', () => {
    for (const { value } of CODE_LANGUAGES) {
      const code = generateCode(value, upload);
      expect(code, value).toContain('Holiday');
      expect(code, value).toContain('/data/beach.png');
      expect(code, value).toContain('/data/notes.bin');
      expect(code, value).toContain('text/markdown');
      // The user's bare multipart Content-Type would lack the boundary; it is never sent as is.
      expect(code, value).not.toMatch(/multipart\/form-data["']/);
    }
  });

  it('uploads the file as the body for binary requests', () => {
    for (const { value } of CODE_LANGUAGES) {
      const code = generateCode(value, binary);
      expect(code, value).toContain('/data/archive.zip');
      expect(code, value).toContain('application/zip');
      expect(code, value).not.toContain('{{');
    }
  });

  it('uses each language’s multipart helper', () => {
    expect(generateCode('fetch', upload)).toContain("import { openAsBlob } from 'node:fs';");
    expect(generateCode('fetch', upload)).toContain('body: form,');
    expect(generateCode('python', upload)).toContain('("title", (None, "Holiday")),');
    expect(generateCode('python', upload)).toContain(
      '("photo", ("beach.png", open("/data/beach.png", "rb"), "image/png")),',
    );
    expect(generateCode('go', upload)).toContain('req.Header.Set("Content-Type", writer.FormDataContentType())');
    expect(generateCode('php', upload)).toContain("new CURLFile('/data/beach.png', 'image/png', 'beach.png')");
    expect(generateCode('csharp', upload)).toContain('form.Add(file2, "doc", "notes.bin");');
    expect(generateCode('dart', upload)).toContain('http.MultipartRequest(');
    expect(generateCode('cpp', upload)).toContain('curl_mime_filedata(part, "/data/beach.png");');
    expect(generateCode('rust', upload)).toContain('features = ["blocking", "multipart"]');
    for (const lang of ['java', 'swift', 'objc'] as const) {
      expect(generateCode(lang, upload), lang).toContain('multipart/form-data; boundary=');
    }
  });

  it('encodes URL-encoded forms as the body text', () => {
    const form = req({
      method: 'POST',
      url: 'https://x.io',
      bodyType: 'urlencoded',
      form: [{ key: 'q', value: 'a b&c', type: 'text', enabled: true }],
      auth: { type: 'none' },
    });
    expect(generateCode('python', form)).toContain('data="q=a%20b%26c".encode("utf-8")');
    expect(generateCode('python', form)).toContain('"Content-Type": "application/x-www-form-urlencoded"');
  });
});
