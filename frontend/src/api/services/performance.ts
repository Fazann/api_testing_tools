// Response-time checks and trends.
import * as App from '@wails/go/main/App';

import type { PerformanceCheck, ResponseTimeRow, ResponseTimeTrend } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const performanceService = {
  getPerformanceCheck: (requestId: string) =>
    request<PerformanceCheck>(Endpoint.GetPerformanceCheck, () => App.GetPerformanceCheck(requestId)),
  savePerformanceCheck: (c: PerformanceCheck) =>
    request<PerformanceCheck>(Endpoint.SavePerformanceCheck, () => App.SavePerformanceCheck(wire(c))),
  getResponseTimeTrend: (requestId: string, envId: string) =>
    request<ResponseTimeTrend>(Endpoint.GetResponseTimeTrend, () => App.GetResponseTimeTrend(requestId, envId)),
  getCollectionResponseTimes: (collectionId: string, envId: string) =>
    request<ResponseTimeRow[]>(Endpoint.GetCollectionResponseTimes, () =>
      App.GetCollectionResponseTimes(collectionId, envId),
    ),
};
