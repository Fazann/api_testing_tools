// Settings and local data.
import * as App from '@wails/go/main/App';

import type { DataInfo, Settings } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const settingsService = {
  getSettings: () => request<Settings>(Endpoint.GetSettings, () => App.GetSettings()),
  saveSettings: (s: Settings) => request<void>(Endpoint.SaveSettings, () => App.SaveSettings(wire(s))),
  dataInfo: () => request<DataInfo>(Endpoint.DataInfo, () => App.DataInfo()),
  compactDatabase: () => request<void>(Endpoint.CompactDatabase, () => App.CompactDatabase()),
  deleteAllData: () => request<void>(Endpoint.DeleteAllData, () => App.DeleteAllData()),
  chooseCACertFile: () => request<string>(Endpoint.ChooseCACertFile, () => App.ChooseCACertFile()),
};
