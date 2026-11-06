import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it } from 'vitest';

import { newRequest } from '../services/model';
import type { CollectionTree, Method, Request } from '../types';
import CollectionEndpoints from './CollectionEndpoints.vue';

function req(id: string, name: string, method: Method, url: string, parentId = ''): Request {
  return { ...newRequest('c1', parentId), id, name, method, url };
}

const tree: CollectionTree = {
  collection: { id: 'c1', name: 'Shop', description: '', requiredVariables: [], variables: [], auth: { type: 'none' } },
  folders: [
    { id: 'f1', collectionId: 'c1', parentId: '', name: 'Orders', sortOrder: 0 },
    { id: 'f2', collectionId: 'c1', parentId: 'f1', name: 'Admin', sortOrder: 0 },
  ],
  requests: [
    req('r1', 'Health', 'GET', '{{base_url}}/health'),
    req('r2', 'List orders', 'GET', '{{base_url}}/orders', 'f1'),
    req('r3', 'Create order', 'POST', '{{base_url}}/orders', 'f1'),
    req('r4', 'Delete order', 'DELETE', '{{base_url}}/orders/1', 'f2'),
  ],
};

function setup() {
  const pinia = createPinia();
  setActivePinia(pinia);
  return mount(CollectionEndpoints, { props: { tree }, global: { plugins: [pinia] } });
}

const names = (w: ReturnType<typeof setup>) => w.findAll('tbody td.name').map((t) => t.text());

describe('CollectionEndpoints', () => {
  it('lists every request in sidebar order with folder paths and method counts', () => {
    const w = setup();
    expect(names(w)).toEqual(['Delete order', 'List orders', 'Create order', 'Health']);
    expect(w.text()).toContain('Orders / Admin');
    const chips = w.findAll('.filters button').map((b) => b.text().replace(/\s+/g, ' '));
    expect(chips).toEqual(['All 4', 'GET 2', 'POST 1', 'DELETE 1']);
  });

  it('filters by method, toggles back to all, and searches', async () => {
    const w = setup();
    await w.findAll('.filters button')[1].trigger('click');
    expect(names(w)).toEqual(['List orders', 'Health']);
    await w.findAll('.filters button')[1].trigger('click');
    expect(names(w)).toHaveLength(4);
    await w.find('input.search').setValue('admin');
    expect(names(w)).toEqual(['Delete order']);
    await w.find('input.search').setValue('nothing-matches');
    expect(w.text()).toContain('No endpoints match this filter.');
  });

  it('emits open for a clicked or Enter-pressed row', async () => {
    const w = setup();
    const rows = w.findAll('tbody tr');
    await rows[2].trigger('click');
    await rows[3].trigger('keydown', { key: 'Enter' });
    expect(w.emitted('open')?.map(([r]) => (r as Request).id)).toEqual(['r3', 'r1']);
  });
});

describe('CollectionEndpoints version filter', () => {
  const versioned: CollectionTree = {
    ...tree,
    collection: {
      ...tree.collection,
      variables: [{ key: 'legacy', value: 'https://old.test/api/v1', secret: false, enabled: true }],
    },
    folders: [],
    requests: [
      req('a', 'List v2', 'GET', '{{base_url}}/v2/orders'),
      req('b', 'Create v1', 'POST', '{{base_url}}/v1/orders'),
      req('c', 'Legacy', 'GET', '{{legacy}}/orders'),
      req('d', 'Health', 'GET', '{{base_url}}/health'),
      req('e', 'List v10', 'GET', '{{base_url}}/v10/orders'),
    ],
  };

  function mountVersioned() {
    const pinia = createPinia();
    setActivePinia(pinia);
    return mount(CollectionEndpoints, { props: { tree: versioned }, global: { plugins: [pinia] } });
  }
  const chips = (w: ReturnType<typeof mountVersioned>) =>
    w.findAll('.versions button').map((b) => b.text().replace(/\s+/g, ' '));

  it('shows a chip per version, sorted, plus requests without one', () => {
    const w = mountVersioned();
    expect(chips(w)).toEqual(['All', 'v1 2', 'v2 1', 'v10 1', 'No version 1']);
    expect(w.findAll('thead th').map((t) => t.text())).toContain('Version');
  });

  it('filters by version, combined with the method filter', async () => {
    const w = mountVersioned();
    await w.findAll('.versions button')[1].trigger('click'); // v1
    expect(names(w)).toEqual(['Create v1', 'Legacy']);
    await w.findAll('.filters:not(.versions) button')[1].trigger('click'); // GET
    expect(names(w)).toEqual(['Legacy']);
    await w.findAll('.versions button')[4].trigger('click'); // No version
    expect(names(w)).toEqual(['Health']);
  });

  it('hides the version row when no request has a version', () => {
    const w = setup();
    expect(w.find('.versions').exists()).toBe(false);
    expect(w.findAll('thead th').map((t) => t.text())).not.toContain('Version');
  });
});
