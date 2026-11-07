/**
 * Turns a request into code that sends the same thing: cURL, JavaScript fetch,
 * axios, Python requests or Go net/http. Every generator starts from
 * prepareRequest (curl.ts), so auth, query parameters, Content-Type,
 * redirects and timeouts match what the app sends. Scripts are not included.
 */
import { prepareRequest, toCurl, type CurlOptions, type GeneratorRequest, type PreparedRequest } from './curl';

export type CodeLanguage =
  | 'curl'
  | 'fetch'
  | 'axios'
  | 'python'
  | 'go'
  | 'php'
  | 'java'
  | 'csharp'
  | 'dart'
  | 'swift'
  | 'objc'
  | 'cpp'
  | 'rust';

export const CODE_LANGUAGES: { value: CodeLanguage; label: string }[] = [
  { value: 'curl', label: 'cURL' },
  { value: 'fetch', label: 'JavaScript — fetch' },
  { value: 'axios', label: 'JavaScript — axios' },
  { value: 'python', label: 'Python — requests' },
  { value: 'go', label: 'Go — net/http' },
  { value: 'php', label: 'PHP — cURL' },
  { value: 'java', label: 'Java — HttpClient' },
  { value: 'csharp', label: 'C# — HttpClient' },
  { value: 'dart', label: 'Dart — http' },
  { value: 'swift', label: 'Swift — URLSession' },
  { value: 'objc', label: 'Objective-C — NSURLSession' },
  { value: 'cpp', label: 'C++ — libcurl' },
  { value: 'rust', label: 'Rust — reqwest' },
];

/** Generates code for req in language. */
export function generateCode(language: CodeLanguage, req: GeneratorRequest, opts: CurlOptions = {}): string {
  if (language === 'curl') return toCurl(req, opts);
  const p = prepareRequest(req, opts);
  switch (language) {
    case 'fetch':
      return toFetch(p);
    case 'axios':
      return toAxios(p);
    case 'python':
      return toPython(p);
    case 'go':
      return toGo(p);
    case 'php':
      return toPhp(p);
    case 'java':
      return toJava(p);
    case 'csharp':
      return toCSharp(p);
    case 'dart':
      return toDart(p);
    case 'swift':
      return toSwift(p);
    case 'objc':
      return toObjC(p);
    case 'cpp':
      return toCpp(p);
    case 'rust':
      return toRust(p);
  }
}

/** A double-quoted string literal; valid in JavaScript, Python and Go. */
const str = (s: string) => JSON.stringify(s);

/** The body parsed as JSON when it is a JSON body without unresolved {{variables}}. */
function jsonBody(p: PreparedRequest): { value: unknown } | null {
  if (p.body === undefined || p.bodyType !== 'json' || p.body.includes('{{')) return null;
  try {
    return { value: JSON.parse(p.body) };
  } catch {
    return null;
  }
}

function indent(text: string, by: string): string {
  return text.replace(/\n/g, '\n' + by);
}

function basicHeader(p: PreparedRequest): [string, string][] {
  if (!p.basic) return p.headers;
  let bin = '';
  for (const b of new TextEncoder().encode(`${p.basic.username}:${p.basic.password}`)) bin += String.fromCharCode(b);
  return [...p.headers, ['Authorization', `Basic ${btoa(bin)}`]];
}

const timeoutSeconds = (p: PreparedRequest) => +(p.timeoutMs / 1000).toFixed(3);

// ---- JavaScript ----

function jsHeaders(headers: [string, string][], pad: string): string {
  if (headers.length === 0) return '{}';
  return `{\n${headers.map(([k, v]) => `${pad}  ${str(k)}: ${str(v)},`).join('\n')}\n${pad}}`;
}

function toFetch(p: PreparedRequest): string {
  const headers = basicHeader(p);
  const lines = [`  method: ${str(p.method)},`];
  if (headers.length) lines.push(`  headers: ${jsHeaders(headers, '  ')},`);
  const json = jsonBody(p);
  if (json) lines.push(`  body: JSON.stringify(${indent(JSON.stringify(json.value, null, 2), '  ')}),`);
  else if (p.body !== undefined) lines.push(`  body: ${str(p.body)},`);
  if (!p.followRedirects) lines.push(`  redirect: 'manual',`);
  lines.push(`  signal: AbortSignal.timeout(${p.timeoutMs}),`);
  return [
    `const response = await fetch(${str(p.url)}, {`,
    ...lines,
    `});`,
    p.method === 'HEAD' ? `console.log(response.status);` : `console.log(response.status, await response.text());`,
  ].join('\n');
}

function toAxios(p: PreparedRequest): string {
  const lines = [`  method: ${str(p.method.toLowerCase())},`, `  url: ${str(p.url)},`];
  if (p.headers.length) lines.push(`  headers: ${jsHeaders(p.headers, '  ')},`);
  if (p.basic) lines.push(`  auth: { username: ${str(p.basic.username)}, password: ${str(p.basic.password)} },`);
  const json = jsonBody(p);
  if (json) lines.push(`  data: ${indent(JSON.stringify(json.value, null, 2), '  ')},`);
  else if (p.body !== undefined) lines.push(`  data: ${str(p.body)},`);
  if (!p.followRedirects) lines.push(`  maxRedirects: 0,`);
  lines.push(`  timeout: ${p.timeoutMs},`);
  // Like the app, report every status instead of throwing on 4xx/5xx.
  lines.push(`  validateStatus: () => true,`);
  return [
    `import axios from 'axios';`,
    ``,
    `const response = await axios({`,
    ...lines,
    `});`,
    `console.log(response.status, response.data);`,
  ].join('\n');
}

// ---- Python ----

/** A JSON value as a Python literal (True, False, None). */
function pyValue(v: unknown, pad: string): string {
  if (v === null) return 'None';
  if (v === true) return 'True';
  if (v === false) return 'False';
  if (typeof v === 'number' || typeof v === 'string') return JSON.stringify(v);
  const inner = pad + '    ';
  if (Array.isArray(v)) {
    if (v.length === 0) return '[]';
    return `[\n${v.map((x) => inner + pyValue(x, inner)).join(',\n')},\n${pad}]`;
  }
  const entries = Object.entries(v as Record<string, unknown>);
  if (entries.length === 0) return '{}';
  return `{\n${entries.map(([k, x]) => `${inner}${str(k)}: ${pyValue(x, inner)}`).join(',\n')},\n${pad}}`;
}

function toPython(p: PreparedRequest): string {
  const args = [`    ${str(p.method)},`, `    ${str(p.url)},`];
  const json = jsonBody(p);
  // requests sets Content-Type: application/json itself for json=…
  const headers = json
    ? p.headers.filter(([k, v]) => !(k.toLowerCase() === 'content-type' && v === 'application/json'))
    : p.headers;
  const pre: string[] = [];
  if (headers.length) {
    pre.push(`headers = {\n${headers.map(([k, v]) => `    ${str(k)}: ${str(v)},`).join('\n')}\n}`);
    args.push(`    headers=headers,`);
  }
  if (json) {
    pre.push(`payload = ${pyValue(json.value, '')}`);
    args.push(`    json=payload,`);
  } else if (p.body !== undefined) {
    args.push(`    data=${str(p.body)}.encode("utf-8"),`);
  }
  if (p.basic) args.push(`    auth=(${str(p.basic.username)}, ${str(p.basic.password)}),`);
  if (!p.followRedirects || p.method === 'HEAD')
    args.push(`    allow_redirects=${p.followRedirects ? 'True' : 'False'},`);
  args.push(`    timeout=${timeoutSeconds(p)},`);
  return [
    `import requests`,
    ``,
    ...pre.flatMap((x) => [x, '']),
    `response = requests.request(`,
    ...args,
    `)`,
    p.method === 'HEAD' ? `print(response.status_code)` : `print(response.status_code, response.text)`,
  ].join('\n');
}

// ---- Go ----

/** A Go string literal: a raw `…` string for multi-line text when possible. */
function goString(s: string): string {
  if (s.includes('\n') && !s.includes('`') && !s.includes('\r')) return '`' + s + '`';
  return str(s);
}

function toGo(p: PreparedRequest): string {
  const json = jsonBody(p);
  const body = json ? JSON.stringify(json.value, null, 2) : p.body;
  const imports = ['"fmt"', '"io"', '"net/http"', '"time"'];
  if (body !== undefined) imports.push('"strings"');
  imports.sort();

  const out = [`package main`, ``, `import (`, ...imports.map((i) => `\t${i}`), `)`, ``, `func main() {`];
  if (body !== undefined) out.push(`\tbody := strings.NewReader(${goString(body)})`);
  out.push(
    `\treq, err := http.NewRequest(${str(p.method)}, ${str(p.url)}, ${body !== undefined ? 'body' : 'nil'})`,
    `\tif err != nil {`,
    `\t\tpanic(err)`,
    `\t}`,
    ...p.headers.map(([k, v]) => `\treq.Header.Set(${str(k)}, ${str(v)})`),
  );
  if (p.basic) out.push(`\treq.SetBasicAuth(${str(p.basic.username)}, ${str(p.basic.password)})`);
  out.push(``, `\tclient := &http.Client{`, `\t\tTimeout: ${goDuration(p.timeoutMs)},`);
  if (!p.followRedirects) {
    out.push(`\t\tCheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },`);
  }
  out.push(
    `\t}`,
    `\tresp, err := client.Do(req)`,
    `\tif err != nil {`,
    `\t\tpanic(err)`,
    `\t}`,
    `\tdefer resp.Body.Close()`,
    `\tdata, err := io.ReadAll(resp.Body)`,
    `\tif err != nil {`,
    `\t\tpanic(err)`,
    `\t}`,
    `\tfmt.Println(resp.Status, string(data))`,
    `}`,
  );
  return out.join('\n');
}

function goDuration(ms: number): string {
  if (ms % 1000 === 0) return `${ms / 1000} * time.Second`;
  return `${ms} * time.Millisecond`;
}

// ---- string literals for the other languages ----

/**
 * A PHP string: single-quoted (no $ interpolation) when possible; double-quoted
 * with escapes when it holds control characters, which single quotes cannot express.
 */
function phpStr(s: string): string {
  if (!/[\x00-\x1f\x7f]/.test(s)) return `'${s.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`;
  let out = '"';
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (ch === '"' || ch === '\\' || ch === '$') out += '\\' + ch;
    else if (ch === '\n') out += '\\n';
    else if (ch === '\r') out += '\\r';
    else if (ch === '\t') out += '\\t';
    else if (c < 0x20 || c === 0x7f) out += '\\x' + c.toString(16).padStart(2, '0');
    else out += ch;
  }
  return out + '"';
}

/** A Dart string: JSON escapes are valid; $ would interpolate. */
const dartStr = (s: string) => JSON.stringify(s).replace(/\$/g, '\\$');

/** A Swift or Rust string: \u{…} for control characters (neither has \b, \f or \uXXXX). */
function swiftStr(s: string): string {
  let out = '"';
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (ch === '"') out += '\\"';
    else if (ch === '\\') out += '\\\\';
    else if (ch === '\n') out += '\\n';
    else if (ch === '\r') out += '\\r';
    else if (ch === '\t') out += '\\t';
    else if (c < 0x20 || c === 0x7f) out += `\\u{${c.toString(16)}}`;
    else out += ch;
  }
  return out + '"';
}

/** A C / C++ / Objective-C string body: octal escapes for control characters, UTF-8 kept as is. */
function cEscape(s: string): string {
  let out = '';
  let prev = '';
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    if (ch === '"') out += '\\"';
    else if (ch === '\\') out += '\\\\';
    else if (ch === '\n') out += '\\n';
    else if (ch === '\r') out += '\\r';
    else if (ch === '\t') out += '\\t';
    // "??=" and friends are trigraphs in older C/C++; escaping the second "?" defuses them.
    else if (ch === '?' && prev === '?') out += '\\?';
    // Octal escapes are always 3 digits, so a following digit cannot extend them.
    else if (c < 0x20 || c === 0x7f) out += '\\' + c.toString(8).padStart(3, '0');
    else out += ch;
    prev = ch;
  }
  return out;
}
const cStr = (s: string) => `"${cEscape(s)}"`;
const objcStr = (s: string) => `@"${cEscape(s)}"`;

/** The body to send: pretty-printed when it is valid JSON, else as written. */
function bodyText(p: PreparedRequest): string | undefined {
  const json = jsonBody(p);
  return json ? JSON.stringify(json.value, null, 2) : p.body;
}

// ---- PHP ----

function toPhp(p: PreparedRequest): string {
  const body = bodyText(p);
  const opts = [
    `    CURLOPT_URL => ${phpStr(p.url)},`,
    p.method === 'HEAD' ? `    CURLOPT_NOBODY => true,` : `    CURLOPT_CUSTOMREQUEST => ${phpStr(p.method)},`,
    `    CURLOPT_RETURNTRANSFER => true,`,
    `    CURLOPT_FOLLOWLOCATION => ${p.followRedirects ? 'true' : 'false'},`,
    `    CURLOPT_TIMEOUT_MS => ${p.timeoutMs},`,
  ];
  if (p.headers.length) {
    opts.push(
      `    CURLOPT_HTTPHEADER => [`,
      ...p.headers.map(([k, v]) => `        ${phpStr(`${k}: ${v}`)},`),
      `    ],`,
    );
  }
  if (p.basic) opts.push(`    CURLOPT_USERPWD => ${phpStr(`${p.basic.username}:${p.basic.password}`)},`);
  if (body !== undefined) opts.push(`    CURLOPT_POSTFIELDS => ${phpStr(body)},`);
  return [
    `<?php`,
    ``,
    `$curl = curl_init();`,
    `curl_setopt_array($curl, [`,
    ...opts,
    `]);`,
    `$response = curl_exec($curl);`,
    `if ($response === false) {`,
    `    throw new RuntimeException(curl_error($curl));`,
    `}`,
    `echo curl_getinfo($curl, CURLINFO_RESPONSE_CODE), " ", $response, PHP_EOL;`,
    `curl_close($curl);`,
  ].join('\n');
}

// ---- Java ----

/** Headers Java's HttpClient sets itself and refuses (IllegalArgumentException). */
const JAVA_RESTRICTED = new Set(['connection', 'content-length', 'expect', 'host', 'upgrade']);

function toJava(p: PreparedRequest): string {
  const body = bodyText(p);
  const headers = basicHeader(p);
  const skipped = headers.filter(([k]) => JAVA_RESTRICTED.has(k.toLowerCase()));
  const lines = [
    `import java.net.URI;`,
    `import java.net.http.HttpClient;`,
    `import java.net.http.HttpRequest;`,
    `import java.net.http.HttpResponse;`,
    `import java.time.Duration;`,
    ``,
    `public class Main {`,
    `    public static void main(String[] args) throws Exception {`,
    `        HttpClient client = HttpClient.newBuilder()`,
    `                .followRedirects(HttpClient.Redirect.${p.followRedirects ? 'NORMAL' : 'NEVER'})`,
    `                .build();`,
  ];
  if (body !== undefined) lines.push(`        String body = ${str(body)};`);
  lines.push(
    `        HttpRequest request = HttpRequest.newBuilder(URI.create(${str(p.url)}))`,
    `                .timeout(Duration.ofMillis(${p.timeoutMs}))`,
  );
  if (skipped.length) {
    lines.push(`                // Set by HttpClient itself: ${skipped.map(([k]) => k).join(', ')}`);
  }
  for (const [k, v] of headers)
    if (!JAVA_RESTRICTED.has(k.toLowerCase())) lines.push(`                .header(${str(k)}, ${str(v)})`);
  lines.push(
    body !== undefined
      ? `                .method(${str(p.method)}, HttpRequest.BodyPublishers.ofString(body))`
      : `                .method(${str(p.method)}, HttpRequest.BodyPublishers.noBody())`,
    `                .build();`,
    `        HttpResponse<String> response = client.send(request, HttpResponse.BodyHandlers.ofString());`,
    `        System.out.println(response.statusCode() + " " + response.body());`,
    `    }`,
    `}`,
  );
  return lines.join('\n');
}

// ---- C# ----

/** Headers that belong to the content in .NET, not to the request. */
const DOTNET_CONTENT_HEADERS = new Set([
  'allow',
  'content-disposition',
  'content-encoding',
  'content-language',
  'content-length',
  'content-location',
  'content-md5',
  'content-range',
  'content-type',
  'expires',
  'last-modified',
]);

function toCSharp(p: PreparedRequest): string {
  const body = bodyText(p);
  const headers = basicHeader(p);
  const content = headers.filter(([k]) => DOTNET_CONTENT_HEADERS.has(k.toLowerCase()));
  const lines = [
    `using System.Text;`,
    ``,
    `using var handler = new HttpClientHandler { AllowAutoRedirect = ${p.followRedirects ? 'true' : 'false'} };`,
    `using var client = new HttpClient(handler) { Timeout = TimeSpan.FromMilliseconds(${p.timeoutMs}) };`,
    `using var request = new HttpRequestMessage(new HttpMethod(${str(p.method)}), ${str(p.url)});`,
  ];
  for (const [k, v] of headers) {
    if (!DOTNET_CONTENT_HEADERS.has(k.toLowerCase()))
      lines.push(`request.Headers.TryAddWithoutValidation(${str(k)}, ${str(v)});`);
  }
  if (body !== undefined) {
    lines.push(`request.Content = new StringContent(${str(body)}, Encoding.UTF8);`);
    // StringContent adds "text/plain; charset=utf-8"; replace it with the request's own content headers.
    if (content.length) lines.push(`request.Content.Headers.Remove("Content-Type");`);
    for (const [k, v] of content) lines.push(`request.Content.Headers.TryAddWithoutValidation(${str(k)}, ${str(v)});`);
  }
  lines.push(
    ``,
    `using var response = await client.SendAsync(request);`,
    `Console.WriteLine($"{(int)response.StatusCode} {await response.Content.ReadAsStringAsync()}");`,
  );
  return lines.join('\n');
}

// ---- Dart ----

function toDart(p: PreparedRequest): string {
  const body = bodyText(p);
  const headers = basicHeader(p);
  const lines = [
    `import 'package:http/http.dart' as http;`,
    ``,
    `Future<void> main() async {`,
    `  final request = http.Request(${dartStr(p.method)}, Uri.parse(${dartStr(p.url)}))`,
    `    ..followRedirects = ${p.followRedirects ? 'true' : 'false'};`,
  ];
  if (headers.length) {
    lines.push(`  request.headers.addAll({`, ...headers.map(([k, v]) => `    ${dartStr(k)}: ${dartStr(v)},`), `  });`);
  }
  if (body !== undefined) lines.push(`  request.body = ${dartStr(body)};`);
  lines.push(
    ``,
    `  final client = http.Client();`,
    `  try {`,
    `    final streamed = await client.send(request).timeout(const Duration(milliseconds: ${p.timeoutMs}));`,
    `    final response = await http.Response.fromStream(streamed);`,
    `    print('\${response.statusCode} \${response.body}');`,
    `  } finally {`,
    `    client.close();`,
    `  }`,
    `}`,
  );
  return lines.join('\n');
}

// ---- Swift ----

function toSwift(p: PreparedRequest): string {
  const body = bodyText(p);
  const headers = basicHeader(p);
  const lines = [
    `import Foundation`,
    ``,
    `var request = URLRequest(url: URL(string: ${swiftStr(p.url)})!, timeoutInterval: ${timeoutSeconds(p)})`,
    `request.httpMethod = ${swiftStr(p.method)}`,
    ...headers.map(([k, v]) => `request.setValue(${swiftStr(v)}, forHTTPHeaderField: ${swiftStr(k)})`),
  ];
  if (body !== undefined) lines.push(`request.httpBody = Data(${swiftStr(body)}.utf8)`);
  if (p.followRedirects) {
    lines.push(``, `let (data, response) = try await URLSession.shared.data(for: request)`);
  } else {
    lines.push(
      ``,
      `/// Returns redirects as they are instead of following them.`,
      `final class NoRedirects: NSObject, URLSessionTaskDelegate {`,
      `    func urlSession(`,
      `        _ session: URLSession, task: URLSessionTask,`,
      `        willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest`,
      `    ) async -> URLRequest? { nil }`,
      `}`,
      `let (data, response) = try await URLSession.shared.data(for: request, delegate: NoRedirects())`,
    );
  }
  lines.push(
    `let status = (response as? HTTPURLResponse)?.statusCode ?? 0`,
    `print(status, String(decoding: data, as: UTF8.self))`,
  );
  return lines.join('\n');
}

// ---- Objective-C ----

function toObjC(p: PreparedRequest): string {
  const body = bodyText(p);
  const headers = basicHeader(p);
  const lines = [`#import <Foundation/Foundation.h>`, ``];
  if (!p.followRedirects) {
    lines.push(
      `// Returns redirects as they are instead of following them.`,
      `@interface NoRedirects : NSObject <NSURLSessionTaskDelegate>`,
      `@end`,
      `@implementation NoRedirects`,
      `- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task`,
      `    willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request`,
      `    completionHandler:(void (^)(NSURLRequest *))completionHandler {`,
      `    completionHandler(nil);`,
      `}`,
      `@end`,
      ``,
    );
  }
  lines.push(
    `int main(int argc, const char *argv[]) {`,
    `    @autoreleasepool {`,
    `        NSURL *url = [NSURL URLWithString:${objcStr(p.url)}];`,
    `        NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url`,
    `                                                               cachePolicy:NSURLRequestUseProtocolCachePolicy`,
    `                                                           timeoutInterval:${timeoutSeconds(p)}];`,
    `        request.HTTPMethod = ${objcStr(p.method)};`,
    ...headers.map(([k, v]) => `        [request setValue:${objcStr(v)} forHTTPHeaderField:${objcStr(k)}];`),
  );
  if (body !== undefined)
    lines.push(`        request.HTTPBody = [${objcStr(body)} dataUsingEncoding:NSUTF8StringEncoding];`);
  lines.push(
    ``,
    p.followRedirects
      ? `        NSURLSession *session = [NSURLSession sharedSession];`
      : [
          `        NSURLSessionConfiguration *config = [NSURLSessionConfiguration defaultSessionConfiguration];`,
          `        NSURLSession *session = [NSURLSession sessionWithConfiguration:config`,
          `                                                              delegate:[NoRedirects new]`,
          `                                                         delegateQueue:nil];`,
        ].join('\n'),
    `        dispatch_semaphore_t done = dispatch_semaphore_create(0);`,
    `        [[session dataTaskWithRequest:request`,
    `                    completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {`,
    `            if (error) {`,
    `                NSLog(@"%@", error);`,
    `            } else {`,
    `                NSLog(@"%ld %@", (long)((NSHTTPURLResponse *)response).statusCode,`,
    `                      [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding]);`,
    `            }`,
    `            dispatch_semaphore_signal(done);`,
    `        }] resume];`,
    `        dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);`,
    `    }`,
    `    return 0;`,
    `}`,
  );
  return lines.join('\n');
}

// ---- C++ ----

function toCpp(p: PreparedRequest): string {
  const body = bodyText(p);
  const lines = [
    `// Build: g++ main.cpp -lcurl`,
    `#include <curl/curl.h>`,
    ``,
    `#include <iostream>`,
    `#include <string>`,
    ``,
    `static size_t collect(char *data, size_t size, size_t count, void *out) {`,
    `    static_cast<std::string *>(out)->append(data, size * count);`,
    `    return size * count;`,
    `}`,
    ``,
    `int main() {`,
    `    curl_global_init(CURL_GLOBAL_DEFAULT);`,
    `    CURL *curl = curl_easy_init();`,
    `    if (!curl) return 1;`,
    ``,
    `    curl_easy_setopt(curl, CURLOPT_URL, ${cStr(p.url)});`,
    p.method === 'HEAD'
      ? `    curl_easy_setopt(curl, CURLOPT_NOBODY, 1L);`
      : `    curl_easy_setopt(curl, CURLOPT_CUSTOMREQUEST, ${cStr(p.method)});`,
  ];
  if (p.headers.length) {
    lines.push(`    struct curl_slist *headers = nullptr;`);
    for (const [k, v] of p.headers) lines.push(`    headers = curl_slist_append(headers, ${cStr(`${k}: ${v}`)});`);
    lines.push(`    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);`);
  }
  if (p.basic)
    lines.push(`    curl_easy_setopt(curl, CURLOPT_USERPWD, ${cStr(`${p.basic.username}:${p.basic.password}`)});`);
  if (body !== undefined) {
    lines.push(
      `    std::string body = ${cStr(body)};`,
      `    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, body.c_str());`,
      `    curl_easy_setopt(curl, CURLOPT_POSTFIELDSIZE, static_cast<long>(body.size()));`,
    );
  }
  lines.push(
    `    curl_easy_setopt(curl, CURLOPT_FOLLOWLOCATION, ${p.followRedirects ? '1L' : '0L'});`,
    `    curl_easy_setopt(curl, CURLOPT_TIMEOUT_MS, ${p.timeoutMs}L);`,
    `    std::string response;`,
    `    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, collect);`,
    `    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response);`,
    ``,
    `    CURLcode res = curl_easy_perform(curl);`,
    `    if (res != CURLE_OK) {`,
    `        std::cerr << curl_easy_strerror(res) << std::endl;`,
    `    } else {`,
    `        long status = 0;`,
    `        curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &status);`,
    `        std::cout << status << " " << response << std::endl;`,
    `    }`,
  );
  if (p.headers.length) lines.push(`    curl_slist_free_all(headers);`);
  lines.push(`    curl_easy_cleanup(curl);`, `    curl_global_cleanup();`, `    return res == CURLE_OK ? 0 : 1;`, `}`);
  return lines.join('\n');
}

// ---- Rust ----

const rustStr = swiftStr;

function toRust(p: PreparedRequest): string {
  const body = bodyText(p);
  const method = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'].includes(p.method)
    ? `reqwest::Method::${p.method}`
    : `reqwest::Method::from_bytes(${rustStr(p.method)}.as_bytes())?`;
  const lines = [
    `// Cargo.toml: reqwest = { version = "0.12", features = ["blocking"] }`,
    `use std::time::Duration;`,
    ``,
    `fn main() -> Result<(), Box<dyn std::error::Error>> {`,
    `    let client = reqwest::blocking::Client::builder()`,
    `        .timeout(Duration::from_millis(${p.timeoutMs}))`,
  ];
  if (!p.followRedirects) lines.push(`        .redirect(reqwest::redirect::Policy::none())`);
  lines.push(`        .build()?;`, `    let response = client`, `        .request(${method}, ${rustStr(p.url)})`);
  for (const [k, v] of p.headers) lines.push(`        .header(${rustStr(k)}, ${rustStr(v)})`);
  if (p.basic) lines.push(`        .basic_auth(${rustStr(p.basic.username)}, Some(${rustStr(p.basic.password)}))`);
  if (body !== undefined) lines.push(`        .body(${rustStr(body)})`);
  lines.push(
    `        .send()?;`,
    `    println!("{} {}", response.status().as_u16(), response.text()?);`,
    `    Ok(())`,
    `}`,
  );
  return lines.join('\n');
}
