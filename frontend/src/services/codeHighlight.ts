/**
 * Syntax highlighting for the "Copy as code" output, one style per language.
 *
 * Each language gets its own comments, string forms and keyword lists. Like
 * highlight.ts, it returns plain tokens that are rendered as text, never as HTML.
 */
import type { CodeLanguage } from './codegen';
import { HIGHLIGHT_LIMIT, highlightJs, type Token, type TokenKind } from './highlight';

interface LangSpec {
  /** Line comment starters. */
  line: string[];
  /** C-style block comments. */
  block?: boolean;
  /** Quote characters for one-line strings with backslash escapes. */
  quotes: string;
  /** """ (and ''' when ' is a quote) span lines. */
  triple?: boolean;
  /** A quote character for raw, multi-line strings without escapes (Go's backtick). */
  raw?: string;
  /** Declaration keywords, literals and built-in types. */
  keywords: string[];
  /** Flow-control keywords. */
  control: string[];
  /** $name is a variable (PHP). */
  dollarVars?: boolean;
  /** # at the start of a line is a preprocessor directive (C, C++, Objective-C). */
  preprocessor?: boolean;
  /** @name is an annotation or attribute. */
  annotations?: boolean;
  /** @"…" strings and @keywords (Objective-C), or @"…" / $"…" strings (C#). */
  atStrings?: boolean;
  /** name!( is a macro call (Rust). */
  macros?: boolean;
}

const C_CONTROL = [
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
  'goto',
];

const SPECS: Record<Exclude<CodeLanguage, 'curl' | 'fetch' | 'axios'>, LangSpec> = {
  python: {
    line: ['#'],
    quotes: `"'`,
    triple: true,
    keywords: [
      'def',
      'class',
      'lambda',
      'True',
      'False',
      'None',
      'and',
      'or',
      'not',
      'in',
      'is',
      'global',
      'nonlocal',
      'del',
      'pass',
    ],
    control: [
      'if',
      'elif',
      'else',
      'for',
      'while',
      'break',
      'continue',
      'return',
      'try',
      'except',
      'finally',
      'raise',
      'with',
      'as',
      'import',
      'from',
      'yield',
      'await',
      'async',
    ],
  },
  go: {
    line: ['//'],
    block: true,
    quotes: `"'`,
    raw: '`',
    keywords: [
      'package',
      'func',
      'var',
      'const',
      'type',
      'struct',
      'interface',
      'map',
      'chan',
      'nil',
      'true',
      'false',
      'iota',
      'string',
      'int',
      'int64',
      'uint',
      'byte',
      'rune',
      'bool',
      'error',
      'float64',
      'any',
    ],
    control: [
      'import',
      'if',
      'else',
      'for',
      'range',
      'switch',
      'case',
      'default',
      'break',
      'continue',
      'return',
      'go',
      'defer',
      'select',
      'fallthrough',
      'goto',
    ],
  },
  php: {
    line: ['//', '#'],
    block: true,
    quotes: `"'`,
    dollarVars: true,
    keywords: [
      'php',
      'function',
      'class',
      'new',
      'echo',
      'print',
      'array',
      'true',
      'false',
      'null',
      'TRUE',
      'FALSE',
      'NULL',
      'public',
      'private',
      'static',
      'const',
      'fn',
    ],
    control: [
      'if',
      'else',
      'elseif',
      'foreach',
      'for',
      'while',
      'return',
      'throw',
      'try',
      'catch',
      'finally',
      'use',
      'require',
      'require_once',
      'include',
      'as',
    ],
  },
  java: {
    line: ['//'],
    block: true,
    quotes: `"'`,
    triple: true,
    annotations: true,
    keywords: [
      'public',
      'private',
      'protected',
      'static',
      'final',
      'class',
      'interface',
      'void',
      'new',
      'var',
      'this',
      'null',
      'true',
      'false',
      'int',
      'long',
      'boolean',
      'char',
      'double',
      'package',
      'extends',
      'implements',
      'throws',
    ],
    control: [...C_CONTROL, 'import', 'throw', 'try', 'catch', 'finally'],
  },
  csharp: {
    line: ['//'],
    block: true,
    quotes: `"'`,
    atStrings: true,
    keywords: [
      'using',
      'var',
      'new',
      'public',
      'private',
      'static',
      'class',
      'void',
      'string',
      'int',
      'bool',
      'null',
      'true',
      'false',
      'this',
      'namespace',
      'readonly',
    ],
    control: [...C_CONTROL, 'await', 'async', 'throw', 'try', 'catch', 'finally', 'foreach', 'in'],
  },
  dart: {
    line: ['//'],
    block: true,
    quotes: `"'`,
    triple: true,
    annotations: true,
    keywords: [
      'var',
      'final',
      'const',
      'void',
      'class',
      'new',
      'null',
      'true',
      'false',
      'this',
      'async',
      'String',
      'int',
      'bool',
      'dynamic',
      'Future',
    ],
    control: [...C_CONTROL, 'import', 'as', 'show', 'await', 'throw', 'try', 'catch', 'finally', 'in'],
  },
  swift: {
    line: ['//'],
    block: true,
    quotes: '"',
    triple: true,
    annotations: true,
    keywords: [
      'var',
      'let',
      'func',
      'class',
      'struct',
      'final',
      'nil',
      'true',
      'false',
      'self',
      'async',
      'as',
      'is',
      'in',
      'inout',
    ],
    control: [
      'import',
      'if',
      'else',
      'guard',
      'for',
      'while',
      'switch',
      'case',
      'default',
      'break',
      'continue',
      'return',
      'throw',
      'try',
      'await',
      'do',
      'catch',
    ],
  },
  objc: {
    line: ['//'],
    block: true,
    quotes: `"'`,
    preprocessor: true,
    atStrings: true,
    keywords: [
      'int',
      'void',
      'char',
      'long',
      'BOOL',
      'YES',
      'NO',
      'nil',
      'NULL',
      'id',
      'self',
      'static',
      'const',
      'struct',
      '__block',
      'autoreleasepool',
      'interface',
      'implementation',
      'end',
    ],
    control: [...C_CONTROL],
  },
  cpp: {
    line: ['//'],
    block: true,
    quotes: `"'`,
    preprocessor: true,
    keywords: [
      'int',
      'void',
      'char',
      'long',
      'size_t',
      'bool',
      'auto',
      'const',
      'static',
      'struct',
      'class',
      'nullptr',
      'true',
      'false',
      'static_cast',
      'reinterpret_cast',
      'std',
      'namespace',
      'using',
    ],
    control: [...C_CONTROL],
  },
  rust: {
    line: ['//'],
    block: true,
    // ' starts lifetimes as well as chars, so only " is a string.
    quotes: '"',
    macros: true,
    keywords: [
      'fn',
      'let',
      'mut',
      'pub',
      'struct',
      'enum',
      'impl',
      'use',
      'mod',
      'crate',
      'self',
      'Self',
      'true',
      'false',
      'dyn',
      'ref',
      'as',
      'u16',
      'u64',
      'i64',
      'usize',
      'str',
      'String',
      'bool',
    ],
    control: ['if', 'else', 'for', 'while', 'loop', 'match', 'break', 'continue', 'return', 'in'],
  },
};

const SETS = new Map(
  Object.entries(SPECS).map(([k, s]) => [k, { keywords: new Set(s.keywords), control: new Set(s.control) }]),
);

function push(out: Token[], text: string, kind: TokenKind) {
  if (!text) return;
  const last = out[out.length - 1];
  if (last && last.kind === kind) last.text += text;
  else out.push({ text, kind });
}

/** Pushes a string, marking {{variable}} placeholders that were left unresolved. */
function pushString(out: Token[], text: string) {
  let last = 0;
  for (const m of text.matchAll(/\{\{[^{}\n]*\}\}/g)) {
    push(out, text.slice(last, m.index), 'string');
    push(out, m[0], 'template');
    last = m.index + m[0].length;
  }
  push(out, text.slice(last), 'string');
}

/** Index just past a one-line quoted string at i; unterminated strings end at the line end. */
function endOfString(text: string, i: number, escapes = true): number {
  const q = text[i];
  let j = i + 1;
  while (j < text.length && text[j] !== q) {
    if (text[j] === '\\' && escapes) j++;
    else if (text[j] === '\n') return j;
    j++;
  }
  return Math.min(j + 1, text.length);
}

/** Index just past a delimited block (triple-quoted or raw string) starting at i. */
function endOfBlock(text: string, i: number, open: string, escapes: boolean): number {
  let j = i + open.length;
  while (j < text.length) {
    if (escapes && text[j] === '\\') j += 2;
    else if (text.startsWith(open, j)) return j + open.length;
    else j++;
  }
  return text.length;
}

const IDENT = /[A-Za-z_][\w]*/y;
const NUMBER = /(?:0[xX][\da-fA-F_]+|\d[\d_]*(?:\.\d+)?(?:[eE][+-]?\d+)?)[A-Za-z_\d]*/y;

function match(re: RegExp, text: string, i: number): string {
  re.lastIndex = i;
  return re.exec(text)?.[0] ?? '';
}

function lastNonSpace(text: string, i: number): string {
  let j = i - 1;
  while (j >= 0 && (text[j] === ' ' || text[j] === '\t')) j--;
  return j >= 0 ? text[j] : '';
}

function atLineStart(text: string, i: number): boolean {
  let j = i - 1;
  while (j >= 0 && (text[j] === ' ' || text[j] === '\t')) j--;
  return j < 0 || text[j] === '\n';
}

function highlightWith(lang: keyof typeof SPECS, text: string): Token[] {
  const spec = SPECS[lang];
  const { keywords, control } = SETS.get(lang)!;
  const out: Token[] = [];
  // The previous word, to colour `new Name(` as a type.
  let prevWord = '';
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    const next = text[i + 1] ?? '';
    if (!/[\s\w]/.test(c)) prevWord = '';

    if (/\s/.test(c)) {
      let j = i + 1;
      while (j < text.length && /\s/.test(text[j])) j++;
      push(out, text.slice(i, j), '');
      i = j;
      continue;
    }
    if (spec.preprocessor && c === '#' && atLineStart(text, i)) {
      // #include <x> / #import <x>: the directive is a keyword, the rest a string.
      const word = '#' + match(IDENT, text, i + 1);
      push(out, word, 'control');
      i += word.length;
      const eol = text.indexOf('\n', i);
      const end = eol < 0 ? text.length : eol;
      const rest = text.slice(i, end);
      const lead = /^\s*/.exec(rest)![0];
      push(out, lead, '');
      push(out, rest.slice(lead.length), 'string');
      i = end;
      continue;
    }
    const lineComment = spec.line.find((s) => text.startsWith(s, i));
    if (lineComment && !(lang === 'php' && c === '#' && next === '[')) {
      const j = text.indexOf('\n', i);
      const end = j < 0 ? text.length : j;
      push(out, text.slice(i, end), 'comment');
      i = end;
      continue;
    }
    if (spec.block && c === '/' && next === '*') {
      const j = text.indexOf('*/', i + 2);
      const end = j < 0 ? text.length : j + 2;
      push(out, text.slice(i, end), 'comment');
      i = end;
      continue;
    }
    if (spec.atStrings && (c === '@' || c === '$') && next === '"') {
      // Objective-C @"…", C# @"…" (verbatim: "" escapes a quote) and $"…".
      let end = endOfString(text, i + 1, !(lang === 'csharp' && c === '@'));
      while (lang === 'csharp' && c === '@' && text[end] === '"') end = endOfString(text, end, false);
      pushString(out, text.slice(i, end));
      i = end;
      continue;
    }
    if (spec.triple && (c === '"' || c === "'") && spec.quotes.includes(c) && text.startsWith(c.repeat(3), i)) {
      const end = endOfBlock(text, i, c.repeat(3), true);
      pushString(out, text.slice(i, end));
      i = end;
      continue;
    }
    if (spec.raw && c === spec.raw) {
      const end = endOfBlock(text, i, c, false);
      pushString(out, text.slice(i, end));
      i = end;
      continue;
    }
    if (spec.quotes.includes(c)) {
      // PHP's single-quoted strings only escape \' and \\, which the generic scan already handles.
      const end = endOfString(text, i);
      pushString(out, text.slice(i, end));
      i = end;
      continue;
    }
    if (/\d/.test(c)) {
      const num = match(NUMBER, text, i) || c;
      push(out, num, 'number');
      i += num.length;
      continue;
    }
    if (c === '$' && spec.dollarVars) {
      const name = match(IDENT, text, i + 1);
      if (name) {
        push(out, '$' + name, 'variable');
        i += name.length + 1;
        continue;
      }
    }
    if (c === '@' && (spec.annotations || spec.atStrings)) {
      const name = match(IDENT, text, i + 1);
      if (name) {
        // Objective-C @autoreleasepool is a keyword; Java/Dart/Swift @Override, @main are attributes.
        push(out, '@' + name, lang === 'objc' ? 'keyword' : 'type');
        i += name.length + 1;
        continue;
      }
    }
    if (c === '<' && next === '?' && lang === 'php') {
      push(out, '<?', 'keyword');
      i += 2;
      continue;
    }
    const word = match(IDENT, text, i);
    if (word) {
      let j = i + word.length;
      const macro = spec.macros && text[j] === '!' && text[j + 1] !== '=';
      if (macro) j++;
      while (j < text.length && (text[j] === ' ' || text[j] === '\t')) j++;
      const prev = lastNonSpace(text, i);
      const member = prev === '.' || (prev === '>' && text[i - 1] === '>' && text[i - 2] === '-');
      // Capitalised names are types (not ALL_CAPS constants); Objective-C classes start with NS.
      const typeName = /^[A-Z]\w*[a-z]/.test(word) || (lang === 'objc' && /^NS[A-Z]/.test(word));
      let kind: TokenKind;
      if (!member && control.has(word)) kind = 'control';
      else if (!member && keywords.has(word)) kind = 'keyword';
      else if (typeName && prevWord === 'new') kind = 'type';
      else if (text[j] === '(' || macro) kind = 'function';
      else if (typeName) kind = 'type';
      else if (member) kind = 'property';
      else kind = '';
      const len = word.length + (macro ? 1 : 0);
      push(out, text.slice(i, i + len), kind);
      i += len;
      prevWord = word;
      continue;
    }
    push(out, c, 'punct');
    i++;
  }
  return out;
}

/** A POSIX shell command, as the cURL generator writes it. */
function highlightShell(text: string): Token[] {
  const out: Token[] = [];
  let commandNext = true;
  let i = 0;
  while (i < text.length) {
    const c = text[i];
    if (c === '\\' && i + 1 < text.length) {
      // A line continuation or an escaped character (such as the \' in 'it'\''s').
      push(out, text.slice(i, i + 2), 'punct');
      i += 2;
      continue;
    }
    if (/\s/.test(c)) {
      let j = i + 1;
      while (j < text.length && /\s/.test(text[j])) j++;
      if (text.slice(i, j).includes('\n') && text[i - 1] !== '\\') commandNext = true;
      push(out, text.slice(i, j), '');
      i = j;
      continue;
    }
    if (c === '#' && (i === 0 || /\s/.test(text[i - 1]))) {
      const j = text.indexOf('\n', i);
      const end = j < 0 ? text.length : j;
      push(out, text.slice(i, end), 'comment');
      i = end;
      continue;
    }
    if (c === "'" || c === '"') {
      // Single quotes have no escapes in a shell; double quotes do.
      let end = endOfBlock(text, i, c, c === '"');
      if (c === "'") end = Math.min(text.indexOf("'", i + 1) + 1 || text.length, text.length);
      pushString(out, text.slice(i, end));
      i = end;
      commandNext = false;
      continue;
    }
    if (c === '|' || c === ';' || c === '&') {
      push(out, c, 'punct');
      i++;
      commandNext = true;
      continue;
    }
    let j = i;
    while (j < text.length && !/[\s'"\\|;&]/.test(text[j])) j++;
    const word = text.slice(i, j);
    if (commandNext) push(out, word, 'function');
    else if (word.startsWith('-')) push(out, word, 'keyword');
    else pushString(out, word);
    commandNext = false;
    i = j;
  }
  return out;
}

/** Tokenizes generated code in the style of its language. */
export function highlightCode(language: CodeLanguage, text: string): Token[] {
  if (text.length > HIGHLIGHT_LIMIT) return [{ text, kind: '' }];
  if (language === 'curl') return highlightShell(text);
  if (language === 'fetch' || language === 'axios') {
    const out: Token[] = [];
    for (const t of highlightJs(text)) {
      if (t.kind === 'string') pushString(out, t.text);
      else push(out, t.text, t.kind);
    }
    return out;
  }
  return highlightWith(language, text);
}
