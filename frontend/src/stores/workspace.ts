import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

import { api } from '@/services/api';
import { newRequest } from '@/services/model';
import type { Collection, CollectionTree, Request, RunResult } from '@/types';
import { clone } from '@/utils/object';

/** A request open in the editor. Edits stay in `draft` until saved; `snapshot` is the saved state. */
export interface OpenTab {
  key: string;
  draft: Request;
  snapshot: string;
}

let nextTabKey = 1;

/** Where the open tabs are remembered between starts (this machine only). */
export const TABS_STORAGE_KEY = 'arg.openTabs';

/** The remembered tabs: saved requests only, without unsaved edits (which may hold secrets). */
interface SavedTabs {
  tabs: { id: string; collectionId: string }[];
  /** Request id of the active tab. */
  active: string;
}

function readSavedTabs(): SavedTabs | null {
  try {
    const v = JSON.parse(localStorage.getItem(TABS_STORAGE_KEY) ?? 'null') as SavedTabs | null;
    return v && Array.isArray(v.tabs) ? v : null;
  } catch {
    return null;
  }
}

export const useWorkspace = defineStore('workspace', () => {
  const collections = ref<Collection[]>([]);
  const trees = ref<Record<string, CollectionTree>>({});
  const expanded = ref<Record<string, boolean>>({});
  const loaded = ref(false);

  /** Requests open in editor tabs, in tab order; each keeps its own unsaved edits. */
  const tabs = ref<OpenTab[]>([]);
  const activeKey = ref('');
  /** Latest result per request id ('' for unsaved drafts). */
  const results = ref<Record<string, RunResult>>({});

  const activeTab = computed(() => tabs.value.find((t) => t.key === activeKey.value) ?? null);
  /** The request in the active tab. */
  const draft = computed(() => activeTab.value?.draft ?? null);
  const isDirty = (t: OpenTab) => JSON.stringify(t.draft) !== t.snapshot;
  const dirty = computed(() => !!activeTab.value && isDirty(activeTab.value));
  const currentResult = computed(() => (draft.value ? (results.value[draft.value.id] ?? null) : null));
  const currentCollection = computed(() =>
    draft.value ? (collections.value.find((c) => c.id === draft.value!.collectionId) ?? null) : null,
  );

  // Remember the open tabs once the ones from the last session are back, so
  // an early empty state cannot overwrite them.
  let tabsRestored = false;
  watch(
    () => JSON.stringify({ tabs: tabs.value.map((t) => t.draft.id), active: draft.value?.id ?? '' }),
    () => {
      if (!tabsRestored) return;
      const saved: SavedTabs = {
        tabs: tabs.value.filter((t) => t.draft.id).map((t) => ({ id: t.draft.id, collectionId: t.draft.collectionId })),
        active: draft.value?.id ?? '',
      };
      try {
        localStorage.setItem(TABS_STORAGE_KEY, JSON.stringify(saved));
      } catch {
        /* storage may be unavailable; restoring tabs is a convenience only */
      }
    },
  );

  /**
   * Reopens the tabs of the last session, in order, with their saved state.
   * Requests or collections deleted since are skipped. Call after load().
   */
  async function restoreTabs() {
    try {
      const saved = readSavedTabs();
      if (!saved) return;
      const known = new Set(collections.value.map((c) => c.id));
      const ids = [...new Set(saved.tabs.map((t) => t.collectionId))].filter((id) => known.has(id));
      await Promise.all(ids.map((id) => (trees.value[id] ? null : loadTree(id).catch(() => null))));
      let activeKeyToRestore = '';
      for (const t of saved.tabs) {
        const r = trees.value[t.collectionId]?.requests.find((x) => x.id === t.id);
        if (!r || tabs.value.some((x) => x.draft.id === r.id)) continue;
        open(r);
        if (r.id === saved.active) activeKeyToRestore = activeKey.value;
      }
      if (activeKeyToRestore) activeKey.value = activeKeyToRestore;
    } finally {
      tabsRestored = true;
    }
  }

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

  /** Renames without resending the rest of the collection, so script-updated variables are kept. */
  async function renameCollection(id: string, name: string) {
    await api.renameCollection(id, name);
    await load();
  }

  /** Makes every request in the collection use the collection's auth; returns how many changed. */
  async function inheritCollectionAuth(id: string) {
    const changed = await api.inheritCollectionAuth(id);
    const tree = await loadTree(id);
    // Follow along in the editor unless the open request has unsaved edits.
    if (draft.value?.collectionId === id && !dirty.value) {
      const fresh = tree.requests.find((r) => r.id === draft.value!.id);
      if (fresh) replaceActive(fresh);
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
    closeTabsWhere((t) => t.draft.collectionId === id);
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
    const remaining = new Set(trees.value[collectionId]?.requests.map((r) => r.id));
    closeTabsWhere((t) => t.draft.collectionId === collectionId && !!t.draft.id && !remaining.has(t.draft.id));
  }

  /**
   * Shows a request in a tab: switches to its tab when it is already open
   * (keeping any unsaved edits there), otherwise appends a new tab after the
   * current one. Other tabs are never touched.
   */
  function open(req: Request) {
    const existing = req.id ? tabs.value.find((t) => t.draft.id === req.id) : undefined;
    if (existing) {
      activeKey.value = existing.key;
      return;
    }
    const d = clone(req);
    const tab: OpenTab = { key: `tab-${nextTabKey++}`, draft: d, snapshot: JSON.stringify(d) };
    const at = tabs.value.findIndex((t) => t.key === activeKey.value);
    tabs.value.splice(at < 0 ? tabs.value.length : at + 1, 0, tab);
    activeKey.value = tab.key;
  }

  /** Replaces the active tab's request with its saved state. */
  function replaceActive(req: Request) {
    const tab = activeTab.value;
    if (!tab) return open(req);
    tab.draft = clone(req);
    tab.snapshot = JSON.stringify(tab.draft);
  }

  function activate(key: string) {
    if (tabs.value.some((t) => t.key === key)) activeKey.value = key;
  }

  /** Closes a tab without asking; callers confirm unsaved edits first (see tabIsDirty). */
  function closeTab(key: string) {
    const i = tabs.value.findIndex((t) => t.key === key);
    if (i < 0) return;
    tabs.value.splice(i, 1);
    if (activeKey.value === key) {
      // Like a browser: the tab to the right takes its place, else the one to the left.
      activeKey.value = tabs.value[Math.min(i, tabs.value.length - 1)]?.key ?? '';
    }
  }

  function closeTabsWhere(match: (t: OpenTab) => boolean) {
    for (const t of tabs.value.filter(match)) closeTab(t.key);
  }

  function tabIsDirty(key: string) {
    const t = tabs.value.find((x) => x.key === key);
    return !!t && isDirty(t);
  }

  /** Creates and opens a request; `fields` overrides the defaults, e.g. from a pasted cURL command. */
  async function createRequest(collectionId: string, parentId = '', fields: Partial<Request> = {}) {
    const saved = await api.saveRequest({ ...newRequest(collectionId, parentId), ...clone(fields) });
    expanded.value[collectionId] = true;
    if (parentId) expanded.value[parentId] = true;
    await loadTree(collectionId);
    open(saved); // a new tab next to the current one; the current tab keeps its edits
    return saved;
  }

  async function saveDraft() {
    if (!draft.value) return;
    const tab = activeTab.value!;
    const saved = await api.saveRequest(clone(tab.draft));
    const prevId = tab.draft.id;
    tab.draft = clone(saved);
    tab.snapshot = JSON.stringify(tab.draft);
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
    // An open tab of this request shows the new name without losing its other edits.
    const tab = tabs.value.find((t) => t.draft.id === r.id);
    if (tab) {
      tab.draft.name = name;
      const saved = JSON.parse(tab.snapshot) as Request;
      tab.snapshot = JSON.stringify({ ...saved, name });
    }
    await loadTree(r.collectionId);
  }

  async function deleteRequest(r: Request) {
    await api.deleteRequest(r.id);
    closeTabsWhere((t) => t.draft.id === r.id);
    delete results.value[r.id];
    await loadTree(r.collectionId);
  }

  /** Closes every tab (e.g. after deleting all data). */
  function closeAll() {
    tabs.value = [];
    activeKey.value = '';
  }

  function setResult(requestId: string, r: RunResult) {
    results.value[requestId] = r;
  }

  return {
    collections,
    trees,
    expanded,
    loaded,
    tabs,
    activeKey,
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
    renameCollection,
    inheritCollectionAuth,
    duplicateCollection,
    deleteCollection,
    createFolder,
    renameFolder,
    deleteFolder,
    open,
    restoreTabs,
    activate,
    closeTab,
    tabIsDirty,
    closeAll,
    createRequest,
    saveDraft,
    duplicateRequest,
    renameRequest,
    deleteRequest,
    setResult,
  };
});
