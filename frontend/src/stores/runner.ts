import { defineStore } from 'pinia';
import { ref } from 'vue';

import { api, onRunProgress } from '../services/api';
import { clone, uid } from '../services/model';
import type { Run, RunResult, Warning } from '../types';
import { useEnvironments } from './environments';
import { useUi } from './ui';
import { useWorkspace } from './workspace';

/** State of the collection run shown in the Run view. */
interface CollectionRun {
  opId: string;
  collectionId: string;
  folderId: string;
  title: string;
  done: number;
  total: number;
  results: RunResult[];
  run: Run | null;
  running: boolean;
}

function hasScripts(x?: { preRequestScript?: string; postResponseScript?: string } | null): boolean {
  return !!(x?.preRequestScript?.trim() || x?.postResponseScript?.trim());
}

/** Scripts may have saved environment or collection variables during a run. */
async function reloadVariables() {
  await Promise.all([useEnvironments().load(), useWorkspace().load()]);
}

export const useRunner = defineStore('runner', () => {
  const sendingOp = ref('');
  const collectionRun = ref<CollectionRun | null>(null);
  let unsubscribe: (() => void) | null = null;

  /**
   * Shows preflight warnings. Returns whether to proceed and whether the user
   * explicitly confirmed (required for protected environments).
   */
  async function reviewWarnings(warnings: Warning[]): Promise<{ proceed: boolean; confirmed: boolean }> {
    const ui = useUi();
    const blocking = warnings.filter((w) => w.requiresConfirmation);
    const errors = warnings.filter((w) => w.level === 'error');
    if (blocking.length > 0) {
      const ok = await ui.confirm({
        title: 'Confirm run against a protected environment',
        message: 'Review these warnings before sending. Only continue if this environment and data are safe to modify.',
        details: warnings.filter((w) => w.level !== 'info' || w.requiresConfirmation).map((w) => w.message),
        confirmLabel: 'Run anyway',
        danger: true,
      });
      return { proceed: ok, confirmed: ok };
    }
    for (const e of errors) ui.toast(e.message, 'error');
    return { proceed: true, confirmed: false };
  }

  async function send() {
    const ws = useWorkspace();
    const env = useEnvironments();
    const ui = useUi();
    if (!ws.draft || sendingOp.value) return;
    const req = clone(ws.draft);
    try {
      const { proceed, confirmed } = await reviewWarnings(await api.preflightRequest(req, env.selectedId));
      if (!proceed) return;
      const opId = uid();
      sendingOp.value = opId;
      const result = await api.sendRequest(opId, req, env.selectedId, confirmed);
      ws.setResult(req.id, result);
      if (hasScripts(req) || hasScripts(ws.currentCollection)) await reloadVariables();
    } catch (e) {
      ui.error(e);
    } finally {
      sendingOp.value = '';
    }
  }

  async function cancelSend() {
    if (sendingOp.value) await api.cancel(sendingOp.value);
  }

  async function runCollection(collectionId: string, folderId = '', title = '') {
    const env = useEnvironments();
    const ui = useUi();
    if (collectionRun.value?.running) {
      ui.toast('A collection run is already in progress', 'error');
      ui.view = 'run';
      return;
    }
    try {
      const { proceed, confirmed } = await reviewWarnings(
        await api.preflightCollection(collectionId, folderId, env.selectedId),
      );
      if (!proceed) return;
      const opId = uid();
      collectionRun.value = {
        opId,
        collectionId,
        folderId,
        title,
        done: 0,
        total: 0,
        results: [],
        run: null,
        running: true,
      };
      ui.view = 'run';
      unsubscribe?.();
      unsubscribe = onRunProgress((p) => {
        const cr = collectionRun.value;
        if (!cr || p.opId !== cr.opId) return;
        cr.done = p.done;
        cr.total = p.total;
        cr.results.push(p.result);
      });
      const run = await api.runCollection(opId, collectionId, folderId, env.selectedId, confirmed);
      if (collectionRun.value?.opId === opId) {
        collectionRun.value.run = run;
        collectionRun.value.results = run.results ?? [];
        collectionRun.value.done = collectionRun.value.total = collectionRun.value.results.length;
      }
      const ws = useWorkspace();
      for (const r of run.results ?? []) if (r.requestId) ws.setResult(r.requestId, r);
      const tree = ws.trees[collectionId];
      if (!tree || hasScripts(tree.collection) || tree.requests.some(hasScripts)) await reloadVariables();
    } catch (e) {
      ui.error(e);
    } finally {
      if (collectionRun.value) collectionRun.value.running = false;
      unsubscribe?.();
      unsubscribe = null;
    }
  }

  async function cancelCollectionRun() {
    if (collectionRun.value?.running) await api.cancel(collectionRun.value.opId);
  }

  return { sendingOp, collectionRun, send, cancelSend, runCollection, cancelCollectionRun };
});
