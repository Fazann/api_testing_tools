<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';

import { axisLabel, geometry, nearestIndex } from '@/services/responseTimes';
import type { ResponseTimePoint } from '@/types';
import { formatDuration, formatTime } from '@/utils/format';

const props = defineProps<{ points: ResponseTimePoint[]; referenceMs?: number; thresholdMs?: number }>();

const el = ref<HTMLElement | null>(null);
const width = ref(600);
const HEIGHT = 180;
const box = computed(() => ({ width: width.value, height: HEIGHT, left: 52, right: 56, top: 10, bottom: 22 }));
const hover = ref<number | null>(null);
const ariaLabel = computed(
  () => `Response times of the last ${props.points.length} responses, oldest first. Use the arrow keys to read values.`,
);

let observer: ResizeObserver | null = null;
onMounted(() => {
  if (!el.value) return;
  width.value = el.value.clientWidth || 600;
  if (typeof ResizeObserver !== 'undefined') {
    observer = new ResizeObserver(([e]) => (width.value = Math.max(240, e.contentRect.width)));
    observer.observe(el.value);
  }
});
onBeforeUnmount(() => observer?.disconnect());

const geo = computed(() =>
  geometry(
    props.points.map((p) => p.durationMs),
    box.value,
    [props.referenceMs ?? 0, props.thresholdMs ?? 0],
  ),
);
const plotRight = computed(() => box.value.width - box.value.right);
const plotBottom = computed(() => box.value.height - box.value.bottom);
const active = computed(() => (hover.value === null ? null : props.points[hover.value]));
const tipLeft = computed(() => {
  if (hover.value === null) return 0;
  const x = geo.value.xs[hover.value];
  // Keep the tooltip inside the chart: flip to the left of the crosshair past the middle.
  return x > box.value.width / 2 ? x - 12 : x + 12;
});
const tipFlip = computed(() => hover.value !== null && geo.value.xs[hover.value] > box.value.width / 2);

function onMove(e: PointerEvent) {
  const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
  hover.value = props.points.length ? nearestIndex(geo.value.xs, e.clientX - rect.left) : null;
}

function onKey(e: KeyboardEvent) {
  if (!props.points.length) return;
  const last = props.points.length - 1;
  if (e.key === 'ArrowLeft') hover.value = Math.max(0, (hover.value ?? last + 1) - 1);
  else if (e.key === 'ArrowRight') hover.value = Math.min(last, (hover.value ?? -1) + 1);
  else if (e.key === 'Escape') hover.value = null;
  else return;
  e.preventDefault();
}
</script>

<template>
  <div ref="el" class="chart">
    <svg
      :width="box.width"
      :height="HEIGHT"
      role="img"
      tabindex="0"
      :aria-label="ariaLabel"
      @pointermove="onMove"
      @pointerleave="hover = null"
      @keydown="onKey"
      @blur="hover = null"
    >
      <g class="grid">
        <template v-for="t in geo.ticks" :key="t.value">
          <line :x1="box.left" :x2="plotRight" :y1="t.y" :y2="t.y" />
          <text :x="box.left - 6" :y="t.y" text-anchor="end" dominant-baseline="middle">{{ axisLabel(t.value) }}</text>
        </template>
      </g>
      <g v-if="referenceMs" class="ref median">
        <line :x1="box.left" :x2="plotRight" :y1="geo.y(referenceMs)" :y2="geo.y(referenceMs)" />
        <text :x="plotRight + 6" :y="geo.y(referenceMs)" dominant-baseline="middle">median</text>
      </g>
      <g v-if="thresholdMs" class="ref limit">
        <line :x1="box.left" :x2="plotRight" :y1="geo.y(thresholdMs)" :y2="geo.y(thresholdMs)" />
        <text :x="plotRight + 6" :y="geo.y(thresholdMs)" dominant-baseline="middle">limit</text>
      </g>
      <path v-if="points.length > 1" class="series" :d="geo.path" />
      <template v-for="(p, i) in points" :key="p.resultId">
        <circle v-if="p.slower" class="dot slow" :cx="geo.xs[i]" :cy="geo.ys[i]" r="4.5" />
      </template>
      <circle
        v-if="points.length"
        class="dot end"
        :cx="geo.xs[points.length - 1]"
        :cy="geo.ys[points.length - 1]"
        r="4"
      />
      <g v-if="active && hover !== null" class="cross">
        <line :x1="geo.xs[hover]" :x2="geo.xs[hover]" :y1="box.top" :y2="plotBottom" />
        <circle class="dot" :class="{ slow: active.slower }" :cx="geo.xs[hover]" :cy="geo.ys[hover]" r="5" />
      </g>
      <text class="axis-note" :x="box.left" :y="HEIGHT - 4">oldest</text>
      <text class="axis-note" :x="plotRight" :y="HEIGHT - 4" text-anchor="end">latest</text>
      <rect class="hit" :x="0" :y="0" :width="box.width" :height="HEIGHT" />
    </svg>
    <div v-if="active" class="tip" :class="{ flip: tipFlip }" :style="{ left: `${tipLeft}px` }" role="status">
      <strong>{{ formatDuration(active.durationMs) }}</strong>
      <span v-if="active.slower" class="slow-label">▲ slower than usual</span>
      <span class="muted">{{ formatTime(active.at) }}</span>
      <span class="muted">HTTP {{ active.statusCode || '—' }} · {{ active.status }}</span>
    </div>
  </div>
</template>

<style scoped>
.chart {
  position: relative;
  width: 100%;
}
svg {
  display: block;
  overflow: visible;
}
svg:focus-visible {
  outline: 2px solid var(--focus);
  border-radius: var(--radius);
}
.grid line {
  stroke: var(--line);
  stroke-width: 1;
}
.grid text,
.ref text,
.axis-note {
  fill: var(--fg-muted);
  font-size: 11px;
}
.ref line {
  stroke-width: 1;
}
.ref.median line {
  stroke: var(--fg-muted);
}
.ref.limit line {
  stroke: var(--warn);
}
.series {
  fill: none;
  stroke: var(--accent);
  stroke-width: 2;
  stroke-linejoin: round;
  stroke-linecap: round;
}
.dot {
  fill: var(--accent);
  stroke: var(--bg);
  stroke-width: 2;
}
.dot.slow {
  fill: var(--warn);
}
.cross line {
  stroke: var(--fg-muted);
  stroke-width: 1;
}
.hit {
  fill: transparent;
}
.tip {
  position: absolute;
  top: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 8px;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--bg);
  box-shadow: var(--shadow-sm);
  font-size: 12px;
  pointer-events: none;
  white-space: nowrap;
}
.tip.flip {
  transform: translateX(-100%);
}
.slow-label {
  color: var(--fg);
}
.slow-label::first-letter {
  color: var(--warn);
}
</style>
