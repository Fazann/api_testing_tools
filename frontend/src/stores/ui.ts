import { defineStore } from 'pinia';
import { ref } from 'vue';

import { errorMessage } from '@/services/api';

export type View = 'requests' | 'run' | 'history' | 'environments' | 'settings';

interface Toast {
  id: number;
  kind: 'info' | 'success' | 'error';
  text: string;
}

export interface DialogState {
  type: 'confirm' | 'prompt';
  title: string;
  message?: string;
  details?: string[];
  confirmLabel: string;
  danger: boolean;
  value: string;
  resolve: (v: string | boolean | null) => void;
}

export const useUi = defineStore('ui', () => {
  const view = ref<View>('requests');
  const toasts = ref<Toast[]>([]);
  const dialog = ref<DialogState | null>(null);
  /** A run for the History view to open when it is shown next (set by showRun). */
  const historyRunId = ref('');
  let nextId = 1;

  function toast(text: string, kind: Toast['kind'] = 'info') {
    const id = nextId++;
    toasts.value.push({ id, kind, text });
    setTimeout(() => dismiss(id), kind === 'error' ? 8000 : 4000);
  }

  function dismiss(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id);
  }

  function error(e: unknown) {
    toast(errorMessage(e), 'error');
  }

  function confirm(opts: {
    title: string;
    message?: string;
    details?: string[];
    confirmLabel?: string;
    danger?: boolean;
  }): Promise<boolean> {
    return new Promise((resolve) => {
      dialog.value = {
        type: 'confirm',
        title: opts.title,
        message: opts.message,
        details: opts.details,
        confirmLabel: opts.confirmLabel ?? 'Confirm',
        danger: opts.danger ?? false,
        value: '',
        resolve: (v) => resolve(v === true),
      };
    });
  }

  function prompt(opts: {
    title: string;
    message?: string;
    value?: string;
    confirmLabel?: string;
  }): Promise<string | null> {
    return new Promise((resolve) => {
      dialog.value = {
        type: 'prompt',
        title: opts.title,
        message: opts.message,
        confirmLabel: opts.confirmLabel ?? 'Save',
        danger: false,
        value: opts.value ?? '',
        resolve: (v) => resolve(typeof v === 'string' ? v : null),
      };
    });
  }

  function closeDialog(result: string | boolean | null) {
    const d = dialog.value;
    dialog.value = null;
    d?.resolve(result);
  }

  /** Switches to History and opens the run with this id. */
  function showRun(runId: string) {
    historyRunId.value = runId;
    view.value = 'history';
  }

  return { view, toasts, dialog, historyRunId, toast, dismiss, error, confirm, prompt, closeDialog, showRun };
});
