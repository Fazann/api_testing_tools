import { afterEach, describe, expect, it, vi } from 'vitest';

import { api } from '.';
import { BackendEvent, Endpoint } from './endpoints';
import { subscribe } from './events';
import { interceptors } from './interceptors';
import { BackendUnavailableError, request } from './request';

function bind(methods: Record<string, (...args: unknown[]) => Promise<unknown>>) {
  window.go = { main: { App: methods } };
}

afterEach(() => {
  interceptors.request.clear();
  interceptors.response.clear();
  interceptors.error.clear();
  delete window.go;
  delete window.runtime;
});

describe('services', () => {
  it('call the generated Wails binding with the args in Go order', async () => {
    const GetRun = vi.fn().mockResolvedValue({ id: 'r1' });
    const SaveRequest = vi.fn().mockResolvedValue({ id: 'q1' });
    bind({ GetRun, SaveRequest });
    await expect(api.getRun('r1')).resolves.toEqual({ id: 'r1' });
    expect(GetRun).toHaveBeenCalledWith('r1');
    const req = { id: 'q1', name: 'n' } as never;
    await api.saveRequest(req);
    expect(SaveRequest).toHaveBeenCalledWith(req);
  });

  it('expose the remote backend status that Go reports', async () => {
    const status = { mode: 'mock', configured: false, baseUrl: '', reachable: true, offline: false, latencyMs: 1 };
    bind({ BackendStatus: vi.fn().mockResolvedValue(status) });
    await expect(api.backendStatus()).resolves.toEqual(status);
  });
});

describe('request', () => {
  it('throws BackendUnavailableError outside the desktop app without calling', async () => {
    const call = vi.fn();
    await expect(request(Endpoint.AppInfo, call)).rejects.toBeInstanceOf(BackendUnavailableError);
    expect(call).not.toHaveBeenCalled();
  });

  it('turns Wails string rejections into Errors', async () => {
    bind({});
    await expect(request(Endpoint.GetRun, () => Promise.reject('run not found'))).rejects.toThrow('run not found');
  });

  it('runs request and response interceptors in order', async () => {
    bind({});
    const seen: string[] = [];
    interceptors.request.use((ctx) => void seen.push(`a:${ctx.endpoint}`));
    interceptors.request.use((ctx) => void seen.push(`b:${ctx.endpoint}`));
    interceptors.response.use((data) => (data as number) * 10);
    interceptors.response.use((data) => (data as number) + 1);
    await expect(request(Endpoint.ClearHistory, () => Promise.resolve(2))).resolves.toBe(21);
    expect(seen).toEqual(['a:ClearHistory', 'b:ClearHistory']);
  });

  it('lets a request interceptor stop the call, and use() returns a remover', async () => {
    bind({});
    const call = vi.fn();
    const remove = interceptors.request.use(() => {
      throw new Error('blocked');
    });
    await expect(request(Endpoint.DeleteAllData, call)).rejects.toThrow('blocked');
    expect(call).not.toHaveBeenCalled();
    remove();
    expect(interceptors.request.all()).toHaveLength(0);
  });

  it('lets error interceptors rethrow a new error or recover with a value', async () => {
    bind({});
    const fail = () => Promise.reject('raw failure');
    interceptors.error.use((e, ctx) => {
      throw new Error(`${ctx.endpoint}: ${e.message}`);
    });
    await expect(request(Endpoint.GetRun, fail)).rejects.toThrow('GetRun: raw failure');
    interceptors.error.use(() => 'fallback');
    await expect(request(Endpoint.GetRun, fail)).resolves.toBe('fallback');
  });
});

describe('subscribe', () => {
  it('uses the Wails runtime and is a no-op outside the desktop app', () => {
    expect(subscribe(BackendEvent.RunProgress, () => {})).toBeTypeOf('function');
    const off = vi.fn();
    const EventsOnMultiple = vi.fn((_name: string, cb: (d: unknown) => void) => {
      cb({ done: 1 });
      return off;
    });
    window.runtime = { EventsOnMultiple };
    const cb = vi.fn();
    subscribe(BackendEvent.CompareProgress, cb)();
    expect(EventsOnMultiple).toHaveBeenCalledWith('compare:progress', expect.any(Function), -1);
    expect(cb).toHaveBeenCalledWith({ done: 1 });
    expect(off).toHaveBeenCalled();
  });
});
