<script setup lang="ts">
import { computed, ref, watch } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import EmptyState from '@/components/base/EmptyState.vue';
import SelectField from '@/components/base/SelectField.vue';
import TextField from '@/components/base/TextField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { isSocket } from '@/services/socket';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { CollectionTree, Rule, RuleKind, RuleOperator, RulesCheck, RuleTerm, RuleValue } from '@/types';
import { formatTime } from '@/utils/format';
import { clone, uid } from '@/utils/object';

import RuleValueEditor from './RuleValueEditor.vue';

// Rules across requests, checked after every collection run. See docs/collection-rules.md.
const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ dirty: [dirty: boolean] }>();
const ws = useWorkspace();
const ui = useUi();
const { busy: saving, run: runSave } = useAsyncAction();
const { busy: checking, run: runCheck } = useAsyncAction();

const MAX_RULES = 50;
const MAX_TERMS = 10;
const rules = ref<Rule[]>(clone(props.tree.collection.rules ?? []));
const snapshot = ref(JSON.stringify(rules.value));
const dirty = computed(() => JSON.stringify(rules.value) !== snapshot.value);
watch(dirty, (d) => emit('dirty', d));
const check = ref<RulesCheck | null>(null);

const requests = computed(() => props.tree.requests.filter((r) => !isSocket(r.kind)));

const KINDS: { value: RuleKind; label: string }[] = [
  { value: 'compare', label: 'Compare values' },
  { value: 'contains', label: 'Value appears in a list' },
];
const OPERATORS: Record<RuleKind, { value: RuleOperator; label: string }[]> = {
  compare: [
    { value: 'eq', label: '=' },
    { value: 'ne', label: '≠' },
    { value: 'gt', label: '>' },
    { value: 'gte', label: '≥' },
    { value: 'lt', label: '<' },
    { value: 'lte', label: '≤' },
  ],
  contains: [
    { value: 'in', label: 'appears in' },
    { value: 'not_in', label: 'does not appear in' },
  ],
};
const SIGNS = [
  { value: '+' as const, label: '+' },
  { value: '-' as const, label: '−' },
];

const firstRequest = () => requests.value[0]?.id ?? '';
const responseValue = (): RuleValue => ({ source: 'response', requestId: firstRequest(), path: '', aggregate: '' });

function addRule() {
  if (rules.value.length >= MAX_RULES) return ui.toast(`At most ${MAX_RULES} rules per collection`, 'error');
  rules.value.push({
    id: uid(),
    name: `Rule ${rules.value.length + 1}`,
    enabled: true,
    kind: 'compare',
    left: responseValue(),
    operator: 'eq',
    right: [{ sign: '+', value: responseValue() }],
    tolerance: 0,
  });
}

function setKind(rule: Rule, kind: RuleKind) {
  rule.kind = kind;
  rule.operator = OPERATORS[kind][0].value;
  if (kind === 'contains') {
    const first = rule.right[0]?.value;
    rule.right = [{ sign: '+', value: first && first.source !== 'constant' ? first : responseValue() }];
  }
}

function addTerm(rule: Rule) {
  if (rule.right.length < MAX_TERMS) rule.right.push({ sign: '-', value: { source: 'constant', literal: '' } });
}

function removeTerm(rule: Rule, t: RuleTerm) {
  rule.right = rule.right.filter((x) => x !== t);
}

async function removeRule(rule: Rule) {
  const ok = await ui.confirm({ title: `Delete rule "${rule.name}"?`, confirmLabel: 'Delete', danger: true });
  if (ok) rules.value = rules.value.filter((r) => r !== rule);
}

async function save() {
  const saved = await runSave(() => api.saveCollectionRules(props.tree.collection.id, clone(rules.value)));
  if (!saved) return;
  rules.value = saved;
  snapshot.value = JSON.stringify(saved);
  await ws.loadTree(props.tree.collection.id).catch(ui.error);
  ui.toast('Rules saved. They run after every collection run.', 'success');
}

async function checkLastRun() {
  const res = await runCheck(() => api.checkCollectionRules(props.tree.collection.id, clone(rules.value)));
  if (res) check.value = res;
}

const outcomes = computed(() => new Map((check.value?.outcomes ?? []).map((o) => [o.ruleId, o])));
</script>

<template>
  <div class="rules stack">
    <p class="muted small">
      Rules combine values from several requests of one collection run, for example
      <em>balance after = balance before − transfer amount</em> or
      <em>the created order's id appears in the order list</em>. They run after every collection run (and in the CLI)
      and fail the run when they do not hold.
    </p>
    <div class="row">
      <BaseButton :disabled="requests.length === 0" @click="addRule">Add rule</BaseButton>
      <BaseButton :disabled="checking || rules.length === 0" @click="checkLastRun">Check against last run</BaseButton>
      <span class="grow" />
      <BaseButton variant="primary" :disabled="!dirty || saving" @click="save">Save rules</BaseButton>
    </div>
    <p v-if="check" class="muted small">
      Checked against the run of {{ formatTime(check.startedAt) }}
      <template v-if="check.environmentName"> ({{ check.environmentName }})</template>. Nothing was sent.
    </p>

    <EmptyState v-if="rules.length === 0" small>
      <p v-if="requests.length === 0">Add HTTP requests to this collection first.</p>
      <p v-else>No rules yet. Press <strong>Add rule</strong> to check business logic across requests.</p>
    </EmptyState>

    <section v-for="(rule, i) in rules" :key="rule.id" class="rule stack" :class="{ disabled: !rule.enabled }">
      <div class="row head">
        <CheckboxField v-model="rule.enabled" :aria-label="`Rule ${i + 1} enabled`" />
        <BaseInput v-model="rule.name" class="grow name" :aria-label="`Rule ${i + 1} name`" maxlength="200" />
        <SelectField
          :model-value="rule.kind"
          :options="KINDS"
          :aria-label="`Rule ${i + 1} kind`"
          @update:model-value="setKind(rule, $event as RuleKind)"
        />
        <BaseButton small variant="ghost" :aria-label="`Delete rule ${rule.name}`" @click="removeRule(rule)">
          Delete
        </BaseButton>
      </div>

      <RuleValueEditor v-model="rule.left" :requests="requests" label="Left value" />
      <div class="row">
        <SelectField v-model="rule.operator" class="op" :options="OPERATORS[rule.kind]" aria-label="Operator" />
        <span v-if="rule.kind === 'compare' && rule.right.length > 1" class="muted small">
          the result of the values below
        </span>
      </div>

      <template v-if="rule.kind === 'compare'">
        <div v-for="(t, j) in rule.right" :key="j" class="row term">
          <SelectField v-model="t.sign" class="sign" :options="SIGNS" :aria-label="`Right value ${j + 1} sign`" />
          <RuleValueEditor
            v-model="t.value"
            class="grow"
            :requests="requests"
            allow-constant
            :label="`Right value ${j + 1}`"
          />
          <BaseButton
            v-if="rule.right.length > 1"
            small
            variant="ghost"
            :aria-label="`Remove right value ${j + 1}`"
            @click="removeTerm(rule, t)"
            >×</BaseButton
          >
        </div>
        <div class="row">
          <BaseButton small variant="ghost" :disabled="rule.right.length >= MAX_TERMS" @click="addTerm(rule)">
            + Add value
          </BaseButton>
          <TextField v-model="rule.tolerance" class="tol" label="Tolerance" type="number" min="0" step="any" />
        </div>
      </template>
      <RuleValueEditor v-else v-model="rule.right[0].value" :requests="requests" for-list label="List" />

      <p v-if="outcomes.get(rule.id)" class="small outcome">
        <template v-if="outcomes.get(rule.id)!.skipped">
          <span class="muted">Not checked: {{ outcomes.get(rule.id)!.reason }}.</span>
        </template>
        <template v-else>
          <span :class="outcomes.get(rule.id)!.result.passed ? 'pass' : 'fail'">
            {{ outcomes.get(rule.id)!.result.passed ? '✔' : '✘' }} {{ outcomes.get(rule.id)!.result.message }}
          </span>
        </template>
      </p>
    </section>
  </div>
</template>

<style scoped>
.rule {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 10px;
  gap: 8px;
}
.rule.disabled {
  opacity: 0.6;
}
.head {
  gap: 8px;
}
.name :deep(input),
.name {
  font-weight: 600;
}
.op {
  width: 180px;
}
.sign {
  width: 56px;
}
.term {
  gap: 6px;
  align-items: flex-start;
}
.tol :deep(input) {
  width: 110px;
}
.outcome {
  margin: 0;
}
</style>
