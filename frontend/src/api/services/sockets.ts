// WebSocket and Socket.IO console.
import * as App from '@wails/go/main/App';

import type { HeartbeatStatus, Request, SocketFrame, SocketMessage, SocketSession } from '@/types';

import { Endpoint } from '../endpoints';
import { request, wire } from '../request';

export const socketsService = {
  connectSocket: (sessionId: string, r: Request, envId: string, confirmed: boolean) =>
    request<SocketSession>(Endpoint.ConnectSocket, () => App.ConnectSocket(sessionId, wire(r), envId, confirmed)),
  startSocketHeartbeat: (sessionId: string, m: SocketMessage, intervalMs: number) =>
    request<HeartbeatStatus>(Endpoint.StartSocketHeartbeat, () =>
      App.StartSocketHeartbeat(sessionId, wire(m), intervalMs),
    ),
  stopSocketHeartbeat: (sessionId: string) =>
    request<HeartbeatStatus>(Endpoint.StopSocketHeartbeat, () => App.StopSocketHeartbeat(sessionId)),
  sendSocketMessage: (sessionId: string, m: SocketMessage) =>
    request<SocketFrame>(Endpoint.SendSocketMessage, () => App.SendSocketMessage(sessionId, wire(m))),
  disconnectSocket: (sessionId: string) =>
    request<boolean>(Endpoint.DisconnectSocket, () => App.DisconnectSocket(sessionId)),
};
