import { useTraceStore } from "@/stores/trace";
import type { TraceEvent } from "@/types";

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectDelay = 1000;
const MAX_RECONNECT_DELAY = 30000;

function getWsUrl(): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/ws/trace`;
}

export function connectTrace() {
  const store = useTraceStore.getState();

  if (
    ws &&
    (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)
  ) {
    return;
  }

  store.setConnectionStatus("connecting");

  try {
    ws = new WebSocket(getWsUrl());
  } catch {
    store.setConnectionStatus("disconnected");
    scheduleReconnect();
    return;
  }

  ws.onopen = () => {
    useTraceStore.getState().setConnectionStatus("connected");
    reconnectDelay = 1000; // Reset backoff on successful connect
  };

  ws.onmessage = (msg) => {
    try {
      const event = JSON.parse(msg.data) as TraceEvent;
      useTraceStore.getState().processEvent(event);
    } catch {
      // Ignore malformed messages
    }
  };

  ws.onclose = () => {
    useTraceStore.getState().setConnectionStatus("disconnected");
    ws = null;
    scheduleReconnect();
  };

  ws.onerror = () => {
    // onclose will fire after onerror
  };
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY);
    connectTrace();
  }, reconnectDelay);
}

export function disconnectTrace() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (ws) {
    ws.close();
    ws = null;
  }
  useTraceStore.getState().setConnectionStatus("disconnected");
}
