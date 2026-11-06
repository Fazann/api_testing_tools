// Mirrors the Go domain models' JSON shapes (internal/domain).

export type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS';
export const METHODS: Method[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

export interface KeyValue {
  key: string;
  value: string;
  enabled: boolean;
}

export type AuthType = 'none' | 'bearer' | 'basic' | 'apikey' | 'inherit';

export interface Auth {
  type: AuthType;
  token?: string;
  username?: string;
  password?: string;
  keyName?: string;
  keyValue?: string;
  keyIn?: 'header' | 'query';
}

export type AssertionKind =
  | 'status'
  | 'header'
  | 'json_path'
  | 'response_time'
  | 'valid_json'
  | 'required_fields'
  | 'body'
  | 'aggregate';

export type Operator =
  | ''
  | 'exists'
  | 'not_exists'
  | 'eq'
  | 'neq'
  | 'gt'
  | 'gte'
  | 'lt'
  | 'lte'
  | 'contains'
  | 'not_contains'
  | 'matches'
  | 'type';

export interface Assertion {
  id: string;
  requestId?: string;
  kind: AssertionKind;
  path: string;
  operator: Operator;
  /** JSON value, e.g. 200, "USD", {"$path": "data.total"} */
  expected: unknown;
  tolerance: number;
  function?: 'count' | 'sum' | 'min' | 'max' | '';
  description?: string;
  enabled: boolean;
}

/** The protocol of a request. Socket requests keep a connection open (see SocketConfig). */
export type RequestKind = 'http' | 'websocket' | 'socketio';
export const REQUEST_KINDS: { value: RequestKind; label: string }[] = [
  { value: 'http', label: 'HTTP' },
  { value: 'websocket', label: 'WebSocket' },
  { value: 'socketio', label: 'Socket.IO' },
];

export type SocketBodyType = 'json' | 'text';

/** A message to send: saved for the console, or the payload of a send step. */
export interface SocketMessage {
  id: string;
  name: string;
  /** Socket.IO event name; WebSocket messages ignore it. */
  event: string;
  body: string;
  bodyType: SocketBodyType;
  /** Ask the Socket.IO server to acknowledge the event. */
  ack?: boolean;
}

export type SocketStepKind = 'send' | 'expect' | 'wait';

export interface SocketStep {
  id: string;
  kind: SocketStepKind;
  description?: string;
  enabled: boolean;
  message: SocketMessage;
  /** Expect: only this Socket.IO event ('' = any message). */
  event?: string;
  /** Expect: the acknowledgement of a sent event instead of an event. */
  expectAck?: boolean;
  /** Expect: checks on the message body. */
  assertions: Assertion[];
  /** How long an expect step waits, or how long a wait step pauses. */
  timeoutMs: number;
}

export interface SocketConfig {
  /** Offered in Sec-WebSocket-Protocol (WebSocket only). */
  subprotocols: string[];
  /** Socket.IO server path; the URL's path is the namespace. */
  path: string;
  /** JSON object sent with the Socket.IO CONNECT packet (socket.handshake.auth). */
  authPayload: string;
  messages: SocketMessage[];
  steps: SocketStep[];
}

export interface SocketFrame {
  direction: 'sent' | 'received' | 'system';
  event?: string;
  /** Message text; for Socket.IO the event's argument as JSON. Binary frames are base64. */
  data: string;
  binary?: boolean;
  ack?: boolean;
  /** Milliseconds since the connection opened. */
  atMs: number;
  truncated?: boolean;
}

export interface SocketTranscript {
  kind: RequestKind;
  subprotocol?: string;
  sessionId?: string;
  frames: SocketFrame[];
  dropped?: number;
  closeCode?: number;
  closeReason?: string;
}

/** A console connection that has just opened. */
export interface SocketSession {
  sessionId: string;
  kind: RequestKind;
  url: string;
  subprotocol?: string;
  engineSessionId?: string;
  statusCode: number;
  headers: KeyValue[];
  logs?: ScriptLog[];
}

/** Something that happened on a console connection. */
export interface SocketEvent {
  sessionId: string;
  frame?: SocketFrame;
  closed?: boolean;
  closeCode?: number;
  error?: string;
}

export interface Request {
  id: string;
  collectionId: string;
  parentId: string;
  name: string;
  /** 'http' when absent (older data). */
  kind: RequestKind;
  /** Always GET for socket requests (the WebSocket handshake). */
  method: Method;
  url: string;
  headers: KeyValue[];
  query: KeyValue[];
  body: string;
  bodyType: 'none' | 'json' | 'text';
  auth: Auth;
  timeoutMs: number;
  followRedirects: boolean;
  sortOrder: number;
  assertions: Assertion[];
  /** JavaScript run before variables are resolved and the request is sent. */
  preRequestScript: string;
  /** JavaScript run after the response arrives (pm.test, pm.environment.set, ...). */
  postResponseScript: string;
  /** Set for WebSocket and Socket.IO requests. */
  socket?: SocketConfig;
  createdAt?: string;
  updatedAt?: string;
}

export interface Collection {
  id: string;
  name: string;
  description: string;
  requiredVariables: string[];
  /** Shared by every request in the collection; environment variables with the same name win. */
  variables: Variable[];
  /** Used by requests whose auth type is "inherit". */
  auth: Auth;
  /** Run before every request's own pre-request script. */
  preRequestScript?: string;
  /** Run before every request's own post-response script. */
  postResponseScript?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface Folder {
  id: string;
  collectionId: string;
  parentId: string;
  name: string;
  sortOrder: number;
}

export interface CollectionTree {
  collection: Collection;
  folders: Folder[];
  requests: Request[];
}

export interface Variable {
  key: string;
  value: string;
  secret: boolean;
  enabled: boolean;
}

export interface Environment {
  id: string;
  name: string;
  variables: Variable[];
  protected: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export type ResultStatus = 'passed' | 'failed' | 'error' | 'cancelled' | 'running';

export interface RequestSummary {
  name: string;
  method: string;
  url: string;
  headers: KeyValue[];
  body?: string;
}

export interface ResponseSummary {
  statusCode: number;
  status: string;
  headers: KeyValue[];
  body: string;
  bodyBytes: number;
  truncated: boolean;
  contentType: string;
  redirects?: string[];
}

export interface AssertionResult {
  assertionId: string;
  /** 'script' is a pm.test() result from a post-response script; its name is in description. */
  kind: AssertionKind | 'script' | 'socket_step';
  path?: string;
  operator?: Operator;
  description?: string;
  passed: boolean;
  expected: string;
  actual: string;
  message: string;
}

export type DiffKind = 'added' | 'removed' | 'changed' | 'type_changed';

export interface DiffEntry {
  path: string;
  kind: DiffKind;
  before?: string;
  after?: string;
}

export interface RegressionResult {
  baselineId: string;
  passed: boolean;
  diffs: DiffEntry[];
  message?: string;
}

export interface ScriptLog {
  phase: 'pre-request' | 'post-response';
  level: 'log' | 'info' | 'warn' | 'error';
  message: string;
}

export interface RunResult {
  id: string;
  runId: string;
  requestId: string;
  request: RequestSummary;
  response?: ResponseSummary;
  assertionResults: AssertionResult[];
  regression?: RegressionResult;
  /** Response-time check against earlier responses; absent when off or unsaved. */
  performance?: PerformanceResult;
  /** console output from scripts, already redacted. */
  logs?: ScriptLog[];
  /** Message transcript of a WebSocket or Socket.IO test. */
  socket?: SocketTranscript;
  durationMs: number;
  status: ResultStatus;
  error?: string;
  createdAt: string;
}

export interface Run {
  id: string;
  collectionId: string;
  collectionName: string;
  environmentName: string;
  status: ResultStatus;
  startedAt: string;
  finishedAt?: string;
  passedCount: number;
  failedCount: number;
  errorCount: number;
  durationMs: number;
  results?: RunResult[];
}

export interface RunFilter {
  collectionId?: string;
  requestId?: string;
  from?: string | null;
  to?: string | null;
  status?: string;
  limit?: number;
  offset?: number;
}

/** `values` compares field values; `schema` compares only field names, presence and JSON types. */
export type BaselineMode = 'values' | 'schema';

export interface Baseline {
  id: string;
  requestId: string;
  environmentId: string;
  mode: BaselineMode;
  snapshot: unknown;
  selectedPaths: string[];
  ignoredPaths: string[];
  tolerance: number;
  createdAt: string;
  updatedAt: string;
}

export interface Warning {
  level: 'info' | 'warning' | 'error';
  code: string;
  requiresConfirmation: boolean;
  message: string;
}

export interface PreviousComparison {
  found: boolean;
  previousRunId: string;
  previousStartedAt: string;
  statusBefore: ResultStatus;
  statusAfter: ResultStatus;
  codeBefore: number;
  codeAfter: number;
  newlyFailing: string[];
  diffs: DiffEntry[];
  message: string;
}

export interface SecretFinding {
  location: string;
  detail: string;
}

export interface ImportResult {
  collection: Collection;
  requests: number;
  folders: number;
  environments: number;
  baselines: number;
  /** Baselines whose environment exists neither in the file nor on this machine. */
  baselinesSkipped: number;
}

export interface Settings {
  maxResponseBytes: number;
  historyBodyLimitBytes: number;
  historyRetentionRuns: number;
  concurrency: number;
  insecureSkipVerify: boolean;
  caCertFile: string;
  proxyMode: 'none' | 'system' | 'custom';
  proxyUrl: string;
  allowSensitiveDestinations: boolean;
}

export interface DataInfo {
  path: string;
  sizeBytes: number;
  collections: number;
  requests: number;
  environments: number;
  runs: number;
  baselines: number;
}

export interface AppInfo {
  name: string;
  version: string;
  dataPath: string;
}

export interface RunProgress {
  opId: string;
  done: number;
  total: number;
  result: RunResult;
}

// ---- security tests ----

/** Fires one saved request many times at once and checks concurrency invariants. */
export interface RaceTest {
  requestId: string;
  count: number;
  /** '' means no limit on successful (2xx) responses. */
  successOp: '' | 'eq' | 'lte' | 'gte';
  successCount: number;
  uniquePath: string;
  allowServerErrors: boolean;
  check?: RaceCheck | null;
  updatedAt?: string;
}

/** Reads a number before and after the burst; it must change by successes × deltaPerSuccess. */
export interface RaceCheck {
  requestId: string;
  path: string;
  deltaPerSuccess: number;
  tolerance: number;
}

export interface StatusCount {
  /** 0 means no response. */
  code: number;
  count: number;
}

export interface RaceReport {
  runId: string;
  passed: boolean;
  cancelled: boolean;
  count: number;
  successes: number;
  serverErrors: number;
  noResponse: number;
  statusCounts: StatusCount[];
  minMs: number;
  maxMs: number;
  avgMs: number;
  before?: number;
  after?: number;
  checks: AssertionResult[];
  results: RunResult[];
}

export type Access = '' | 'allow' | 'deny';

export interface Role {
  id: string;
  name: string;
  /** Sends requests without auth, Authorization or Cookie headers. */
  anonymous: boolean;
  /** Override environment and collection variables, e.g. token = {{bob_token}}. */
  variables: Variable[];
}

export interface PermissionMatrix {
  collectionId: string;
  roles: Role[];
  requestIds: string[];
  /** request id → role id → expected access. */
  expected: Record<string, Record<string, Access>>;
  updatedAt?: string;
}

export type MatrixOutcome = 'allowed' | 'denied' | 'not_found' | 'rejected' | 'server_error' | 'error';
export type MatrixVerdict = 'pass' | 'fail' | 'warn' | 'unset' | 'error';

export interface MatrixCell {
  requestId: string;
  roleId: string;
  expected: Access;
  outcome: MatrixOutcome;
  statusCode: number;
  durationMs: number;
  verdict: MatrixVerdict;
  message?: string;
  resultId: string;
  suggested: Access;
}

export interface MatrixReport {
  runId: string;
  passed: boolean;
  cancelled: boolean;
  roles: { id: string; name: string; anonymous: boolean }[];
  requests: { id: string; name: string; method: string }[];
  cells: MatrixCell[];
  passes: number;
  failures: number;
  warnings: number;
  unset: number;
  errors: number;
}

// ---- response times ----

export type PerformanceMode = 'off' | 'warn' | 'fail';

export interface PerformanceCheck {
  requestId: string;
  mode: PerformanceMode;
  /** Slower than usual from this multiple of the median… */
  factor: number;
  /** …and at least this many milliseconds more. */
  minDeltaMs: number;
  /** How many recent responses the median is taken from. */
  window: number;
  /** Earlier responses needed before checking. */
  minSamples: number;
  updatedAt?: string;
}

export interface PerformanceResult {
  mode: PerformanceMode;
  referenceMs: number;
  thresholdMs: number;
  samples: number;
  needed: number;
  ratio: number;
  slower: boolean;
  message: string;
}

export interface ResponseTimePoint {
  runId: string;
  resultId: string;
  at: string;
  durationMs: number;
  status: ResultStatus;
  statusCode: number;
  slower: boolean;
}

export interface ResponseTimeStats {
  count: number;
  minMs: number;
  medianMs: number;
  p95Ms: number;
  maxMs: number;
}

export interface ResponseTimeTrend {
  requestId: string;
  points: ResponseTimePoint[];
  stats: ResponseTimeStats;
  check: PerformanceCheck;
  /** Reference and limit the next response will be checked against. */
  next: PerformanceResult;
}

export interface ResponseTimeRow {
  requestId: string;
  name: string;
  method: Method;
  parentId: string;
  stats: ResponseTimeStats;
  lastMs: number;
  /** Median of the latest 5 vs the earlier ones, in percent; null without enough history. */
  changePct: number | null;
  recent: number[];
  lastSlower: boolean;
  mode: PerformanceMode;
}
