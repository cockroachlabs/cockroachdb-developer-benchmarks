import { useEffect, useRef, useState, useCallback } from 'react';
import { type MetricSnapshot, type ChaosEvent, agentKey } from '../types';

const MAX_BUFFER = 300;

interface WSMessage {
  type: 'metric' | 'chaos_event';
  metric?: MetricSnapshot;
  event?: ChaosEvent;
}

export function useWebSocket() {
  const [metrics, setMetrics] = useState<Map<string, MetricSnapshot[]>>(new Map());
  const [chaosEvents, setChaosEvents] = useState<ChaosEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const connect = useCallback(() => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${window.location.host}/ws/metrics`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);

    ws.onclose = () => {
      setConnected(false);
      reconnectTimer.current = setTimeout(connect, 5000);
    };

    ws.onerror = () => {
      if (ws.readyState === WebSocket.CONNECTING) return;
      ws.close();
    };

    ws.onmessage = (ev) => {
      const msg: WSMessage = JSON.parse(ev.data);

      if (msg.type === 'metric' && msg.metric) {
        const m = msg.metric;
        const key = agentKey(m.language, m.mode);
        setMetrics((prev) => {
          const next = new Map(prev);
          const arr = next.get(key) ?? [];
          const updated = arr.length >= MAX_BUFFER ? [...arr.slice(1), m] : [...arr, m];
          next.set(key, updated);
          return next;
        });
      }

      if (msg.type === 'chaos_event' && msg.event) {
        setChaosEvents((prev) => [...prev, msg.event!]);
      }
    };
  }, []);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
    };
  }, [connect]);

  const clearMetrics = useCallback(() => {
    setMetrics(new Map());
    setChaosEvents([]);
  }, []);

  return { metrics, chaosEvents, connected, clearMetrics };
}
