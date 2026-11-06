import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import { h } from 'vue';

import BaseButton from './BaseButton.vue';
import BaseInput from './BaseInput.vue';
import CheckboxField from './CheckboxField.vue';
import DataTable from './DataTable.vue';
import EmptyState from './EmptyState.vue';
import FilterChip from './FilterChip.vue';
import ModalFrame from './ModalFrame.vue';
import RadioGroup from './RadioGroup.vue';
import SelectableList from './SelectableList.vue';
import SelectField from './SelectField.vue';
import StatTile from './StatTile.vue';
import TabBar from './TabBar.vue';
import TextField from './TextField.vue';

/** The arguments of the latest emit of an event. */
function last(events: unknown[][] | undefined): unknown[] | undefined {
  return events?.[events.length - 1];
}

type Row = { id: string; name: string; ms: number };
type Item = { id: string; name: string };

describe('BaseButton', () => {
  it('renders the variant as a class and defaults to type="button"', async () => {
    const w = mount(BaseButton, { props: { variant: 'primary', small: true }, slots: { default: 'Save' } });
    expect(w.attributes('type')).toBe('button');
    expect(w.classes()).toEqual(expect.arrayContaining(['primary', 'small']));
    await w.trigger('click');
    expect(w.emitted('click')).toHaveLength(1);
    expect(mount(BaseButton, { props: { type: 'submit' } }).attributes('type')).toBe('submit');
  });
});

describe('BaseInput', () => {
  it('renders a bare input with every attribute and emits numbers for type="number"', async () => {
    const w = mount(BaseInput, {
      props: { modelValue: 1 },
      attrs: { type: 'number', class: 'mono', 'aria-label': 'Count' },
    });
    expect(w.element.tagName).toBe('INPUT');
    expect(w.classes()).toContain('mono');
    expect(w.attributes('aria-label')).toBe('Count');
    await w.find('input').setValue('7');
    expect(last(w.emitted('update:modelValue'))).toEqual([7]);
  });

  it('renders a textarea when multiline and exposes focus()', async () => {
    const w = mount(BaseInput, { props: { modelValue: 'a', multiline: true, rows: 5 }, attachTo: document.body });
    expect(w.element.tagName).toBe('TEXTAREA');
    expect(w.attributes('rows')).toBe('5');
    w.vm.focus();
    expect(document.activeElement).toBe(w.element);
    w.unmount();
  });
});

describe('TextField', () => {
  it('binds v-model, puts class on the wrapper and other attributes on the input', async () => {
    const w = mount(TextField, {
      props: { modelValue: 'a', label: 'Name', hint: 'Shown below', inputClass: 'mono' },
      attrs: { class: 'wide', placeholder: 'name', maxlength: '20' },
    });
    expect(w.classes()).toEqual(expect.arrayContaining(['field', 'wide']));
    const input = w.find('input');
    expect(input.classes()).toEqual(['mono']);
    expect(input.attributes('placeholder')).toBe('name');
    expect(input.attributes('class')).not.toContain('wide');
    expect(w.text()).toContain('Name');
    expect(w.text()).toContain('Shown below');
    await input.setValue('b');
    expect(last(w.emitted('update:modelValue'))).toEqual(['b']);
  });

  it('emits numbers for type="number" and renders a textarea when multiline', async () => {
    const n = mount(TextField, { props: { modelValue: 1 }, attrs: { type: 'number' } });
    await n.find('input').setValue('42');
    expect(last(n.emitted('update:modelValue'))).toEqual([42]);
    expect(
      mount(TextField, { props: { multiline: true, rows: 4 } })
        .find('textarea')
        .attributes('rows'),
    ).toBe('4');
  });
});

describe('SelectField', () => {
  const options = [
    { value: 'a', label: 'Alpha' },
    { value: 'b', label: 'Beta', disabled: true },
  ];

  it('renders options and emits the chosen value', async () => {
    const w = mount(SelectField, { props: { modelValue: 'a', options, label: 'Pick' } });
    expect(w.findAll('option').map((o) => o.text())).toEqual(['Alpha', 'Beta']);
    expect(w.findAll('option')[1].attributes('disabled')).toBeDefined();
    await w.find('select').setValue('b');
    expect(last(w.emitted('update:modelValue'))).toEqual(['b']);
  });

  it('renders a bare select without a label', () => {
    const w = mount(SelectField, { props: { modelValue: 'a', options }, attrs: { class: 'small' } });
    expect(w.element.tagName).toBe('SELECT');
    expect(w.classes()).toContain('small');
  });
});

describe('CheckboxField', () => {
  it('toggles a boolean', async () => {
    const w = mount(CheckboxField, { props: { modelValue: false, label: 'Secret' } });
    await w.find('input').setValue(true);
    expect(last(w.emitted('update:modelValue'))).toEqual([true]);
    expect(w.text()).toBe('Secret');
  });

  it('renders a bare checkbox without a label, for table cells', async () => {
    const w = mount(CheckboxField, {
      props: { modelValue: true },
      attrs: { 'aria-label': 'Enable row 1', class: 'x' },
    });
    expect(w.element.tagName).toBe('INPUT');
    expect(w.attributes('aria-label')).toBe('Enable row 1');
    expect(w.classes()).toContain('x');
    await w.find('input').setValue(false);
    expect(last(w.emitted('update:modelValue'))).toEqual([false]);
  });

  it('adds its value to an array model', async () => {
    const w = mount(CheckboxField, { props: { modelValue: ['x'] }, attrs: { value: 'y' } });
    await w.find('input').setValue(true);
    expect(last(w.emitted('update:modelValue'))).toEqual([['x', 'y']]);
  });
});

describe('RadioGroup', () => {
  it('selects an option', async () => {
    const options = [
      { value: 'warn', label: 'Warn' },
      { value: 'fail', label: 'Fail' },
    ];
    const w = mount(RadioGroup, { props: { modelValue: 'warn', options, name: 'mode', label: 'Mode' } });
    expect(w.attributes('aria-label')).toBe('Mode');
    await w.findAll('input')[1].setValue();
    expect(last(w.emitted('update:modelValue'))).toEqual(['fail']);
  });
});

describe('TabBar', () => {
  it('shows counts, hides and disables tabs, and switches', async () => {
    const tabs = [
      { id: 'a', label: 'Tests', count: '2 failed', alert: true },
      { id: 'b', label: 'Body', disabled: true },
      { id: 'c', label: 'Hidden', hidden: true },
      { id: 'd', label: 'Headers', count: '' },
    ];
    const w = mount(TabBar, { props: { modelValue: 'a', tabs } });
    const buttons = w.findAll('button');
    expect(buttons.map((b) => b.text())).toEqual(['Tests2 failed', 'Body', 'Headers']);
    expect(buttons[0].attributes('aria-selected')).toBe('true');
    expect(buttons[0].find('.count').classes()).toContain('fail');
    expect(buttons[1].attributes('disabled')).toBeDefined();
    expect(buttons[2].find('.count').exists()).toBe(false);
    await buttons[2].trigger('click');
    expect(last(w.emitted('update:modelValue'))).toEqual(['d']);
  });
});

describe('DataTable', () => {
  const columns = [
    { key: 'name', label: 'Name' },
    { key: 'ms', label: 'Time', class: 'num' },
  ];
  const rows = [
    { id: 'r1', name: 'Orders', ms: 120 },
    { id: 'r2', name: 'Search', ms: 300 },
  ];

  it('renders headers, default cells and slot overrides', () => {
    const w = mount(DataTable, {
      props: { columns, rows, rowKey: (r: unknown) => (r as Row).id },
      slots: { 'cell-ms': ({ row }: { row: unknown }) => `${(row as Row).ms} ms` },
    });
    expect(w.findAll('th').map((t) => t.text())).toEqual(['Name', 'Time']);
    expect(w.find('th').classes()).not.toContain('num');
    expect(w.findAll('th')[1].classes()).toContain('num');
    const cells = w.findAll('tbody tr').map((tr) => tr.findAll('td').map((td) => td.text()));
    expect(cells).toEqual([
      ['Orders', '120 ms'],
      ['Search', '300 ms'],
    ]);
  });

  it('emits row-click with the mouse and Enter, and adds rows after a row', async () => {
    const w = mount(DataTable, {
      props: { columns, rows, clickable: true, rowClass: (r: unknown) => ({ hot: (r as Row).ms > 200 }) },
      slots: {
        'after-row': ({ row }: { row: unknown }) =>
          (row as Row).id === 'r2' ? h('tr', { class: 'detail' }, [h('td', { colspan: 2 }, 'details')]) : null,
      },
    });
    const trs = w.findAll('tbody tr');
    expect(trs).toHaveLength(3);
    expect(trs[1].classes()).toEqual(expect.arrayContaining(['clickable', 'hot']));
    expect(trs[2].classes()).toContain('detail');
    await trs[0].trigger('click');
    await trs[1].trigger('keydown', { key: 'Enter' });
    expect(w.emitted('row-click')?.map((e) => (e[0] as Row).id)).toEqual(['r1', 'r2']);
  });

  it('shows the empty text only when asked', () => {
    expect(
      mount(DataTable, { props: { columns, rows: [], empty: 'None' } })
        .find('tbody')
        .text(),
    ).toBe('None');
    expect(mount(DataTable, { props: { columns, rows: [] } }).findAll('tbody tr')).toHaveLength(0);
  });
});

describe('SelectableList', () => {
  it('marks the active item and emits the selected one', async () => {
    const items = [
      { id: 'a', name: 'Local' },
      { id: 'b', name: 'Staging' },
    ];
    const w = mount(SelectableList, {
      props: { items, itemKey: (i: unknown) => (i as Item).id, activeKey: 'b' },
      slots: { default: ({ item }: { item: unknown }) => (item as Item).name },
    });
    const buttons = w.findAll('button');
    expect(buttons.map((b) => b.text())).toEqual(['Local', 'Staging']);
    expect(buttons[1].classes()).toContain('active');
    expect(buttons[1].attributes('aria-current')).toBe('true');
    await buttons[0].trigger('click');
    expect(w.emitted('select')?.[0]).toEqual([items[0]]);
  });

  it('shows the empty slot without items', () => {
    const w = mount(SelectableList, {
      props: { items: [], itemKey: () => '' },
      slots: { empty: 'Nothing here' },
    });
    expect(w.text()).toBe('Nothing here');
  });
});

describe('small pieces', () => {
  it('FilterChip shows its count and pressed state', () => {
    const w = mount(FilterChip, { props: { active: true, count: 3 }, slots: { default: 'GET' } });
    expect(w.text()).toBe('GET 3');
    expect(w.attributes('aria-pressed')).toBe('true');
    expect(w.classes()).toContain('on');
  });

  it('StatTile shows a label, a value and a tone', () => {
    const w = mount(StatTile, { props: { label: 'Failed', value: 2, tone: 'fail' } });
    expect(w.find('.muted').text()).toBe('Failed');
    expect(w.find('b').text()).toBe('2');
    expect(w.find('b').classes()).toContain('fail');
  });

  it('EmptyState renders title, message and actions', () => {
    const w = mount(EmptyState, {
      props: { title: 'No runs', message: 'Run a collection first.' },
      slots: { actions: '<button>Run</button>' },
    });
    expect(w.find('h2').text()).toBe('No runs');
    expect(w.find('p').text()).toBe('Run a collection first.');
    expect(w.find('.row button').text()).toBe('Run');
  });
});

describe('ModalFrame', () => {
  it('closes from the close button and raises itself when stacked', async () => {
    const w = mount(ModalFrame, { props: { title: 'Confirm', stacked: true } });
    expect(w.classes()).toContain('stacked');
    await w.find('button[aria-label="Close"]').trigger('click');
    expect(w.emitted('close')).toHaveLength(1);
  });
});
