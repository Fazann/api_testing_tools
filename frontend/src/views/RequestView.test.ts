import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '../services/model';
import { useWorkspace } from '../stores/workspace';
import type { RunResult } from '../types';
import RequestView from './RequestView.vue';

function result(over: Partial<RunResult> = {}): RunResult {
  return {
    id: 'res1',
    runId: 'run1',
    requestId: 'q1',
    request: { name: 'Get settings', method: 'GET', url: 'http://localhost/x', headers: [] },
    response: {
      statusCode: 200,
      status: '200 OK',
      headers: [{ key: 'Content-Type', value: 'application/json', enabled: true }],
      body: '{"ok":true}',
      bodyBytes: 11,
      truncated: false,
      contentType: 'application/json',
    },
    assertionResults: [],
    durationMs: 12,
    status: 'passed',
    createdAt: '2026-11-05T08:00:00Z',
    ...over,
  };
}

function setup(sent: RunResult) {
  const calls: string[] = [];
  window.go = {
    main: {
      App: new Proxy(
        {},
        {
          get: (_t, name: string) => async () => {
            calls.push(name);
            if (name === 'PreflightRequest') return [];
            if (name === 'SendRequest') return sent;
            return null;
          },
        },
      ),
    },
  };
  const pinia = createPinia();
  setActivePinia(pinia);
  useWorkspace().open({ ...newRequest('c1'), id: 'q1', url: 'http://localhost/x' });
  const wrapper = mount(RequestView, { global: { plugins: [pinia] }, attachTo: document.body });
  return { wrapper, calls };
}

async function clickSend(wrapper: ReturnType<typeof mount>) {
  await wrapper
    .findAll('button')
    .find((b) => b.text() === 'Send')!
    .trigger('click');
  await flushPromises();
}

describe('RequestView send', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  it('shows the response after Send', async () => {
    const errors = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { wrapper, calls } = setup(result());
    await clickSend(wrapper);
    expect(calls).toEqual(['PreflightRequest', 'SendRequest']);
    expect(wrapper.text()).not.toContain('Sending');
    expect(wrapper.text()).toContain('200 OK');
    expect(errors).not.toHaveBeenCalled();
  });

  // Regression: a matching baseline came back as "diffs": null; reading its length crashed the
  // response panel and left the editor stuck on "Sending…".
  it('does not get stuck when the baseline matches and diffs is null', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const regression = { baselineId: 'b1', passed: true, diffs: null, message: 'response matches the baseline' };
    const { wrapper } = setup(result({ regression: regression as unknown as RunResult['regression'] }));
    await clickSend(wrapper);
    expect(wrapper.text()).not.toContain('Sending');
    expect(wrapper.text()).toContain('Matches baseline');
    expect(wrapper.text()).toContain('Baseline diff0');
    expect(warn).not.toHaveBeenCalled();
  });

  it('shows baseline differences', async () => {
    const regression = {
      baselineId: 'b1',
      passed: false,
      diffs: [{ path: 'ok', kind: 'changed' as const, before: 'true', after: 'false' }],
      message: '1 difference(s) from the baseline',
    };
    const { wrapper } = setup(result({ regression }));
    await clickSend(wrapper);
    expect(wrapper.text()).toContain('1 baseline difference(s)');
  });
});
