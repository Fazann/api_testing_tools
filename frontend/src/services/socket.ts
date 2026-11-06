// WebSocket and Socket.IO requests: defaults and helpers shared by the editor, console and results.
// Rules must match internal/domain/socket.go.
import type {
  AssertionKind,
  Request,
  RequestKind,
  SocketConfig,
  SocketFrame,
  SocketMessage,
  SocketStep,
  SocketStepKind,
} from '@/types';
import { uid } from '@/utils/object';

export const DEFAULT_SOCKETIO_PATH = '/socket.io/';
export const DEFAULT_EXPECT_TIMEOUT_MS = 5000;
export const MAX_WAIT_MS = 60000;

/** Checks that apply to a message body; status, headers and response time do not. */
export const SOCKET_ASSERTION_KINDS: readonly AssertionKind[] = [
  'json_path',
  'body',
  'valid_json',
  'required_fields',
  'aggregate',
];

export const STEP_KINDS: { value: SocketStepKind; label: string }[] = [
  { value: 'send', label: 'Send' },
  { value: 'expect', label: 'Expect' },
  { value: 'wait', label: 'Wait' },
];

/** Event names the Socket.IO client and server use themselves. */
const RESERVED_EVENTS = ['connect', 'connect_error', 'disconnect', 'disconnecting', 'newListener', 'removeListener'];

export function isSocket(kind: RequestKind | undefined): boolean {
  return kind === 'websocket' || kind === 'socketio';
}

export function newSocketConfig(kind: RequestKind): SocketConfig {
  return {
    subprotocols: [],
    path: kind === 'socketio' ? DEFAULT_SOCKETIO_PATH : '',
    authPayload: '',
    messages: [],
    steps: [],
  };
}

export function newSocketMessage(kind: RequestKind): SocketMessage {
  return { id: uid(), name: '', event: kind === 'socketio' ? 'message' : '', body: '', bodyType: 'json' };
}

export function newSocketStep(kind: RequestKind, stepKind: SocketStepKind): SocketStep {
  return {
    id: uid(),
    kind: stepKind,
    enabled: true,
    message: newSocketMessage(kind),
    event: '',
    assertions: [],
    timeoutMs: stepKind === 'wait' ? 500 : DEFAULT_EXPECT_TIMEOUT_MS,
  };
}

/**
 * Switches a request between HTTP, WebSocket and Socket.IO. HTTP fields (method, body, tests) are kept, so
 * switching back restores them; socket requests always use GET for the handshake.
 */
export function setRequestKind(req: Request, kind: RequestKind) {
  if (req.kind === kind) return;
  req.kind = kind;
  if (!isSocket(kind)) return;
  req.method = 'GET';
  if (!req.socket) req.socket = newSocketConfig(kind);
  if (kind === 'socketio' && !req.socket.path) req.socket.path = DEFAULT_SOCKETIO_PATH;
}

/** The short label shown instead of an HTTP method. */
export function requestBadge(r: Pick<Request, 'kind' | 'method'>): string {
  if (r.kind === 'websocket') return 'WS';
  if (r.kind === 'socketio') return 'SIO';
  return r.method;
}

/** A socket test may change server state when it sends anything. */
export function sendsMessages(cfg: SocketConfig | undefined): boolean {
  return !!cfg?.steps.some((s) => s.enabled && s.kind === 'send');
}

/** Problem with a message before sending, or '' when it can be sent. */
export function messageProblem(kind: RequestKind, m: Pick<SocketMessage, 'event' | 'body' | 'bodyType'>): string {
  if (kind === 'socketio') {
    if (!m.event.trim()) return 'Socket.IO messages need an event name';
    if (RESERVED_EVENTS.includes(m.event.trim())) return `"${m.event}" is a reserved Socket.IO event name`;
  }
  // Variables may make the template invalid JSON until resolved; only check without them.
  if (m.bodyType === 'json' && m.body.trim() && !m.body.includes('{{')) {
    try {
      JSON.parse(m.body);
    } catch (e) {
      return `Invalid JSON: ${(e as Error).message}`;
    }
  }
  return '';
}

/** A frame's data pretty-printed when it is JSON. */
export function prettyFrameData(f: SocketFrame): string {
  if (f.binary || !f.data) return f.data;
  try {
    return JSON.stringify(JSON.parse(f.data), null, 2);
  } catch {
    return f.data;
  }
}

/** One-line summary of a step for lists and results. */
export function stepSummary(kind: RequestKind, s: SocketStep): string {
  if (s.description?.trim()) return s.description;
  switch (s.kind) {
    case 'send':
      return kind === 'socketio' ? `Send ${s.message.event || '(no event)'}` : `Send ${clip(s.message.body, 40)}`;
    case 'wait':
      return `Wait ${s.timeoutMs} ms`;
    default: {
      const what = s.expectAck ? 'acknowledgement' : 'message';
      const checks = s.assertions.filter((a) => a.enabled).length;
      const suffix = checks ? ` (${checks} check${checks > 1 ? 's' : ''})` : '';
      return `Expect ${what}${s.event ? ` ${s.event}` : ''}${suffix}`;
    }
  }
}

function clip(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n)}…` : s || '(empty)';
}
