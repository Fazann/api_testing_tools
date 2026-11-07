import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import type { Request } from '@/types';

import CodeExportDialog from './CodeExportDialog.vue';

const request: Request = {
  ...newRequest('c1', ''),
  id: 'r1',
  name: 'Create order',
  method: 'POST',
  url: 'https://api.example.com/orders',
  bodyType: 'json',
  body: '{"amount": 10}',
};

describe('CodeExportDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
  });
  afterEach(() => localStorage.clear());

  it('starts with cURL, switches language and remembers the choice', async () => {
    const w = mount(CodeExportDialog, { props: { request } });
    expect(w.find('pre').text()).toMatch(/^curl /);
    await w.find('select').setValue('python');
    expect(w.find('pre').text()).toContain('import requests');
    expect(localStorage.getItem('arg.codeLanguage')).toBe('python');

    const again = mount(CodeExportDialog, { props: { request } });
    expect(again.find('pre').text()).toContain('requests.request(');
  });

  it('copies the code for the chosen language', async () => {
    const writeText = vi.fn(async () => undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    const w = mount(CodeExportDialog, { props: { request } });
    await w.find('select').setValue('rust');
    await w
      .findAll('button')
      .find((b) => b.text() === 'Copy to clipboard')!
      .trigger('click');
    await flushPromises();
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('reqwest::blocking::Client::builder()'));
    expect(w.emitted('close')).toBeTruthy();
  });

  it('ignores an unknown remembered language', () => {
    localStorage.setItem('arg.codeLanguage', 'cobol');
    const w = mount(CodeExportDialog, { props: { request } });
    expect(w.find('pre').text()).toMatch(/^curl /);
  });
});
