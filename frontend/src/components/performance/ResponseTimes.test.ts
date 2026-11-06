import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '../services/model';
import { useWorkspace } from '../stores/workspace';
import type {
  CollectionTree,
  PerformanceCheck,
  Request,
  ResponseTimePoint,
  ResponseTimeRow,
  ResponseTimeTrend,
  RunResult,
} from '../types';
import CollectionResponseTimes from './CollectionResponseTimes.vue';
import PerformancePanel from './PerformancePanel.vue';
import ResponseTimeChart from './ResponseTimeChart.vue';
import ResponseViewer from './ResponseViewer.vue';

function point(i: number, durationMs: number, slower = false): ResponseTimePoint {
  return {
    runId: `run${i}`,
    resultId: `res${i}`,
    at: new Date(2026, 10, 5, 12, i).toISOString(),
    durationMs,
    status: 'passed',
    statusCode: 200,
    slower,
  };
}

const check: PerformanceCheck = {
  requestId: 'r1',
  mode: 'warn',
  factor: 1.5,
  minDeltaMs: 100,
  window: 20,
  minSamples: 5,
};

const request: Request = { ...newRequest('c1', ''), id: 'r1', name: 'Orders', method: 'GET', url: '{{base_url}}/o' };
const tree: CollectionTree = {
  collection: { id: 'c1', name: 'Shop', description: '', requiredVariables: [], variables: [], auth: { type: 'none' } },
  folders: [],
  requests: [request, { ...request, id: 'r2', name: 'Search' }, { ...request, id: 'r3', name: 'New' }],
};

function row(
  id: string,
  name: string,
  medianMs: number,
  changePct: number | null,
  extra: Partial<ResponseTimeRow> = {},
) {
  return {
    requestId: id,
    name,
    method: 'GET',
    parentId: '',
    stats: { count: 10, minMs: medianMs, medianMs, p95Ms: medianMs * 2, maxMs: medianMs * 3 },
    lastMs: medianMs,
    changePct,
    recent: [medianMs, medianMs + 5, medianMs],
    lastSlower: false,
    mode: 'warn',
    ...extra,
  } as ResponseTimeRow;
}

function backend(methods: Record<string, unknown>) {
  window.go = { main: { App: methods as never } };
}

afterEach(() => {
  delete window.go;
});

describe('ResponseTimeChart', () => {
  it('highlights slow responses and reads values with the keyboard', async () => {
    const points = [point(0, 100), point(1, 110), point(2, 400, true), point(3, 105)];
    const w = mount(ResponseTimeChart, { props: { points, referenceMs: 105, thresholdMs: 205 } });
    expect(w.findAll('circle.slow')).toHaveLength(1);
    expect(w.find('.ref.median').exists()).toBe(true);
    expect(w.find('.ref.limit').exists()).toBe(true);
    expect(w.find('path.series').attributes('d')).toMatch(/^M/);

    const svg = w.find('svg');
    await svg.trigger('keydown', { key: 'ArrowLeft' }); // latest
    expect(w.find('.tip').text()).toContain('105 ms');
    await svg.trigger('keydown', { key: 'ArrowLeft' });
    expect(w.find('.tip').text()).toContain('400 ms');
    expect(w.find('.tip').text()).toContain('slower than usual');
    await svg.trigger('keydown', { key: 'Escape' });
    expect(w.find('.tip').exists()).toBe(false);
  });

  it('draws no limit line when the check is off', () => {
    const w = mount(ResponseTimeChart, { props: { points: [point(0, 10)], referenceMs: 10, thresholdMs: 0 } });
    expect(w.find('.ref.limit').exists()).toBe(false);
  });
});

describe('CollectionResponseTimes', () => {
  it('lists requests, sorts by slowdown and opens a request', async () => {
    setActivePinia(createPinia());
    const rows = [
      row('r1', 'Orders', 100, 5),
      row('r2', 'Search', 300, 64, { lastSlower: true, mode: 'fail' }),
      { ...row('r3', 'New', 0, null), stats: { count: 0, minMs: 0, medianMs: 0, p95Ms: 0, maxMs: 0 }, recent: [] },
    ];
    const get = vi.fn(async () => structuredClone(rows));
    backend({ GetCollectionResponseTimes: get });
    const w = mount(CollectionResponseTimes, { props: { tree } });
    await flushPromises();

    expect(get).toHaveBeenCalledWith('c1', expect.any(String));
    expect(w.text()).toContain('2 of 3 requests have response times');
    expect(w.text()).toContain('1 slower than usual in their latest response');
    expect(w.text()).toContain('No responses yet');
    expect(w.text()).toContain('fails if slow');
    const names = () => w.findAll('tbody td.name').map((td) => td.text().split(' ')[0]);
    expect(names()).toEqual(['Orders', 'Search', 'New']);

    await w.find('select').setValue('change');
    expect(names()).toEqual(['Search', 'Orders', 'New']);
    const change = w.findAll('tbody tr')[0].findAll('td')[5];
    expect(change.text()).toBe('+64%');
    expect(change.classes()).toContain('worse');

    await w.findAll('tbody tr')[0].trigger('click');
    expect(w.emitted('open')?.[0]?.[0]).toMatchObject({ id: 'r2' });
  });
});

describe('PerformancePanel', () => {
  function setup(trend: ResponseTimeTrend) {
    setActivePinia(createPinia());
    const ws = useWorkspace();
    ws.trees.c1 = structuredClone(tree);
    ws.open(request);
    const save = vi.fn(async (c: PerformanceCheck) => c);
    backend({ GetResponseTimeTrend: vi.fn(async () => structuredClone(trend)), SavePerformanceCheck: save });
    return { save, w: mount(PerformancePanel) };
  }

  it('shows the summary and saves a stricter check', async () => {
    const { w, save } = setup({
      requestId: 'r1',
      points: [point(0, 100), point(1, 120), point(2, 90), point(3, 400, true), point(4, 110), point(5, 100)],
      stats: { count: 6, minMs: 90, medianMs: 105, p95Ms: 400, maxMs: 400 },
      check,
      next: {
        mode: 'warn',
        referenceMs: 105,
        thresholdMs: 205,
        samples: 6,
        needed: 5,
        ratio: 0,
        slower: false,
        message: '',
      },
    });
    await flushPromises();
    expect(w.text()).toContain('105 ms');
    expect(w.text()).toContain('205 ms');
    expect(w.text()).toContain('1 slower than usual');

    const button = w.findAll('button').find((b) => b.text() === 'Save check')!;
    expect(button.attributes('disabled')).toBeDefined();
    await w.find('input[value="fail"]').setValue();
    expect(button.attributes('disabled')).toBeUndefined();
    await button.trigger('click');
    await flushPromises();
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ requestId: 'r1', mode: 'fail', factor: 1.5 }));

    await w
      .findAll('button')
      .find((b) => b.text() === 'Show table')!
      .trigger('click');
    expect(w.findAll('tbody tr')).toHaveLength(6);
    expect(w.find('tbody tr').text()).toContain('100 ms'); // latest first
  });

  it('says how much history is still needed', async () => {
    const { w } = setup({
      requestId: 'r1',
      points: [point(0, 100), point(1, 120)],
      stats: { count: 2, minMs: 100, medianMs: 110, p95Ms: 120, maxMs: 120 },
      check,
      next: {
        mode: 'warn',
        referenceMs: 0,
        thresholdMs: 0,
        samples: 2,
        needed: 5,
        ratio: 0,
        slower: false,
        message: '',
      },
    });
    await flushPromises();
    expect(w.text()).toContain('2/5 responses');
  });

  it('asks to save an unsaved request first', () => {
    setActivePinia(createPinia());
    useWorkspace().open({ ...request, id: '' });
    expect(mount(PerformancePanel).text()).toContain('Save this request to track its response times.');
  });
});

describe('ResponseViewer', () => {
  it('flags a response that is slower than usual', () => {
    setActivePinia(createPinia());
    const result: RunResult = {
      id: 'x',
      runId: 'run',
      requestId: 'r1',
      request: { name: 'Orders', method: 'GET', url: 'http://x', headers: [] },
      response: {
        statusCode: 200,
        status: '200 OK',
        headers: [],
        body: '{}',
        bodyBytes: 2,
        truncated: false,
        contentType: 'application/json',
      },
      assertionResults: [],
      durationMs: 320,
      status: 'failed',
      createdAt: new Date().toISOString(),
      performance: {
        mode: 'fail',
        referenceMs: 200,
        thresholdMs: 300,
        samples: 5,
        needed: 5,
        ratio: 1.6,
        slower: true,
        message: '320 ms is slower than usual',
      },
    };
    const w = mount(ResponseViewer, { props: { result } });
    const badge = w.find('.perf-slow');
    expect(badge.text()).toBe('▲ 1.6× slower than usual (fails)');
    expect(badge.attributes('title')).toBe('320 ms is slower than usual');

    const ok = mount(ResponseViewer, {
      props: { result: { ...result, performance: { ...result.performance!, slower: false } } },
    });
    expect(ok.find('.perf-slow').exists()).toBe(false);
  });
});
