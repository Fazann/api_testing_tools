import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';

import { newRequest } from './model';
import { useOpenRequest } from './navigation';

const target = { ...newRequest('c1', 'f2'), id: 'r2', name: 'Target' };

describe('useOpenRequest', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    const ws = useWorkspace();
    ws.trees.c1 = {
      collection: {
        id: 'c1',
        name: 'C',
        description: '',
        requiredVariables: [],
        variables: [],
        auth: { type: 'none' },
      },
      folders: [
        { id: 'f1', collectionId: 'c1', parentId: '', name: 'A', sortOrder: 0 },
        { id: 'f2', collectionId: 'c1', parentId: 'f1', name: 'B', sortOrder: 0 },
      ],
      requests: [target],
    };
  });

  it('opens the request, reveals it in the sidebar and switches to Requests', async () => {
    const ws = useWorkspace();
    const ui = useUi();
    ui.view = 'history';
    expect(await useOpenRequest()(target)).toBe(true);
    expect(ws.draft?.id).toBe('r2');
    expect(ws.expanded).toMatchObject({ c1: true, f1: true, f2: true });
    expect(ui.view).toBe('requests');
  });

  it('opens another request in a new tab and keeps unsaved edits in the current one', async () => {
    const ws = useWorkspace();
    const ui = useUi();
    ws.open({ ...newRequest('c1', ''), id: 'r1', name: 'Current' });
    ws.draft!.url = 'http://changed';
    const confirm = vi.spyOn(ui, 'confirm');
    expect(await useOpenRequest()(target)).toBe(true);
    expect(confirm).not.toHaveBeenCalled();
    expect(ws.tabs.map((t) => t.draft.id)).toEqual(['r1', 'r2']);
    expect(ws.draft?.id).toBe('r2');
    expect(ws.tabs[0].draft.url).toBe('http://changed');
  });
});
