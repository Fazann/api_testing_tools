import { describe, expect, it } from 'vitest';

import { newRequest } from '@/services/model';

import {
  messageProblem,
  newSocketStep,
  prettyFrameData,
  requestBadge,
  sendsMessages,
  setRequestKind,
  stepSummary,
} from './socket';

describe('setRequestKind', () => {
  it('adds socket settings, forces GET and keeps HTTP fields for switching back', () => {
    const r = { ...newRequest('c1'), method: 'POST' as const, body: '{"a":1}' };
    setRequestKind(r, 'socketio');
    expect(r.kind).toBe('socketio');
    expect(r.method).toBe('GET');
    expect(r.socket?.path).toBe('/socket.io/');
    r.socket!.steps.push(newSocketStep('socketio', 'send'));
    setRequestKind(r, 'websocket');
    setRequestKind(r, 'http');
    expect(r.kind).toBe('http');
    expect(r.body).toBe('{"a":1}');
    // The socket settings survive a round trip through HTTP.
    setRequestKind(r, 'socketio');
    expect(r.socket?.steps).toHaveLength(1);
  });
});

describe('badges and data changes', () => {
  it('labels socket requests and treats only enabled send steps as changing data', () => {
    expect(requestBadge({ kind: 'websocket', method: 'GET' })).toBe('WS');
    expect(requestBadge({ kind: 'socketio', method: 'GET' })).toBe('SIO');
    expect(requestBadge({ kind: 'http', method: 'DELETE' })).toBe('DELETE');
    const send = newSocketStep('websocket', 'send');
    const expectStep = newSocketStep('websocket', 'expect');
    expect(sendsMessages({ subprotocols: [], path: '', authPayload: '', messages: [], steps: [expectStep] })).toBe(
      false,
    );
    expect(sendsMessages({ subprotocols: [], path: '', authPayload: '', messages: [], steps: [send] })).toBe(true);
    send.enabled = false;
    expect(sendsMessages({ subprotocols: [], path: '', authPayload: '', messages: [], steps: [send] })).toBe(false);
  });
});

describe('messageProblem', () => {
  it('requires a non-reserved Socket.IO event and valid JSON unless variables are used', () => {
    expect(messageProblem('socketio', { event: '', body: '', bodyType: 'json' })).toMatch(/event name/);
    expect(messageProblem('socketio', { event: 'connect', body: '', bodyType: 'json' })).toMatch(/reserved/);
    expect(messageProblem('websocket', { event: '', body: '{bad', bodyType: 'json' })).toMatch(/Invalid JSON/);
    expect(messageProblem('websocket', { event: '', body: '{"t": {{token}}}', bodyType: 'json' })).toBe('');
    expect(messageProblem('websocket', { event: '', body: 'hello', bodyType: 'text' })).toBe('');
  });
});

describe('display helpers', () => {
  it('pretty-prints JSON frames and summarizes steps', () => {
    expect(prettyFrameData({ direction: 'received', data: '{"a":1}', atMs: 0 })).toBe('{\n  "a": 1\n}');
    expect(prettyFrameData({ direction: 'received', data: 'plain', atMs: 0 })).toBe('plain');
    expect(prettyFrameData({ direction: 'received', data: '3q0=', binary: true, atMs: 0 })).toBe('3q0=');
    const s = newSocketStep('socketio', 'expect');
    s.event = 'joined';
    s.expectAck = true;
    s.assertions.push({
      id: 'a',
      kind: 'json_path',
      path: 'ok',
      operator: 'eq',
      expected: true,
      tolerance: 0,
      enabled: true,
    });
    expect(stepSummary('socketio', s)).toBe('Expect acknowledgement joined (1 check)');
    expect(stepSummary('socketio', newSocketStep('socketio', 'wait'))).toBe('Wait 500 ms');
  });
});
