import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { nextTick } from 'vue';

import CodeEditor from './CodeEditor.vue';

function setup(value: string) {
  const wrapper = mount(CodeEditor, {
    props: { modelValue: value, 'onUpdate:modelValue': (v: string) => wrapper.setProps({ modelValue: v }) },
    attachTo: document.body,
  });
  return wrapper;
}

describe('CodeEditor', () => {
  it('renders highlighted tokens and one line number per line', () => {
    const w = setup('const a = 1;\n// two');
    expect(w.find('.highlight .tk-keyword').text()).toBe('const');
    expect(w.find('.highlight .tk-comment').text()).toBe('// two');
    expect(w.findAll('.gutter div div').map((d) => d.text())).toEqual(['1', '2']);
  });

  it('indents with Tab and keeps indentation on Enter', async () => {
    const w = setup('if (x) {');
    const ta = w.find('textarea');
    const el = ta.element as HTMLTextAreaElement;
    el.setSelectionRange(8, 8);
    await ta.trigger('keydown', { key: 'Enter' });
    expect(w.props('modelValue')).toBe('if (x) {\n  ');
    await nextTick();
    el.setSelectionRange(el.value.length, el.value.length);
    await ta.trigger('keydown', { key: 'Tab' });
    expect(w.props('modelValue')).toBe('if (x) {\n    ');
  });
});
