import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Request } from '@/types';

/**
 * Returns a function that opens a request in the editor: in its existing tab,
 * or in a new tab (other tabs and their unsaved edits stay open). It expands
 * the request's collection and folders in the sidebar and switches to the
 * Requests view.
 */
export function useOpenRequest() {
  const ws = useWorkspace();
  const ui = useUi();

  return async function openRequest(r: Request): Promise<boolean> {
    ws.open(r);
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
