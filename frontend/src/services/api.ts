// Typed access to the Go methods bound by Wails (see app.go). Wails exposes
// them on window.go.main.App and rejects promises with plain strings.
import type {
  AppInfo,
  Baseline,
  BaselineMode,
  ChangingReport,
  Collection,
  CollectionTree,
  CompareReport,
  ContractCoverage,
  ContractInfo,
  DataInfo,
  EnvComparison,
  Environment,
  Folder,
  ImportResult,
  MatrixReport,
  PerformanceCheck,
  PermissionMatrix,
  PreviousComparison,
  RaceReport,
  RaceTest,
  Request,
  ResponseTimeRow,
  ResponseTimeTrend,
  Rule,
  RulesCheck,
  Run,
  RunFilter,
  RunProgress,
  RunResult,
  SecretFinding,
  Settings,
  SignoffInput,
  SignoffResult,
  SocketEvent,
  SocketFrame,
  SocketMessage,
  SocketSession,
  UpdateInfo,
  UpdateProgress,
  Warning,
} from '@/types';

type Bound = Record<string, (...args: unknown[]) => Promise<unknown>>;

declare global {
  interface Window {
    go?: { main?: { App?: Bound } };
    runtime?: {
      EventsOn: (name: string, cb: (...data: unknown[]) => void) => () => void;
    };
  }
}

export class BackendUnavailableError extends Error {
  constructor() {
    super('The desktop backend is not available. Start the app with "wails dev" or the packaged binary.');
  }
}

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const fn = window.go?.main?.App?.[method];
  if (!fn) throw new BackendUnavailableError();
  try {
    return (await fn(...args)) as T;
  } catch (e) {
    throw e instanceof Error ? e : new Error(String(e));
  }
}

export const api = {
  appInfo: () => call<AppInfo>('AppInfo'),
  checkForUpdate: () => call<UpdateInfo>('CheckForUpdate'),
  installUpdate: () => call<void>('InstallUpdate'),
  openReleasePage: (url: string) => call<void>('OpenReleasePage', url),

  listCollections: () => call<Collection[]>('ListCollections'),
  getCollectionTree: (id: string) => call<CollectionTree>('GetCollectionTree', id),
  saveCollection: (c: Partial<Collection>) => call<Collection>('SaveCollection', c),
  renameCollection: (id: string, name: string) => call<Collection>('RenameCollection', id, name),
  inheritCollectionAuth: (id: string) => call<number>('InheritCollectionAuth', id),
  duplicateCollection: (id: string) => call<Collection>('DuplicateCollection', id),
  deleteCollection: (id: string) => call<void>('DeleteCollection', id),
  saveFolder: (f: Partial<Folder>) => call<Folder>('SaveFolder', f),
  deleteFolder: (id: string) => call<void>('DeleteFolder', id),
  saveRequest: (r: Request) => call<Request>('SaveRequest', r),
  duplicateRequest: (id: string) => call<Request>('DuplicateRequest', id),
  deleteRequest: (id: string) => call<void>('DeleteRequest', id),

  listEnvironments: () => call<Environment[]>('ListEnvironments'),
  saveEnvironment: (e: Partial<Environment>) => call<Environment>('SaveEnvironment', e),
  duplicateEnvironment: (id: string) => call<Environment>('DuplicateEnvironment', id),
  deleteEnvironment: (id: string) => call<void>('DeleteEnvironment', id),

  preflightRequest: (r: Request, envId: string) => call<Warning[]>('PreflightRequest', r, envId),
  preflightCollection: (collectionId: string, folderId: string, envId: string) =>
    call<Warning[]>('PreflightCollection', collectionId, folderId, envId),
  sendRequest: (opId: string, r: Request, envId: string, confirmed: boolean) =>
    call<RunResult>('SendRequest', opId, r, envId, confirmed),
  runCollection: (opId: string, collectionId: string, folderId: string, envId: string, confirmed: boolean) =>
    call<Run>('RunCollection', opId, collectionId, folderId, envId, confirmed),
  cancel: (opId: string) => call<boolean>('Cancel', opId),

  getPerformanceCheck: (requestId: string) => call<PerformanceCheck>('GetPerformanceCheck', requestId),
  savePerformanceCheck: (c: PerformanceCheck) => call<PerformanceCheck>('SavePerformanceCheck', c),
  getResponseTimeTrend: (requestId: string, envId: string) =>
    call<ResponseTimeTrend>('GetResponseTimeTrend', requestId, envId),
  getCollectionResponseTimes: (collectionId: string, envId: string) =>
    call<ResponseTimeRow[]>('GetCollectionResponseTimes', collectionId, envId),
  getRaceTest: (requestId: string) => call<RaceTest | null>('GetRaceTest', requestId),
  runRaceTest: (opId: string, t: RaceTest, envId: string, confirmed: boolean) =>
    call<RaceReport>('RunRaceTest', opId, t, envId, confirmed),
  getPermissionMatrix: (collectionId: string) => call<PermissionMatrix>('GetPermissionMatrix', collectionId),
  savePermissionMatrix: (m: PermissionMatrix) => call<PermissionMatrix>('SavePermissionMatrix', m),
  runPermissionMatrix: (opId: string, collectionId: string, envId: string, confirmed: boolean) =>
    call<MatrixReport>('RunPermissionMatrix', opId, collectionId, envId, confirmed),

  buildSignoffReport: (input: SignoffInput) => call<SignoffResult>('BuildSignoffReport', input),
  /** Opens a save dialog; resolves to the chosen path, or '' when cancelled. */
  saveSignoffReport: (fileName: string, html: string) => call<string>('SaveSignoffReport', fileName, html),

  saveCollectionRules: (collectionId: string, rules: Rule[]) =>
    call<Rule[]>('SaveCollectionRules', collectionId, rules),
  checkCollectionRules: (collectionId: string, rules: Rule[]) =>
    call<RulesCheck>('CheckCollectionRules', collectionId, rules),

  getContract: (collectionId: string) => call<ContractInfo>('GetContract', collectionId),
  /** Opens a file dialog; null when cancelled. */
  attachContract: (collectionId: string) => call<ContractInfo | null>('AttachContract', collectionId),
  setContractOptions: (collectionId: string, enabled: boolean, allowUndocumented: boolean) =>
    call<ContractInfo>('SetContractOptions', collectionId, enabled, allowUndocumented),
  removeContract: (collectionId: string) => call<void>('RemoveContract', collectionId),
  getContractCoverage: (collectionId: string, envId: string) =>
    call<ContractCoverage>('GetContractCoverage', collectionId, envId),
  getEnvComparison: (collectionId: string) => call<EnvComparison>('GetEnvComparison', collectionId),
  saveEnvComparison: (c: EnvComparison) => call<EnvComparison>('SaveEnvComparison', c),
  compareEnvironments: (opId: string, c: EnvComparison, confirmed: boolean) =>
    call<CompareReport>('CompareEnvironments', opId, c, confirmed),
  detectChangingFields: (
    opId: string,
    input: { request: Request; environmentId: string; ignoredPaths: string[]; confirmed: boolean },
  ) => call<ChangingReport>('DetectChangingFields', opId, input),

  getBaseline: (requestId: string, envId: string) => call<Baseline | null>('GetBaseline', requestId, envId),
  approveBaseline: (input: {
    requestId: string;
    environmentId: string;
    body: string;
    truncated: boolean;
    selectedPaths: string[];
    ignoredPaths: string[];
    tolerance: number;
    mode: BaselineMode;
    approvedBy: string;
  }) => call<Baseline>('ApproveBaseline', input),
  updateBaselineRules: (id: string, ignored: string[], tolerance: number) =>
    call<void>('UpdateBaselineRules', id, ignored, tolerance),
  deleteBaseline: (id: string) => call<void>('DeleteBaseline', id),

  listRuns: (f: RunFilter) => call<Run[]>('ListRuns', f),
  getRun: (id: string) => call<Run>('GetRun', id),
  deleteRun: (id: string) => call<void>('DeleteRun', id),
  clearHistory: () => call<number>('ClearHistory'),
  compareWithPrevious: (runId: string, resultId: string) =>
    call<PreviousComparison>('CompareWithPrevious', runId, resultId),
  exportReport: (runId: string, format: 'json' | 'html') => call<string>('ExportReport', runId, format),

  exportPreview: (collectionId: string, envIds: string[]) =>
    call<SecretFinding[]>('ExportPreview', collectionId, envIds),
  exportCollection: (collectionId: string, envIds: string[], includeSecrets: boolean, includeBaselines: boolean) =>
    call<string>('ExportCollection', collectionId, envIds, includeSecrets, includeBaselines),
  importCollection: () => call<ImportResult | null>('ImportCollection'),

  getSettings: () => call<Settings>('GetSettings'),
  saveSettings: (s: Settings) => call<void>('SaveSettings', s),
  dataInfo: () => call<DataInfo>('DataInfo'),
  compactDatabase: () => call<void>('CompactDatabase'),
  deleteAllData: () => call<void>('DeleteAllData'),
  chooseCACertFile: () => call<string>('ChooseCACertFile'),

  connectSocket: (sessionId: string, r: Request, envId: string, confirmed: boolean) =>
    call<SocketSession>('ConnectSocket', sessionId, r, envId, confirmed),
  sendSocketMessage: (sessionId: string, m: SocketMessage) => call<SocketFrame>('SendSocketMessage', sessionId, m),
  disconnectSocket: (sessionId: string) => call<boolean>('DisconnectSocket', sessionId),
};

/** Subscribes to console socket events (messages and closes of every session). Returns an unsubscribe function. */
export function onSocketEvent(cb: (e: SocketEvent) => void): () => void {
  if (!window.runtime?.EventsOn) return () => {};
  return window.runtime.EventsOn('socket:event', (e) => cb(e as SocketEvent));
}

/** Subscribes to collection-run progress events. Returns an unsubscribe function. */
export function onRunProgress(cb: (p: RunProgress) => void): () => void {
  if (!window.runtime?.EventsOn) return () => {};
  return window.runtime.EventsOn('run:progress', (p) => cb(p as RunProgress));
}

/** Subscribes to permission-matrix progress events. Returns an unsubscribe function. */
export function onMatrixProgress(cb: (p: RunProgress) => void): () => void {
  if (!window.runtime?.EventsOn) return () => {};
  return window.runtime.EventsOn('matrix:progress', (p) => cb(p as RunProgress));
}

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** Subscribes to update download progress. Returns an unsubscribe function. */
export function onUpdateProgress(cb: (p: UpdateProgress) => void): () => void {
  if (!window.runtime?.EventsOn) return () => {};
  return window.runtime.EventsOn('update:progress', (p) => cb(p as UpdateProgress));
}

/** Subscribes to environment-comparison progress events. Returns an unsubscribe function. */
export function onCompareProgress(cb: (p: RunProgress) => void): () => void {
  if (!window.runtime?.EventsOn) return () => {};
  return window.runtime.EventsOn('compare:progress', (p) => cb(p as RunProgress));
}
