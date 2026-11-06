<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import SelectField from '@/components/base/SelectField.vue';
import VariableInput from '@/components/variables/VariableInput.vue';
import {
  formatExpected,
  KIND_LABELS,
  needsExpected,
  needsPath,
  newAssertion,
  OPERATOR_LABELS,
  OPERATORS,
  parseExpected,
} from '@/services/model';
import type { Assertion, AssertionKind } from '@/types';

const props = defineProps<{ assertions: Assertion[] }>();
const kinds = Object.keys(KIND_LABELS) as AssertionKind[];
const KIND_OPTIONS = kinds.map((k) => ({ value: k, label: KIND_LABELS[k] }));
const FUNCTIONS = (['count', 'sum', 'min', 'max'] as const).map((f) => ({ value: f, label: f }));
const QUICK_ADD: AssertionKind[] = [
  'status',
  'json_path',
  'header',
  'response_time',
  'aggregate',
  'required_fields',
  'valid_json',
  'body',
];
const operatorOptions = (kind: AssertionKind) =>
  OPERATORS[kind].map((op) => ({ value: op, label: OPERATOR_LABELS[op] }));

function add(kind: AssertionKind) {
  props.assertions.push(newAssertion(kind));
}

function changeKind(a: Assertion, kind: AssertionKind) {
  const fresh = newAssertion(kind);
  Object.assign(a, {
    kind,
    operator: fresh.operator,
    path: fresh.path,
    expected: fresh.expected,
    function: fresh.function,
    tolerance: 0,
  });
}

function changeOperator(a: Assertion) {
  if (a.operator === 'type' && typeof a.expected !== 'string') a.expected = 'string';
}

function expectedText(a: Assertion): string {
  if (a.kind === 'required_fields' && Array.isArray(a.expected)) return (a.expected as string[]).join(', ');
  return formatExpected(a.expected);
}

function setExpected(a: Assertion, text: string) {
  if (a.kind === 'required_fields') {
    a.expected = text
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    return;
  }
  // Header and body checks, patterns and type names always compare literal text.
  const literal = a.kind === 'header' || a.kind === 'body' || a.operator === 'matches' || a.operator === 'type';
  a.expected = literal ? text : parseExpected(text);
}

function placeholder(a: Assertion): string {
  if (a.kind === 'required_fields') return 'data.id, data.name';
  if (a.operator === 'type') return 'string | number | integer | boolean | object | array | null';
  if (a.operator === 'matches') return '^[A-Z]{3}$';
  if (a.kind === 'aggregate') return '{"$path": "total"} or a number';
  return '200, "text", true, {"$path": "data.total"}';
}

function remove(i: number) {
  props.assertions.splice(i, 1);
}
function move(i: number, d: number) {
  const j = i + d;
  if (j < 0 || j >= props.assertions.length) return;
  const [x] = props.assertions.splice(i, 1);
  props.assertions.splice(j, 0, x);
}
</script>

<template>
  <div class="stack">
    <CalloutBox v-if="assertions.length === 0" tone="info">
      No tests yet. Add checks for the status code and the business values that matter, for example
      <code>data.balance</code> is a number or the item amounts sum to <code>total</code>.
    </CalloutBox>
    <div v-for="(a, i) in assertions" :key="a.id" class="assertion" :class="{ off: !a.enabled }">
      <CheckboxField v-model="a.enabled" :aria-label="`Enable test ${i + 1}`" />
      <SelectField
        :model-value="a.kind"
        :options="KIND_OPTIONS"
        aria-label="Check"
        @update:model-value="(k) => k && changeKind(a, k)"
      />
      <SelectField v-if="a.kind === 'aggregate'" v-model="a.function" :options="FUNCTIONS" aria-label="Function" />
      <BaseInput
        v-if="needsPath(a.kind)"
        v-model="a.path"
        class="mono path"
        :placeholder="a.kind === 'header' ? 'Header-Name' : 'data.items[*].amount'"
        aria-label="Path"
      />
      <SelectField
        v-if="OPERATORS[a.kind][0] !== ''"
        v-model="a.operator"
        :options="operatorOptions(a.kind)"
        aria-label="Operator"
        @change="changeOperator(a)"
      />
      <VariableInput
        v-if="needsExpected(a)"
        :model-value="expectedText(a)"
        class="grow"
        :placeholder="placeholder(a)"
        aria-label="Expected value"
        @change="setExpected(a, ($event.target as HTMLInputElement).value)"
      />
      <span v-else class="grow" />
      <label
        v-if="a.kind === 'aggregate' || (a.kind === 'json_path' && (a.operator === 'eq' || a.operator === 'neq'))"
        class="row small muted tol"
        title="Absolute tolerance for numeric equality"
      >
        ±<BaseInput v-model="a.tolerance" type="number" min="0" step="any" aria-label="Tolerance" />
      </label>
      <span class="row actions">
        <BaseButton variant="ghost" :aria-label="`Move test ${i + 1} up`" @click="move(i, -1)">↑</BaseButton>
        <BaseButton variant="ghost" :aria-label="`Move test ${i + 1} down`" @click="move(i, 1)">↓</BaseButton>
        <BaseButton variant="ghost" :aria-label="`Remove test ${i + 1}`" @click="remove(i)">✕</BaseButton>
      </span>
      <BaseInput
        v-model="a.description"
        class="desc small"
        placeholder="Description (optional), e.g. rejected transaction keeps balance"
        aria-label="Description"
      />
    </div>
    <div class="row add">
      <span class="muted small">Add:</span>
      <BaseButton v-for="k in QUICK_ADD" :key="k" @click="add(k)">{{ KIND_LABELS[k] }}</BaseButton>
    </div>
    <p class="muted small">
      Paths: <code>data.items[0].id</code>, <code>items[*].amount</code> (all items), <code>meta["request-id"]</code>.
      Expected values are JSON; compare to another field with <code>{"$path": "data.total"}</code>; use
      <code v-pre>{{ variables }}</code> from the environment.
    </p>
  </div>
</template>

<style scoped>
.assertion {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  padding: 8px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--bg-subtle);
}
.assertion.off {
  opacity: 0.6;
}
.path {
  width: 210px;
}
.tol input {
  width: 72px;
}
.actions {
  gap: 0;
}
.desc {
  flex-basis: 100%;
  border-style: dashed;
}
.add {
  flex-wrap: wrap;
}
</style>
