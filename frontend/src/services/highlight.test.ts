import { describe, expect, it } from 'vitest';

import { highlightJs, highlightJson, type Token } from './highlight';

/** Non-whitespace tokens as "kind:text". */
const show = (tokens: Token[]) => tokens.filter((t) => t.text.trim()).map((t) => `${t.kind}:${t.text}`);
const kindOf = (tokens: Token[], text: string) => tokens.find((t) => t.text === text)?.kind;

describe('highlightJson', () => {
  it('tells keys from string values and classifies literals', () => {
    const t = highlightJson('{\n  "id": "a:1",\n  "n": -1.5e3,\n  "ok": true,\n  "x": null,\n  "list": [1, "s"]\n}');
    expect(kindOf(t, '"id"')).toBe('key');
    expect(kindOf(t, '"a:1"')).toBe('string');
    expect(kindOf(t, '-1.5e3')).toBe('number');
    expect(kindOf(t, 'true')).toBe('literal');
    expect(kindOf(t, 'null')).toBe('literal');
    expect(kindOf(t, '"s"')).toBe('string');
  });

  it('keeps every character, including escapes', () => {
    const src = '{"a\\"b": "c\\\\d", "e": [ ]}';
    expect(
      highlightJson(src)
        .map((t) => t.text)
        .join(''),
    ).toBe(src);
  });
});

describe('highlightJs', () => {
  it('classifies like VS Code Dark+', () => {
    const t = highlightJs(
      [
        '// sign the body',
        'const key = pm.environment.get("secret");',
        'if (!key) return;',
        'const sig = CryptoJS.HmacSHA256(`${pm.request.body.raw}`, key).toString();',
        "pm.variables.set('signature', sig); /* done */",
        'let n = 0x1f + 2.5;',
      ].join('\n'),
    );
    expect(show(t)).toContain('comment:// sign the body');
    expect(kindOf(t, 'const')).toBe('keyword');
    expect(kindOf(t, 'if')).toBe('control');
    expect(kindOf(t, 'return')).toBe('control');
    expect(kindOf(t, 'pm')).toBe('variable');
    expect(kindOf(t, 'environment')).toBe('property');
    expect(kindOf(t, 'get')).toBe('function');
    expect(kindOf(t, '"secret"')).toBe('string');
    expect(kindOf(t, 'CryptoJS')).toBe('type');
    expect(kindOf(t, 'HmacSHA256')).toBe('function');
    expect(kindOf(t, '`${pm.request.body.raw}`')).toBe('string');
    expect(kindOf(t, "'signature'")).toBe('string');
    expect(kindOf(t, '/* done */')).toBe('comment');
    expect(kindOf(t, '0x1f')).toBe('number');
    expect(kindOf(t, '2.5')).toBe('number');
  });

  it('tells a regex from a division', () => {
    const t = highlightJs('const re = /a\\/b[/]+/gi;\nconst half = total / 2 / 1;\nreturn /x/.test(s);');
    expect(kindOf(t, '/a\\/b[/]+/gi')).toBe('regex');
    expect(kindOf(t, '/x/')).toBe('regex');
    expect(t.filter((x) => x.kind === 'regex')).toHaveLength(2);
  });

  it('handles nested template expressions and unterminated input', () => {
    const src = 'const s = `a ${fn(`b ${c}`)} d`;\nconst bad = "no end\nnext();';
    const t = highlightJs(src);
    expect(kindOf(t, '`a ${fn(`b ${c}`)} d`')).toBe('string');
    expect(kindOf(t, 'next')).toBe('function');
    expect(t.map((x) => x.text).join('')).toBe(src);
  });

  it('does not color keywords used as property names', () => {
    const t = highlightJs('pm.response.to.have.status(200); obj.default = 1;');
    expect(kindOf(t, 'default')).toBe('property');
    expect(kindOf(t, 'status')).toBe('function');
  });
});
