<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';

import { api, onMatrixProgress } from '../services/api';
import { clone, isDestructive, uid } from '../services/model';
import { useEnvironments } from '../stores/environments';
import { useUi } from '../stores/ui';
import type { Access, CollectionTree, MatrixCell, MatrixReport, PermissionMatrix, Request, Role } from '../types';
import VariableTable from './VariableTable.vue';

const props = defineProps<{ tree: CollectionTree }>();
const emit = defineEmits<{ dirty: [boolean]; open: [Request] }>();
const ui = useUi();
const envs = useEnvironments();

const matrix = ref<PermissionMatrix | null>(null);
const snapshot = ref('');
const report = ref<MatrixReport | null>(null);
const openRole = ref<string | null>(null);
const opId = ref('');
const progress = ref({ done: 0, total: 0 });
const saving = ref(false);
let unsubscribe: (() => void) | null = null;

const dirty = computed(() => !!matrix.value && JSON.stringify(matrix.value) !== snapshot.value);
watch(dirty, (d) => emit('dirty', d));

async function load() {
  try {
    const m = await api.getPermissionMatrix(props.tree.collection.id);
    m.roles ??= [];
    m.requestIds ??= [];
    m.expected ??= {};
    matrix.value = m;
    snapshot.value = JSON.stringify(m);
  } catch (e) {
    ui.error(e);
  }
}
onMounted(load);
onBeforeUnmount(() => unsubscribe?.());

const selected = computed(() => new Set(matrix.value?.requestIds ?? []));
const chosen = computed(() => props.tree.requests.filter((r) => selected.value.has(r.id)));
const destructive = computed(() => chosen.value.filter((r) => isDestructive(r.method)));

function toggleRequest(id: string) {
  const m = matrix.value!;
  m.requestIds = selected.value.has(id) ? m.requestIds.filter((x) => x !== id) : [...m.requestIds, id];
}
function selectWhere(pred: (r: Request) => boolean) {
  matrix.value!.requestIds = props.tree.requests.filter(pred).map((r) => r.id);
}

// ---- roles ----

function addRole(anonymous = false) {
  const m = matrix.value!;
  const taken = new Set(m.roles.map((r) => r.name.toLowerCase()));
  let name = anonymous ? 'Anonymous' : `Role ${m.roles.length + 1}`;
  for (let i = 2; taken.has(name.toLowerCase()); i++) name = `${anonymous ? 'Anonymous' : 'Role'} ${i}`;
  // No variable rows to start with: a new role behaves like the environment until you add an override.
  // (A pre-filled empty `token` row would silently replace the token with nothing.)
  const role: Role = { id: uid(), name, anonymous, variables: [] };
  m.roles.push(role);
  if (!anonymous) openRole.value = role.id;
}
async function removeRole(role: Role) {
  const ok = await ui.confirm({
    title: `Remove role "${role.name}"?`,
    message: 'Its expectations are removed too.',
    confirmLabel: 'Remove',
    danger: true,
  });
  if (!ok) return;
  const m = matrix.value!;
  m.roles = m.roles.filter((r) => r.id !== role.id);
  for (const cells of Object.values(m.expected)) delete cells[role.id];
}
const hasAnonymous = computed(() => matrix.value?.roles.some((r) => r.anonymous));

// ---- expectations ----

function expected(reqId: string, roleId: string): Access {
  return matrix.value?.expected[reqId]?.[roleId] ?? '';
}
function setExpected(reqId: string, roleId: string, a: Access) {
  const m = matrix.value!;
  const cells = (m.expected[reqId] ??= {});
  if (a) cells[roleId] = a;
  else delete cells[roleId];
  if (Object.keys(cells).length === 0) delete m.expected[reqId];
}
const NEXT: Record<Access, Access> = { '': 'allow', allow: 'deny', deny: '' };
function cycle(reqId: string, roleId: string) {
  setExpected(reqId, roleId, NEXT[expected(reqId, roleId)]);
}

const cells = computed(() => {
  const map = new Map<string, MatrixCell>();
  for (const c of report.value?.cells ?? []) map.set(`${c.requestId}/${c.roleId}`, c);
  return map;
});
function cell(reqId: string, roleId: string): MatrixCell | undefined {
  return cells.value.get(`${reqId}/${roleId}`);
}
/** Verdict against the current (possibly edited) expectation, so editing updates the colours at once. */
function liveVerdict(c: MatrixCell, exp: Access): MatrixCell['verdict'] {
  if (c.verdict === 'error') return 'error';
  if (!exp) return 'unset';
  if (exp === c.expected) return c.verdict;
  if (c.suggested === exp) return 'pass';
  if (exp === 'deny' && c.outcome === 'rejected') return 'warn';
  return 'fail';
}

const approvable = computed(
  () => report.value?.cells.filter((c) => c.suggested && expected(c.requestId, c.roleId) !== c.suggested).length ?? 0,
);
async function approveResults() {
  const r = report.value;
  if (!r) return;
  const ok = await ui.confirm({
    title: 'Approve the observed access as expected?',
    message:
      `${approvable.value} cell${approvable.value === 1 ? '' : 's'} will be set from the last run: 2xx/3xx → allow, ` +
      '401/403/404 → deny. Other results (400, 5xx, no response) keep their expectation. ' +
      'Only do this after checking that every "allowed" really should be allowed.',
    confirmLabel: 'Approve',
  });
  if (!ok) return;
  for (const c of r.cells) if (c.suggested) setExpected(c.requestId, c.roleId, c.suggested);
  await save();
}

// ---- save and run ----

async function save(): Promise<boolean> {
  const m = matrix.value;
  if (!m) return false;
  for (const role of m.roles) role.variables = role.variables.filter((v) => v.key.trim() || v.value);
  saving.value = true;
  try {
    const savedMatrix = await api.savePermissionMatrix(clone(m));
    savedMatrix.expected ??= {};
    matrix.value = savedMatrix;
    snapshot.value = JSON.stringify(savedMatrix);
    return true;
  } catch (e) {
    ui.error(e);
    return false;
  } finally {
    saving.value = false;
  }
}

async function run() {
  const m = matrix.value;
  if (!m || opId.value) return;
  if (m.roles.length === 0) return ui.toast('Add at least one role first', 'error');
  if (chosen.value.length === 0) return ui.toast('Tick at least one request first', 'error');
  const env = envs.selected;
  const details: string[] = [];
  if (destructive.value.length)
    details.push(
      `${destructive.value.length} request(s) may modify data and run once per role: ` +
        destructive.value
          .slice(0, 5)
          .map((r) => `${r.method} ${r.name}`)
          .join(', '),
    );
  if (env?.protected) details.push(`"${env.name}" is a protected environment.`);
  const total = m.roles.length * chosen.value.length;
  const ok = await ui.confirm({
    title: `Run ${total} requests (${chosen.value.length} × ${m.roles.length} roles)?`,
    message: `Every ticked request is sent once as each role${env ? ` against "${env.name}"` : ''}.`,
    details,
    confirmLabel: 'Run matrix',
    danger: destructive.value.length > 0 || !!env?.protected,
  });
  if (!ok) return;
  if (dirty.value && !(await save())) return;

  opId.value = uid();
  progress.value = { done: 0, total };
  unsubscribe?.();
  unsubscribe = onMatrixProgress((p) => {
    if (p.opId === opId.value) progress.value = { done: p.done, total: p.total };
  });
  try {
    report.value = await api.runPermissionMatrix(opId.value, props.tree.collection.id, envs.selectedId, true);
  } catch (e) {
    ui.error(e);
  } finally {
    opId.value = '';
    unsubscribe?.();
    unsubscribe = null;
  }
}

function cancel() {
  if (opId.value) api.cancel(opId.value).catch(ui.error);
}

const SYMBOL: Record<Access, string> = { '': '·', allow: '✓', deny: '✕' };
const VERDICT_LABEL: Record<MatrixCell['verdict'], string> = {
  pass: 'as expected',
  fail: 'MISMATCH',
  warn: 'suspicious',
  unset: 'no expectation',
  error: 'no response',
};

function cellTitle(reqId: string, roleId: string): string {
  const exp = expected(reqId, roleId);
  const lines = [`Expected: ${exp || 'not set'} (click to change)`];
  const c = cell(reqId, roleId);
  if (c) {
    lines.push(`Last run: ${c.statusCode || c.outcome} → ${VERDICT_LABEL[liveVerdict(c, exp)]}`);
    if (c.message) lines.push(c.message);
  }
  return lines.join('\n');
}

const problems = computed(() => {
  const r = report.value;
  if (!r) return [];
  const names = new Map(props.tree.requests.map((q) => [q.id, q]));
  const roles = new Map(r.roles.map((x) => [x.id, x.name]));
  return r.cells
    .map((c) => ({ c, v: liveVerdict(c, expected(c.requestId, c.roleId)) }))
    .filter(({ v }) => v === 'fail' || v === 'warn' || v === 'error')
    .map(({ c, v }) => ({ c, v, req: names.get(c.requestId), role: roles.get(c.roleId) ?? '?' }));
});
/** Verdict counts against the current expectations, so approving or editing updates them at once. */
const counts = computed(() => {
  const n = { pass: 0, fail: 0, warn: 0, unset: 0, error: 0 };
  for (const c of report.value?.cells ?? []) n[liveVerdict(c, expected(c.requestId, c.roleId))]++;
  return n;
});
</script>

<template>
  <div v-if="!matrix" class="muted small">Loading…</div>
  <div v-else class="matrix stack">
    <p class="muted small intro">
      Run endpoints as every role and see who can call what. Roles switch identity by overriding variables — if the
      collection's auth is Bearer <code v-pre>{{ token }}</code
      >, a role sets <code>token</code> to <code v-pre>{{ bob_token }}</code
      >. Set the expected access per cell (click to cycle ✓ allow / ✕ deny / · none), or run first and approve what you
      see. A <strong>✓ that should be ✕</strong> is broken access control; for IDOR, put another user's resource ID in
      the URL and expect ✕ for everyone but its owner.
    </p>

    <h3 class="section-title">Roles</h3>
    <div class="roles stack">
      <p v-if="matrix.roles.length === 0" class="muted small">No roles yet.</p>
      <div v-for="role in matrix.roles" :key="role.id" class="role">
        <div class="row">
          <input v-model="role.name" class="name" maxlength="200" :aria-label="`Role name ${role.name}`" />
          <label class="row small"><input v-model="role.anonymous" type="checkbox" /> No credentials</label>
          <button
            v-if="!role.anonymous"
            class="ghost small"
            :aria-expanded="openRole === role.id"
            @click="openRole = openRole === role.id ? null : role.id"
          >
            {{ openRole === role.id ? '▾' : '▸' }} Variables
            <span class="muted">({{ role.variables.filter((v) => v.enabled && v.key).length }})</span>
          </button>
          <span v-else class="muted small">Sent without auth, Authorization or Cookie headers</span>
          <span class="spacer" />
          <button class="ghost" :aria-label="`Remove role ${role.name}`" @click="removeRole(role)">✕</button>
        </div>
        <div v-if="openRole === role.id && !role.anonymous" class="vars">
          <p v-if="role.variables.length === 0" class="muted small">
            No overrides: this role uses the environment's variables as they are. Add a variable such as
            <code>token</code> = <code v-pre>{{ bob_token }}</code> to switch identity.
          </p>
          <VariableTable :variables="role.variables" />
          <p class="muted small">
            These override environment and collection variables for this role only. Prefer referencing a secret
            environment variable (<code v-pre>{{ bob_token }}</code
            >) over pasting a token here.
          </p>
        </div>
      </div>
      <div class="row">
        <button class="small" @click="addRole()">＋ Role</button>
        <button v-if="!hasAnonymous" class="small" @click="addRole(true)">＋ Anonymous</button>
      </div>
    </div>

    <div class="row toolbar">
      <h3 class="section-title grow">Requests × roles</h3>
      <span class="muted small">Tick:</span>
      <button class="ghost small" @click="selectWhere((r) => r.method === 'GET' || r.method === 'HEAD')">
        Read-only
      </button>
      <button class="ghost small" @click="selectWhere(() => true)">All</button>
      <button class="ghost small" @click="selectWhere(() => false)">None</button>
    </div>
    <p v-if="destructive.length" class="callout small">
      {{ destructive.length }} ticked request{{ destructive.length === 1 ? '' : 's' }} may modify data and will run once
      per role. Use test data.
    </p>

    <div class="table-wrap">
      <table class="grid cells">
        <thead>
          <tr>
            <th class="pick" />
            <th class="req">Request</th>
            <th v-for="role in matrix.roles" :key="role.id" class="role-col">{{ role.name }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in tree.requests" :key="r.id" :class="{ off: !selected.has(r.id) }">
            <td class="pick">
              <input
                type="checkbox"
                :checked="selected.has(r.id)"
                :aria-label="`Include ${r.name}`"
                @change="toggleRequest(r.id)"
              />
            </td>
            <td class="req">
              <span class="method" :class="r.method">{{ r.method }}</span>
              <button class="link" :title="r.url" @click="emit('open', r)">{{ r.name }}</button>
            </td>
            <td v-for="role in matrix.roles" :key="role.id" class="cell-td">
              <button
                class="cell"
                :class="[
                  `exp-${expected(r.id, role.id) || 'none'}`,
                  cell(r.id, role.id) ? `v-${liveVerdict(cell(r.id, role.id)!, expected(r.id, role.id))}` : '',
                ]"
                :disabled="!selected.has(r.id)"
                :title="cellTitle(r.id, role.id)"
                :aria-label="`${r.name} as ${role.name}: ${cellTitle(r.id, role.id)}`"
                @click="cycle(r.id, role.id)"
              >
                <span class="sym">{{ SYMBOL[expected(r.id, role.id)] }}</span>
                <span v-if="cell(r.id, role.id)" class="code">{{ cell(r.id, role.id)!.statusCode || '—' }}</span>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="tree.requests.length === 0" class="muted small">This collection has no requests.</p>
    </div>

    <div class="row actions">
      <button v-if="opId" class="danger" @click="cancel">Cancel</button>
      <button v-else class="primary" :disabled="saving" @click="run">Run matrix</button>
      <button :disabled="!dirty || saving || !!opId" @click="save">Save</button>
      <button v-if="report && approvable > 0 && !opId" @click="approveResults">
        Approve results as expected ({{ approvable }})
      </button>
      <span v-if="opId" class="muted small">{{ progress.done }} / {{ progress.total }}</span>
      <span v-else-if="dirty" class="muted small">Unsaved changes</span>
    </div>

    <section v-if="report" class="summary stack">
      <div class="row">
        <strong :class="report.cancelled ? 'warn' : counts.fail === 0 && counts.error === 0 ? 'pass' : 'fail'">
          {{
            report.cancelled
              ? 'Cancelled'
              : counts.fail === 0 && counts.error === 0
                ? '✔ Access matches the expectations'
                : counts.fail === 0
                  ? `✘ ${counts.error} request${counts.error === 1 ? '' : 's'} got no response`
                  : `✘ ${counts.fail} access mismatch${counts.fail === 1 ? '' : 'es'}`
          }}
        </strong>
        <span class="muted small">
          {{ counts.pass }} as expected · {{ counts.warn }} suspicious · {{ counts.unset }} without expectation
          <template v-if="counts.error"> · {{ counts.error }} without response</template>
        </span>
      </div>
      <ul v-if="problems.length" class="problems">
        <li v-for="p in problems" :key="p.c.resultId" :class="p.v === 'warn' ? 'warn' : 'fail'">
          <strong>{{ p.req?.name ?? '?' }}</strong> as <strong>{{ p.role }}</strong
          >: {{ p.c.message || `${p.c.statusCode || p.c.outcome}` }}
        </li>
      </ul>
      <p class="muted small">The run is saved in History with every request and response.</p>
    </section>
  </div>
</template>

<style scoped>
.intro {
  margin: 0;
}
.role {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  padding: 6px 8px;
}
.role .name {
  width: 200px;
  font-weight: 600;
}
.vars {
  margin-top: 6px;
}
.toolbar .section-title {
  margin: 8px 0 4px;
}
.table-wrap {
  overflow-x: auto;
  max-height: 46vh;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: var(--radius);
}
.cells th {
  position: sticky;
  top: 0;
  background: var(--bg);
  z-index: 1;
}
.cells .pick {
  width: 28px;
}
.cells .req {
  min-width: 220px;
}
.cells .req .method {
  display: inline-block;
  width: 56px;
}
.role-col {
  text-align: center !important;
  min-width: 90px;
  white-space: nowrap;
}
.cell-td {
  text-align: center !important;
  padding: 3px 6px !important;
}
tr.off td {
  opacity: 0.45;
}
button.link {
  border: none;
  background: none;
  padding: 0;
  color: var(--fg);
  cursor: pointer;
  text-align: left;
}
button.link:hover {
  text-decoration: underline;
}
.cell {
  width: 100%;
  min-width: 70px;
  display: inline-flex;
  justify-content: center;
  gap: 6px;
  font-family: var(--mono);
  padding: 3px 6px;
  border: 1px solid var(--line);
  background: var(--bg);
}
.cell .sym {
  font-weight: 700;
}
.cell.exp-allow .sym {
  color: var(--pass);
}
.cell.exp-deny .sym {
  color: var(--fail);
}
.cell.exp-none .sym {
  color: var(--fg-muted);
}
.cell.v-pass {
  background: var(--add-bg);
}
.cell.v-fail {
  background: var(--rem-bg);
  border-color: var(--fail);
  border-width: 2px;
}
.cell.v-warn {
  background: var(--chg-bg);
  border-color: var(--warn);
}
.cell.v-error {
  border-style: dashed;
  border-color: var(--warn);
}
.cell .code {
  color: var(--fg-muted);
}
.cell.v-fail .code {
  color: var(--fail);
  font-weight: 700;
}
.summary {
  border-top: 1px solid var(--line);
  padding-top: 8px;
}
.problems {
  margin: 0;
  padding-left: 18px;
  font-size: 13px;
}
.problems li {
  margin: 2px 0;
}
</style>
