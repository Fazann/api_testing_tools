<script setup lang="ts">
import { computed, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { isSocket } from '@/services/socket';
import { useEnvironments } from '@/stores/environments';
import { useRunner } from '@/stores/runner';
import { useUi } from '@/stores/ui';
import type { ChangingField, ChangingReport, Request } from '@/types';
import { isDestructive } from '@/utils/http';
import { clone, uid } from '@/utils/object';

// "Detect changing fields": sends the request a few times, lists the fields whose value changed and emits the
// ticked ones so the parent can add them to its ignored paths. See docs/changing-fields.md.
const props = defineProps<{ request: Request | null; ignored: string[] }>();
const emit = defineEmits<{ add: [paths: string[]] }>();
const envs = useEnvironments();
const runner = useRunner();
const ui = useUi();
const { run } = useAsyncAction();

const SAMPLES = 3;
const REASONS: Record<ChangingField['reason'], string> = {
  value: 'value changes',
  type: 'type changes',
  missing: 'present in some responses only',
  length: 'list length changes (ignoring it ignores everything in it)',
};

const report = ref<ChangingReport | null>(null);
const picked = ref<string[]>([]);
const opId = ref('');
const available = computed(() => !!props.request && !isSocket(props.request.kind));

async function detect() {
  const req = props.request;
  if (!req || opId.value) return;
  const warnings = await run(() => api.preflightRequest(clone(req), envs.selectedId));
  if (!warnings) return;
  const review = await runner.reviewWarnings(warnings);
  if (!review.proceed) return;
  let confirmed = review.confirmed;
  if (isDestructive(req.method)) {
    confirmed = await ui.confirm({
      title: `Send ${req.method} ${req.name} ${SAMPLES} more times?`,
      message:
        `Detecting changing fields sends the request ${SAMPLES} times in a row. ` +
        'Each call may create or change data.',
      confirmLabel: `Send ${SAMPLES} times`,
      danger: true,
    });
    if (!confirmed) return;
  }
  const id = uid();
  opId.value = id;
  report.value = null;
  const res = await run(() =>
    api.detectChangingFields(id, {
      request: clone(req),
      environmentId: envs.selectedId,
      ignoredPaths: [...props.ignored],
      confirmed,
    }),
  );
  opId.value = '';
  if (res) {
    report.value = res;
    picked.value = res.fields.filter((f) => f.suggested).map((f) => f.path);
  }
}

function cancel() {
  if (opId.value) api.cancel(opId.value).catch(ui.error);
}

function add() {
  emit('add', [...picked.value]);
  report.value = null;
}
</script>

<template>
  <div v-if="available" class="changing stack">
    <div class="row">
      <BaseButton small :disabled="!!opId" @click="detect">
        {{ opId ? 'Detecting…' : 'Detect changing fields' }}
      </BaseButton>
      <BaseButton v-if="opId" small variant="ghost" @click="cancel">Cancel</BaseButton>
      <span class="muted small">Sends the request {{ SAMPLES }} times and suggests fields whose value changed.</span>
    </div>
    <template v-if="report">
      <CalloutBox v-if="report.statusChanged" small>
        The status code changed between calls ({{ report.statusCodes.join(', ') }}), so this request may not be
        repeatable.
      </CalloutBox>
      <p v-if="report.fields.length === 0" class="muted small">
        No field changed in {{ report.samples }} calls (fields that are already ignored are not checked).
      </p>
      <template v-else>
        <div class="list">
          <CheckboxField v-for="f in report.fields" :key="f.path" v-model="picked" class="small" :value="f.path">
            <code>{{ f.path }}</code>
            <span class="muted"> — {{ REASONS[f.reason] }}</span>
            <span v-if="f.examples.length" class="mono examples">{{ f.examples.join(' · ') }}</span>
          </CheckboxField>
        </div>
        <div class="row">
          <BaseButton small variant="primary" :disabled="picked.length === 0" @click="add">
            Add {{ picked.length }} to ignored paths
          </BaseButton>
          <BaseButton small variant="ghost" @click="report = null">Dismiss</BaseButton>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.list {
  max-height: 200px;
  overflow: auto;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.examples {
  display: block;
  color: var(--fg-muted);
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 560px;
}
</style>
