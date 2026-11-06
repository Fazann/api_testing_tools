import { ref } from 'vue';

import { useUi } from '@/stores/ui';

/**
 * Runs async work with a `busy` flag and shows any error as a toast, replacing
 * the try / catch / finally block around backend calls.
 *
 *   const { busy: saving, run } = useAsyncAction();
 *   const save = () => run(async () => { await api.save(x); ui.toast('Saved', 'success'); });
 *
 * `run` resolves to the work's result, or `undefined` when it failed.
 */
export function useAsyncAction() {
  const ui = useUi();
  const busy = ref(false);

  async function run<T>(work: () => Promise<T>): Promise<T | undefined> {
    busy.value = true;
    try {
      return await work();
    } catch (e) {
      ui.error(e);
      return undefined;
    } finally {
      busy.value = false;
    }
  }

  return { busy, run };
}
