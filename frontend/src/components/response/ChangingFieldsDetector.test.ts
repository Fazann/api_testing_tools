import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import { useUi } from '@/stores/ui';
import type { ChangingReport, Method, Request } from '@/types';

import ChangingFieldsDetector from './ChangingFieldsDetector.vue';

const req = (method: Method = 'GET'): Request => ({ ...newRequest('c1', ''), id: 'r1', name: 'Health', method });

const report: ChangingReport = {
  samples: 3,
  statusCodes: [200, 200, 200],
  statusChanged: false,
  fields: [
    { path: 'checked_at', reason: 'value', examples: ['"10:00"', '"10:01"'], suggested: true },
    { path: 'events', reason: 'length', examples: ['3 items', '4 items'], suggested: false },
  ],
};

function setup(request: Request, ignored: string[] = []) {
  const backend = {
    PreflightRequest: vi.fn(async () => []),
    DetectChangingFields: vi.fn(async () => structuredClone(report)),
  };
  window.go = { main: { App: backend as never } };
  const pinia = createPinia();
  setActivePinia(pinia);
  const w = mount(ChangingFieldsDetector, { props: { request, ignored }, global: { plugins: [pinia] } });
  return { w, backend };
}

const button = (w: ReturnType<typeof setup>['w'], text: string) =>
  w.findAll('button').find((b) => b.text().startsWith(text));

afterEach(() => {
  delete window.go;
});

describe('ChangingFieldsDetector', () => {
  it('detects, ticks suggestions but not length changes, and emits the ticked paths', async () => {
    const { w, backend } = setup(req(), ['meta.id']);
    await button(w, 'Detect changing fields')!.trigger('click');
    await flushPromises();
    expect(backend.DetectChangingFields).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ ignoredPaths: ['meta.id'], confirmed: false }),
    );
    const boxes = w.findAll('input[type="checkbox"]');
    expect(boxes.map((b) => (b.element as HTMLInputElement).checked)).toEqual([true, false]);
    expect(w.text()).toContain('"10:00" · "10:01"');

    await boxes[1].setValue(true);
    await button(w, 'Add 2 to ignored paths')!.trigger('click');
    expect(w.emitted('add')?.[0]).toEqual([['checked_at', 'events']]);
  });

  it('asks before repeating a request that may modify data', async () => {
    const { w, backend } = setup(req('POST'));
    await button(w, 'Detect changing fields')!.trigger('click');
    await flushPromises();
    expect(useUi().dialog?.title).toBe('Send POST Health 3 more times?');
    useUi().closeDialog(false);
    await flushPromises();
    expect(backend.DetectChangingFields).not.toHaveBeenCalled();

    await button(w, 'Detect changing fields')!.trigger('click');
    await flushPromises();
    useUi().closeDialog(true);
    await flushPromises();
    const call = backend.DetectChangingFields.mock.calls[0] as unknown as [string, unknown];
    expect(call[1]).toMatchObject({ confirmed: true });
  });

  it('is hidden for socket requests', () => {
    const { w } = setup({ ...req(), kind: 'websocket' });
    expect(w.find('button').exists()).toBe(false);
  });
});
