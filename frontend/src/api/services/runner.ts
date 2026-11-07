// Preflight checks, sending requests and running collections.
import * as App from '@wails/go/main/App';

import type { Request, Run, RunResult, Warning } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const runnerService = {
  preflightRequest: (r: Request, envId: string) =>
    request<Warning[]>(Endpoint.PreflightRequest, () => App.PreflightRequest(wire(r), envId)),
  preflightCollection: (collectionId: string, folderId: string, envId: string) =>
    request<Warning[]>(Endpoint.PreflightCollection, () => App.PreflightCollection(collectionId, folderId, envId)),
  sendRequest: (opId: string, r: Request, envId: string, confirmed: boolean) =>
    request<RunResult>(Endpoint.SendRequest, () => App.SendRequest(opId, wire(r), envId, confirmed)),
  runCollection: (opId: string, collectionId: string, folderId: string, envId: string, confirmed: boolean) =>
    request<Run>(Endpoint.RunCollection, () => App.RunCollection(opId, collectionId, folderId, envId, confirmed)),
  cancel: (opId: string) => request<boolean>(Endpoint.Cancel, () => App.Cancel(opId)),
};
