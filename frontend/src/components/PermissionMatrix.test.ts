import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '../services/model';
import { useUi } from '../stores/ui';
import type { CollectionTree, PermissionMatrix as Matrix, MatrixReport, Method, Request } from '../types';
import PermissionMatrix from './PermissionMatrix.vue';

function req(id: string, name: string, method: Method): Request {
  return { ...newRequest('c1', ''), id, name, method, url: `{{base_url}}/${id}` };
}

const tree: CollectionTree = {
  collection: {
    id: 'c1',
    name: 'Wallet',
    description: '',
    requiredVariables: [],
    variables: [],
    auth: { type: 'none' },
  },
  folders: [],
  requests: [req('r1', 'Alice wallet', 'GET'), req('r2', 'Delete user', 'DELETE')],
};

const stored: Matrix = {
  collectionId: 'c1',
  roles: [
    { id: 'alice', name: 'Alice', anonymous: false, variables: [] },
    { id: 'bob', name: 'Bob', anonymous: false, variables: [] },
  ],
  requestIds: ['r1'],
  expected: { r1: { alice: 'allow', bob: 'deny' } },
};

const report: MatrixReport = {
  runId: 'run1',
  passed: false,
  cancelled: false,
  roles: [
    { id: 'alice', name: 'Alice', anonymous: false },
    { id: 'bob', name: 'Bob', anonymous: false },
  ],
  requests: [{ id: 'r1', name: 'Alice wallet', method: 'GET' }],
  cells: [
    {
      requestId: 'r1',
      roleId: 'alice',
      expected: 'allow',
      outcome: 'allowed',
      statusCode: 200,
      durationMs: 3,
      verdict: 'pass',
      resultId: 'a',
      suggested: 'allow',
    },
    {
      requestId: 'r1',
      roleId: 'bob',
      expected: 'deny',
      outcome: 'allowed',
      statusCode: 200,
      durationMs: 3,
      verdict: 'fail',
      message: 'Bob should be denied but got 200 — broken access control',
      resultId: 'b',
      suggested: 'allow',
    },
  ],
  passes: 1,
  failures: 1,
  warnings: 0,
  unset: 0,
  errors: 0,
};

function setup() {
  const backend = {
    GetPermissionMatrix: vi.fn(async () => structuredClone(stored)),
    SavePermissionMatrix: vi.fn(async (m: unknown) => m),
    RunPermissionMatrix: vi.fn(async () => structuredClone(report)),
  };
  window.go = { main: { App: backend as never } };
  const pinia = createPinia();
  setActivePinia(pinia);
  const w = mount(PermissionMatrix, { props: { tree }, global: { plugins: [pinia] } });
  return { w, backend };
}

const cellButtons = (w: ReturnType<typeof setup>['w']) => w.findAll('tbody tr')[0].findAll('button.cell');

/** Clicks a button that opens a confirmation dialog and accepts it. */
async function clickAndConfirm(w: ReturnType<typeof setup>['w'], text: string) {
  const button = w.findAll('button').find((b) => b.text().startsWith(text));
  expect(button, text).toBeTruthy();
  await button!.trigger('click');
  await flushPromises();
  useUi().closeDialog(true);
  await flushPromises();
}

afterEach(() => {
  delete window.go;
});

describe('PermissionMatrix', () => {
  it('shows saved expectations and cycles a cell allow → deny → none', async () => {
    const { w } = setup();
    await flushPromises();
    expect(cellButtons(w).map((b) => b.find('.sym').text())).toEqual(['✓', '✕']);
    const alice = cellButtons(w)[0];
    await alice.trigger('click');
    expect(alice.find('.sym').text()).toBe('✕');
    await alice.trigger('click');
    expect(alice.find('.sym').text()).toBe('·');
    expect(w.emitted('dirty')?.slice(-1)[0]).toEqual([true]);
    // Requests that are not ticked cannot be edited.
    expect(w.findAll('tbody tr')[1].find('button.cell').attributes('disabled')).toBeDefined();
  });

  it('runs, highlights broken access control, and approves results', async () => {
    const { w, backend } = setup();
    await flushPromises();
    await clickAndConfirm(w, 'Run matrix');
    expect(backend.RunPermissionMatrix).toHaveBeenCalledWith(expect.any(String), 'c1', '', true);
    const bob = cellButtons(w)[1];
    expect(bob.classes()).toContain('v-fail');
    expect(bob.find('.code').text()).toBe('200');
    expect(w.text()).toContain('1 access mismatch');
    expect(w.find('.problems').text()).toContain('broken access control');

    await clickAndConfirm(w, 'Approve results as expected (1)');
    const saved = backend.SavePermissionMatrix.mock.calls.slice(-1)[0][0] as Matrix;
    expect(saved.expected.r1).toEqual({ alice: 'allow', bob: 'allow' });
    expect(cellButtons(w)[1].classes()).toContain('v-pass');
    expect(w.text()).toContain('Access matches the expectations');
  });

  it('adds a role and an anonymous role with unique names', async () => {
    const { w } = setup();
    await flushPromises();
    await w
      .findAll('button')
      .find((b) => b.text() === '＋ Anonymous')!
      .trigger('click');
    await w
      .findAll('button')
      .find((b) => b.text() === '＋ Role')!
      .trigger('click');
    const heads = w.findAll('thead th.role-col').map((t) => t.text());
    expect(heads).toEqual(['Alice', 'Bob', 'Anonymous', 'Role 4']);
    // A new role starts without overrides, so it cannot blank the token by accident.
    expect(w.text()).toContain("No overrides: this role uses the environment's variables");
    expect(w.findAll('.vars tbody tr')).toHaveLength(0);
    expect(w.findAll('button').some((b) => b.text() === '＋ Anonymous')).toBe(false);
  });
});
