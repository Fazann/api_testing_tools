import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { Collection, Environment, SignoffResult } from '@/types';

import SignoffDialog from './SignoffDialog.vue';

const collection: Collection = {
  id: 'c1',
  name: 'Shop',
  description: '',
  requiredVariables: [],
  variables: [],
  auth: { type: 'none' },
};

const built: SignoffResult = {
  html: '<!doctype html><title>x</title><h1>Release sign-off: v2.3.0</h1>',
  fileName: 'signoff-Shop-v2.3.0-20261106.html',
  ready: false,
  reasons: ['1 regression from its baseline'],
};

function setup() {
  const backend = {
    GetSettings: vi.fn(async () => ({ reviewerName: 'Dana (QA)' })),
    BuildSignoffReport: vi.fn(async () => structuredClone(built)),
    SaveSignoffReport: vi.fn(async () => '/home/dana/signoff.html'),
  };
  window.go = { main: { App: backend as never } };
  const pinia = createPinia();
  setActivePinia(pinia);
  const envs = useEnvironments();
  envs.list = [{ id: 'e1', name: 'Staging', variables: [] } as unknown as Environment];
  envs.selectedId = 'e1';
  const w = mount(SignoffDialog, { props: { collection }, global: { plugins: [pinia] }, attachTo: document.body });
  return { w, backend };
}

const button = (w: ReturnType<typeof setup>['w'], text: string) =>
  w.findAll('button').find((b) => b.text().startsWith(text));

afterEach(() => {
  delete window.go;
  document.body.innerHTML = '';
});

describe('SignoffDialog', () => {
  it('requires a release before building', async () => {
    const { w, backend } = setup();
    await flushPromises();
    await button(w, 'Preview')!.trigger('click');
    await flushPromises();
    expect(backend.BuildSignoffReport).not.toHaveBeenCalled();
    expect(useUi().toasts[0].text).toContain('Enter the release');
  });

  it('builds with the selected environment and the default name, then previews in a script-free sandbox', async () => {
    const { w, backend } = setup();
    await flushPromises();
    const inputs = w.findAll('input');
    expect((inputs[1].element as HTMLInputElement).value).toBe('Dana (QA)');
    await inputs[0].setValue('v2.3.0');
    await button(w, 'Preview')!.trigger('click');
    await flushPromises();
    expect(backend.BuildSignoffReport).toHaveBeenCalledWith(
      expect.objectContaining({ collectionId: 'c1', environmentId: 'e1', release: 'v2.3.0', preparedBy: 'Dana (QA)' }),
    );
    const frame = w.find('iframe');
    expect(frame.attributes('srcdoc')).toContain('Release sign-off: v2.3.0');
    expect(frame.attributes('sandbox')).not.toContain('allow-scripts');
    expect(w.text()).toContain('Not ready — 1 regression from its baseline');
  });

  it('saves the previewed HTML and prints the preview', async () => {
    const { w, backend } = setup();
    await flushPromises();
    await w.findAll('input')[0].setValue('v2.3.0');
    await button(w, 'Preview')!.trigger('click');
    await flushPromises();

    await button(w, 'Save HTML')!.trigger('click');
    await flushPromises();
    expect(backend.SaveSignoffReport).toHaveBeenCalledWith(built.fileName, built.html);
    expect(useUi().toasts.map((t) => t.text)).toContain('Report saved to /home/dana/signoff.html');

    const win = (w.find('iframe').element as HTMLIFrameElement).contentWindow!;
    // jsdom has no print(); the app calls the webview's.
    const print = vi.fn();
    Object.defineProperty(win, 'print', { value: print, configurable: true });
    await button(w, 'Print / Save as PDF')!.trigger('click');
    expect(print).toHaveBeenCalledOnce();

    await button(w, 'Back')!.trigger('click');
    expect(w.find('iframe').exists()).toBe(false);
  });
});
