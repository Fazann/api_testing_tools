import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it } from 'vitest';

import { useUi } from '@/stores/ui';

import { useAsyncAction } from './useAsyncAction';

describe('useAsyncAction', () => {
  it('tracks busy and returns the result', async () => {
    setActivePinia(createPinia());
    const { busy, run } = useAsyncAction();
    let seen = false;
    const result = await run(async () => {
      seen = busy.value;
      return 42;
    });
    expect(seen).toBe(true);
    expect(busy.value).toBe(false);
    expect(result).toBe(42);
  });

  it('shows errors as a toast and resolves to undefined', async () => {
    setActivePinia(createPinia());
    const { busy, run } = useAsyncAction();
    const result = await run(async () => {
      throw new Error('boom');
    });
    expect(result).toBeUndefined();
    expect(busy.value).toBe(false);
    expect(useUi().toasts.map((t) => [t.kind, t.text])).toEqual([['error', 'boom']]);
  });
});
