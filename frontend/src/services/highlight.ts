/**
 * Small, dependency-free syntax highlighters for JSON and JavaScript.
 *
 * They return plain tokens that components render as text inside <span>s
 * (never as HTML), so highlighted content cannot inject markup.
 */

export type TokenKind =
  | ''
  | 'key'
  | 'string'
  | 'number'
  | 'literal'
  | 'punct'
  | 'comment'
  | 'keyword'
  | 'control'
  | 'function'
  | 'variable'
  | 'property'
  | 'type'
  | 'regex'
  /** A {{variable}} placeholder in a request body. */
  | 'template';

export interface Token {
  text: string;
  kind: TokenKind;
}

/** Above this size, text is shown without highlighting to keep the UI responsive. */
export const HIGHLIGHT_LIMIT = 400_000;

function push(out: Token[], text: string, kind: TokenKind) {
  if (!text) return;
  const last = out[out.length - 1];
  if (last && last.kind === kind) last.text += text;
  else out.push({ text, kind });
}

// ---- JSON ----

const JSON_TOKEN =
  /("(?:[^"\\]|\\.)*")|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b|(\{\{[^{}\n]*\}\})|([{}[\],:])|(\s+)/y;
const TEMPLATE = /\{\{[^{}\n]*\}\}/g;

/** Pushes a string token, splitting out {{variable}} placeholders when templates are on. */
function pushString(out: Token[], text: string, kind: TokenKind, templates: boolean) {
  if (!templates) return push(out, text, kind);
  let last = 0;
  for (const m of text.matchAll(TEMPLATE)) {
    push(out, text.slice(last, m.index), kind);
    push(out, m[0], 'template');
    last = m.index + m[0].length;
  }
  push(out, text.slice(last), kind);
}

export interface JsonHighlightOptions {
  /** Mark {{variable}} placeholders (request bodies), also inside strings. */
  templates?: boolean;
}

/** Tokenizes JSON text; a string followed by ':' is a key. Unknown characters are kept as plain text. */
export function highlightJson(text: string, options: JsonHighlightOptions = {}): Token[] {
  const templates = options.templates ?? false;
  const out: Token[] = [];
  let i = 0;
  while (i < text.length) {
    JSON_TOKEN.lastIndex = i;
    const m = JSON_TOKEN.exec(text);
    if (!m) {
      push(out, text[i], '');
      i++;
      continue;
    }
    const [all, str, num, lit, tpl, punct] = m;
    if (tpl !== undefined && !templates) {
      // Not a placeholder here: tokenize it like any other text, one character at a time.
      push(out, '{', 'punct');
      i++;
      continue;
    }
    if (str !== undefined) {
      let j = i + all.length;
      while (j < text.length && /\s/.test(text[j])) j++;
      pushString(out, all, text[j] === ':' ? 'key' : 'string', templates);
    } else if (num !== undefined) push(out, all, 'number');
    else if (lit !== undefined) push(out, all, 'literal');
    else if (tpl !== undefined) push(out, all, 'template');
    else if (punct !== undefined) push(out, all, 'punct');
    else push(out, all, '');
    i += all.length;
  }
  return out;
}

// ---- JavaScript ----

const CONTROL = new Set([
  'if',
  'else',
  'for',
  'while',
  'do',
  'switch',
  'case',
  'default',
  'break',
  'continue',
  'return',
  'throw',
  'try',
  'catch',
  'finally',
  'await',
  'yield',
  'import',
  'export',
  'from',
  'as',
]);
const KEYWORDS = new Set([
  'const',
  'let',
  'var',
  'function',
  'class',
  'extends',
  'new',
  'delete',
  'typeof',
  'instanceof',
  'in',
  'of',
  'void',
  'this',
  'super',
  'async',
  'static',
  'get',
  'set',
]);
const LITERALS = new Set(['true', 'false', 'null', 'undefined', 'NaN', 'Infinity']);
/** After these keywords a '/' starts a regular expression, not a division. */
const REGEX_AFTER = new Set([
  'return',
  'typeof',
  'case',
  'do',
  'else',
  'in',
  'of',
  'new',
  'delete',
  'void',
  'throw',
  'yield',
  'await',
]);

const IDENT = /[A-Za-z_$][\w$]*/y;
const NUMBER =
  /(?:0[xX][\da-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|(?:\d[\d_]*(?:\.[\d_]*)?|\.\d[\d_]*)(?:[eE][+-]?\d+)?)n?/y;

function match(re: RegExp, text: string, i: number): string {
  re.lastIndex = i;
  const m = re.exec(text);
  return m ? m[0] : '';
}

/** Index just past a quoted string starting at i (unterminated strings end at the line end). */
function endOfString(text: string, i: number): number {
  const q = text[i];
  let j = i + 1;
  while (j < text.length && text[j] !== q) {
    if (text[j] === '\\') j++;
    else if (text[j] === '\n') return j;
    j++;
  }
  return Math.min(j + 1, text.length);
}

/** Index just past a template literal, skipping nested `${ ... }` expressions. */
function endOfTemplate(text: string, i: number): number {
  let j = i + 1;
  while (j < text.length) {
    const c = text[j];
    if (c === '\\') j += 2;
    else if (c === '`') return j + 1;
    else if (c === '$' && text[j + 1] === '{') {
      let depth = 1;
      j += 2;
      while (j < text.length && depth > 0) {
        const d = text[j];
        if (d === '{') depth++;
        else if (d === '}') depth--;
        else if (d === '"' || d === "'") {
          j = endOfString(text, j);
          continue;
        } else if (d === '`') {
          j = endOfTemplate(text, j);
          continue;
        }
        j++;
      }
    } else j++;
  }
  return text.length;
}

/** Index just past a regex literal at i, or -1 if it is not one on this line. */
function endOfRegex(text: string, i: number): number {
  let j = i + 1;
  let inClass = false;
  while (j < text.length && text[j] !== '\n') {
    const c = text[j];
    if (c === '\\') j++;
    else if (c === '[') inClass = true;
    else if (c === ']') inClass = false;
    else if (c === '/' && !inClass) {
      j++;
      while (j < text.length && /[a-z]/i.test(text[j])) j++;
      return j;
    }
    j++;
  }
  return -1;
}

/** Tokenizes JavaScript for display, using the token kinds of VS Code's Dark+ theme. */
export function highlightJs(text: string): Token[] {
  const out: Token[] = [];
  // What came before, to tell a regex from a division.
  let prevValue = false;
  let prevWord = '';
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    const next = text[i + 1];

    if (/\s/.test(c)) {
      let j = i + 1;
      while (j < text.length && /\s/.test(text[j])) j++;
      push(out, text.slice(i, j), '');
      i = j;
      continue;
    }
    if (c === '/' && next === '/') {
      const j = text.indexOf('\n', i);
      const end = j < 0 ? text.length : j;
      push(out, text.slice(i, end), 'comment');
      i = end;
      continue;
    }
    if (c === '/' && next === '*') {
      const j = text.indexOf('*/', i + 2);
      const end = j < 0 ? text.length : j + 2;
      push(out, text.slice(i, end), 'comment');
      i = end;
      continue;
    }
    if (c === '"' || c === "'" || c === '`') {
      const end = c === '`' ? endOfTemplate(text, i) : endOfString(text, i);
      push(out, text.slice(i, end), 'string');
      i = end;
      prevValue = true;
      prevWord = '';
      continue;
    }
    if (c === '/' && (!prevValue || REGEX_AFTER.has(prevWord))) {
      const end = endOfRegex(text, i);
      if (end > 0) {
        push(out, text.slice(i, end), 'regex');
        i = end;
        prevValue = true;
        prevWord = '';
        continue;
      }
    }
    if (/\d/.test(c) || (c === '.' && /\d/.test(next ?? ''))) {
      const num = match(NUMBER, text, i) || c;
      push(out, num, 'number');
      i += num.length;
      prevValue = true;
      prevWord = '';
      continue;
    }
    const word = match(IDENT, text, i);
    if (word) {
      let j = i + word.length;
      while (j < text.length && (text[j] === ' ' || text[j] === '\t')) j++;
      const called = text[j] === '(';
      const afterDot = lastNonSpace(text, i) === '.';
      let kind: TokenKind;
      if (!afterDot && CONTROL.has(word)) kind = 'control';
      else if (!afterDot && (KEYWORDS.has(word) || LITERALS.has(word))) kind = 'keyword';
      else if (called) kind = 'function';
      else if (afterDot) kind = 'property';
      else if (/^[A-Z]/.test(word)) kind = 'type';
      else kind = 'variable';
      push(out, word, kind);
      i += word.length;
      prevValue =
        (kind !== 'control' && kind !== 'keyword') || word === 'this' || word === 'super' || LITERALS.has(word);
      prevWord = word;
      continue;
    }
    push(out, c, 'punct');
    i++;
    prevValue = c === ')' || c === ']' || c === '}';
    prevWord = '';
  }
  return out;
}

function lastNonSpace(text: string, i: number): string {
  let j = i - 1;
  while (j >= 0 && /\s/.test(text[j])) j--;
  return j >= 0 ? text[j] : '';
}
