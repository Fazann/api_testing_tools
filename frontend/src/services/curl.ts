// Converts between cURL command lines and requests. Parsing understands the
// options browsers and API docs emit ("Copy as cURL"); output mirrors how
// internal/runner/runner.go builds the HTTP request, so pasting it into a
// terminal sends what the app would send.
import {
  METHODS,
  type Auth,
  type Environment,
  type FormField,
  type KeyValue,
  type Method,
  type Request,
  type Variable,
} from '@/types';

import { describeVariable, splitVariables } from './variables';

/** The fields of a request that a cURL command describes. */
export type CurlRequest = Pick<
  Request,
  'name' | 'method' | 'url' | 'headers' | 'body' | 'bodyType' | 'auth' | 'timeoutMs' | 'followRedirects'
> &
  Partial<Pick<Request, 'form' | 'bodyFile'>>;

export interface ParsedCurl {
  request: CurlRequest;
  /** Options that were ignored or could not be represented. */
  warnings: string[];
}

export class CurlParseError extends Error {}

/** Reports whether text looks like a cURL command, e.g. to intercept a paste into the URL bar. */
export function looksLikeCurl(text: string): boolean {
  return /^\s*(?:\$\s*)?curl(?:\.exe)?\s/i.test(text);
}

// ---- tokenizing ----

/** Splits a POSIX shell command line into words, handling quotes, escapes and line continuations. */
export function shellSplit(input: string): string[] {
  const words: string[] = [];
  let word = '';
  let inWord = false;
  let i = 0;
  const s = input;
  while (i < s.length) {
    const c = s[i];
    if (c === '\\') {
      const next = s[i + 1];
      if (next === '\n') {
        i += 2;
        continue;
      }
      if (next === '\r' && s[i + 2] === '\n') {
        i += 3;
        continue;
      }
      if (next !== undefined) word += next;
      inWord = true;
      i += 2;
    } else if (c === "'") {
      const end = s.indexOf("'", i + 1);
      if (end < 0) throw new CurlParseError('Unterminated single quote');
      word += s.slice(i + 1, end);
      inWord = true;
      i = end + 1;
    } else if (c === '$' && s[i + 1] === "'") {
      const [text, next] = ansiC(s, i + 2);
      word += text;
      inWord = true;
      i = next;
    } else if (c === '"') {
      i++;
      for (;;) {
        if (i >= s.length) throw new CurlParseError('Unterminated double quote');
        const d = s[i];
        if (d === '"') break;
        if (d === '\\' && '$`"\\\n'.includes(s[i + 1] ?? '')) {
          if (s[i + 1] !== '\n') word += s[i + 1];
          i += 2;
          continue;
        }
        word += d;
        i++;
      }
      inWord = true;
      i++;
    } else if (/\s/.test(c)) {
      if (inWord) words.push(word);
      word = '';
      inWord = false;
      i++;
    } else {
      word += c;
      inWord = true;
      i++;
    }
  }
  if (inWord) words.push(word);
  return words;
}

/** Reads bash $'...' quoting starting after the opening quote; returns the text and the index after the close. */
function ansiC(s: string, start: number): [string, number] {
  const simple: Record<string, string> = {
    n: '\n',
    t: '\t',
    r: '\r',
    a: '\x07',
    b: '\b',
    e: '\x1b',
    E: '\x1b',
    f: '\f',
    v: '\v',
    '\\': '\\',
    "'": "'",
    '"': '"',
    '?': '?',
  };
  let out = '';
  let i = start;
  while (i < s.length) {
    const c = s[i];
    if (c === "'") return [out, i + 1];
    if (c !== '\\') {
      out += c;
      i++;
      continue;
    }
    const e = s[i + 1];
    if (e in simple) {
      out += simple[e];
      i += 2;
    } else if (e === 'x' || e === 'u' || e === 'U') {
      const max = e === 'x' ? 2 : e === 'u' ? 4 : 8;
      const hex = /^[0-9a-fA-F]+/.exec(s.slice(i + 2, i + 2 + max))?.[0] ?? '';
      if (!hex) {
        out += '\\' + e;
      } else if (e === 'x') {
        // \xHH are bytes; collect consecutive ones and decode them as UTF-8.
        const bytes = [parseInt(hex, 16)];
        let j = i + 2 + hex.length;
        for (let m; (m = /^\\x([0-9a-fA-F]{1,2})/.exec(s.slice(j))); j += m[0].length) bytes.push(parseInt(m[1], 16));
        out += new TextDecoder().decode(new Uint8Array(bytes));
        i = j;
        continue;
      } else {
        out += String.fromCodePoint(parseInt(hex, 16));
      }
      i += 2 + hex.length;
    } else if (e !== undefined && /[0-7]/.test(e)) {
      const oct = /^[0-7]{1,3}/.exec(s.slice(i + 1))![0];
      out += String.fromCharCode(parseInt(oct, 8));
      i += 1 + oct.length;
    } else {
      out += '\\' + (e ?? '');
      i += 2;
    }
  }
  throw new CurlParseError("Unterminated $'…' quote");
}

// ---- parsing ----

// Options that take a value; every other option is treated as a flag.
const SHORT_WITH_VALUE = new Set('XHdubAemFoxUcTErwzyYKDQtP'.split(''));
const LONG_WITH_VALUE = new Set([
  'request',
  'header',
  'data',
  'data-raw',
  'data-binary',
  'data-ascii',
  'data-urlencode',
  'json',
  'user',
  'cookie',
  'user-agent',
  'referer',
  'max-time',
  'connect-timeout',
  'url',
  'form',
  'form-string',
  'output',
  'write-out',
  'proxy',
  'proxy-user',
  'proxy-header',
  'max-redirs',
  'retry',
  'retry-delay',
  'retry-max-time',
  'cookie-jar',
  'upload-file',
  'cert',
  'cert-type',
  'key',
  'key-type',
  'pass',
  'cacert',
  'capath',
  'resolve',
  'connect-to',
  'oauth2-bearer',
  'range',
  'time-cond',
  'config',
  'limit-rate',
  'speed-limit',
  'speed-time',
  'interface',
  'dump-header',
  'trace',
  'trace-ascii',
  'stderr',
  'quote',
  'telnet-option',
  'unix-socket',
  'abstract-unix-socket',
  'doh-url',
  'noproxy',
  'pinnedpubkey',
  'aws-sigv4',
  'variable',
  'ciphers',
  'tls-max',
  'local-port',
  'dns-servers',
  'expect100-timeout',
  'happy-eyeballs-timeout-ms',
  'keepalive-time',
  'max-filesize',
  'output-dir',
  'parallel-max',
  'request-target',
  'service-name',
  'url-query',
]);

// Long names of the short options this parser acts on.
const SHORT_NAMES: Record<string, string> = {
  X: 'request',
  H: 'header',
  d: 'data',
  u: 'user',
  b: 'cookie',
  A: 'user-agent',
  e: 'referer',
  m: 'max-time',
  F: 'form',
  G: 'get',
  I: 'head',
  L: 'location',
  k: 'insecure',
  s: 'silent',
  S: 'show-error',
  v: 'verbose',
  i: 'include',
  f: 'fail',
  g: 'globoff',
  N: 'no-buffer',
  O: 'remote-name',
  J: 'remote-header-name',
  o: 'output',
  w: 'write-out',
  x: 'proxy',
  T: 'upload-file',
  '0': 'http1.0',
  '#': 'progress-bar',
  c: 'cookie-jar',
  D: 'dump-header',
};

// Options with no effect on what is sent, or that the app controls elsewhere.
const IGNORED = new Set([
  'silent',
  'show-error',
  'verbose',
  'include',
  'compressed',
  'fail',
  'fail-with-body',
  'progress-bar',
  'no-progress-meter',
  'globoff',
  'http1.0',
  'http1.1',
  'http2',
  'http2-prior-knowledge',
  'http3',
  'tlsv1',
  'tlsv1.2',
  'tlsv1.3',
  'no-keepalive',
  'output',
  'write-out',
  'max-redirs',
  'retry',
  'retry-delay',
  'retry-max-time',
  'connect-timeout',
  'remote-name',
  'remote-header-name',
  'no-buffer',
  'path-as-is',
  'raw',
  'location-trusted',
  'dump-header',
  'cookie-jar',
]);

interface Option {
  name: string;
  value?: string;
}

/** Turns curl arguments into options and URLs. */
function readOptions(args: string[]): { options: Option[]; urls: string[] } {
  const options: Option[] = [];
  const urls: string[] = [];
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    const take = (flag: string) => {
      if (i + 1 >= args.length) throw new CurlParseError(`${flag} needs a value`);
      return args[++i];
    };
    if (a === '--') {
      urls.push(...args.slice(i + 1));
      break;
    }
    if (a.startsWith('--') && a.length > 2) {
      const name = a.slice(2);
      options.push(LONG_WITH_VALUE.has(name) ? { name, value: take(a) } : { name });
    } else if (a.startsWith('-') && a.length > 1) {
      // Short options can be combined (-sSL) and take values attached (-XPOST) or as the next word.
      for (let j = 1; j < a.length; j++) {
        const c = a[j];
        const name = SHORT_NAMES[c] ?? `-${c}`;
        if (SHORT_WITH_VALUE.has(c)) {
          options.push({ name, value: j + 1 < a.length ? a.slice(j + 1) : take(`-${c}`) });
          break;
        }
        options.push({ name });
      }
    } else {
      urls.push(a);
    }
  }
  return { options, urls };
}

function encodeDataUrlencode(v: string): string {
  // curl: "content", "=content", "name=content"; the content is URL-encoded, the name is not.
  const eq = v.indexOf('=');
  if (eq < 0) return encodeURIComponent(v);
  const name = v.slice(0, eq);
  return (name ? name + '=' : '') + encodeURIComponent(v.slice(eq + 1));
}

function headerIndex(headers: KeyValue[], name: string): number {
  return headers.findIndex((h) => h.key.toLowerCase() === name.toLowerCase());
}

function setHeader(headers: KeyValue[], key: string, value: string) {
  const i = headerIndex(headers, key);
  if (i >= 0) headers[i] = { key: headers[i].key, value, enabled: true };
  else headers.push({ key, value, enabled: true });
}

function decodeBase64(s: string): string | null {
  try {
    const bin = atob(s.trim());
    return new TextDecoder('utf-8', { fatal: true }).decode(Uint8Array.from(bin, (ch) => ch.charCodeAt(0)));
  } catch {
    return null;
  }
}

function defaultName(method: string, url: string): string {
  const path = url
    .replace(/^[a-z][a-z0-9+.-]*:\/\/[^/?#]*/i, '')
    .replace(/^\{\{[^}]+\}\}/, '')
    .split(/[?#]/)[0];
  return `${method} ${path || '/'}`.slice(0, 200);
}

/** Parses a cURL command into request fields. Throws CurlParseError when it cannot be used. */
export function parseCurl(command: string): ParsedCurl {
  const words = shellSplit(command.trim().replace(/^\$\s+/, ''));
  if (!words.length || !/^curl(\.exe)?$/i.test(words[0])) throw new CurlParseError('The command must start with curl');
  const { options, urls } = readOptions(words.slice(1));
  const warnings: string[] = [];
  const warn = (w: string) => warnings.includes(w) || warnings.push(w);

  const headers: KeyValue[] = [];
  const data: string[] = [];
  const form: FormField[] = [];
  let binaryFile = '';
  let uploadFile = '';
  let explicitMethod = '';
  let forceGet = false;
  let head = false;
  let json = false;
  let user = '';
  let timeoutMs = 30000;

  for (const o of options) {
    const v = o.value ?? '';
    switch (o.name) {
      case 'request':
        explicitMethod = v.toUpperCase();
        break;
      case 'header': {
        if (v.startsWith('@')) {
          warn(`Headers from a file (${v}) were skipped`);
          break;
        }
        const colon = v.indexOf(':');
        if (colon < 0) {
          // "Name;" sends an empty header; a bare name is ignored by curl too.
          if (v.endsWith(';')) headers.push({ key: v.slice(0, -1).trim(), value: '', enabled: true });
          break;
        }
        const key = v.slice(0, colon).trim();
        const value = v.slice(colon + 1).trim();
        // "Name:" removes a header curl would add; there is nothing to remove here.
        if (key && value) headers.push({ key, value, enabled: true });
        break;
      }
      case 'data':
      case 'data-ascii':
      case 'data-binary':
        if (o.name === 'data-binary' && v.startsWith('@') && v !== '@-') {
          binaryFile = v.slice(1);
          break;
        }
        if (v.startsWith('@')) {
          warn(`Body from a file (${v}) is not supported; the file name was kept as the body`);
        }
        data.push(v);
        break;
      case 'data-raw':
        data.push(v);
        break;
      case 'data-urlencode':
        if (/^[^=]*@/.test(v)) warn(`Body from a file (${v}) is not supported`);
        data.push(encodeDataUrlencode(v));
        break;
      case 'json':
        json = true;
        data.push(v);
        break;
      case 'url':
        urls.push(v);
        break;
      case 'user':
        user = v;
        break;
      case 'oauth2-bearer':
        headers.push({ key: 'Authorization', value: `Bearer ${v}`, enabled: true });
        break;
      case 'cookie':
        if (v.includes('=')) setHeader(headers, 'Cookie', v);
        else warn(`Cookies from a file (${v}) were skipped`);
        break;
      case 'user-agent':
        setHeader(headers, 'User-Agent', v);
        break;
      case 'referer':
        setHeader(headers, 'Referer', v.replace(/;auto$/, ''));
        break;
      case 'max-time': {
        const secs = Number(v);
        if (secs > 0) timeoutMs = Math.min(600000, Math.max(1, Math.round(secs * 1000)));
        break;
      }
      case 'get':
        forceGet = true;
        break;
      case 'head':
        head = true;
        break;
      case 'location':
        break;
      case 'insecure':
        warn('--insecure was ignored; TLS verification is a global setting in Settings');
        break;
      case 'form':
      case 'form-string': {
        const field = parseFormOption(v, o.name === 'form-string');
        if (field) form.push(field);
        else warn(`Form field ${v} was skipped`);
        break;
      }
      case 'upload-file':
        if (v === '-' || v === '.') warn('Uploading from standard input (-T -) is not supported');
        else uploadFile = v;
        break;
      case 'proxy':
        warn('--proxy was ignored; the proxy is a global setting in Settings');
        break;
      default:
        if (!IGNORED.has(o.name)) warn(`Option ${o.name.startsWith('-') ? o.name : '--' + o.name} was ignored`);
    }
  }

  if (urls.length === 0) throw new CurlParseError('No URL found in the command');
  if (urls.length > 1) warn(`Only the first of ${urls.length} URLs was used`);
  let url = urls[0];
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(url) && !url.startsWith('{{')) url = 'http://' + url;

  let body = data.join('&');
  if (form.length && (data.length || binaryFile || uploadFile)) {
    warn('Only the form fields (-F) were used; curl cannot combine them with other body options either');
  } else if ((binaryFile || uploadFile) && data.length) {
    warn('The file body was used; other data options were skipped');
  }
  const fileBody = form.length ? '' : binaryFile || uploadFile;
  const hasBody = (data.length > 0 || form.length > 0 || fileBody !== '') && !forceGet;
  if (forceGet && data.length) {
    url += (url.includes('?') ? '&' : '?') + body;
    body = '';
  }

  let method = explicitMethod || (head ? 'HEAD' : uploadFile && !form.length ? 'PUT' : hasBody ? 'POST' : 'GET');
  if (!(METHODS as string[]).includes(method)) {
    warn(`Method ${method} is not supported; GET was used`);
    method = 'GET';
  }

  if (json) {
    if (headerIndex(headers, 'Content-Type') < 0) headers.push(kv('Content-Type', 'application/json'));
    if (headerIndex(headers, 'Accept') < 0) headers.push(kv('Accept', 'application/json'));
  } else if (form.length) {
    // The multipart Content-Type carries a boundary the app writes itself.
    const i = headerIndex(headers, 'Content-Type');
    if (i >= 0 && /^multipart\/form-data/i.test(headers[i].value)) headers.splice(i, 1);
  } else if (fileBody) {
    // curl sends --data-binary files as a form too; the app guesses the type from the file name.
  } else if (hasBody && headerIndex(headers, 'Content-Type') < 0) {
    // curl sends -d data as a form unless told otherwise; the app would send text/plain.
    headers.push(kv('Content-Type', 'application/x-www-form-urlencoded'));
  }

  const auth = authFromHeaders(headers, user);
  if (user && auth.type !== 'basic') warn('-u was ignored because the command also sets an Authorization header');

  const contentType = headers[headerIndex(headers, 'Content-Type')]?.value ?? '';
  let bodyType: Request['bodyType'] = 'none';
  if (form.length) {
    bodyType = 'form-data';
    body = '';
  } else if (fileBody) {
    bodyType = 'binary';
    body = '';
  } else if (hasBody) bodyType = /json/i.test(contentType) || isJson(body) ? 'json' : 'text';
  if (fileBody || form.some((f) => f.type === 'file')) {
    warn('File paths were kept as written; choose the files again if they are relative or on another machine');
  }
  if (bodyType !== 'none' && (method === 'GET' || method === 'HEAD')) {
    warn(`${method} requests are sent without a body; switch the method to send it`);
  }

  return {
    request: {
      name: defaultName(method, url),
      method: method as Method,
      url,
      headers,
      body,
      bodyType,
      form: bodyType === 'form-data' ? form : [],
      bodyFile: bodyType === 'binary' ? fileBody : '',
      auth,
      timeoutMs,
      followRedirects: true,
    },
    warnings,
  };
}

/**
 * Reads a -F / --form-string value: name=value, name=@file;type=…, or name=<file
 * (a file's content as a text field, kept as an upload). Returns null when there is no name.
 */
function parseFormOption(v: string, literal: boolean): FormField | null {
  const eq = v.indexOf('=');
  if (eq <= 0) return null;
  const key = v.slice(0, eq);
  const rest = v.slice(eq + 1);
  if (literal || !(rest.startsWith('@') || rest.startsWith('<'))) {
    return { key, value: literal ? rest : rest.replace(/;type=[^;]*$/, ''), type: 'text', enabled: true };
  }
  let spec = rest.slice(1);
  let file: string;
  if (spec.startsWith('"')) {
    const end = /^"((?:[^"\\]|\\.)*)"/.exec(spec);
    file = end ? end[1].replace(/\\(.)/g, '$1') : spec.slice(1);
    spec = end ? spec.slice(end[0].length) : '';
  } else {
    const semi = spec.indexOf(';');
    file = semi >= 0 ? spec.slice(0, semi) : spec;
    spec = semi >= 0 ? spec.slice(semi) : '';
  }
  const contentType = /;type=([^;]+)/.exec(spec)?.[1]?.trim() ?? '';
  return { key, value: '', type: 'file', file, contentType, enabled: true };
}

function kv(key: string, value: string): KeyValue {
  return { key, value, enabled: true };
}

function isJson(s: string): boolean {
  const t = s.trim();
  if (!t.startsWith('{') && !t.startsWith('[')) return false;
  try {
    JSON.parse(t);
    return true;
  } catch {
    return false;
  }
}

/** Moves Bearer/Basic credentials from an Authorization header (or -u) into the request's auth settings. */
function authFromHeaders(headers: KeyValue[], user: string): Auth {
  const i = headerIndex(headers, 'Authorization');
  if (i >= 0) {
    const value = headers[i].value;
    const bearer = /^Bearer\s+(.+)$/i.exec(value);
    if (bearer) {
      headers.splice(i, 1);
      return { type: 'bearer', token: bearer[1].trim() };
    }
    const basic = /^Basic\s+(\S+)$/i.exec(value);
    const decoded = basic ? decodeBase64(basic[1]) : null;
    if (decoded !== null && decoded.includes(':')) {
      headers.splice(i, 1);
      const colon = decoded.indexOf(':');
      return { type: 'basic', username: decoded.slice(0, colon), password: decoded.slice(colon + 1) };
    }
    return { type: 'none' };
  }
  if (user) {
    const colon = user.indexOf(':');
    return colon < 0
      ? { type: 'basic', username: user, password: '' }
      : { type: 'basic', username: user.slice(0, colon), password: user.slice(colon + 1) };
  }
  return { type: 'inherit' };
}

// ---- generating ----

export interface CurlOptions {
  /** Auth of the request's collection, used when the request's auth is "inherit". */
  collectionAuth?: Auth;
  /** When set, {{variables}} are replaced with values from the environment, then the collection. */
  resolve?: {
    environment: Environment | null;
    collectionVariables: Variable[];
    /** Also fill in variables marked secret; otherwise they stay as {{name}}. */
    includeSecrets: boolean;
  };
}

/** Quotes a word for a POSIX shell, leaving simple words bare. */
export function shellQuote(s: string): string {
  if (s !== '' && /^[A-Za-z0-9_@%+=:,./-]+$/.test(s)) return s;
  return `'${s.replace(/'/g, `'\\''`)}'`;
}

/** Fills in {{variables}} the way the Go resolver does; unresolved and (optionally) secret ones are kept. */
export function resolveVariables(text: string, opts: CurlOptions['resolve']): string {
  if (!opts || !text) return text;
  return splitVariables(text)
    .map((seg) => {
      if (!seg.name) return seg.text;
      const info = describeVariable(seg.name, opts.environment, opts.collectionVariables);
      if (info.state === 'builtin') return builtin(seg.name) ?? seg.text;
      if (!info.variable?.enabled) return seg.text;
      if (info.variable.secret && !opts.includeSecrets) return seg.text;
      return info.variable.value;
    })
    .join('');
}

function builtin(name: string): string | undefined {
  if (name === '$uuid') return crypto.randomUUID();
  if (name === '$timestamp') return String(Math.floor(Date.now() / 1000));
  if (name === '$isoTimestamp') return new Date().toISOString();
  return undefined;
}

/** Percent-encodes text but leaves {{variable}} references readable. */
function encodeKeepingVariables(s: string): string {
  return splitVariables(s)
    .map((seg) => (seg.name ? seg.text : encodeURIComponent(seg.text)))
    .join('');
}

function utf8Base64(s: string): string {
  let bin = '';
  for (const b of new TextEncoder().encode(s)) bin += String.fromCharCode(b);
  return btoa(bin);
}

/** A request as the app would send it: auth applied, query appended, Content-Type set. */
export interface PreparedRequest {
  method: string;
  url: string;
  headers: [string, string][];
  /** Set when a text body is sent (never for GET or HEAD); URL-encoded forms are encoded into it. */
  body?: string;
  bodyType: Request['bodyType'];
  /** The parts of a form-data body. Content-Type is left to the client library, which adds the boundary. */
  form?: FormPart[];
  /** The file a binary body sends. */
  file?: string;
  /** Basic auth credentials, when the generator can express them natively. */
  basic?: { username: string; password: string };
  followRedirects: boolean;
  timeoutMs: number;
}

/** A form-data part: a text field, or a file with its file name and content type. */
export type FormPart =
  | { name: string; value: string; file?: undefined }
  | { name: string; file: string; fileName: string; contentType: string };

export type GeneratorRequest = Pick<
  Request,
  'method' | 'url' | 'headers' | 'query' | 'body' | 'bodyType' | 'auth' | 'timeoutMs'
> &
  Partial<Pick<Request, 'followRedirects' | 'form' | 'bodyFile'>>;

/** Whether the prepared request sends a body of any kind. */
export const hasBody = (p: PreparedRequest) => p.body !== undefined || p.form !== undefined || p.file !== undefined;

const MIME_TYPES: Record<string, string> = {
  json: 'application/json',
  txt: 'text/plain',
  csv: 'text/csv',
  html: 'text/html',
  xml: 'application/xml',
  pdf: 'application/pdf',
  zip: 'application/zip',
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  svg: 'image/svg+xml',
  mp4: 'video/mp4',
  mp3: 'audio/mpeg',
};

/** The last segment of a path, for Windows and POSIX paths alike. */
export const fileName = (path: string) => path.split(/[\\/]/).pop() || path;

/** A content type guessed from the file extension, like the app does. */
export function guessContentType(path: string): string {
  const ext = /\.([A-Za-z0-9]+)$/.exec(path)?.[1]?.toLowerCase() ?? '';
  return MIME_TYPES[ext] ?? 'application/octet-stream';
}

/**
 * Resolves what the runner would send for req, shared by every code generator.
 * Basic auth stays separate when the user name has no colon (so curl -u, Python's
 * auth=… and axios' auth work); otherwise it becomes an Authorization header.
 */
export function prepareRequest(req: GeneratorRequest, opts: CurlOptions = {}): PreparedRequest {
  const r = (s: string) => resolveVariables(s, opts.resolve);
  const headers: [string, string][] = req.headers
    .filter((h) => h.enabled && h.key.trim())
    .map((h) => [r(h.key.trim()), r(h.value)]);
  const has = (name: string) => headers.some(([k]) => k.toLowerCase() === name.toLowerCase());
  const set = (name: string, value: string) => {
    const i = headers.findIndex(([k]) => k.toLowerCase() === name.toLowerCase());
    if (i >= 0) headers[i] = [headers[i][0], value];
    else headers.push([name, value]);
  };

  const query: [string, string][] = req.query
    .filter((q) => q.enabled && q.key.trim())
    .map((q) => [r(q.key), r(q.value)]);

  const method = req.method;
  const sendsBody = req.bodyType !== 'none' && method !== 'GET' && method !== 'HEAD';
  const fields = (req.form ?? []).filter((f) => f.enabled && f.key.trim());
  let body: string | undefined;
  let form: FormPart[] | undefined;
  let file: string | undefined;
  if (sendsBody) {
    switch (req.bodyType) {
      case 'urlencoded': {
        const enc = opts.resolve ? encodeURIComponent : encodeKeepingVariables;
        body = fields
          .filter((f) => f.type !== 'file')
          .map((f) => `${enc(r(f.key))}=${enc(r(f.value))}`)
          .join('&');
        if (!has('Content-Type')) set('Content-Type', 'application/x-www-form-urlencoded');
        break;
      }
      case 'form-data':
        form = fields.map((f): FormPart => {
          if (f.type !== 'file') return { name: r(f.key), value: r(f.value) };
          const path = r(f.file ?? '');
          return {
            name: r(f.key),
            file: path,
            fileName: fileName(path),
            contentType: r(f.contentType ?? '').trim() || guessContentType(path),
          };
        });
        // The library writes the multipart Content-Type with its own boundary, as the app does.
        for (let i = headers.length - 1; i >= 0; i--) {
          if (headers[i][0].toLowerCase() === 'content-type') headers.splice(i, 1);
        }
        break;
      case 'binary':
        file = r(req.bodyFile ?? '');
        if (!has('Content-Type')) set('Content-Type', guessContentType(file));
        break;
      default:
        body = r(req.body);
        if (!has('Content-Type')) {
          set('Content-Type', req.bodyType === 'json' ? 'application/json' : 'text/plain; charset=utf-8');
        }
    }
  }

  const auth = req.auth.type === 'inherit' ? (opts.collectionAuth ?? { type: 'none' }) : req.auth;
  let basic: PreparedRequest['basic'];
  switch (auth.type) {
    case 'bearer':
      set('Authorization', `Bearer ${r(auth.token ?? '')}`);
      break;
    case 'basic': {
      const u = r(auth.username ?? '');
      const p = r(auth.password ?? '');
      if (u.includes(':')) set('Authorization', `Basic ${utf8Base64(`${u}:${p}`)}`);
      else basic = { username: u, password: p };
      break;
    }
    case 'apikey':
      if (auth.keyName) {
        if (auth.keyIn === 'query') query.push([r(auth.keyName), r(auth.keyValue ?? '')]);
        else set(r(auth.keyName), r(auth.keyValue ?? ''));
      }
      break;
  }

  let url = r(req.url);
  if (query.length) {
    const enc = opts.resolve ? encodeURIComponent : encodeKeepingVariables;
    const qs = query.map(([k, v]) => `${enc(k)}=${enc(v)}`).join('&');
    const hash = url.indexOf('#');
    const [base, frag] = hash >= 0 ? [url.slice(0, hash), url.slice(hash)] : [url, ''];
    url = base + (base.includes('?') ? (/[?&]$/.test(base) ? '' : '&') : '?') + qs + frag;
  }
  return {
    method,
    url,
    headers,
    body,
    bodyType: req.bodyType,
    form,
    file,
    basic,
    followRedirects: req.followRedirects ?? true,
    timeoutMs: req.timeoutMs || 30000,
  };
}

/** Builds a multi-line cURL command that sends what the app would send for this request. */
export function toCurl(req: GeneratorRequest, opts: CurlOptions = {}): string {
  const p = prepareRequest(req, opts);
  const { method, url, headers } = p;
  const sendsBody = hasBody(p);
  const user = p.basic ? `${p.basic.username}:${p.basic.password}` : '';
  const followRedirects = p.followRedirects;
  const timeoutMs = req.timeoutMs;

  const parts: string[] = [];
  if (method === 'HEAD') parts.push('--head');
  else if (!(method === 'GET' && !sendsBody) && !(method === 'POST' && sendsBody)) parts.push(`-X ${method}`);
  parts.push(shellQuote(url));
  for (const [k, v] of headers) parts.push(`-H ${shellQuote(`${k}: ${v}`)}`);
  if (user) parts.push(`-u ${shellQuote(user)}`);
  if (p.body !== undefined) parts.push(`--data-raw ${shellQuote(p.body)}`);
  for (const f of p.form ?? []) {
    // --form-string sends text as is; -F would read a value starting with @ or < from a file.
    if (f.file === undefined) parts.push(`--form-string ${shellQuote(`${f.name}=${f.value}`)}`);
    else parts.push(`-F ${shellQuote(`${f.name}=@"${f.file.replace(/(["\\])/g, '\\$1')}";type=${f.contentType}`)}`);
  }
  if (p.file !== undefined) parts.push(`--data-binary ${shellQuote('@' + p.file)}`);
  if (followRedirects) parts.push('-L');
  if (timeoutMs && timeoutMs !== 30000) parts.push(`--max-time ${+(timeoutMs / 1000).toFixed(3)}`);

  const [first, ...rest] = parts;
  return [`curl ${first}`, ...rest].join(' \\\n  ');
}
