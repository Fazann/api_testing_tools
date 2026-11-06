import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useEnvironments } from '@/stores/environments';
import { useWorkspace } from '@/stores/workspace';
import type { Environment } from '@/types';

import VariableInput from './VariableInput.vue';

const env: Environment = {
  id: 'e1',
  name: 'Local',
  protected: false,
  variables: [
    { key: 'base_url', value: 'http://localhost:8080', secret: false, enabled: true },
    { key: 'token', value: 'super-secret', secret: true, enabled: true },
  ],
};

function setup(value: string) {
  const pinia = createPinia();
  setActivePinia(pinia);
  const envs = useEnvironments();
  envs.list = [structuredClone(env)];
  envs.selectedId = 'e1';
  const wrapper = mount(VariableInput, {
    props: { modelValue: value, 'aria-label': 'URL' },
    global: { plugins: [pinia] },
    attachTo: document.body,
  });
  return { wrapper, envs };
}

function popover() {
  return document.body.querySelector('.var-popover');
}

describe('VariableInput', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('highlights defined and missing variables', () => {
    const { wrapper } = setup('{{base_url}}/users/{{user_id}}');
    const tokens = wrapper.findAll('.token');
    expect(tokens.map((t) => t.text())).toEqual(['{{base_url}}', '{{user_id}}']);
    expect(tokens[0].classes()).toContain('defined');
    expect(tokens[1].classes()).toContain('missing');
    expect(wrapper.find('input').attributes('aria-label')).toBe('URL');
  });

  it('shows the value and environment on hover', async () => {
    const { wrapper } = setup('{{base_url}}/health');
    await wrapper.find('.token').trigger('mouseenter');
    expect(popover()?.textContent).toContain('http://localhost:8080');
    expect(popover()?.textContent).toContain('Local');
    wrapper.unmount();
  });

  it('masks secret values until revealed', async () => {
    const { wrapper } = setup('Bearer {{token}}');
    await wrapper.find('.token').trigger('mouseenter');
    expect(popover()?.textContent).not.toContain('super-secret');
    const show = [...popover()!.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Show')!;
    show.click();
    await wrapper.vm.$nextTick();
    expect(popover()?.textContent).toContain('super-secret');
    wrapper.unmount();
  });

  it('adds a missing variable to the selected environment', async () => {
    const { wrapper, envs } = setup('{{base_url}}/users/{{user_id}}');
    const save = vi.spyOn(envs, 'save').mockResolvedValue(env);
    await wrapper.findAll('.token')[1].trigger('mouseenter');
    const add = [...popover()!.querySelectorAll('button')].find((b) => b.textContent?.includes('Add to Local'))!;
    add.click();
    await wrapper.vm.$nextTick();
    const input = popover()!.querySelector<HTMLInputElement>('input[aria-label="Value of user_id"]')!;
    input.value = '42';
    input.dispatchEvent(new Event('input'));
    [...popover()!.querySelectorAll('button')].find((b) => b.textContent?.trim() === 'Save')!.click();
    await wrapper.vm.$nextTick();
    expect(save).toHaveBeenCalledOnce();
    const saved = save.mock.calls[0][0] as Environment;
    expect(saved.variables.find((v) => v.key === 'user_id')).toEqual({
      key: 'user_id',
      value: '42',
      secret: false,
      enabled: true,
    });
    wrapper.unmount();
  });

  it('renders a plain input when there are no variables', () => {
    const { wrapper } = setup('https://example.com');
    expect(wrapper.find('.mirror').exists()).toBe(false);
    expect(wrapper.find('input').classes()).not.toContain('masked');
  });

  it('resolves collection variables for requests in that collection', async () => {
    const { wrapper } = setup('{{api_root}}/x');
    expect(wrapper.find('.token').classes()).toContain('missing');
    const ws = useWorkspace();
    ws.collections = [
      {
        id: 'c1',
        name: 'Wallet',
        description: '',
        requiredVariables: [],
        variables: [{ key: 'api_root', value: 'https://collection.test', secret: false, enabled: true }],
        auth: { type: 'none' },
      },
    ];
    ws.open({
      id: 'r1',
      collectionId: 'c1',
      parentId: '',
      name: 'R',
      method: 'GET',
      url: '{{api_root}}/x',
      headers: [],
      query: [],
      body: '',
      bodyType: 'none',
      auth: { type: 'inherit' },
      timeoutMs: 1000,
      followRedirects: true,
      sortOrder: 0,
      assertions: [],
      preRequestScript: '',
      postResponseScript: '',
    });
    await wrapper.vm.$nextTick();
    expect(wrapper.find('.token').classes()).toContain('defined');
    await wrapper.find('.token').trigger('mouseenter');
    expect(popover()?.textContent).toContain('Collection variable');
    expect(popover()?.textContent).toContain('https://collection.test');
  });
});
