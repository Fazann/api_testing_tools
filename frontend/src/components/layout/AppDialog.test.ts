import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { describe, expect, it } from 'vitest';
import { nextTick } from 'vue';

import { useUi } from '@/stores/ui';

import AppDialog from './AppDialog.vue';

function setup() {
  const pinia = createPinia();
  setActivePinia(pinia);
  const wrapper = mount(AppDialog, { global: { plugins: [pinia] }, attachTo: document.body });
  return { wrapper, ui: useUi() };
}

describe('AppDialog', () => {
  it('resolves a prompt with the trimmed value when the form is submitted from the footer button', async () => {
    const { wrapper, ui } = setup();
    const result = ui.prompt({ title: 'Rename', value: 'old' });
    await nextTick();
    await nextTick();
    const input = wrapper.find('input');
    expect(document.activeElement).toBe(input.element);
    await input.setValue('  new name  ');
    // The submit button sits in the ModalFrame footer, outside the form, and is linked to it by `form`.
    const submit = wrapper.find('button[type="submit"]');
    expect(submit.attributes('form')).toBe(wrapper.find('form').attributes('id'));
    await wrapper.find('form').trigger('submit');
    await expect(result).resolves.toBe('new name');
    expect(ui.dialog).toBeNull();
    wrapper.unmount();
  });

  it('resolves a confirm with false on cancel and renders above other modals', async () => {
    const { wrapper, ui } = setup();
    const result = ui.confirm({ title: 'Delete?', details: ['3 requests'] });
    await nextTick();
    expect(wrapper.find('.backdrop').classes()).toContain('stacked');
    expect(wrapper.text()).toContain('3 requests');
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'Cancel')!
      .trigger('click');
    await expect(result).resolves.toBe(false);
    wrapper.unmount();
  });
});
