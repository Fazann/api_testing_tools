import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import { useUi } from '@/stores/ui';
import type { CollectionTree, Method, Request, Rule, RulesCheck } from '@/types';

import CollectionRules from './CollectionRules.vue';

function req(id: string, name: string, method: Method = 'GET'): Request {
  return { ...newRequest('c1', ''), id, name, method };
}

const saved: Rule = {
  id: 'r1',
  name: 'Withdrawal is deducted',
  enabled: true,
  kind: 'compare',
  left: { source: 'response', requestId: 'after', path: 'data.balance' },
  operator: 'eq',
  right: [
    { sign: '+', value: { source: 'response', requestId: 'before', path: 'data.balance' } },
    { sign: '-', value: { source: 'request', requestId: 'gone', path: 'amount' } },
  ],
  tolerance: 0,
};

function tree(rules: Rule[] = []): CollectionTree {
  return {
    collection: {
      id: 'c1',
      name: 'Wallet',
      description: '',
      requiredVariables: [],
      variables: [],
      auth: { type: 'none' },
      rules,
    },
    folders: [],
    requests: [req('before', 'Balance before'), req('after', 'Balance after'), req('ws', 'Live', 'GET')].map((r) =>
      r.id === 'ws' ? { ...r, kind: 'websocket' as const } : r,
    ),
  };
}

const check: RulesCheck = {
  runId: 'run1',
  startedAt: '2026-11-06T10:00:00Z',
  environmentName: 'Local',
  outcomes: [
    {
      ruleId: 'r1',
      skipped: false,
      result: {
        assertionId: 'r1',
        kind: 'rule',
        passed: false,
        expected: '',
        actual: '',
        message: '90 ≠ 100 − 20 = 80',
      },
    },
  ],
};

function setup(t: CollectionTree) {
  const backend = {
    SaveCollectionRules: vi.fn(async (_id: unknown, rules: unknown) => rules),
    CheckCollectionRules: vi.fn(async () => structuredClone(check)),
    GetCollectionTree: vi.fn(async () => t),
  };
  window.go = { main: { App: backend as never } };
  const pinia = createPinia();
  setActivePinia(pinia);
  const w = mount(CollectionRules, { props: { tree: t }, global: { plugins: [pinia] } });
  return { w, backend };
}

const button = (w: ReturnType<typeof setup>['w'], text: string) =>
  w.findAll('button').find((b) => b.text().startsWith(text));

afterEach(() => {
  delete window.go;
});

describe('CollectionRules', () => {
  it('adds a rule with HTTP requests only, marks the tab dirty and saves', async () => {
    const { w, backend } = setup(tree());
    expect(w.text()).toContain('No rules yet');
    await button(w, 'Add rule')!.trigger('click');
    expect(w.emitted('dirty')?.slice(-1)[0]).toEqual([true]);
    const requestOptions = w
      .find('[aria-label="Left value: request"]')
      .findAll('option')
      .map((o) => o.text());
    expect(requestOptions).toEqual(['GET Balance before', 'GET Balance after']);

    await w.find('[aria-label="Left value: JSON path"]').setValue('data.balance');
    await button(w, 'Save rules')!.trigger('click');
    await flushPromises();
    const [id, rules] = backend.SaveCollectionRules.mock.calls[0] as unknown as [string, Rule[]];
    expect(id).toBe('c1');
    expect(rules).toHaveLength(1);
    expect(rules[0]).toMatchObject({
      kind: 'compare',
      operator: 'eq',
      left: { requestId: 'before', path: 'data.balance' },
    });
    expect(w.emitted('dirty')?.slice(-1)[0]).toEqual([false]);
  });

  it('shows a saved rule, warns about a deleted request and shows the check result', async () => {
    const { w, backend } = setup(tree([saved]));
    expect(w.find('[aria-label="Rule 1 name"]').element).toHaveProperty('value', 'Withdrawal is deducted');
    expect(w.text()).toContain('request no longer exists');
    await button(w, 'Check against last run')!.trigger('click');
    await flushPromises();
    expect(backend.CheckCollectionRules).toHaveBeenCalledOnce();
    expect(w.text()).toContain('✘ 90 ≠ 100 − 20 = 80');
    expect(w.text()).toContain('(Local)');
  });

  it('switching to a list check keeps one non-constant value and the right operators', async () => {
    const { w } = setup(tree([{ ...saved, right: [{ sign: '+', value: { source: 'constant', literal: '1' } }] }]));
    await w.find('[aria-label="Rule 1 kind"]').setValue('contains');
    expect(
      w
        .find('[aria-label="Operator"]')
        .findAll('option')
        .map((o) => o.text()),
    ).toEqual(['appears in', 'does not appear in']);
    expect(w.find('[aria-label="List: source"]').element).toHaveProperty('value', 'response');
    expect(w.find('[aria-label="List: function"]').exists()).toBe(false);
  });

  it('adds and removes values on the right side', async () => {
    const { w } = setup(tree([saved]));
    await button(w, '+ Add value')!.trigger('click');
    expect(w.findAll('.term')).toHaveLength(3);
    await w.find('[aria-label="Remove right value 3"]').trigger('click');
    expect(w.findAll('.term')).toHaveLength(2);
  });

  it('asks before deleting a rule', async () => {
    const { w } = setup(tree([saved]));
    await w.find('[aria-label="Delete rule Withdrawal is deducted"]').trigger('click');
    await flushPromises();
    useUi().closeDialog(true);
    await flushPromises();
    expect(w.text()).toContain('No rules yet');
  });
});
