import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import { useUi } from '@/stores/ui';
import type { CollectionTree, ContractCoverage, ContractInfo, Request } from '@/types';

import CollectionContract from './CollectionContract.vue';

function req(id: string, name: string): Request {
  return { ...newRequest('c1', ''), id, name };
}

const tree: CollectionTree = {
  collection: { id: 'c1', name: 'Pets', description: '', requiredVariables: [], variables: [], auth: { type: 'none' } },
  folders: [],
  requests: [req('r1', 'Get a pet'), req('r2', 'Health')],
};

const attached: ContractInfo = {
  attached: true,
  sourceName: 'pets.json',
  title: 'Pets',
  version: '1.0',
  operations: 3,
  enabled: true,
  allowUndocumented: false,
  updatedAt: '2026-11-07T09:00:00Z',
};

const coverage: ContractCoverage = {
  requests: [
    { requestId: 'r1', name: 'Get a pet', method: 'GET', operation: 'GET /pets/{petId}' },
    { requestId: 'r2', name: 'Health', method: 'GET', operation: '' },
  ],
  operations: [
    { operation: 'GET /pets/{petId}', summary: 'Get a pet', requests: 1 },
    { operation: 'DELETE /pets/{petId}', summary: 'Delete a pet', requests: 0 },
  ],
};

function setup(info: ContractInfo) {
  const backend = {
    GetContract: vi.fn(async () => structuredClone(info)),
    GetContractCoverage: vi.fn(async () => structuredClone(coverage)),
    AttachContract: vi.fn(async () => structuredClone(attached)),
    SetContractOptions: vi.fn(async (_id: string, enabled: boolean, allowUndocumented: boolean) => ({
      ...info,
      enabled,
      allowUndocumented,
    })),
    RemoveContract: vi.fn(async () => undefined),
  };
  window.go = { main: { App: backend as never } };
  setActivePinia(createPinia());
  const wrapper = mount(CollectionContract, { props: { tree } });
  return { wrapper, backend };
}

afterEach(() => {
  window.go = undefined;
});

describe('CollectionContract', () => {
  it('offers to attach a spec when none is attached', async () => {
    const { wrapper, backend } = setup({ ...attached, attached: false });
    await flushPromises();
    expect(wrapper.text()).toContain('No spec attached');
    expect(backend.GetContractCoverage).not.toHaveBeenCalled();
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Attach spec…')!
      .trigger('click');
    await flushPromises();
    expect(backend.AttachContract).toHaveBeenCalledWith('c1');
    expect(wrapper.text()).toContain('3 operations');
  });

  it('shows the spec, coverage and operations without a request', async () => {
    const { wrapper } = setup(attached);
    await flushPromises();
    const text = wrapper.text();
    expect(wrapper.find('.spec strong').text()).toBe('Pets');
    expect(text).toContain('1.0');
    expect(text).toContain('from pets.json');
    expect(text).toContain('GET /pets/{petId}');
    expect(text).toContain('not in the spec');
    expect(text).toContain('1 request is not in the spec');
    expect(text).toContain('Operations without a request (1)');
    expect(text).toContain('DELETE /pets/{petId}');
  });

  it('saves the options and opens a request from the coverage table', async () => {
    const { wrapper, backend } = setup(attached);
    await flushPromises();
    const boxes = wrapper.findAll('input[type="checkbox"]');
    await boxes[1].setValue(true);
    await flushPromises();
    expect(backend.SetContractOptions).toHaveBeenCalledWith('c1', true, true);
    await boxes[0].setValue(false);
    await flushPromises();
    expect(backend.SetContractOptions).toHaveBeenLastCalledWith('c1', false, true);

    await wrapper.find('tbody tr').trigger('click');
    expect(wrapper.emitted('open')?.[0]?.[0]).toMatchObject({ id: 'r1' });
  });

  it('removes the spec after confirmation', async () => {
    const { wrapper, backend } = setup(attached);
    await flushPromises();
    const ui = useUi();
    vi.spyOn(ui, 'confirm').mockResolvedValue(true);
    backend.GetContract.mockResolvedValueOnce({ ...attached, attached: false });
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Remove')!
      .trigger('click');
    await flushPromises();
    expect(backend.RemoveContract).toHaveBeenCalledWith('c1');
    expect(wrapper.text()).toContain('No spec attached');
  });
});
