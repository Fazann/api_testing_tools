<script setup lang="ts">
import { computed } from 'vue';

import BaseInput from '@/components/base/BaseInput.vue';
import SelectField from '@/components/base/SelectField.vue';
import type { Request, RuleAggregate, RuleSource, RuleValue } from '@/types';

// One value of a collection rule: a request + where to read (body path, status, header), or a constant.
const props = defineProps<{
  requests: readonly Request[];
  /** Offer "Constant" as a source. */
  allowConstant?: boolean;
  /** The value selects a list (right side of "appears in"), so count/sum/min/max are not offered. */
  forList?: boolean;
  label: string;
}>();
const model = defineModel<RuleValue>({ required: true });

const SOURCES: { value: RuleSource; label: string }[] = [
  { value: 'response', label: 'Response body' },
  { value: 'request', label: 'Request body' },
  { value: 'status', label: 'Status code' },
  { value: 'header', label: 'Response header' },
];
const sources = computed(() =>
  props.allowConstant ? [...SOURCES, { value: 'constant' as const, label: 'Constant' }] : SOURCES,
);
const AGGREGATES: { value: RuleAggregate; label: string }[] = [
  { value: '', label: 'value' },
  { value: 'count', label: 'count' },
  { value: 'sum', label: 'sum' },
  { value: 'min', label: 'min' },
  { value: 'max', label: 'max' },
];
const requestOptions = computed(() => props.requests.map((r) => ({ value: r.id, label: `${r.method} ${r.name}` })));
const isBody = computed(() => model.value.source === 'response' || model.value.source === 'request');
const missing = computed(
  () =>
    model.value.source !== 'constant' &&
    !!model.value.requestId &&
    !props.requests.some((r) => r.id === model.value.requestId),
);

const source = computed({
  get: () => model.value.source,
  set: (s: RuleSource) => {
    const v: RuleValue = { ...model.value, source: s };
    if (s === 'constant') {
      v.literal = v.literal ?? '';
    } else if (!v.requestId) {
      v.requestId = props.requests[0]?.id ?? '';
    }
    if (s === 'status') v.path = '';
    if (!isBodySource(s)) v.aggregate = '';
    model.value = v;
  },
});
const isBodySource = (s: RuleSource) => s === 'response' || s === 'request';

function update(field: keyof RuleValue, value: string) {
  model.value = { ...model.value, [field]: value };
}
</script>

<template>
  <div class="rule-value row" :aria-label="label">
    <SelectField v-model="source" class="source" :options="sources" :aria-label="`${label}: source`" />
    <template v-if="model.source === 'constant'">
      <BaseInput
        :model-value="model.literal ?? ''"
        class="grow mono"
        placeholder='1, 0.5, paid, "text", true'
        :aria-label="`${label}: constant`"
        @update:model-value="update('literal', String($event))"
      />
    </template>
    <template v-else>
      <SelectField
        :model-value="model.requestId"
        class="request"
        :class="{ missing }"
        :options="requestOptions"
        :aria-label="`${label}: request`"
        @update:model-value="update('requestId', String($event ?? ''))"
      />
      <SelectField
        v-if="isBody && !forList"
        :model-value="model.aggregate ?? ''"
        class="aggregate"
        :options="AGGREGATES"
        :aria-label="`${label}: function`"
        @update:model-value="update('aggregate', String($event ?? ''))"
      />
      <BaseInput
        v-if="model.source !== 'status'"
        :model-value="model.path ?? ''"
        class="grow mono"
        :placeholder="
          model.source === 'header' ? 'Location' : forList ? 'orders[*].id' : model.aggregate ? 'items' : 'data.balance'
        "
        :aria-label="`${label}: ${model.source === 'header' ? 'header name' : 'JSON path'}`"
        @update:model-value="update('path', String($event))"
      />
    </template>
    <span v-if="missing" class="fail small">request no longer exists</span>
  </div>
</template>

<style scoped>
.rule-value {
  gap: 6px;
  flex-wrap: wrap;
}
.source {
  width: 150px;
}
.request {
  min-width: 180px;
  max-width: 260px;
}
.aggregate {
  width: 90px;
}
.missing {
  outline: 1px solid var(--fail);
  border-radius: var(--radius);
}
</style>
