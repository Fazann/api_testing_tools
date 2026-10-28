import { computed, inject, provide, type ComputedRef, type InjectionKey, type Ref } from 'vue';

import { useWorkspace } from '../stores/workspace';
import type { Variable } from '../types';

interface Scope {
  /** Variables being edited in place, e.g. by the collection settings dialog. */
  editing: Readonly<Ref<Variable[]>> | null;
}

const KEY: InjectionKey<Scope> = Symbol('collectionVariables');

/** Lets descendants resolve {{variables}} against a collection form that is still being edited. */
export function provideCollectionVariables(variables: Readonly<Ref<Variable[]>>) {
  provide(KEY, { editing: variables });
}

/**
 * The collection variables that {{references}} resolve against: those of a collection being edited, otherwise those
 * of the open request's collection.
 */
export function useCollectionVariables(): {
  variables: ComputedRef<Variable[]>;
  /** Set while a collection form is being edited; changes go there instead of being saved directly. */
  editing: Readonly<Ref<Variable[]>> | null;
} {
  const scope = inject(KEY, { editing: null });
  const ws = useWorkspace();
  return {
    variables: computed(() => scope.editing?.value ?? ws.currentCollection?.variables ?? []),
    editing: scope.editing,
  };
}
