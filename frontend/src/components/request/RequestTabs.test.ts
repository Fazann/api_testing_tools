import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';

import RequestTabs from './RequestTabs.vue';

describe('RequestTabs', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const ws = useWorkspace();
    ws.open({ ...newRequest('c1', ''), id: 'a', name: 'List users' });
    ws.open({ ...newRequest('c1', ''), id: 'b', name: 'Create user' });
  });

  it('shows a tab per open request and switches on click', async () => {
    const ws = useWorkspace();
    const w = mount(RequestTabs);
    const tabs = w.findAll('[role="tab"]');
    expect(tabs.map((t) => t.text())).toEqual([
      expect.stringContaining('List users'),
      expect.stringContaining('Create user'),
    ]);
    await tabs[0].trigger('click');
    expect(ws.draft?.id).toBe('a');
  });

  it('closes a saved tab without asking', async () => {
    const ws = useWorkspace();
    const confirm = vi.spyOn(useUi(), 'confirm');
    const w = mount(RequestTabs);
    await w.findAll('.rtab-close')[0].trigger('click');
    expect(confirm).not.toHaveBeenCalled();
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['b']);
  });

  it('asks before closing a tab with unsaved changes and keeps it when declined', async () => {
    const ws = useWorkspace();
    ws.draft!.url = 'http://edited';
    const confirm = vi.spyOn(useUi(), 'confirm').mockResolvedValue(false);
    const w = mount(RequestTabs);
    expect(w.findAll('.rtab-dirty')).toHaveLength(1);
    await w.findAll('.rtab-close')[1].trigger('click');
    await Promise.resolve();
    expect(confirm).toHaveBeenCalledOnce();
    expect(ws.tabs).toHaveLength(2);

    confirm.mockResolvedValue(true);
    await w.findAll('.rtab-close')[1].trigger('click');
    await Promise.resolve();
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['a']);
  });

  it('closes a tab on middle-click', async () => {
    const ws = useWorkspace();
    const w = mount(RequestTabs);
    await w.findAll('.rtab')[0].trigger('mousedown', { button: 1 });
    await Promise.resolve();
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['b']);
  });
});
