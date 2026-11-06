<script setup lang="ts" generic="T">
export interface Column {
  key: string;
  label?: string;
  /** Class for the header and body cells, e.g. 'num' or 'mono'. */
  class?: string;
  /** Class for the header cell only; defaults to `class`. */
  headClass?: string;
  width?: string;
  /** Header tooltip. */
  title?: string;
}

// A `grid` table. Override any cell with the `#cell-<key>` slot ({ row, index, value })
// and any header with `#head-<key>`. Rows render `row[key]` by default. `#after-row`
// ({ row, index }) adds rows below each row, e.g. an expanded detail row.
const props = withDefaults(
  defineProps<{
    columns: readonly Column[];
    rows: readonly T[];
    rowKey?: (row: T, index: number) => string | number;
    rowClass?: (row: T, index: number) => unknown;
    rowTitle?: (row: T, index: number) => string;
    /** Rows emit `row-click` and are keyboard-focusable. */
    clickable?: boolean;
    /** Text for the single row shown when `rows` is empty; omit to show no row. */
    empty?: string;
    caption?: string;
  }>(),
  { rowKey: undefined, rowClass: undefined, rowTitle: undefined, empty: undefined, caption: undefined },
);
const emit = defineEmits<{ 'row-click': [row: T, index: number] }>();

function value(row: T, key: string): unknown {
  return (row as Record<string, unknown>)[key];
}
function keyOf(row: T, i: number) {
  return props.rowKey ? props.rowKey(row, i) : i;
}
</script>

<template>
  <table class="grid">
    <caption v-if="caption" class="sr-only">
      {{
        caption
      }}
    </caption>
    <thead>
      <tr>
        <th
          v-for="c in columns"
          :key="c.key"
          :class="c.headClass ?? c.class"
          :style="c.width ? { width: c.width } : undefined"
          :title="c.title"
        >
          <slot :name="`head-${c.key}`" :column="c">{{ c.label }}</slot>
        </th>
      </tr>
    </thead>
    <tbody>
      <template v-for="(row, i) in rows" :key="keyOf(row, i)">
        <tr
          :class="[rowClass?.(row, i), { clickable }]"
          :tabindex="clickable ? 0 : undefined"
          :title="rowTitle?.(row, i)"
          @click="clickable && emit('row-click', row, i)"
          @keydown.enter.prevent="clickable && emit('row-click', row, i)"
        >
          <td v-for="c in columns" :key="c.key" :class="c.class">
            <slot :name="`cell-${c.key}`" :row="row" :index="i" :value="value(row, c.key)">{{
              value(row, c.key)
            }}</slot>
          </td>
        </tr>
        <slot name="after-row" :row="row" :index="i" />
      </template>
      <tr v-if="rows.length === 0 && (empty !== undefined || $slots.empty)">
        <td :colspan="columns.length" class="muted small">
          <slot name="empty">{{ empty }}</slot>
        </td>
      </tr>
      <slot name="append" />
    </tbody>
  </table>
</template>

<style scoped>
tr.clickable {
  cursor: pointer;
}
tr.clickable:hover td,
tr.clickable:focus-visible td {
  background: var(--bg-subtle);
}
tr.clickable:focus-visible {
  outline: 2px solid var(--focus);
  outline-offset: -2px;
}
</style>
