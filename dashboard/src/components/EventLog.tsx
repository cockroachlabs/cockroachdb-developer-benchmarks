import { useEffect, useRef, useState, useCallback } from 'react'
import { type MetricSnapshot } from '../types'

export interface LogEntry {
  ts: number;
  level: 'info' | 'success' | 'warning' | 'error' | 'workload';
  message: string;
}

interface Props {
  entries: LogEntry[];
  activeAgents?: number;
  activeWorkload?: string;
}

const LEVEL_COLORS: Record<string, string> = {
  info: '#888',
  success: '#4ecdc4',
  warning: '#ffd93d',
  error: '#ff6b6b',
  workload: '#c084fc',
};

const LEVEL_LABELS: Record<string, string> = {
  info: 'INFO',
  success: 'OK',
  warning: 'WARN',
  error: 'ERR',
  workload: 'RUN',
};

function formatTime(ts: number): string {
  const d = new Date(ts);
  return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

const MIN_HEIGHT = 60;
const MAX_HEIGHT = 500;
const DEFAULT_HEIGHT = 160;

export function EventLog({ entries, activeAgents = 0, activeWorkload }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState(DEFAULT_HEIGHT);
  const [collapsed, setCollapsed] = useState(false);
  const heightRef = useRef(DEFAULT_HEIGHT);
  const startYRef = useRef(0);

  useEffect(() => {
    if (scrollRef.current && !collapsed) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [entries.length, collapsed]);

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    startYRef.current = e.clientY;
    heightRef.current = height;
  }, [height]);

  const onPointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!e.currentTarget.hasPointerCapture(e.pointerId)) return;
    const delta = startYRef.current - e.clientY;
    setHeight(Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, heightRef.current + delta)));
  }, []);

  const onPointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.currentTarget.releasePointerCapture(e.pointerId);
  }, []);

  return (
    <div style={{
      position: 'fixed',
      bottom: 0,
      left: 0,
      right: 0,
      zIndex: 1000,
      background: '#0a0a0a',
      borderTop: '1px solid #333',
      display: 'flex',
      flexDirection: 'column',
    }}>
      <div
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        style={{
          height: 12,
          cursor: 'ns-resize',
          background: 'linear-gradient(to bottom, #333 0%, #1a1a1a 100%)',
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center',
          touchAction: 'none',
        }}
      >
        <div style={{ width: 48, height: 3, background: '#666', borderRadius: 2 }} />
      </div>

      <div
        onClick={() => setCollapsed(c => !c)}
        style={{
          padding: '4px 12px',
          fontSize: 11,
          fontWeight: 600,
          color: '#888',
          cursor: 'pointer',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          borderBottom: collapsed ? 'none' : '1px solid #1a1a1a',
          userSelect: 'none',
        }}
      >
        <span style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <span>{collapsed ? '▶' : '▼'} Event Log</span>
          {activeAgents > 0 && (
            <span style={{
              background: 'rgba(78,205,196,0.15)',
              color: '#4ecdc4',
              padding: '1px 8px',
              borderRadius: 3,
              fontSize: 10,
              fontWeight: 400,
            }}>
              {activeAgents} agents active
            </span>
          )}
          {activeWorkload && (
            <span style={{
              background: 'rgba(192,132,252,0.15)',
              color: '#c084fc',
              padding: '1px 8px',
              borderRadius: 3,
              fontSize: 10,
              fontWeight: 400,
            }}>
              {activeWorkload.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())}
            </span>
          )}
        </span>
        <span style={{ fontWeight: 400 }}>{entries.length} events</span>
      </div>

      {!collapsed && (
        <div
          ref={scrollRef}
          style={{
            height,
            overflowY: 'auto',
            padding: '2px 0',
            fontFamily: 'monospace',
            fontSize: 12,
            lineHeight: 1.6,
          }}
        >
          {entries.length === 0 && (
            <div style={{ padding: '8px 12px', color: '#555' }}>
              Waiting for events...
            </div>
          )}
          {entries.map((entry, i) => {
            const color = LEVEL_COLORS[entry.level] || '#888';
            const label = LEVEL_LABELS[entry.level] || 'LOG';
            return (
              <div key={i} style={{ padding: '1px 12px', display: 'flex', gap: 8 }}>
                <span style={{ color: '#555', flexShrink: 0 }}>{formatTime(entry.ts)}</span>
                <span style={{
                  color,
                  fontWeight: 600,
                  flexShrink: 0,
                  minWidth: 36,
                }}>{label}</span>
                <span style={{ color: entry.level === 'info' ? '#aaa' : color }}>
                  {entry.message}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function buildLogEntries(
  metrics: Map<string, MetricSnapshot[]>,
  connected: boolean,
): LogEntry[] {
  const entries: LogEntry[] = [];
  const seen = new Set<string>();

  const allSnaps: MetricSnapshot[] = [];
  for (const snaps of metrics.values()) {
    allSnaps.push(...snaps);
  }
  allSnaps.sort((a, b) => a.ts - b.ts);

  if (connected && allSnaps.length === 0) {
    entries.push({ ts: 0, level: 'info', message: 'Waiting for benchmark...' });
    return entries;
  }

  const wlLabel = (wl: string) => wl.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase());

  const workloadStats = new Map<string, { sum: number; count: number; firstTs: number; lastTs: number }>();
  const workloadStarted = new Set<string>();
  const agentStarted = new Set<string>();

  for (const snap of allSnaps) {
    const agentName = `${snap.language.charAt(0).toUpperCase() + snap.language.slice(1)} ${snap.mode === 'library' ? 'Lib' : 'Raw'}`;
    const wl = snap.workload;

    if (snap.ops_per_sec > 0) {
      if (!workloadStarted.has(wl)) {
        workloadStarted.add(wl);
        entries.push({ ts: snap.ts, level: 'workload', message: `▶ ${wlLabel(wl)} — workload started` });
      }

      const agentWlKey = `${snap.language}-${snap.mode}-${wl}`;
      if (!agentStarted.has(agentWlKey)) {
        agentStarted.add(agentWlKey);
        entries.push({
          ts: snap.ts,
          level: 'info',
          message: `  ${agentName} → ${wlLabel(wl)}: ${snap.ops_per_sec.toFixed(0)} ops/s`,
        });
      }

      const stats = workloadStats.get(wl) ?? { sum: 0, count: 0, firstTs: snap.ts, lastTs: snap.ts };
      stats.sum += snap.ops_per_sec;
      stats.count++;
      stats.lastTs = snap.ts;
      workloadStats.set(wl, stats);
    }

    if (snap.errors > 0 && snap.retries > 0) {
      const errKey = `err-${snap.language}-${snap.mode}-${wl}`;
      if (!seen.has(errKey)) {
        seen.add(errKey);
        entries.push({
          ts: snap.ts,
          level: 'warning',
          message: `  ${agentName} → ${wlLabel(wl)}: ${snap.retries} retries, ${snap.errors} errors`,
        });
      }
    }
  }

  for (const [wl, stats] of workloadStats) {
    let nextWlStartTs = Infinity;
    for (const [otherWl, otherStats] of workloadStats) {
      if (otherWl !== wl && otherStats.firstTs > stats.firstTs && otherStats.firstTs < nextWlStartTs) {
        nextWlStartTs = otherStats.firstTs;
      }
    }
    if (nextWlStartTs < Infinity || allSnaps.length > 0) {
      const isCompleted = nextWlStartTs < Infinity || !allSnaps.some(s => s.workload === wl && s.ops_per_sec > 0 && s.ts > stats.lastTs - 3000);
      if (isCompleted && stats.count > 2) {
        const avg = Math.round(stats.sum / stats.count);
        const durationSecs = Math.round((stats.lastTs - stats.firstTs) / 1000);
        entries.push({
          ts: stats.lastTs,
          level: 'success',
          message: `✓ ${wlLabel(wl)} — completed in ${durationSecs}s (avg ${avg} ops/s across all agents)`,
        });
      }
    }
  }

  return entries;
}
