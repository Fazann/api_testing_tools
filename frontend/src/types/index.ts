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

export interface Request {
  id: string;
  collectionId: string;
  parentId: string;
  name: string;
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
  kind: AssertionKind | 'script';
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
  /** console output from scripts, already redacted. */
  logs?: ScriptLog[];
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
