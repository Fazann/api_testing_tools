import { defineStore } from 'pinia';
import { reactive } from 'vue';

import { api, onSocketEvent } from '@/services/api';
import type { Request, SocketFrame, SocketMessage, SocketSession } from '@/types';
import { clone, uid } from '@/utils/object';

import { useEnvironments } from './environments';
import { useRunner } from './runner';
import { useUi } from './ui';

/** The console log keeps at most this many messages; older ones are dropped. */
export const MAX_CONSOLE_FRAMES = 1000;

export type ConsoleStatus = 'idle' | 'connecting' | 'open' | 'closed';

/** The console of one request: its connection and message log. */
export interface SocketConsole {
  sessionId: string;
  status: ConsoleStatus;
  frames: SocketFrame[];
  session: SocketSession | null;
  error: string;
}

/** Console sessions, keyed by request (the draft's ID, or '' for an unsaved request). */
export const useSockets = defineStore('sockets', () => {
  const consoles = reactive<Record<string, SocketConsole>>({});
  let unsubscribe: (() => void) | null = null;

  function consoleFor(key: string): SocketConsole {
    if (!consoles[key]) consoles[key] = { sessionId: '', status: 'idle', frames: [], session: null, error: '' };
    return consoles[key];
  }

  function push(c: SocketConsole, f: SocketFrame) {
    c.frames.push(f);
    if (c.frames.length > MAX_CONSOLE_FRAMES) c.frames.splice(0, c.frames.length - MAX_CONSOLE_FRAMES);
  }

  function system(c: SocketConsole, data: string) {
    push(c, { direction: 'system', data, atMs: c.frames[c.frames.length - 1]?.atMs ?? 0 });
  }

  function listen() {
    if (unsubscribe) return;
    unsubscribe = onSocketEvent((e) => {
      const c = Object.values(consoles).find((x) => x.sessionId === e.sessionId);
      if (!c) return;
      if (e.frame) push(c, e.frame);
      if (e.closed) {
        c.status = 'closed';
        c.error = e.error ?? '';
      }
    });
  }

  /** Connects the console of `req`, after the same preflight review as Send. */
  async function connect(key: string, req: Request) {
    const ui = useUi();
    const envs = useEnvironments();
    const runner = useRunner();
    const c = consoleFor(key);
    if (c.status === 'connecting') return;
    if (c.status === 'open') await disconnect(key);
    listen();
    const draft = clone(req);
    try {
      const review = await runner.reviewWarnings(await api.preflightRequest(draft, envs.selectedId));
      if (!review.proceed) return;
      let confirmed = review.confirmed;
      if (!confirmed && envs.selected?.protected) {
        confirmed = await ui.confirm({
          title: `Connect to "${envs.selected.name}"?`,
          message: 'This is a protected environment. Messages you send from the console may modify data.',
          confirmLabel: 'Connect',
          danger: true,
        });
        if (!confirmed) return;
      }
      c.sessionId = uid();
      c.status = 'connecting';
      c.error = '';
      const session = await api.connectSocket(c.sessionId, draft, envs.selectedId, confirmed);
      c.session = session;
      c.status = 'open';
      for (const l of session.logs ?? []) system(c, `${l.phase} script ${l.level}: ${l.message}`);
      system(
        c,
        `connected to ${session.url} (${session.statusCode})` +
          (session.subprotocol ? `, subprotocol ${session.subprotocol}` : '') +
          (session.engineSessionId ? `, session ${session.engineSessionId}` : ''),
      );
      if (draft.preRequestScript?.trim()) await runner.reloadVariables();
    } catch (e) {
      c.status = 'closed';
      c.error = (e as Error).message;
      system(c, `could not connect: ${c.error}`);
      ui.error(e);
    }
  }

  async function cancelConnect(key: string) {
    const c = consoles[key];
    if (c?.status === 'connecting') await api.cancel(c.sessionId);
  }

  async function disconnect(key: string) {
    const c = consoles[key];
    if (!c || c.status !== 'open') return;
    await api.disconnectSocket(c.sessionId).catch(useUi().error);
  }

  /** Sends a message; returns whether it was sent. */
  async function send(key: string, m: SocketMessage): Promise<boolean> {
    const c = consoles[key];
    if (!c || c.status !== 'open') return false;
    try {
      push(c, await api.sendSocketMessage(c.sessionId, clone(m)));
      return true;
    } catch (e) {
      useUi().error(e);
      return false;
    }
  }

  function clear(key: string) {
    const c = consoles[key];
    if (c) c.frames = [];
  }

  return { consoles, consoleFor, connect, cancelConnect, disconnect, send, clear };
});
