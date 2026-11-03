import { useUi } from '../stores/ui';
import { useWorkspace } from '../stores/workspace';
import type { Request } from '../types';

/**
 * Returns a function that opens a request in the editor: it asks before
 * discarding unsaved edits, expands the request's collection and folders in
 * the sidebar, and switches to the Requests view. It resolves to false when
 * the user keeps the current draft.
 */
export function useOpenRequest() {
  const ws = useWorkspace();
  const ui = useUi();

  return async function openRequest(r: Request): Promise<boolean> {
    if (ws.draft?.id !== r.id) {
      if (ws.dirty) {
        const discard = await ui.confirm({
          title: 'Discard unsaved changes?',
          message: `"${ws.draft?.name}" has unsaved changes.`,
          confirmLabel: 'Discard',
          danger: true,
        });
        if (!discard) return false;
      }
      ws.open(r);
    }
    reveal(r);
    ui.view = 'requests';
    return true;
  };

  function reveal(r: Request) {
    ws.expanded[r.collectionId] = true;
    const folders = ws.trees[r.collectionId]?.folders ?? [];
    const seen = new Set<string>();
    for (let id = r.parentId; id && !seen.has(id); ) {
      seen.add(id);
      ws.expanded[id] = true;
      id = folders.find((f) => f.id === id)?.parentId ?? '';
    }
  }
}
