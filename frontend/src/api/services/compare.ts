// Environment comparison and changing-field detection.
import * as App from '@wails/go/main/App';

import type { ChangingReport, CompareReport, EnvComparison, Request } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const compareService = {
  getEnvComparison: (collectionId: string) =>
    request<EnvComparison>(Endpoint.GetEnvComparison, () => App.GetEnvComparison(collectionId)),
  saveEnvComparison: (c: EnvComparison) =>
    request<EnvComparison>(Endpoint.SaveEnvComparison, () => App.SaveEnvComparison(wire(c))),
  compareEnvironments: (opId: string, c: EnvComparison, confirmed: boolean) =>
    request<CompareReport>(Endpoint.CompareEnvironments, () => App.CompareEnvironments(opId, wire(c), confirmed)),
  detectChangingFields: (
    opId: string,
    input: { request: Request; environmentId: string; ignoredPaths: string[]; confirmed: boolean },
  ) => request<ChangingReport>(Endpoint.DetectChangingFields, () => App.DetectChangingFields(opId, wire(input))),
};
