import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import App from '@/App.vue';
import { newRequest } from '@/services/model';
import { useUi } from '@/stores/ui';
import { TABS_STORAGE_KEY, useWorkspace } from '@/stores/workspace';

const collection = {
  id: 'c1',
  name: 'Shop',
  description: '',
  requiredVariables: [],
  variables: [],
  auth: { type: 'none' },
};
const requests = [
  { ...newRequest('c1', ''), id: 'a', name: 'List' },
  { ...newRequest('c1', ''), id: 'b', name: 'Create' },
];

async function mountApp() {
  window.go = {
    main: {
      App: {
        AppInfo: async () => ({ name: 'API test kits', version: 'test' }),
        ListCollections: async () => [collection],
        GetCollectionTree: async () => structuredClone({ collection, folders: [], requests }),
        ListEnvironments: async () => [],
        GetSettings: async () => ({ checkForUpdates: false }),
      } as never,
    },
  };
  const w = mount(App, {
    attachTo: document.body,
    global: {
      stubs: {
        CollectionSidebar: true,
        TopBar: true,
        UpdateBanner: true,
        RequestView: true,
        RunView: true,
        HistoryView: true,
        EnvironmentsView: true,
        SettingsView: true,
      },
    },
  });
  await flushPromises();
  return w;
}

const ctrlW = (init: KeyboardEventInit = {}) =>
  window.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', ctrlKey: true, cancelable: true, ...init }));

describe('App', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
    localStorage.setItem(
      TABS_STORAGE_KEY,
      JSON.stringify({
        tabs: [
          { id: 'a', collectionId: 'c1' },
          { id: 'b', collectionId: 'c1' },
        ],
        active: 'a',
      }),
    );
  });
  afterEach(() => {
    delete window.go;
    localStorage.clear();
  });

  it('restores the tabs of the last session on start', async () => {
    const w = await mountApp();
    const ws = useWorkspace();
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['a', 'b']);
    expect(ws.draft?.id).toBe('a');
    w.unmount();
  });

  it('closes the active tab with Ctrl+W and asks first when it has unsaved changes', async () => {
    const w = await mountApp();
    const ws = useWorkspace();
    const ui = useUi();
    const confirm = vi.spyOn(ui, 'confirm').mockResolvedValue(false);

    expect(ctrlW()).toBe(false); // default prevented: the window stays open
    await flushPromises();
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['b']);
    expect(confirm).not.toHaveBeenCalled();

    ws.draft!.name = 'Edited';
    ctrlW({ ctrlKey: false, metaKey: true });
    await flushPromises();
    expect(confirm).toHaveBeenCalledOnce();
    expect(ws.tabs).toHaveLength(1);

    ctrlW({ shiftKey: true });
    ui.view = 'history';
    ctrlW();
    await flushPromises();
    expect(confirm).toHaveBeenCalledOnce();
    expect(ws.tabs).toHaveLength(1);
    w.unmount();
  });
});
