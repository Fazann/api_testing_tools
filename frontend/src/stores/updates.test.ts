import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { UpdateInfo } from '@/types';

import { useUpdates } from './updates';

const info = (o: Partial<UpdateInfo> = {}): UpdateInfo => ({
  enabled: true,
  current: '0.1.0',
  latest: '0.1.1',
  available: true,
  notes: '',
  pageUrl: 'https://example.com/r',
  method: 'replace',
  ...o,
});

function bind(methods: Record<string, (...a: unknown[]) => Promise<unknown>>) {
  window.go = { main: { App: methods } };
}

afterEach(() => {
  delete window.go;
});

describe('updates store', () => {
  it('shows the banner for an available update until dismissed', async () => {
    setActivePinia(createPinia());
    bind({ CheckForUpdate: async () => info() });
    const u = useUpdates();
    await u.check(true);
    expect(u.showBanner).toBe(true);
    u.dismiss();
    expect(u.showBanner).toBe(false);
  });

  it('quiet checks swallow errors, manual checks report them', async () => {
    setActivePinia(createPinia());
    bind({
      CheckForUpdate: async () => {
        throw 'offline';
      },
    });
    const u = useUpdates();
    await u.check(true);
    expect(u.error).toBe('');
    await u.check();
    expect(u.error).toBe('offline');
  });

  it('opens the release page instead of installing when the method is manual', async () => {
    setActivePinia(createPinia());
    const open = vi.fn(async () => undefined);
    const install = vi.fn(async () => undefined);
    bind({ CheckForUpdate: async () => info({ method: 'manual' }), OpenReleasePage: open, InstallUpdate: install });
    const u = useUpdates();
    await u.check();
    await u.install();
    expect(open).toHaveBeenCalledWith('https://example.com/r');
    expect(install).not.toHaveBeenCalled();
  });

  it('keeps the error and re-enables the button when installing fails', async () => {
    setActivePinia(createPinia());
    bind({
      CheckForUpdate: async () => info(),
      InstallUpdate: async () => {
        throw 'update checksum mismatch';
      },
    });
    const u = useUpdates();
    await u.check();
    await u.install();
    expect(u.installing).toBe(false);
    expect(u.error).toContain('checksum');
  });
});
