-- WebSocket and Socket.IO requests: the protocol of each request, its socket
-- settings (saved messages, test steps, Socket.IO path and auth payload), and
-- the message transcript of socket test results.
ALTER TABLE requests ADD COLUMN kind TEXT NOT NULL DEFAULT 'http';
ALTER TABLE requests ADD COLUMN socket_json TEXT;
ALTER TABLE run_results ADD COLUMN socket_json TEXT;
