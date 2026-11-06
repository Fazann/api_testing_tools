import { defineStore } from 'pinia';
import { computed, ref } from 'vue';

import { api, errorMessage, onUpdateProgress } from '@/services/api';
import type { UpdateInfo, UpdateProgress } from '@/types';

/** In-place app updates: check the feed, download, install and restart. */
export const useUpdates = defineStore('updates', () => {
  const info = ref<UpdateInfo | null>(null);
  const checking = ref(false);
  const installing = ref(false);
  const progress = ref<UpdateProgress | null>(null);
  const error = ref('');
  /** The version the user chose "Later" for; its banner stays hidden this session. */
  const dismissed = ref('');

  const available = computed(() => !!info.value?.available);
  const showBanner = computed(() => available.value && dismissed.value !== info.value?.latest);
  const percent = computed(() => {
    const p = progress.value;
    return p && p.total > 0 ? Math.min(100, Math.round((p.done / p.total) * 100)) : null;
  });

  /** Checks for an update. Quiet checks (on startup) never surface errors. */
  async function check(quiet = false) {
    if (checking.value) return;
    checking.value = true;
    error.value = '';
    try {
      info.value = await api.checkForUpdate();
    } catch (e) {
      if (!quiet) error.value = errorMessage(e);
    } finally {
      checking.value = false;
    }
  }

  /** Downloads and installs the update; the app restarts itself on success. */
  async function install() {
    const i = info.value;
    if (!i?.available || installing.value) return;
    if (i.method === 'manual') {
      if (i.pageUrl) await api.openReleasePage(i.pageUrl);
      return;
    }
    installing.value = true;
    error.value = '';
    progress.value = null;
    const off = onUpdateProgress((p) => (progress.value = p));
    try {
      await api.installUpdate();
    } catch (e) {
      error.value = errorMessage(e);
      installing.value = false;
    } finally {
      off();
    }
  }

  function dismiss() {
    if (info.value) dismissed.value = info.value.latest;
  }

  return { info, checking, installing, progress, percent, error, available, showBanner, check, install, dismiss };
});
