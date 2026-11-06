<script setup lang="ts">
import { computed, ref } from 'vue';

import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import DataTable from '@/components/base/DataTable.vue';
import FilterChip from '@/components/base/FilterChip.vue';
import MethodBadge from '@/components/base/MethodBadge.vue';
import { apiVersion, compareVersions } from '@/services/model';
import { useWorkspace } from '@/stores/workspace';
import type { CollectionTree, Method, Request } from '@/types';

const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ open: [request: Request] }>();
const ws = useWorkspace();

const METHODS: Method[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'];

const method = ref<Method | ''>('');
/** '' = all versions, NO_VERSION = requests without a detected version. */
const version = ref('');
const NO_VERSION = '\u0000none';
const query = ref('');

/** Version per request id, with {{placeholders}} filled from non-secret collection variables. */
const versions = computed(() => {
  const vars: Record<string, string> = {};
  for (const v of props.tree.collection.variables ?? []) if (v.enabled && !v.secret) vars[v.key] = v.value;
  const out: Record<string, string> = {};
  for (const r of props.tree.requests) out[r.id] = apiVersion(r.url, r.query ?? [], vars);
  return out;
});
const versionCounts = computed(() => {
  const c: Record<string, number> = {};
  for (const r of props.tree.requests) {
    const v = versions.value[r.id] || NO_VERSION;
    c[v] = (c[v] ?? 0) + 1;
  }
  return c;
});
const presentVersions = computed(() =>
  Object.keys(versionCounts.value)
    .filter((v) => v !== NO_VERSION)
    .sort(compareVersions),
);

/** Folder path such as "Wallet / Admin" for each folder id. */
const folderPaths = computed(() => {
  const byId = new Map(props.tree.folders.map((f) => [f.id, f]));
  const paths: Record<string, string> = {};
  for (const f of props.tree.folders) {
    const names: string[] = [];
    const seen = new Set<string>();
    for (let cur = byId.get(f.id); cur && !seen.has(cur.id); cur = byId.get(cur.parentId)) {
      seen.add(cur.id);
      names.unshift(cur.name);
    }
    paths[f.id] = names.join(' / ');
  }
  return paths;
});

/** Requests in sidebar order: folders first (depth-first), then each level's requests. */
const ordered = computed(() => {
  const out: Request[] = [];
  const walk = (parentId: string, seen: Set<string>) => {
    for (const f of props.tree.folders.filter((x) => x.parentId === parentId)) {
      if (seen.has(f.id)) continue;
      seen.add(f.id);
      walk(f.id, seen);
    }
    out.push(...props.tree.requests.filter((r) => r.parentId === parentId));
  };
  walk('', new Set());
  // Requests whose folder no longer exists still belong to the collection.
  const listed = new Set(out.map((r) => r.id));
  out.push(...props.tree.requests.filter((r) => !listed.has(r.id)));
  return out;
});

const counts = computed(() => {
  const c: Partial<Record<Method, number>> = {};
  for (const r of props.tree.requests) c[r.method] = (c[r.method] ?? 0) + 1;
  return c;
});
const presentMethods = computed(() => METHODS.filter((m) => counts.value[m]));

const visible = computed(() => {
  const q = query.value.trim().toLowerCase();
  return ordered.value.filter(
    (r) =>
      (!method.value || r.method === method.value) &&
      (!version.value || (versions.value[r.id] || NO_VERSION) === version.value) &&
      (!q ||
        r.name.toLowerCase().includes(q) ||
        r.url.toLowerCase().includes(q) ||
        versions.value[r.id].includes(q) ||
        (folderPaths.value[r.parentId] ?? '').toLowerCase().includes(q)),
  );
});

const withoutTests = computed(
  () => props.tree.requests.filter((r) => !r.assertions.some((a) => a.enabled) && !r.postResponseScript.trim()).length,
);

function tests(r: Request) {
  return r.assertions.filter((a) => a.enabled).length;
}

const columns = computed(() => [
  { key: 'method', label: 'Method' },
  { key: 'name', label: 'Name', class: 'name' },
  ...(presentVersions.value.length ? [{ key: 'version', label: 'Version', class: 'mono' }] : []),
  { key: 'url', label: 'URL', class: 'mono url' },
  { key: 'folder', label: 'Folder', class: 'muted' },
  { key: 'tests', label: 'Tests', class: 'num' },
  { key: 'lastRun', label: 'Last run' },
]);
</script>

<template>
  <div class="stack endpoints">
    <CalloutBox v-if="tree.requests.length === 0" tone="info">
      This collection has no requests yet. Use <strong>⋯ → New request</strong> in the sidebar.
    </CalloutBox>
    <template v-else>
      <p class="small muted">
        {{ tree.requests.length }} endpoint{{ tree.requests.length === 1 ? '' : 's' }} in
        {{ tree.folders.length }} folder{{ tree.folders.length === 1 ? '' : 's'
        }}<template v-if="withoutTests">, {{ withoutTests }} without tests or a post-response script</template>.
      </p>
      <div class="row filters" role="group" aria-label="Filter by method">
        <FilterChip :active="method === ''" :count="tree.requests.length" @click="method = ''">All</FilterChip>
        <FilterChip
          v-for="m in presentMethods"
          :key="m"
          :active="method === m"
          :count="counts[m]"
          @click="method = method === m ? '' : m"
        >
          <MethodBadge :method="m" />
        </FilterChip>
        <BaseInput v-model="query" class="grow search" placeholder="Search name, URL or folder…" aria-label="Search" />
      </div>
      <div v-if="presentVersions.length" class="row filters versions" role="group" aria-label="Filter by API version">
        <span class="muted small">Version</span>
        <FilterChip :active="version === ''" @click="version = ''">All</FilterChip>
        <FilterChip
          v-for="v in presentVersions"
          :key="v"
          :active="version === v"
          :count="versionCounts[v]"
          @click="version = version === v ? '' : v"
        >
          <span class="mono">{{ v }}</span>
        </FilterChip>
        <FilterChip
          v-if="versionCounts[NO_VERSION]"
          :active="version === NO_VERSION"
          :count="versionCounts[NO_VERSION]"
          title="Requests whose URL has no /v1/-style segment or version query parameter"
          @click="version = version === NO_VERSION ? '' : NO_VERSION"
        >
          No version
        </FilterChip>
      </div>
      <DataTable
        :columns="columns"
        :rows="visible"
        :row-key="(r) => r.id"
        :row-class="(r) => ({ active: ws.draft?.id === r.id })"
        :row-title="(r) => `Open ${r.name}`"
        clickable
        empty="No endpoints match this filter."
        @row-click="(r) => emit('open', r)"
      >
        <template #cell-method="{ row }"><MethodBadge :method="row.method" /></template>
        <template #cell-version="{ row }">{{ versions[row.id] || '—' }}</template>
        <template #cell-folder="{ row }">{{ folderPaths[row.parentId] ?? '' }}</template>
        <template #cell-tests="{ row }">{{ tests(row) || '—' }}</template>
        <template #cell-lastRun="{ row }">
          <span v-if="ws.results[row.id]" class="status" :class="ws.results[row.id].status">{{
            ws.results[row.id].status
          }}</span>
          <span v-else class="muted">—</span>
        </template>
      </DataTable>
    </template>
  </div>
</template>

<style scoped>
.filters {
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.search {
  min-width: 180px;
}
:deep(tr.active td) {
  background: var(--bg-muted);
}
:deep(.name) {
  font-weight: 600;
}
:deep(.url) {
  word-break: break-all;
}
.status.passed {
  color: var(--pass);
}
.status.failed,
.status.error {
  color: var(--fail);
}
</style>
