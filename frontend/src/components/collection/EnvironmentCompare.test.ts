import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { CollectionTree, CompareReport, CompareResult, Environment, Method, Request } from '@/types';

import EnvironmentCompare from './EnvironmentCompare.vue';

function req(id: string, name: string, method: Method): Request {
  return { ...newRequest('c1', ''), id, name, method, url: `{{base_url}}/${id}` };
}

const tree: CollectionTree = {
  collection: { id: 'c1', name: 'Shop', description: '', requiredVariables: [], variables: [], auth: { type: 'none' } },
  folders: [],
  requests: [req('r1', 'Get account', 'GET'), req('r2', 'Health', 'GET'), req('r3', 'Create order', 'POST')],
};

const env = (id: string, name: string): Environment => ({ id, name, protected: false, variables: [] }) as never;

const side = (code: number, ms: number) => ({
  resultId: 'x',
  status: 'passed' as const,
  statusCode: code,
  durationMs: ms,
});
const result = (o: Partial<CompareResult>): CompareResult => ({
  requestId: 'r1',
  name: 'Get account',
  method: 'GET',
  verdict: 'same',
  slower: '',
  a: side(200, 10),
  b: side(200, 12),
  statusChanged: false,
  body: [],
  headers: [],
  changing: [],
  ...o,
});

const report: CompareReport = {
  a: { id: 'e1', name: 'Production' },
  b: { id: 'e2', name: 'Staging' },
  runIdA: 'runA',
  runIdB: 'runB',
  requests: [
    result({
      verdict: 'different',
      body: [{ path: 'balance', kind: 'type_changed', before: 'number 1', after: 'string "1"' }],
      changing: ['request_id'],
    }),
    result({ requestId: 'r2', name: 'Health', verdict: 'same', changing: ['uptime_s'] }),
    result({
      requestId: 'r3',
      name: 'Create order',
      method: 'POST',
      verdict: 'skipped',
      skipReason: 'may modify data',
    }),
  ],
  same: 1,
  different: 1,
  errors: 0,
  skipped: 1,
  slower: 0,
  cancelled: false,
};

function setup(saved: Partial<{ envA: string; envB: string; includeModifying: boolean }> = {}) {
  const backend = {
    GetEnvComparison: vi.fn(async () => ({
      collectionId: 'c1',
      envA: '',
      envB: '',
      ignoredPaths: [],
      includeModifying: false,
      detectChanging: true,
      ...saved,
    })),
    CompareEnvironments: vi.fn(async () => structuredClone(report)),
  };
  window.go = { main: { App: backend as never } };
  const pinia = createPinia();
  setActivePinia(pinia);
  const envs = useEnvironments();
  envs.list = [env('e1', 'Production'), env('e2', 'Staging')];
  envs.selectedId = 'e1';
  const w = mount(EnvironmentCompare, { props: { tree }, global: { plugins: [pinia] } });
  return { w, backend };
}

const button = (w: ReturnType<typeof setup>['w'], text: string) =>
  w.findAll('button').find((b) => b.text().startsWith(text));

afterEach(() => {
  delete window.go;
});

describe('EnvironmentCompare', () => {
  it('pre-fills two different environments and compares read-only requests only', async () => {
    const { w, backend } = setup();
    await flushPromises();
    expect(button(w, 'Compare')!.text()).toBe('Compare 2 requests');
    await button(w, 'Compare')!.trigger('click');
    await flushPromises();
    const [, cfg, confirmed] = backend.CompareEnvironments.mock.calls[0] as unknown as [string, never, boolean];
    expect(cfg).toMatchObject({ collectionId: 'c1', envA: 'e1', envB: 'e2', detectChanging: true });
    expect(confirmed).toBe(false);
  });

  it('shows only differences until identical requests are shown, and expands a row', async () => {
    const { w } = setup();
    await flushPromises();
    await button(w, 'Compare')!.trigger('click');
    await flushPromises();
    expect(w.text()).toContain('Production vs Staging');
    const rows = () => w.findAll('tbody tr');
    expect(rows()).toHaveLength(1);
    expect(rows()[0].text()).toContain('Get account');
    expect(rows()[0].text()).toContain('Different');

    await rows()[0].trigger('click');
    expect(w.find('.detail').text()).toContain('balance');
    expect(w.find('.detail').text()).toContain('request_id');

    const boxes = w.findAll('input[type="checkbox"]');
    await boxes[boxes.length - 1].setValue(true);
    expect(w.text()).toContain('Health');
    expect(w.text()).toContain('Create order');
  });

  it('asks before including requests that may modify data and passes the confirmation', async () => {
    const { w, backend } = setup({ envA: 'e1', envB: 'e2', includeModifying: true });
    await flushPromises();
    expect(button(w, 'Compare')!.text()).toBe('Compare 3 requests');
    await button(w, 'Compare')!.trigger('click');
    await flushPromises();
    expect(useUi().dialog?.title).toContain('may modify data');
    useUi().closeDialog(true);
    await flushPromises();
    const call = backend.CompareEnvironments.mock.calls[0] as unknown as [string, unknown, boolean];
    expect(call[2]).toBe(true);
  });

  it('refuses the same environment twice without calling the backend', async () => {
    const { w, backend } = setup({ envA: 'e1', envB: 'e1' });
    await flushPromises();
    // The saved B equals A, so the form pre-fills B with the other environment; force it back.
    const selects = w.findAll('select');
    await selects[1].setValue('e1');
    await button(w, 'Compare')!.trigger('click');
    await flushPromises();
    expect(backend.CompareEnvironments).not.toHaveBeenCalled();
    expect(useUi().toasts.map((t) => t.text)).toContain('Choose two different environments');
  });

  it('explains that two environments are needed', async () => {
    setActivePinia(createPinia());
    window.go = { main: { App: { GetEnvComparison: vi.fn(async () => ({})) } as never } };
    const pinia = createPinia();
    setActivePinia(pinia);
    useEnvironments().list = [env('e1', 'Only one')];
    const w = mount(EnvironmentCompare, { props: { tree }, global: { plugins: [pinia] } });
    await flushPromises();
    expect(w.text()).toContain('Create at least two environments');
  });
});
