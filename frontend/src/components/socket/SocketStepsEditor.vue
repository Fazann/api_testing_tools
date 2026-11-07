<script setup lang="ts">
import BaseButton from '@/components/base/BaseButton.vue';
import BaseInput from '@/components/base/BaseInput.vue';
import CalloutBox from '@/components/base/CalloutBox.vue';
import CheckboxField from '@/components/base/CheckboxField.vue';
import SelectField from '@/components/base/SelectField.vue';
import TextField from '@/components/base/TextField.vue';
import AssertionEditor from '@/components/request/AssertionEditor.vue';
import {
  MAX_HEARTBEAT_MS,
  MAX_WAIT_MS,
  MIN_HEARTBEAT_MS,
  newSocketStep,
  SOCKET_ASSERTION_KINDS,
  STEP_KINDS,
  stepSummary,
} from '@/services/socket';
import type { RequestKind, SocketConfig, SocketStepKind } from '@/types';
import { clone, uid } from '@/utils/object';

import SocketMessageFields from './SocketMessageFields.vue';

// The test of a socket request: steps that send messages, expect replies and wait, run in order on one connection.
const props = defineProps<{ socket: SocketConfig; kind: RequestKind }>();

const KIND_LABEL = Object.fromEntries(STEP_KINDS.map((k) => [k.value, k.label])) as Record<SocketStepKind, string>;

function add(kind: SocketStepKind) {
  props.socket.steps.push(newSocketStep(props.kind, kind));
}
function addSaved(id: string | undefined) {
  const m = props.socket.messages.find((x) => x.id === id);
  if (!m) return;
  const step = newSocketStep(props.kind, 'send');
  step.message = { ...clone(m), id: uid() };
  props.socket.steps.push(step);
}
function remove(i: number) {
  props.socket.steps.splice(i, 1);
}
function move(i: number, d: number) {
  const j = i + d;
  if (j < 0 || j >= props.socket.steps.length) return;
  const [s] = props.socket.steps.splice(i, 1);
  props.socket.steps.splice(j, 0, s);
}
</script>

<template>
  <div class="stack">
    <CalloutBox v-if="socket.steps.length === 0" tone="info">
      No test steps yet. A socket test connects, then runs its steps in order: <strong>send</strong> a message,
      <strong>expect</strong> a reply that passes some checks within a time limit, or <strong>wait</strong>. Replies may
      arrive in any order; each received message satisfies at most one expect step. For presence (online/offline) tests,
      <strong>start a heartbeat</strong> that repeats a message in the background, stop it, and
      <strong>expect the disconnect</strong>.
    </CalloutBox>
    <div v-for="(s, i) in socket.steps" :key="s.id" class="step" :class="[s.kind, { off: !s.enabled }]">
      <div class="row head">
        <CheckboxField v-model="s.enabled" :aria-label="`Enable step ${i + 1}`" />
        <span class="badge">{{ i + 1 }}. {{ KIND_LABEL[s.kind] }}</span>
        <BaseInput
          v-model="s.description"
          class="grow small"
          :placeholder="stepSummary(kind, s)"
          :aria-label="`Description of step ${i + 1}`"
        />
        <BaseButton variant="ghost" :aria-label="`Move step ${i + 1} up`" @click="move(i, -1)">↑</BaseButton>
        <BaseButton variant="ghost" :aria-label="`Move step ${i + 1} down`" @click="move(i, 1)">↓</BaseButton>
        <BaseButton variant="ghost" :aria-label="`Remove step ${i + 1}`" @click="remove(i)">✕</BaseButton>
      </div>
      <SocketMessageFields v-if="s.kind === 'send'" v-model="s.message" :kind="kind" :id-prefix="`step-${s.id}`" />
      <template v-else-if="s.kind === 'heartbeat_start'">
        <SocketMessageFields v-model="s.message" :kind="kind" :id-prefix="`step-${s.id}`" />
        <div class="row">
          <TextField
            v-model.number="s.intervalMs"
            class="small timeout"
            label="Interval (ms)"
            type="number"
            :min="MIN_HEARTBEAT_MS"
            :max="MAX_HEARTBEAT_MS"
          />
          <p class="muted small">
            Sent now, then on every interval in the background while the next steps run, until a Stop heartbeat step or
            the end of the test.
          </p>
        </div>
      </template>
      <p v-else-if="s.kind === 'heartbeat_stop'" class="muted small">
        Stops the heartbeat, for example to check that the server then marks the user offline.
      </p>
      <div v-else-if="s.kind === 'expect_close'" class="row">
        <TextField
          v-model="s.timeoutMs"
          class="small timeout"
          label="Within (ms)"
          type="number"
          min="1"
          :max="MAX_WAIT_MS"
        />
        <p class="muted small">Passes when the server closes the connection in time; later steps do not run.</p>
      </div>
      <template v-else-if="s.kind === 'expect'">
        <div class="row">
          <BaseInput
            v-if="kind === 'socketio'"
            v-model="s.event"
            class="mono event"
            placeholder="event (any)"
            :aria-label="`Event of step ${i + 1}`"
          />
          <CheckboxField
            v-if="kind === 'socketio'"
            v-model="s.expectAck"
            class="small"
            label="Acknowledgement of a sent event"
          />
          <TextField
            v-model="s.timeoutMs"
            class="small timeout"
            label="Within (ms)"
            type="number"
            min="1"
            :max="MAX_WAIT_MS"
          />
        </div>
        <AssertionEditor :assertions="s.assertions" :kinds="SOCKET_ASSERTION_KINDS" compact />
        <p class="muted small">
          Checks run against the message body{{
            kind === 'socketio' ? ' (the event argument, or an array of them)' : ''
          }}. Without checks, any {{ s.expectAck ? 'acknowledgement' : 'message' }} matches.
        </p>
      </template>
      <TextField
        v-else
        v-model="s.timeoutMs"
        class="small timeout"
        label="Pause (ms)"
        type="number"
        min="1"
        :max="MAX_WAIT_MS"
      />
    </div>
    <div class="row add">
      <span class="muted small">Add step:</span>
      <BaseButton v-for="k in STEP_KINDS" :key="k.value" @click="add(k.value)">{{ k.label }}</BaseButton>
      <SelectField
        v-if="socket.messages.length"
        :model-value="undefined"
        :options="socket.messages.map((m) => ({ value: m.id, label: `Send “${m.name}”` }))"
        aria-label="Add a send step from a saved message"
        @update:model-value="addSaved"
      />
    </div>
  </div>
</template>

<style scoped>
.step {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--line);
  border-left-width: 3px;
  border-radius: var(--radius);
  background: var(--bg-subtle);
}
.step.send {
  border-left-color: var(--accent);
}
.step.expect,
.step.expect_close {
  border-left-color: var(--pass);
}
.step.heartbeat_start,
.step.heartbeat_stop {
  border-left-color: var(--fail);
}
.step.off {
  opacity: 0.6;
}
.badge {
  font-weight: 600;
  white-space: nowrap;
}
.event {
  width: 200px;
}
.timeout :deep(input) {
  width: 120px;
}
</style>
