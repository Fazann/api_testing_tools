// Race tests and the permission matrix.
import * as App from '@wails/go/main/App';

import type { MatrixReport, PermissionMatrix, RaceReport, RaceTest } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const securityService = {
  getRaceTest: (requestId: string) => request<RaceTest | null>(Endpoint.GetRaceTest, () => App.GetRaceTest(requestId)),
  runRaceTest: (opId: string, t: RaceTest, envId: string, confirmed: boolean) =>
    request<RaceReport>(Endpoint.RunRaceTest, () => App.RunRaceTest(opId, wire(t), envId, confirmed)),
  getPermissionMatrix: (collectionId: string) =>
    request<PermissionMatrix>(Endpoint.GetPermissionMatrix, () => App.GetPermissionMatrix(collectionId)),
  savePermissionMatrix: (m: PermissionMatrix) =>
    request<PermissionMatrix>(Endpoint.SavePermissionMatrix, () => App.SavePermissionMatrix(wire(m))),
  runPermissionMatrix: (opId: string, collectionId: string, envId: string, confirmed: boolean) =>
    request<MatrixReport>(Endpoint.RunPermissionMatrix, () =>
      App.RunPermissionMatrix(opId, collectionId, envId, confirmed),
    ),
};
