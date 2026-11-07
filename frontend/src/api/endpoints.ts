// Every Go method the frontend calls. Each value is the method name on App (app.go) and of its
// generated binding in wailsjs/go/main/App. The calls themselves live in services/.
export enum Endpoint {
  // App and updates
  AppInfo = 'AppInfo',
  CheckForUpdate = 'CheckForUpdate',
  InstallUpdate = 'InstallUpdate',
  OpenReleasePage = 'OpenReleasePage',

  // Remote backend (called from Go, internal/backend)
  BackendStatus = 'BackendStatus',

  // Collections, folders, requests
  ListCollections = 'ListCollections',
  GetCollectionTree = 'GetCollectionTree',
  SaveCollection = 'SaveCollection',
  RenameCollection = 'RenameCollection',
  InheritCollectionAuth = 'InheritCollectionAuth',
  DuplicateCollection = 'DuplicateCollection',
  DeleteCollection = 'DeleteCollection',
  SaveFolder = 'SaveFolder',
  DeleteFolder = 'DeleteFolder',
  SaveRequest = 'SaveRequest',
  DuplicateRequest = 'DuplicateRequest',
  DeleteRequest = 'DeleteRequest',

  // Environments
  ListEnvironments = 'ListEnvironments',
  SaveEnvironment = 'SaveEnvironment',
  DuplicateEnvironment = 'DuplicateEnvironment',
  DeleteEnvironment = 'DeleteEnvironment',

  // Sending and running
  PreflightRequest = 'PreflightRequest',
  PreflightCollection = 'PreflightCollection',
  SendRequest = 'SendRequest',
  RunCollection = 'RunCollection',
  Cancel = 'Cancel',

  // Performance
  GetPerformanceCheck = 'GetPerformanceCheck',
  SavePerformanceCheck = 'SavePerformanceCheck',
  GetResponseTimeTrend = 'GetResponseTimeTrend',
  GetCollectionResponseTimes = 'GetCollectionResponseTimes',

  // Security
  GetRaceTest = 'GetRaceTest',
  RunRaceTest = 'RunRaceTest',
  GetPermissionMatrix = 'GetPermissionMatrix',
  SavePermissionMatrix = 'SavePermissionMatrix',
  RunPermissionMatrix = 'RunPermissionMatrix',

  // Sign-off
  BuildSignoffReport = 'BuildSignoffReport',
  SaveSignoffReport = 'SaveSignoffReport',

  // Collection rules
  SaveCollectionRules = 'SaveCollectionRules',
  CheckCollectionRules = 'CheckCollectionRules',

  // OpenAPI contract
  GetContract = 'GetContract',
  AttachContract = 'AttachContract',
  SetContractOptions = 'SetContractOptions',
  RemoveContract = 'RemoveContract',
  GetContractCoverage = 'GetContractCoverage',

  // Environment comparison and changing fields
  GetEnvComparison = 'GetEnvComparison',
  SaveEnvComparison = 'SaveEnvComparison',
  CompareEnvironments = 'CompareEnvironments',
  DetectChangingFields = 'DetectChangingFields',

  // Baselines
  GetBaseline = 'GetBaseline',
  ApproveBaseline = 'ApproveBaseline',
  UpdateBaselineRules = 'UpdateBaselineRules',
  DeleteBaseline = 'DeleteBaseline',

  // History and reports
  ListRuns = 'ListRuns',
  GetRun = 'GetRun',
  DeleteRun = 'DeleteRun',
  ClearHistory = 'ClearHistory',
  CompareWithPrevious = 'CompareWithPrevious',
  ExportReport = 'ExportReport',

  // Import and export
  ExportPreview = 'ExportPreview',
  ExportCollection = 'ExportCollection',
  ImportCollection = 'ImportCollection',

  // Settings and data
  GetSettings = 'GetSettings',
  SaveSettings = 'SaveSettings',
  DataInfo = 'DataInfo',
  CompactDatabase = 'CompactDatabase',
  DeleteAllData = 'DeleteAllData',
  ChooseCACertFile = 'ChooseCACertFile',
  ChooseUploadFile = 'ChooseUploadFile',

  // Sockets
  ConnectSocket = 'ConnectSocket',
  StartSocketHeartbeat = 'StartSocketHeartbeat',
  StopSocketHeartbeat = 'StopSocketHeartbeat',
  SendSocketMessage = 'SendSocketMessage',
  DisconnectSocket = 'DisconnectSocket',
}

/** Events Go emits with runtime.EventsEmit. */
export enum BackendEvent {
  RunProgress = 'run:progress',
  MatrixProgress = 'matrix:progress',
  CompareProgress = 'compare:progress',
  UpdateProgress = 'update:progress',
  SocketEvent = 'socket:event',
}
