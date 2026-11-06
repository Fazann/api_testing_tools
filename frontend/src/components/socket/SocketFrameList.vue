<script setup lang="ts">
import { computed, ref } from 'vue';

import BaseInput from '@/components/base/BaseInput.vue';
import DataTable, { type Column } from '@/components/base/DataTable.vue';
import FilterChip from '@/components/base/FilterChip.vue';
import { prettyFrameData } from '@/services/socket';
import type { RequestKind, SocketFrame } from '@/types';
import { formatDuration } from '@/utils/format';

// The messages of a socket connection: the live console log, or the transcript of a test run.
const props = defineProps<{ frames: readonly SocketFrame[]; kind: RequestKind; empty?: string }>();

type Direction = SocketFrame['direction'] | 'all';
const direction = ref<Direction>('all');
const query = ref('');
const expanded = ref<number | null>(null);

const indexed = computed(() => props.frames.map((frame, index) => ({ frame, index })));
const counts = computed(() => {
  const c: Record<Direction, number> = { all: props.frames.length, sent: 0, received: 0, system: 0 };
  for (const f of props.frames) c[f.direction]++;
  return c;
});
const visible = computed(() => {
  const q = query.value.trim().toLowerCase();
  return indexed.value.filter(
    ({ frame: f }) =>
      (direction.value === 'all' || f.direction === direction.value) &&
      (!q || f.data.toLowerCase().includes(q) || (f.event ?? '').toLowerCase().includes(q)),
  );
});
const columns = computed<Column[]>(() => [
  { key: 'at', label: 'Time', class: 'num time' },
  { key: 'direction', label: '', class: 'dir' },
  ...(props.kind === 'socketio' ? [{ key: 'event', label: 'Event', class: 'mono event' }] : []),
  { key: 'data', label: 'Data', class: 'mono data' },
]);
const FILTERS: { id: Direction; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'sent', label: '↑ Sent' },
  { id: 'received', label: '↓ Received' },
  { id: 'system', label: 'Connection' },
];
const ARROW: Record<SocketFrame['direction'], string> = { sent: '↑', received: '↓', system: '·' };

function toggle(index: number) {
  expanded.value = expanded.value === index ? null : index;
}
</script>

<template>
  <div class="stack frames">
    <div class="row filters" role="group" aria-label="Filter messages">
      <FilterChip
        v-for="f in FILTERS"
        :key="f.id"
        :active="direction === f.id"
        :count="counts[f.id]"
        @click="direction = f.id"
        >{{ f.label }}</FilterChip
      >
      <BaseInput v-model="query" class="grow" placeholder="Search messages…" aria-label="Search messages" />
    </div>
    <DataTable
      :columns="columns"
      :rows="visible"
      :row-key="(r) => r.index"
      :row-class="(r) => [r.frame.direction, { open: expanded === r.index }]"
      :empty="empty ?? 'No messages yet.'"
      clickable
      @row-click="(r) => toggle(r.index)"
    >
      <template #cell-at="{ row }">{{ formatDuration(row.frame.atMs) }}</template>
      <template #cell-direction="{ row }">
        <span :class="row.frame.direction" :title="row.frame.direction">{{ ARROW[row.frame.direction] }}</span>
      </template>
      <template #cell-event="{ row }">
        {{ row.frame.event }}<span v-if="row.frame.ack" class="muted small"> (ack)</span>
      </template>
      <template #cell-data="{ row }">
        <span class="clip">{{ row.frame.data || '(empty)' }}</span>
        <span v-if="row.frame.binary" class="muted small"> binary, base64</span>
        <span v-if="row.frame.truncated" class="warn small"> truncated</span>
      </template>
      <template #after-row="{ row }">
        <tr v-if="expanded === row.index" class="detail">
          <td :colspan="columns.length">
            <pre class="code">{{ prettyFrameData(row.frame) }}</pre>
          </td>
        </tr>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.filters {
  flex-wrap: wrap;
}
.frames :deep(.time) {
  width: 70px;
  white-space: nowrap;
}
.frames :deep(.dir) {
  width: 24px;
  text-align: center;
}
.frames :deep(.event) {
  width: 140px;
}
.frames :deep(.data) {
  max-width: 0;
}
.clip {
  display: inline-block;
  max-width: calc(100% - 4px);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}
.sent {
  color: var(--accent);
}
.received {
  color: var(--pass);
}
.frames :deep(tr.system) {
  color: var(--fg-muted);
  font-style: italic;
}
.detail pre {
  margin: 0;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
