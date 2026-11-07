import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';

/**
 * Closes a request tab, asking first when it has unsaved changes. Used by the
 * tab's × button, middle-click and Ctrl+W. Resolves to true when the tab was closed.
 */
export function useCloseTab() {
  const ws = useWorkspace();
  const ui = useUi();

  return async function closeTab(key: string): Promise<boolean> {
    const tab = ws.tabs.find((t) => t.key === key);
    if (!tab) return false;
    if (ws.tabIsDirty(key)) {
      const discard = await ui.confirm({
        title: 'Close without saving?',
        message: `"${tab.draft.name}" has unsaved changes. Closing the tab discards them.`,
        confirmLabel: 'Discard and close',
        danger: true,
      });
      if (!discard) return false;
    }
    ws.closeTab(key);
    return true;
  };
}
