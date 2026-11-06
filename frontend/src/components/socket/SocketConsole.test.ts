import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { newRequest } from '@/services/model';
import { setRequestKind } from '@/services/socket';
import { useEnvironments } from '@/stores/environments';
import { MAX_CONSOLE_FRAMES, useSockets } from '@/stores/sockets';
import { useUi } from '@/stores/ui';
import { useWorkspace } from '@/stores/workspace';
import type { Request, SocketEvent } from '@/types';

import SocketConsole from './SocketConsole.vue';

type Handler = (...data: unknown[]) => void;

function setup(protectedEnv = false) {
  const calls: { name: string; args: unknown[] }[] = [];
  const handlers: Record<string, Handler[]> = {};
  window.go = {
    main: {
      App: new Proxy(
        {},
        {
          get:
            (_t, name: string) =>
            async (...args: unknown[]) => {
              calls.push({ name, args });
              if (name === 'PreflightRequest') return [];
              if (name === 'ConnectSocket')
                return { sessionId: args[0], kind: 'socketio', url: 'http://h/admin', statusCode: 101, headers: [] };
              if (name === 'SendSocketMessage') {
                const m = args[1] as { event: string; body: string };
                return { direction: 'sent', event: m.event, data: m.body, atMs: 5 };
              }
              if (name === 'DisconnectSocket') return true;
              return null;
            },
        },
      ),
    },
  };
  window.runtime = {
    EventsOn: (name: string, cb: Handler) => {
      (handlers[name] ??= []).push(cb);
      return () => {};
    },
  };
  const emit = (e: SocketEvent) => handlers['socket:event']?.forEach((h) => h(e));

  const pinia = createPinia();
  setActivePinia(pinia);
  const envs = useEnvironments();
  envs.list = [{ id: 'e1', name: 'Prod', variables: [], protected: protectedEnv } as never];
  envs.selectedId = 'e1';
  const req: Request = { ...newRequest('c1'), id: 'q1', url: 'http://h/admin' };
  setRequestKind(req, 'socketio');
  useWorkspace().open(req);
  const wrapper = mount(SocketConsole, {
    props: { request: useWorkspace().draft! },
    global: { plugins: [pinia] },
    attachTo: document.body,
  });
  return { wrapper, calls, emit, sockets: useSockets() };
}

function button(w: ReturnType<typeof mount>, text: string) {
  return w.findAll('button').find((b) => b.text() === text)!;
}

describe('SocketConsole', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    delete window.go;
    delete window.runtime;
    vi.restoreAllMocks();
  });

  it('connects, sends an event, shows received messages and the close', async () => {
    const { wrapper, calls, emit, sockets } = setup();
    await button(wrapper, 'Connect').trigger('click');
    await flushPromises();
    expect(calls.map((c) => c.name)).toEqual(['PreflightRequest', 'ConnectSocket']);
    expect(wrapper.text()).toContain('Connected');
    expect(wrapper.text()).toContain('connected to http://h/admin (101)');

    await wrapper.find('input[aria-label="Event name"]').setValue('echo');
    await wrapper.find('textarea[aria-label="Message body"]').setValue('{"n":1}');
    await button(wrapper, 'Send').trigger('click');
    await flushPromises();
    const sent = calls.find((c) => c.name === 'SendSocketMessage')!;
    expect(sent.args[1]).toMatchObject({ event: 'echo', body: '{"n":1}', bodyType: 'json' });

    const sessionId = sockets.consoles.q1.sessionId;
    emit({ sessionId, frame: { direction: 'received', event: 'echo', data: '{"n":1}', atMs: 9 } });
    emit({ sessionId: 'other', frame: { direction: 'received', event: 'x', data: 'not mine', atMs: 1 } });
    await flushPromises();
    expect(wrapper.text()).toContain('↓');
    expect(wrapper.text()).not.toContain('not mine');

    emit({
      sessionId,
      closed: true,
      closeCode: 1011,
      error: 'server error',
      frame: { direction: 'system', data: 'closed', atMs: 10 },
    });
    await flushPromises();
    expect(wrapper.text()).toContain('Disconnected');
    expect(wrapper.text()).toContain('server error');
    expect(button(wrapper, 'Connect')).toBeTruthy();
  });

  it('does not send invalid messages and asks before connecting to a protected environment', async () => {
    const { wrapper, calls } = setup(true);
    const ui = useUi();
    const confirm = vi.spyOn(ui, 'confirm').mockResolvedValue(false);
    await button(wrapper, 'Connect').trigger('click');
    await flushPromises();
    expect(confirm).toHaveBeenCalledOnce();
    expect(calls.map((c) => c.name)).not.toContain('ConnectSocket');

    confirm.mockResolvedValue(true);
    await button(wrapper, 'Connect').trigger('click');
    await flushPromises();
    expect(calls.find((c) => c.name === 'ConnectSocket')!.args[3]).toBe(true);

    await wrapper.find('input[aria-label="Event name"]').setValue('');
    expect(button(wrapper, 'Send').attributes('disabled')).toBeDefined();
    expect(wrapper.text()).toContain('need an event name');
  });

  it('keeps the log bounded, dropping the oldest messages', async () => {
    const { wrapper, sockets, emit } = setup();
    await button(wrapper, 'Connect').trigger('click');
    await flushPromises();
    const c = sockets.consoles.q1;
    for (let i = 0; i < MAX_CONSOLE_FRAMES + 5; i++) {
      emit({ sessionId: c.sessionId, frame: { direction: 'received', data: `m${i}`, atMs: i } });
    }
    expect(c.frames).toHaveLength(MAX_CONSOLE_FRAMES);
    expect(c.frames[c.frames.length - 1].data).toBe(`m${MAX_CONSOLE_FRAMES + 4}`);
  });
});
