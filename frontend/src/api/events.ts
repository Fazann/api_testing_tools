// Subscriptions to events Go emits with runtime.EventsEmit while long operations run.
import { EventsOn } from '@wails/runtime/runtime';

import type { RunProgress, SocketEvent, UpdateProgress } from '@/types';

import { BackendEvent } from './endpoints';

declare global {
  interface Window {
    runtime?: object;
  }
}

/** Subscribes to a Go event. Returns an unsubscribe function; a no-op outside the desktop app. */
export function subscribe<T>(event: BackendEvent, cb: (data: T) => void): () => void {
  if (!window.runtime) return () => {};
  return EventsOn(event, (data: T) => cb(data));
}

/** Console socket events (messages and closes of every session). */
export function onSocketEvent(cb: (e: SocketEvent) => void): () => void {
  return subscribe(BackendEvent.SocketEvent, cb);
}

/** Collection-run progress. */
export function onRunProgress(cb: (p: RunProgress) => void): () => void {
  return subscribe(BackendEvent.RunProgress, cb);
}

/** Permission-matrix progress. */
export function onMatrixProgress(cb: (p: RunProgress) => void): () => void {
  return subscribe(BackendEvent.MatrixProgress, cb);
}

/** Environment-comparison progress. */
export function onCompareProgress(cb: (p: RunProgress) => void): () => void {
  return subscribe(BackendEvent.CompareProgress, cb);
}

/** Update download progress. */
export function onUpdateProgress(cb: (p: UpdateProgress) => void): () => void {
  return subscribe(BackendEvent.UpdateProgress, cb);
}
