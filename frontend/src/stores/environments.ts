import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

import { api } from '@/services/api';
import type { Environment } from '@/types';

const STORAGE_KEY = 'arg.selectedEnvironment';

function readSelected(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? '';
  } catch {
    return '';
  }
}

export const useEnvironments = defineStore('environments', () => {
  const list = ref<Environment[]>([]);
  const selectedId = ref(readSelected());

  const selected = computed(() => list.value.find((e) => e.id === selectedId.value) ?? null);

  watch(selectedId, (id) => {
    try {
      localStorage.setItem(STORAGE_KEY, id);
    } catch {
      /* storage may be unavailable; the selection is a convenience only */
    }
  });

  async function load() {
    list.value = await api.listEnvironments();
    if (selectedId.value && !list.value.some((e) => e.id === selectedId.value)) selectedId.value = '';
  }

  async function save(e: Partial<Environment>) {
    const saved = await api.saveEnvironment(e);
    await load();
    return saved;
  }

  async function remove(id: string) {
    await api.deleteEnvironment(id);
    if (selectedId.value === id) selectedId.value = '';
    await load();
  }

  async function duplicate(id: string) {
    const copy = await api.duplicateEnvironment(id);
    await load();
    return copy;
  }

  return { list, selectedId, selected, load, save, remove, duplicate };
});
