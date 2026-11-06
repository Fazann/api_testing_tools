<script setup lang="ts">
import { computed, nextTick, ref } from 'vue';

import { HIGHLIGHT_LIMIT, highlightJs, highlightJson, type Token } from '@/services/highlight';

/**
 * A code editor styled like VS Code's Dark+ theme. A native <textarea> with
 * transparent text sits on top of a highlighted <pre>, so typing, selection,
 * undo and accessibility stay native.
 */
const code = defineModel<string>({ required: true });
const props = withDefaults(
  defineProps<{ language?: 'javascript' | 'json'; placeholder?: string; label?: string; minLines?: number }>(),
  { language: 'javascript', placeholder: '', label: 'Code', minLines: 14 },
);

const input = ref<HTMLTextAreaElement | null>(null);
const scrollTop = ref(0);
const scrollLeft = ref(0);
const caretLine = ref(1);

const tokens = computed<Token[]>(() => {
  if (code.value.length > HIGHLIGHT_LIMIT) return [{ text: code.value, kind: '' }];
  return props.language === 'json' ? highlightJson(code.value) : highlightJs(code.value);
});
const shift = computed(() => ({ transform: `translate(${-scrollLeft.value}px, ${-scrollTop.value}px)` }));
/** A final newline so an empty last line still has height. */
const nl = '\n';
const parts = computed(() => tokens.value.map((t) => ({ text: t.text, c: t.kind ? `tk-${t.kind}` : undefined })));
const lineCount = computed(() => code.value.split('\n').length);
const height = computed(() => `calc(${Math.max(props.minLines, 1)} * 1.5em + 16px)`);

function onScroll() {
  const el = input.value;
  if (!el) return;
  scrollTop.value = el.scrollTop;
  scrollLeft.value = el.scrollLeft;
}

function updateCaret() {
  const el = input.value;
  if (el) caretLine.value = el.value.slice(0, el.selectionStart).split('\n').length;
}

function replaceSelection(text: string, caretOffset = text.length) {
  const el = input.value;
  if (!el) return;
  const { selectionStart: start, selectionEnd: end } = el;
  code.value = code.value.slice(0, start) + text + code.value.slice(end);
  void nextTick(() => {
    el.setSelectionRange(start + caretOffset, start + caretOffset);
    updateCaret();
  });
}

/** Tab indents; Enter keeps the current line's indentation (one level more after an opening bracket). */
function onKeydown(e: KeyboardEvent) {
  if (e.ctrlKey || e.altKey || e.metaKey) return;
  const el = e.target as HTMLTextAreaElement;
  if (e.key === 'Tab' && !e.shiftKey) {
    e.preventDefault();
    replaceSelection('  ');
  } else if (e.key === 'Enter' && !e.shiftKey && el.selectionStart === el.selectionEnd) {
    const before = code.value.slice(0, el.selectionStart);
    const line = before.slice(before.lastIndexOf('\n') + 1);
    let indent = /^[ \t]*/.exec(line)![0];
    if (/[{[(]\s*$/.test(line)) indent += '  ';
    e.preventDefault();
    replaceSelection('\n' + indent);
  }
}
</script>

<template>
  <div class="code-editor" :style="{ height }">
    <div class="gutter" aria-hidden="true">
      <div :style="{ transform: `translateY(${-scrollTop}px)` }">
        <div v-for="n in lineCount" :key="n" :class="{ current: n === caretLine }">{{ n }}</div>
      </div>
    </div>
    <div class="area">
      <pre
        class="highlight"
        aria-hidden="true"
      ><code :style="shift"><span v-for="(t, i) in parts" :key="i" :class="t.c" v-text="t.text" />{{ nl }}</code></pre>
      <textarea
        ref="input"
        v-model="code"
        spellcheck="false"
        autocapitalize="off"
        autocomplete="off"
        wrap="off"
        :aria-label="label"
        :placeholder="placeholder"
        @keydown="onKeydown"
        @scroll="onScroll"
        @keyup="updateCaret"
        @click="updateCaret"
        @focus="updateCaret"
      />
    </div>
  </div>
</template>

<style scoped>
/* VS Code "Dark+" colors. */
.code-editor {
  --ce-bg: #1e1e1e;
  --ce-fg: #d4d4d4;
  --ce-border: #3c3c3c;
  --ce-focus: #007fd4;
  --ce-gutter: #858585;
  --ce-gutter-line: #2b2b2b;
  --ce-gutter-current: #c6c6c6;
  --ce-caret: #aeafad;
  --ce-selection: #264f78;
  --ce-placeholder: #6a6a6a;
  --ce-comment: #6a9955;
  --ce-string: #ce9178;
  --ce-regex: #d16969;
  --ce-number: #b5cea8;
  --ce-keyword: #569cd6;
  --ce-control: #c586c0;
  --ce-function: #dcdcaa;
  --ce-variable: #9cdcfe;
  --ce-type: #4ec9b0;
  display: flex;
  min-height: 120px;
  resize: vertical;
  overflow: hidden;
  background: var(--ce-bg);
  color: var(--ce-fg);
  border: 1px solid var(--ce-border);
  border-radius: var(--radius);
  font-family: var(--mono);
  font-size: 12.5px;
  line-height: 1.5;
}
.code-editor:focus-within {
  border-color: var(--ce-focus);
}
.gutter {
  flex-shrink: 0;
  overflow: hidden;
  padding: 8px 10px 8px 12px;
  min-width: 3.2em;
  text-align: right;
  color: var(--ce-gutter);
  user-select: none;
  border-right: 1px solid var(--ce-gutter-line);
}
.gutter .current {
  color: var(--ce-gutter-current);
}
.area {
  position: relative;
  flex: 1;
  min-width: 0;
}
.highlight,
textarea {
  position: absolute;
  inset: 0;
  margin: 0;
  padding: 8px 12px;
  border: none;
  border-radius: 0;
  font: inherit;
  line-height: inherit;
  letter-spacing: normal;
  tab-size: 2;
  white-space: pre;
  overflow-wrap: normal;
}
.highlight {
  overflow: hidden;
  pointer-events: none;
  background: transparent;
  color: var(--ce-fg);
}
.highlight code {
  display: block;
  font: inherit;
  white-space: inherit;
}
textarea {
  width: 100%;
  height: 100%;
  resize: none;
  overflow: auto;
  background: transparent;
  color: transparent;
  caret-color: var(--ce-caret);
  outline: none;
  box-shadow: none;
}
textarea::selection {
  background: var(--ce-selection);
  color: transparent;
}
textarea::placeholder {
  color: var(--ce-placeholder);
}
.tk-comment {
  color: var(--ce-comment);
}
.tk-string {
  color: var(--ce-string);
}
.tk-regex {
  color: var(--ce-regex);
}
.tk-number {
  color: var(--ce-number);
}
.tk-keyword,
.tk-literal {
  color: var(--ce-keyword);
}
.tk-control {
  color: var(--ce-control);
}
.tk-function {
  color: var(--ce-function);
}
.tk-variable,
.tk-property,
.tk-key {
  color: var(--ce-variable);
}
.tk-type {
  color: var(--ce-type);
}
.tk-punct {
  color: var(--ce-fg);
}
</style>
