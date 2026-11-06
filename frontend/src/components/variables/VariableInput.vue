<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useAttrs, watch } from 'vue';

import { useCollectionVariables } from '@/services/collectionVariables';
import { describeVariable, splitVariables, type Segment } from '@/services/variables';
import { useEnvironments } from '@/stores/environments';

import VariablePopover from './VariablePopover.vue';

// A text input that highlights {{variables}} and shows their value on hover.
// The highlighted text is drawn by a mirror layer on top of the input whose
// plain text is transparent; only variable tokens receive mouse events.
defineOptions({ inheritAttrs: false });
const model = defineModel<string | undefined>({ default: '' });
const attrs = useAttrs();
const envs = useEnvironments();
const collectionVars = useCollectionVariables().variables;

const input = ref<HTMLInputElement | null>(null);
const mirror = ref<HTMLDivElement | null>(null);
const hovered = ref<{ name: string; x: number; y: number } | null>(null);
let hideTimer: ReturnType<typeof setTimeout> | undefined;

// Follows what is typed even when the parent only commits on "change".
const text = ref(model.value ?? '');
watch(model, (v) => (text.value = v ?? ''));
const segments = computed(() => splitVariables(text.value));
const hasTokens = computed(() => segments.value.some((s) => s.name));
const inputAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs;
  return rest;
});

function stateOf(seg: Segment) {
  return describeVariable(seg.name!, envs.selected, collectionVars.value).state;
}

function sync() {
  if (mirror.value && input.value) mirror.value.scrollLeft = input.value.scrollLeft;
}
watch(text, () => nextTick(sync));

function onInput(e: Event) {
  text.value = (e.target as HTMLInputElement).value;
}

function show(seg: Segment, e: MouseEvent) {
  clearTimeout(hideTimer);
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  hovered.value = { name: seg.name!, x: rect.left, y: rect.bottom + 6 };
}

function scheduleHide() {
  clearTimeout(hideTimer);
  hideTimer = setTimeout(() => (hovered.value = null), 200);
}

function keepOpen() {
  clearTimeout(hideTimer);
}

// Clicking a token still edits the text: move the caret to the token's end.
function focusAt(pos: number) {
  input.value?.focus();
  input.value?.setSelectionRange(pos, pos);
  sync();
}

onBeforeUnmount(() => clearTimeout(hideTimer));
</script>

<template>
  <span class="var-input" :class="attrs.class as string" :style="attrs.style as string">
    <input
      ref="input"
      v-model="model"
      v-bind="inputAttrs"
      class="mono"
      :class="{ masked: hasTokens }"
      spellcheck="false"
      autocomplete="off"
      @input="onInput"
      @scroll="sync"
      @keyup="sync"
      @click="sync"
      @select="sync"
      @focus="sync"
    />
    <span v-if="hasTokens" ref="mirror" class="mirror mono" aria-hidden="true">
      <template v-for="seg in segments" :key="seg.start">
        <span
          v-if="seg.name"
          class="token"
          :class="stateOf(seg)"
          @mouseenter="show(seg, $event)"
          @mouseleave="scheduleHide"
          @mousedown.prevent="focusAt(seg.end)"
          >{{ seg.text }}</span
        ><span v-else class="plain">{{ seg.text }}</span>
      </template>
    </span>
    <VariablePopover
      v-if="hovered"
      :name="hovered.name"
      :x="hovered.x"
      :y="hovered.y"
      @enter="keepOpen"
      @leave="scheduleHide"
      @close="hovered = null"
    />
  </span>
</template>

<style scoped>
.var-input {
  position: relative;
  display: inline-flex;
  width: 100%;
  min-width: 0;
}
.var-input input {
  width: 100%;
  line-height: 20px;
}
/* The mirror draws the text; the input keeps the caret and selection. */
.var-input input.masked {
  color: transparent;
  caret-color: var(--fg);
}
.mirror {
  position: absolute;
  inset: 0;
  padding: 5px 8px;
  border: 1px solid transparent;
  line-height: 20px;
  white-space: pre;
  overflow: hidden;
  pointer-events: none;
  color: var(--fg);
}
.token {
  pointer-events: auto;
  cursor: text;
  border-radius: 3px;
}
.token.defined,
.token.builtin {
  color: var(--pass);
  background: color-mix(in srgb, var(--pass) 14%, transparent);
}
.token.empty,
.token.disabled {
  color: var(--warn);
  background: color-mix(in srgb, var(--warn) 16%, transparent);
}
.token.missing {
  color: var(--fail);
  background: color-mix(in srgb, var(--fail) 14%, transparent);
}
</style>
