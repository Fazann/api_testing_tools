// The remote backend server. Go makes those calls (internal/backend); the frontend only asks Go.
import * as App from '@wails/go/main/App';

import type { BackendStatus } from '@/types';

import { Endpoint } from '../endpoints';
import { request } from '../request';

export const backendService = {
  /** Whether a backend URL is configured and the server answers its health check. */
  backendStatus: () => request<BackendStatus>(Endpoint.BackendStatus, () => App.BackendStatus()),
};
