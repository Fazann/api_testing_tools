// Schema regression baselines.
import * as App from '@wails/go/main/App';

import type { Baseline, BaselineMode } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const baselinesService = {
  getBaseline: (requestId: string, envId: string) =>
    request<Baseline | null>(Endpoint.GetBaseline, () => App.GetBaseline(requestId, envId)),
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
  }) => request<Baseline>(Endpoint.ApproveBaseline, () => App.ApproveBaseline(wire(input))),
  updateBaselineRules: (id: string, ignored: string[], tolerance: number) =>
    request<void>(Endpoint.UpdateBaselineRules, () => App.UpdateBaselineRules(id, ignored, tolerance)),
  deleteBaseline: (id: string) => request<void>(Endpoint.DeleteBaseline, () => App.DeleteBaseline(id)),
};
