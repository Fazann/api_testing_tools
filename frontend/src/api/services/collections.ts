// Collections, folders and saved requests.
import * as App from '@wails/go/main/App';

import type { Collection, CollectionTree, Folder, Request } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const collectionsService = {
  listCollections: () => request<Collection[]>(Endpoint.ListCollections, () => App.ListCollections()),
  getCollectionTree: (id: string) =>
    request<CollectionTree>(Endpoint.GetCollectionTree, () => App.GetCollectionTree(id)),
  saveCollection: (c: Partial<Collection>) =>
    request<Collection>(Endpoint.SaveCollection, () => App.SaveCollection(wire(c))),
  renameCollection: (id: string, name: string) =>
    request<Collection>(Endpoint.RenameCollection, () => App.RenameCollection(id, name)),
  inheritCollectionAuth: (id: string) =>
    request<number>(Endpoint.InheritCollectionAuth, () => App.InheritCollectionAuth(id)),
  duplicateCollection: (id: string) =>
    request<Collection>(Endpoint.DuplicateCollection, () => App.DuplicateCollection(id)),
  deleteCollection: (id: string) => request<void>(Endpoint.DeleteCollection, () => App.DeleteCollection(id)),
  saveFolder: (f: Partial<Folder>) => request<Folder>(Endpoint.SaveFolder, () => App.SaveFolder(wire(f))),
  deleteFolder: (id: string) => request<void>(Endpoint.DeleteFolder, () => App.DeleteFolder(id)),
  saveRequest: (r: Request) => request<Request>(Endpoint.SaveRequest, () => App.SaveRequest(wire(r))),
  duplicateRequest: (id: string) => request<Request>(Endpoint.DuplicateRequest, () => App.DuplicateRequest(id)),
  deleteRequest: (id: string) => request<void>(Endpoint.DeleteRequest, () => App.DeleteRequest(id)),
};
