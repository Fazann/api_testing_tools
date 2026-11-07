// Go access for the whole frontend, through the standard Wails bindings. Import from '@/api'.
//
//   endpoints.ts    Endpoint and BackendEvent enums (Go method and event names)
//   request.ts      request() helper around the generated bindings, wire()
//   interceptors.ts request/response/error interceptors
//   services/       one service per feature area, calling wailsjs/go/main/App
//   events.ts       typed event subscriptions (wailsjs/runtime EventsOn)
import { appService } from './services/app';
import { backendService } from './services/backend';
import { baselinesService } from './services/baselines';
import { collectionsService } from './services/collections';
import { compareService } from './services/compare';
import { contractService } from './services/contract';
import { environmentsService } from './services/environments';
import { historyService } from './services/history';
import { performanceService } from './services/performance';
import { rulesService } from './services/rules';
import { runnerService } from './services/runner';
import { securityService } from './services/security';
import { settingsService } from './services/settings';
import { signoffService } from './services/signoff';
import { socketsService } from './services/sockets';
import { transferService } from './services/transfer';

/** Every service in one object, for call sites that use api.someMethod(). */
export const api = {
  ...appService,
  ...backendService,
  ...collectionsService,
  ...environmentsService,
  ...runnerService,
  ...performanceService,
  ...securityService,
  ...signoffService,
  ...rulesService,
  ...contractService,
  ...compareService,
  ...baselinesService,
  ...historyService,
  ...transferService,
  ...settingsService,
  ...socketsService,
};

export {
  appService,
  backendService,
  collectionsService,
  environmentsService,
  runnerService,
  performanceService,
  securityService,
  signoffService,
  rulesService,
  contractService,
  compareService,
  baselinesService,
  historyService,
  transferService,
  settingsService,
  socketsService,
};
export * from './events';
export { BackendEvent, Endpoint } from './endpoints';
export { interceptors } from './interceptors';
export type { ErrorInterceptor, RequestContext, RequestInterceptor, ResponseInterceptor } from './interceptors';
export { backendAvailable, BackendUnavailableError, errorMessage, request, wire } from './request';
