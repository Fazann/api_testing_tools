// App info and in-place updates.
import * as App from '@wails/go/main/App';

import type { AppInfo, UpdateInfo } from '@/types';

import { Endpoint } from '../endpoints';
import { request } from '../request';

export const appService = {
  appInfo: () => request<AppInfo>(Endpoint.AppInfo, () => App.AppInfo()),
  checkForUpdate: () => request<UpdateInfo>(Endpoint.CheckForUpdate, () => App.CheckForUpdate()),
  installUpdate: () => request<void>(Endpoint.InstallUpdate, () => App.InstallUpdate()),
  openReleasePage: (url: string) => request<void>(Endpoint.OpenReleasePage, () => App.OpenReleasePage(url)),
};
