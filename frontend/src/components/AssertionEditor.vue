<script setup lang="ts">
import {
  formatExpected,
  KIND_LABELS,
  needsExpected,
  needsPath,
  newAssertion,
  OPERATOR_LABELS,
  OPERATORS,
  parseExpected,
} from '../services/model';
import type { Assertion, AssertionKind } from '../types';
import VariableInput from './VariableInput.vue';

const props = defineProps<{ assertions: Assertion[] }>();
const kinds = Object.keys(KIND_LABELS) as AssertionKind[];

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
    <div v-if="assertions.length === 0" class="callout info">
      No tests yet. Add checks for the status code and the business values that matter, for example
      <code>data.balance</code> is a number or the item amounts sum to <code>total</code>.
    </div>
    <div v-for="(a, i) in assertions" :key="a.id" class="assertion" :class="{ off: !a.enabled }">
      <input v-model="a.enabled" type="checkbox" :aria-label="`Enable test ${i + 1}`" />
      <select
        :value="a.kind"
        aria-label="Check"
        @change="changeKind(a, ($event.target as HTMLSelectElement).value as AssertionKind)"
      >
        <option v-for="k in kinds" :key="k" :value="k">{{ KIND_LABELS[k] }}</option>
      </select>
      <select v-if="a.kind === 'aggregate'" v-model="a.function" aria-label="Function">
        <option value="count">count</option>
        <option value="sum">sum</option>
        <option value="min">min</option>
        <option value="max">max</option>
      </select>
      <input
        v-if="needsPath(a.kind)"
        v-model="a.path"
        class="mono path"
        :placeholder="a.kind === 'header' ? 'Header-Name' : 'data.items[*].amount'"
        aria-label="Path"
      />
      <select v-if="OPERATORS[a.kind][0] !== ''" v-model="a.operator" aria-label="Operator" @change="changeOperator(a)">
        <option v-for="op in OPERATORS[a.kind]" :key="op" :value="op">{{ OPERATOR_LABELS[op] }}</option>
      </select>
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
        ±<input v-model.number="a.tolerance" type="number" min="0" step="any" aria-label="Tolerance" />
      </label>
      <span class="row actions">
        <button class="ghost" :aria-label="`Move test ${i + 1} up`" @click="move(i, -1)">↑</button>
        <button class="ghost" :aria-label="`Move test ${i + 1} down`" @click="move(i, 1)">↓</button>
        <button class="ghost" :aria-label="`Remove test ${i + 1}`" @click="remove(i)">✕</button>
      </span>
      <input
        v-model="a.description"
        class="desc small"
        placeholder="Description (optional), e.g. rejected transaction keeps balance"
        aria-label="Description"
      />
    </div>
    <div class="row add">
      <span class="muted small">Add:</span>
      <button
        v-for="k in [
          'status',
          'json_path',
          'header',
          'response_time',
          'aggregate',
          'required_fields',
          'valid_json',
          'body',
        ]"
        :key="k"
        @click="add(k as AssertionKind)"
      >
        {{ KIND_LABELS[k as AssertionKind] }}
      </button>
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
