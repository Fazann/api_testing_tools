// Collection import and export.
import * as App from '@wails/go/main/App';

import type { ImportResult, SecretFinding } from '@/types';

import { Endpoint } from '../endpoints';
import { request } from '../request';

export const transferService = {
  exportPreview: (collectionId: string, envIds: string[]) =>
    request<SecretFinding[]>(Endpoint.ExportPreview, () => App.ExportPreview(collectionId, envIds)),
  exportCollection: (collectionId: string, envIds: string[], includeSecrets: boolean, includeBaselines: boolean) =>
    request<string>(Endpoint.ExportCollection, () =>
      App.ExportCollection(collectionId, envIds, includeSecrets, includeBaselines),
    ),
  importCollection: () => request<ImportResult | null>(Endpoint.ImportCollection, () => App.ImportCollection()),
};
