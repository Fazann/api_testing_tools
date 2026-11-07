// Run history and reports.
import * as App from '@wails/go/main/App';

import type { PreviousComparison, Run, RunFilter } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const historyService = {
  listRuns: (f: RunFilter) => request<Run[]>(Endpoint.ListRuns, () => App.ListRuns(wire(f))),
  getRun: (id: string) => request<Run>(Endpoint.GetRun, () => App.GetRun(id)),
  deleteRun: (id: string) => request<void>(Endpoint.DeleteRun, () => App.DeleteRun(id)),
  clearHistory: () => request<number>(Endpoint.ClearHistory, () => App.ClearHistory()),
  compareWithPrevious: (runId: string, resultId: string) =>
    request<PreviousComparison>(Endpoint.CompareWithPrevious, () => App.CompareWithPrevious(runId, resultId)),
  exportReport: (runId: string, format: 'json' | 'html') =>
    request<string>(Endpoint.ExportReport, () => App.ExportReport(runId, format)),
};
