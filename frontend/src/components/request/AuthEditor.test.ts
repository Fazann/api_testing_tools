import { mount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { describe, expect, it } from 'vitest';

import type { Auth, Collection } from '../types';
import AuthEditor from './AuthEditor.vue';

const collection: Collection = {
  id: 'c1',
  name: 'Wallet',
  description: '',
  requiredVariables: [],
  variables: [],
  auth: { type: 'bearer', token: '{{token}}' },
};

function mountEditor(auth: Auth, inheritFrom?: Collection | null) {
  return mount(AuthEditor, { props: { auth, inheritFrom }, global: { plugins: [createPinia()] } });
}

describe('AuthEditor', () => {
  it('offers inheritance for requests and shows the collection auth', async () => {
    const wrapper = mountEditor({ type: 'inherit' }, collection);
    expect(wrapper.find('option[value="inherit"]').exists()).toBe(true);
    expect(wrapper.text()).toContain('Wallet');
    expect(wrapper.text()).toContain('Bearer {{token}}');
    await wrapper.find('button').trigger('click');
    expect(wrapper.emitted('editCollection')).toHaveLength(1);
  });

  it('does not offer inheritance for the collection itself', () => {
    const wrapper = mountEditor({ type: 'bearer', token: '{{token}}' });
    expect(wrapper.find('option[value="inherit"]').exists()).toBe(false);
    expect(wrapper.text()).toContain('Inherit auth from collection');
  });

  it('explains when a request has no collection to inherit from', () => {
    const wrapper = mountEditor({ type: 'inherit' }, null);
    expect(wrapper.text()).toContain('nothing to inherit');
  });
});
