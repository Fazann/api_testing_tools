<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';

import BaseButton from '@/components/base/BaseButton.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import ModalFrame from '@/components/base/ModalFrame.vue';
import SelectField from '@/components/base/SelectField.vue';
import TextField from '@/components/base/TextField.vue';
import { useAsyncAction } from '@/composables/useAsyncAction';
import { api } from '@/services/api';
import { useEnvironments } from '@/stores/environments';
import { useUi } from '@/stores/ui';
import type { Collection, SignoffResult } from '@/types';

// Release sign-off report: release details, then a preview to save as HTML or print to PDF.
// See docs/signoff-report.md.
const props = defineProps<{ collection: Collection }>();
const emit = defineEmits<{ close: [] }>();
const envs = useEnvironments();
const ui = useUi();
const { busy, run } = useAsyncAction();

const release = ref('');
const environmentId = ref(envs.selectedId);
const preparedBy = ref('');
const notes = ref('');
const result = ref<SignoffResult | null>(null);
const frame = ref<HTMLIFrameElement | null>(null);

onMounted(async () => {
  preparedBy.value = (await api.getSettings().catch(() => null))?.reviewerName ?? '';
});

const envOptions = computed(() => [
  { value: '', label: 'No environment' },
  ...envs.list.map((e) => ({ value: e.id, label: e.name })),
]);

async function preview() {
  if (!release.value.trim()) return ui.toast('Enter the release being signed off, for example v2.3.0', 'error');
  const res = await run(() =>
    api.buildSignoffReport({
      collectionId: props.collection.id,
      environmentId: environmentId.value ?? '',
      release: release.value,
      preparedBy: preparedBy.value,
      notes: notes.value,
    }),
  );
  if (res) result.value = res;
}

async function save() {
  const r = result.value;
  if (!r) return;
  const path = await run(() => api.saveSignoffReport(r.fileName, r.html));
  if (path) ui.toast(`Report saved to ${path}`, 'success');
}

/** Opens the system print dialog for the preview; "Save as PDF" / "Print to File" produces the PDF. */
function print() {
  const win = frame.value?.contentWindow;
  if (!win) return ui.toast('The preview is not ready yet', 'error');
  win.focus();
  win.print();
}
</script>

<template>
  <ModalFrame :title="`Sign-off report — ${collection.name}`" :wide="!result" :xwide="!!result" @close="emit('close')">
    <template v-if="!result">
      <p class="muted small">
        A summary for QA hand-off and audits: verdict, newest run, regressions, security checks, performance, baselines
        and who approved them, and coverage. It is built from stored results; nothing is sent.
      </p>
      <TextField v-model="release" label="Release" placeholder="v2.3.0" maxlength="200" />
      <SelectField v-model="environmentId" label="Environment the release was tested in" :options="envOptions" />
      <TextField v-model="preparedBy" label="Prepared by" placeholder="Your name" maxlength="200" />
      <TextField v-model="notes" label="Notes for the approver (optional)" multiline rows="3" />
    </template>
    <template v-else>
      <CalloutBox :tone="result.ready ? 'info' : 'error'" small>
        <strong>{{ result.ready ? '✔ Ready for release' : '✘ Not ready' }}</strong>
        <template v-if="!result.ready"> — {{ result.reasons.join('; ') }}</template>
      </CalloutBox>
      <iframe
        ref="frame"
        class="preview"
        title="Sign-off report preview"
        sandbox="allow-same-origin allow-modals"
        :srcdoc="result.html"
      />
    </template>
    <template #footer>
      <template v-if="!result">
        <BaseButton @click="emit('close')">Cancel</BaseButton>
        <BaseButton variant="primary" :disabled="busy" @click="preview">Preview</BaseButton>
      </template>
      <template v-else>
        <BaseButton @click="result = null">Back</BaseButton>
        <BaseButton :disabled="busy" @click="save">Save HTML…</BaseButton>
        <BaseButton variant="primary" @click="print">Print / Save as PDF</BaseButton>
      </template>
    </template>
  </ModalFrame>
</template>

<style scoped>
.preview {
  width: 100%;
  height: 62vh;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: #fff;
}
</style>
