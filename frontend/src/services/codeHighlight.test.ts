import { describe, expect, it } from 'vitest';

import type { Request } from '@/types';

import { CODE_LANGUAGES, generateCode, type CodeLanguage } from './codegen';
import { highlightCode } from './codeHighlight';
import type { Token } from './highlight';
import { newRequest } from './model';

const post: Request = {
  ...newRequest('c', ''),
  method: 'POST',
  url: '{{base_url}}/orders',
  headers: [{ key: 'X-Note', value: 'it\'s "quoted" $x', enabled: true }],
  bodyType: 'json',
  body: '{"amount": 10, "ok": true, "note": null}',
  auth: { type: 'bearer', token: '{{token}}' },
  followRedirects: false,
  timeoutMs: 2500,
};

const tokens = (lang: CodeLanguage) => highlightCode(lang, generateCode(lang, post));
/** The kind of the first token whose text is exactly text. */
const kindOf = (t: Token[], text: string) => t.find((x) => x.text === text)?.kind;
/** The kind of the first token that contains text. */
const kindWith = (t: Token[], text: string) => t.find((x) => x.text.includes(text))?.kind;

describe('highlightCode', () => {
  it('keeps every character of every language and colours strings and placeholders', () => {
    for (const { value } of CODE_LANGUAGES) {
      const code = generateCode(value, post);
      const t = highlightCode(value, code);
      expect(t.map((x) => x.text).join(''), value).toBe(code);
      expect(kindOf(t, '{{token}}'), value).toBe('template');
      expect(kindWith(t, '/orders'), value).toBe('string');
      // A header value with quotes and $ stays one string, so nothing after it is mis-coloured.
      expect(kindWith(t, 'quoted'), value).toBe('string');
    }
  });

  it('colours each language with its own keywords, comments and calls', () => {
    const curl = tokens('curl');
    expect(kindOf(curl, 'curl')).toBe('function');
    expect(kindOf(curl, '--data-raw')).toBe('keyword');

    const py = tokens('python');
    expect(kindOf(py, 'import')).toBe('control');
    expect(kindOf(py, 'True')).toBe('keyword');
    expect(kindOf(py, 'request')).toBe('function');

    const go = tokens('go');
    expect(kindOf(go, 'package')).toBe('keyword');
    expect(kindOf(go, 'func')).toBe('keyword');

    const php = tokens('php');
    expect(kindOf(php, '$curl')).toBe('variable');

    const java = tokens('java');
    expect(kindOf(java, 'import')).toBe('control');
    expect(kindOf(java, 'HttpClient')).toBe('type');

    const cpp = tokens('cpp');
    expect(kindOf(cpp, '#include')).toBe('control');
    expect(kindOf(cpp, '<curl/curl.h>')).toBe('string');
    expect(kindWith(cpp, '// Build')).toBe('comment');

    const objc = tokens('objc');
    expect(kindWith(objc, '@"')).toBe('string');
    expect(kindOf(objc, 'NSURL')).toBe('type');
    expect(kindOf(objc, '@autoreleasepool')).toBe('keyword');

    const cs = tokens('csharp');
    expect(kindOf(cs, 'HttpClient')).toBe('type'); // new HttpClient(…)
    expect(kindOf(cs, 'SendAsync')).toBe('function');

    const rust = tokens('rust');
    expect(kindOf(rust, 'println!')).toBe('function');
    expect(kindOf(rust, 'fn')).toBe('keyword');

    const swift = tokens('swift');
    expect(kindOf(swift, 'let')).toBe('keyword');
    expect(kindWith(swift, '/// Returns')).toBe('comment');

    expect(kindOf(tokens('fetch'), 'await')).toBe('control');
  });

  it('handles shell single quotes, which have no escapes', () => {
    const t = highlightCode('curl', "curl -H 'it'\\''s' https://h");
    expect(t.filter((x) => x.kind === 'string').map((x) => x.text)).toEqual(["'it'", "'s'", 'https://h']);
    expect(kindOf(t, "\\'")).toBe('punct');
  });

  it('reads Go raw strings, Python triple quotes and C# verbatim strings', () => {
    expect(kindOf(highlightCode('go', 'x := `a\n"b`'), '`a\n"b`')).toBe('string');
    expect(kindOf(highlightCode('python', 's = """a\n"b"\n"""'), '"""a\n"b"\n"""')).toBe('string');
    expect(kindOf(highlightCode('csharp', 'var s = @"a""b";'), '@"a""b"')).toBe('string');
  });
});
