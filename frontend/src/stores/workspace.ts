import { defineStore } from 'pinia';
import { computed, ref } from 'vue';

import { api } from '../services/api';
import { clone, newRequest } from '../services/model';
import type { Collection, CollectionTree, Request, RunResult } from '../types';

export const useWorkspace = defineStore('workspace', () => {
  const collections = ref<Collection[]>([]);
  const trees = ref<Record<string, CollectionTree>>({});
  const expanded = ref<Record<string, boolean>>({});
  const loaded = ref(false);

  /** The request open in the editor; edits apply here until saved. */
  const draft = ref<Request | null>(null);
  const savedSnapshot = ref('');
  /** Latest result per request id ('' for unsaved drafts). */
  const results = ref<Record<string, RunResult>>({});

  const dirty = computed(() => draft.value !== null && JSON.stringify(draft.value) !== savedSnapshot.value);
  const currentResult = computed(() => (draft.value ? (results.value[draft.value.id] ?? null) : null));
  const currentCollection = computed(() =>
    draft.value ? (collections.value.find((c) => c.id === draft.value!.collectionId) ?? null) : null,
  );

  async function load() {
    collections.value = await api.listCollections();
    loaded.value = true;
    await Promise.all(collections.value.filter((c) => expanded.value[c.id]).map((c) => loadTree(c.id)));
  }

  async function loadTree(id: string) {
    trees.value[id] = await api.getCollectionTree(id);
    return trees.value[id];
  }

  async function toggle(id: string, collectionId?: string) {
    expanded.value[id] = !expanded.value[id];
    if (expanded.value[id] && collectionId && !trees.value[collectionId]) await loadTree(collectionId);
  }

  async function createCollection(name: string) {
    const c = await api.saveCollection({
      name,
      description: '',
      requiredVariables: [],
      variables: [],
      auth: { type: 'none' },
    });
    expanded.value[c.id] = true;
    await load();
    return c;
  }

  async function updateCollection(c: Collection) {
    await api.saveCollection(c);
    await load();
  }

  /** Makes every request in the collection use the collection's auth; returns how many changed. */
  async function inheritCollectionAuth(id: string) {
    const changed = await api.inheritCollectionAuth(id);
    const tree = await loadTree(id);
    // Follow along in the editor unless the open request has unsaved edits.
    if (draft.value?.collectionId === id && !dirty.value) {
      const fresh = tree.requests.find((r) => r.id === draft.value!.id);
      if (fresh) open(fresh);
    }
    return changed;
  }

  async function duplicateCollection(id: string) {
    const c = await api.duplicateCollection(id);
    await load();
    return c;
  }

  async function deleteCollection(id: string) {
    await api.deleteCollection(id);
    if (draft.value?.collectionId === id) closeDraft();
    delete trees.value[id];
    await load();
  }

  async function createFolder(collectionId: string, parentId: string, name: string) {
    const f = await api.saveFolder({ collectionId, parentId, name, sortOrder: 0 });
    expanded.value[collectionId] = true;
    expanded.value[f.id] = true;
    await loadTree(collectionId);
  }

  async function renameFolder(collectionId: string, folderId: string, name: string) {
    const f = trees.value[collectionId]?.folders.find((x) => x.id === folderId);
    if (!f) return;
    await api.saveFolder({ ...f, name });
    await loadTree(collectionId);
  }

  async function deleteFolder(collectionId: string, folderId: string) {
    await api.deleteFolder(folderId);
    await loadTree(collectionId);
    if (draft.value && !trees.value[collectionId]?.requests.some((r) => r.id === draft.value!.id) && draft.value.id)
      closeDraft();
  }

  function open(req: Request) {
    draft.value = clone(req);
    savedSnapshot.value = JSON.stringify(draft.value);
  }

  /** Creates and opens a request; `fields` overrides the defaults, e.g. from a pasted cURL command. */
  async function createRequest(collectionId: string, parentId = '', fields: Partial<Request> = {}) {
    const saved = await api.saveRequest({ ...newRequest(collectionId, parentId), ...clone(fields) });
    expanded.value[collectionId] = true;
    if (parentId) expanded.value[parentId] = true;
    await loadTree(collectionId);
    open(saved);
    return saved;
  }

  async function saveDraft() {
    if (!draft.value) return;
    const saved = await api.saveRequest(clone(draft.value));
    const prevId = draft.value.id;
    open(saved);
    if (!prevId && results.value['']) results.value[saved.id] = results.value[''];
    await loadTree(saved.collectionId);
    return saved;
  }

  async function duplicateRequest(r: Request) {
    const copy = await api.duplicateRequest(r.id);
    await loadTree(r.collectionId);
    open(copy);
  }

  async function renameRequest(r: Request, name: string) {
    if (draft.value?.id === r.id) {
      draft.value.name = name;
      await saveDraft();
      return;
    }
    await api.saveRequest({ ...clone(r), name });
    await loadTree(r.collectionId);
  }

  async function deleteRequest(r: Request) {
    await api.deleteRequest(r.id);
    if (draft.value?.id === r.id) closeDraft();
    delete results.value[r.id];
    await loadTree(r.collectionId);
  }

  function closeDraft() {
    draft.value = null;
    savedSnapshot.value = '';
  }

  function setResult(requestId: string, r: RunResult) {
    results.value[requestId] = r;
  }

  return {
    collections,
    trees,
    expanded,
    loaded,
    draft,
    dirty,
    results,
    currentResult,
    currentCollection,
    load,
    loadTree,
    toggle,
    createCollection,
    updateCollection,
    inheritCollectionAuth,
    duplicateCollection,
    deleteCollection,
    createFolder,
    renameFolder,
    deleteFolder,
    open,
    createRequest,
    saveDraft,
    duplicateRequest,
    renameRequest,
    deleteRequest,
    closeDraft,
    setResult,
  };
});
