<script setup lang="ts">
import { computed, ref } from 'vue';

import { apiVersion, compareVersions } from '../services/model';
import { useWorkspace } from '../stores/workspace';
import type { CollectionTree, Method, Request } from '../types';

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
</script>

<template>
  <div class="stack endpoints">
    <div v-if="tree.requests.length === 0" class="callout info">
      This collection has no requests yet. Use <strong>⋯ → New request</strong> in the sidebar.
    </div>
    <template v-else>
      <p class="small muted">
        {{ tree.requests.length }} endpoint{{ tree.requests.length === 1 ? '' : 's' }} in
        {{ tree.folders.length }} folder{{ tree.folders.length === 1 ? '' : 's'
        }}<template v-if="withoutTests">, {{ withoutTests }} without tests or a post-response script</template>.
      </p>
      <div class="row filters" role="group" aria-label="Filter by method">
        <button class="small chip" :class="{ on: method === '' }" :aria-pressed="method === ''" @click="method = ''">
          All <span class="count">{{ tree.requests.length }}</span>
        </button>
        <button
          v-for="m in presentMethods"
          :key="m"
          class="small chip"
          :class="{ on: method === m }"
          :aria-pressed="method === m"
          @click="method = method === m ? '' : m"
        >
          <span class="method" :class="m">{{ m }}</span> <span class="count">{{ counts[m] }}</span>
        </button>
        <input v-model="query" class="grow search" placeholder="Search name, URL or folder…" aria-label="Search" />
      </div>
      <div v-if="presentVersions.length" class="row filters versions" role="group" aria-label="Filter by API version">
        <span class="muted small">Version</span>
        <button class="small chip" :class="{ on: version === '' }" :aria-pressed="version === ''" @click="version = ''">
          All
        </button>
        <button
          v-for="v in presentVersions"
          :key="v"
          class="small chip"
          :class="{ on: version === v }"
          :aria-pressed="version === v"
          @click="version = version === v ? '' : v"
        >
          <span class="mono">{{ v }}</span> <span class="count">{{ versionCounts[v] }}</span>
        </button>
        <button
          v-if="versionCounts[NO_VERSION]"
          class="small chip"
          :class="{ on: version === NO_VERSION }"
          :aria-pressed="version === NO_VERSION"
          title="Requests whose URL has no /v1/-style segment or version query parameter"
          @click="version = version === NO_VERSION ? '' : NO_VERSION"
        >
          No version <span class="count">{{ versionCounts[NO_VERSION] }}</span>
        </button>
      </div>
      <table class="grid list">
        <thead>
          <tr>
            <th>Method</th>
            <th>Name</th>
            <th v-if="presentVersions.length">Version</th>
            <th>URL</th>
            <th>Folder</th>
            <th class="num">Tests</th>
            <th>Last run</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="r in visible"
            :key="r.id"
            tabindex="0"
            :class="{ active: ws.draft?.id === r.id }"
            :title="`Open ${r.name}`"
            @click="emit('open', r)"
            @keydown.enter.prevent="emit('open', r)"
          >
            <td>
              <span class="method" :class="r.method">{{ r.method }}</span>
            </td>
            <td class="name">{{ r.name }}</td>
            <td v-if="presentVersions.length" class="mono">{{ versions[r.id] || '—' }}</td>
            <td class="mono url">{{ r.url }}</td>
            <td class="muted">{{ folderPaths[r.parentId] ?? '' }}</td>
            <td class="num">{{ tests(r) || '—' }}</td>
            <td>
              <span v-if="ws.results[r.id]" class="status" :class="ws.results[r.id].status">{{
                ws.results[r.id].status
              }}</span>
              <span v-else class="muted">—</span>
            </td>
          </tr>
          <tr v-if="visible.length === 0" class="none">
            <td :colspan="presentVersions.length ? 7 : 6" class="muted small">No endpoints match this filter.</td>
          </tr>
        </tbody>
      </table>
    </template>
  </div>
</template>

<style scoped>
.filters {
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.chip.on {
  border-color: var(--accent);
  background: var(--bg-muted);
}
.count {
  color: var(--fg-muted);
  font-size: 11px;
}
.search {
  min-width: 180px;
}
.list tbody tr:not(.none) {
  cursor: pointer;
}
.list tbody tr:not(.none):hover,
.list tbody tr:focus-visible,
.list tbody tr.active {
  background: var(--bg-muted);
}
.name {
  font-weight: 600;
}
.url {
  word-break: break-all;
}
.num {
  text-align: right;
}
.status.passed {
  color: var(--pass);
}
.status.failed,
.status.error {
  color: var(--fail);
}
</style>
