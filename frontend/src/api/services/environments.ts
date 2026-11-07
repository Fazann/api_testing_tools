// Environments.
import * as App from '@wails/go/main/App';

import type { Environment } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const environmentsService = {
  listEnvironments: () => request<Environment[]>(Endpoint.ListEnvironments, () => App.ListEnvironments()),
  saveEnvironment: (e: Partial<Environment>) =>
    request<Environment>(Endpoint.SaveEnvironment, () => App.SaveEnvironment(wire(e))),
  duplicateEnvironment: (id: string) =>
    request<Environment>(Endpoint.DuplicateEnvironment, () => App.DuplicateEnvironment(id)),
  deleteEnvironment: (id: string) => request<void>(Endpoint.DeleteEnvironment, () => App.DeleteEnvironment(id)),
};
