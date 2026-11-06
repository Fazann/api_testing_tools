import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { newRequest } from '@/services/model';
import type { Request } from '@/types';

import { useWorkspace } from './workspace';

const req = (id: string, name = id, collectionId = 'c1'): Request => ({ ...newRequest(collectionId, ''), id, name });

describe('workspace request tabs', () => {
  beforeEach(() => setActivePinia(createPinia()));
  afterEach(() => {
    delete window.go;
  });

  it('appends a tab after the current one and activates it', () => {
    const ws = useWorkspace();
    ws.open(req('a'));
    ws.open(req('b'));
    ws.activate(ws.tabs[0].key);
    ws.open(req('c'));
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['a', 'c', 'b']);
    expect(ws.draft?.id).toBe('c');
  });

  it('switches to an already open request without losing its edits', () => {
    const ws = useWorkspace();
    ws.open(req('a'));
    ws.draft!.url = 'http://edited';
    ws.open(req('b'));
    ws.open(req('a', 'fresh copy from the sidebar'));
    expect(ws.tabs).toHaveLength(2);
    expect(ws.draft?.url).toBe('http://edited');
    expect(ws.dirty).toBe(true);
  });

  it('tracks unsaved edits per tab', () => {
    const ws = useWorkspace();
    ws.open(req('a'));
    ws.draft!.name = 'changed';
    ws.open(req('b'));
    expect(ws.dirty).toBe(false);
    expect(ws.tabIsDirty(ws.tabs[0].key)).toBe(true);
    expect(ws.tabIsDirty(ws.tabs[1].key)).toBe(false);
  });

  it('closing the active tab activates its right neighbour, else the left one', () => {
    const ws = useWorkspace();
    ['a', 'b', 'c'].forEach((id) => ws.open(req(id)));
    ws.activate(ws.tabs[1].key);
    ws.closeTab(ws.tabs[1].key);
    expect(ws.draft?.id).toBe('c');
    ws.closeTab(ws.tabs[1].key);
    expect(ws.draft?.id).toBe('a');
    ws.closeTab(ws.tabs[0].key);
    expect(ws.draft).toBeNull();
    expect(ws.tabs).toHaveLength(0);
  });

  it('closing an inactive tab keeps the current one', () => {
    const ws = useWorkspace();
    ['a', 'b'].forEach((id) => ws.open(req(id)));
    ws.closeTab(ws.tabs[0].key);
    expect(ws.draft?.id).toBe('b');
  });

  it('creating a request keeps the current tab and appends one for the new request', async () => {
    const ws = useWorkspace();
    let n = 0;
    window.go = {
      main: {
        App: {
          SaveRequest: async (r: unknown) => ({ ...(r as Request), id: `new${++n}` }),
          GetCollectionTree: async () => ({ collection: { id: 'c1' }, folders: [], requests: [] }),
        },
      },
    };
    ws.open(req('a'));
    ws.draft!.url = 'http://unsaved';
    await ws.createRequest('c1');
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['a', 'new1']);
    expect(ws.draft?.id).toBe('new1');
    expect(ws.tabs[0].draft.url).toBe('http://unsaved');
  });

  it('saving updates only the active tab', async () => {
    const ws = useWorkspace();
    window.go = {
      main: {
        App: {
          SaveRequest: async (r: unknown) => r,
          GetCollectionTree: async () => ({ collection: { id: 'c1' }, folders: [], requests: [] }),
        },
      },
    };
    ws.open(req('a'));
    ws.draft!.url = 'http://a-edit';
    ws.open(req('b'));
    ws.draft!.url = 'http://b-edit';
    await ws.saveDraft();
    expect(ws.dirty).toBe(false);
    expect(ws.tabIsDirty(ws.tabs[0].key)).toBe(true);
  });

  it('deleting a request or collection closes its tabs', async () => {
    const ws = useWorkspace();
    window.go = {
      main: {
        App: {
          DeleteRequest: async () => undefined,
          DeleteCollection: async () => undefined,
          ListCollections: async () => [],
          GetCollectionTree: async () => ({ collection: { id: 'c1' }, folders: [], requests: [] }),
        },
      },
    };
    ws.open(req('a'));
    ws.open(req('b'));
    ws.open(req('x', 'other', 'c2'));
    await ws.deleteRequest(req('a'));
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['b', 'x']);
    await ws.deleteCollection('c2');
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['b']);
  });
});
