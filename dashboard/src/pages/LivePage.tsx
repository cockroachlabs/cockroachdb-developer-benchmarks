import { useState, useMemo, useRef, useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  type WorkloadType,
  type MetricKey,
  METRIC_OPTIONS,
  DEFAULT_VISIBLE_METRICS,
  WORKLOAD_INFO,
  getMetricInfo,
} from '../types'
import { useWebSocket } from '../hooks/useWebSocket'
import { useApi } from '../hooks/useApi'
import { BenchmarkControls } from '../components/BenchmarkControls'
import { WorkloadTabs } from '../components/WorkloadTabs'
import { MetricsChart } from '../components/MetricsChart'
import { AgentStatusBar } from '../components/AgentStatusBar'
import { ChaosPanel } from '../components/ChaosPanel'
import { EventLog, buildLogEntries, type LogEntry } from '../components/EventLog'

const styles = {
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 16,
  } as const,
  title: {
    fontSize: 18,
    fontWeight: 600,
    display: 'flex',
    alignItems: 'center',
    gap: 8,
  } as const,
  connDot: {
    width: 8,
    height: 8,
    borderRadius: '50%',
  } as const,
  chartGrid: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fit, minmax(500px, 1fr))',
    gap: 12,
    marginBottom: 12,
  } as const,
  chartPanel: {
    position: 'relative' as const,
  },
  chartLabel: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    padding: '8px 12px',
    fontSize: 13,
    fontWeight: 600,
    color: 'var(--text-primary)',
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderBottom: 'none',
    borderRadius: '8px 8px 0 0',
  } as const,
  dirBadge: {
    fontSize: 11,
    fontWeight: 400,
    padding: '2px 8px',
    borderRadius: 4,
  } as const,
  metricToggles: {
    display: 'flex',
    gap: 6,
    flexWrap: 'wrap' as const,
    marginBottom: 12,
  },
  metricToggle: {
    padding: '4px 10px',
    fontSize: 11,
    borderRadius: 4,
    cursor: 'pointer',
    border: '1px solid var(--border)',
    transition: 'all 0.2s',
  } as const,
}

export function LivePage() {
  const location = useLocation();
  const navigate = useNavigate();
  const selectedScenario = (location.state as any)?.scenario as string | undefined;
  const { metrics, chaosEvents, connected, clearMetrics } = useWebSocket();
  const { getStatus, exportHTML, exportJSON, runScenario } = useApi();
  const [workload, setWorkload] = useState<WorkloadType>('crud');
  const [hiddenAgents, setHiddenAgents] = useState<Set<string>>(new Set());
  const [currentRunId, setCurrentRunId] = useState<string | null>(null);
  const [targetOps, setTargetOps] = useState(0);
  const [topology, setTopology] = useState('');
  const [benchmarkRunning, setBenchmarkRunning] = useState(false);
  const [scenarioName, setScenarioName] = useState(selectedScenario || '');
  const isStandardMode = !scenarioName || scenarioName === '__standard_suite__';

  useEffect(() => {
    if (selectedScenario) setScenarioName(selectedScenario);
  }, [selectedScenario]);
  const [visibleMetrics, setVisibleMetrics] = useState<Set<MetricKey>>(
    new Set(DEFAULT_VISIBLE_METRICS)
  );

  const toggleAgent = (key: string) => {
    setHiddenAgents(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const toggleMetric = (key: MetricKey) => {
    setVisibleMetrics(prev => {
      const next = new Set(prev);
      if (next.has(key)) { if (next.size > 1) next.delete(key); }
      else next.add(key);
      return next;
    });
  };

  const logEntriesRef = useRef<LogEntry[]>([]);
  const logSeenRef = useRef<Set<string>>(new Set());

  const logEntries = useMemo(() => {
    const freshEntries = buildLogEntries(metrics, connected);
    for (const entry of freshEntries) {
      if (!logSeenRef.current.has(entry.message)) {
        logSeenRef.current.add(entry.message);
        logEntriesRef.current.push(entry);
      }
    }
    return [...logEntriesRef.current];
  }, [metrics, connected]);

  const { activeCount, currentWorkload } = useMemo(() => {
    let count = 0;
    let wl = '';
    for (const snaps of metrics.values()) {
      if (snaps.length > 0) {
        const last = snaps[snaps.length - 1];
        if (last.ops_per_sec > 0) {
          count++;
          if (!wl) wl = last.workload;
        }
      }
    }
    return { activeCount: count, currentWorkload: wl };
  }, [metrics]);

  const activeMetrics = METRIC_OPTIONS.filter(m => visibleMetrics.has(m.value));
  const chartHeight = activeMetrics.length <= 2 ? 300 : 220;

  return (
    <div style={{ paddingBottom: 220 }}>
      {!isStandardMode && scenarioName && (
        <div style={{
          padding: 16, marginBottom: 12,
          background: 'rgba(78,205,196,0.05)', border: '1px solid rgba(78,205,196,0.2)',
          borderRadius: 8,
        }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <span style={{ fontSize: 18, fontWeight: 600, color: '#4ecdc4' }}>{scenarioName}</span>
              <button
                onClick={() => { setScenarioName(''); navigate('/scenarios'); }}
                style={{ padding: '2px 8px', fontSize: 11, borderRadius: 4, cursor: 'pointer', background: 'transparent', border: '1px solid #555', color: '#888' }}
              >
                ← Back
              </button>
            </div>
            <button
              onClick={async () => {
                clearMetrics();
                logEntriesRef.current = [];
                logSeenRef.current.clear();
                try {
                  const res = await runScenario(scenarioName);
                  setCurrentRunId(res.run_id);
                } catch {}
              }}
              style={{
                padding: '8px 20px', fontSize: 13, borderRadius: 4, cursor: 'pointer',
                background: '#4ecdc4', color: '#0a0a0a', fontWeight: 600, border: 'none',
              }}
            >
              Run Scenario
            </button>
          </div>

          <BenchmarkControls
            hideWorkloads
            hideScenarios
            onStarted={(runId?: string) => {
              clearMetrics();
              logEntriesRef.current = [];
              logSeenRef.current.clear();
              if (runId) setCurrentRunId(runId);
              getStatus().then(s => {
                if (!runId && s.current_run) setCurrentRunId(s.current_run.id);
                setTopology(s.topology);
                setBenchmarkRunning(s.benchmark);
              });
            }}
            onTargetChange={setTargetOps}
          />
        </div>
      )}

      <div style={styles.header}>
        <div style={styles.title}>
          <span style={{ ...styles.connDot, background: connected ? '#4ecdc4' : '#ff6b6b' }} />
          Live Benchmark {scenarioName && `— ${scenarioName}`}
        </div>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            onClick={() => { const id = currentRunId || 'latest'; exportHTML(id); }}
            style={{
              padding: '6px 14px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
              background: 'rgba(78,205,196,0.15)', border: '1px solid #4ecdc4', color: '#4ecdc4',
            }}
          >
            Export HTML Report
          </button>
          <button
            onClick={() => { const id = currentRunId || 'latest'; exportJSON(id); }}
            style={{
              padding: '6px 14px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
              background: 'rgba(255,255,255,0.05)', border: '1px solid #555', color: '#aaa',
            }}
          >
            Export JSON
          </button>
        </div>
      </div>

      {isStandardMode && (
        <BenchmarkControls
          onStarted={(runId?: string) => {
            clearMetrics();
            logEntriesRef.current = [];
            logSeenRef.current.clear();
            if (runId) setCurrentRunId(runId);
            getStatus().then(s => {
              if (!runId && s.current_run) setCurrentRunId(s.current_run.id);
              setTopology(s.topology);
              setBenchmarkRunning(s.benchmark);
            });
          }}
          onTargetChange={setTargetOps}
        />
      )}

      {isStandardMode && (
        <>
          <WorkloadTabs active={workload} onChange={setWorkload} metrics={metrics} />
          {(() => {
            const info = WORKLOAD_INFO[workload];
            return (
              <div style={{
                display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, padding: 12,
                margin: '8px 0 12px', background: 'var(--bg-card)',
                border: '1px solid var(--border)', borderRadius: 'var(--radius)',
                fontSize: 12, lineHeight: 1.6,
              }}>
                <div>
                  <div style={{ color: '#888', fontSize: 10, textTransform: 'uppercase', marginBottom: 4, fontWeight: 600 }}>What this benchmark does</div>
                  <div style={{ color: 'var(--text-primary)' }}>{info.description}</div>
                </div>
                <div>
                  <div style={{ color: '#888', fontSize: 10, textTransform: 'uppercase', marginBottom: 4, fontWeight: 600 }}>What we measure</div>
                  <div style={{ color: 'var(--text-primary)' }}>{info.measures}</div>
                </div>
                <div>
                  <div style={{ color: '#4ecdc4', fontSize: 10, textTransform: 'uppercase', marginBottom: 4, fontWeight: 600 }}>Library advantage</div>
                  <div style={{ color: 'var(--text-primary)' }}>{info.libraryAdvantage}</div>
                </div>
                <div>
                  <div style={{ color: '#ffd93d', fontSize: 10, textTransform: 'uppercase', marginBottom: 4, fontWeight: 600 }}>What to expect</div>
                  <div style={{ color: 'var(--text-primary)' }}>{info.expect}</div>
                </div>
              </div>
            );
          })()}
        </>
      )}

      <div style={styles.metricToggles}>
        {METRIC_OPTIONS.map(m => {
          const active = visibleMetrics.has(m.value);
          return (
            <div
              key={m.value}
              onClick={() => toggleMetric(m.value)}
              style={{
                ...styles.metricToggle,
                background: active ? 'rgba(78,205,196,0.15)' : 'transparent',
                borderColor: active ? '#4ecdc4' : '#333',
                color: active ? '#4ecdc4' : '#666',
              }}
            >
              {m.direction === 'higher' ? '↑' : '↓'} {m.label}
            </div>
          );
        })}
      </div>

      <div style={styles.chartGrid}>
        {activeMetrics.map(m => {
          const info = getMetricInfo(m.value);
          const isGood = info.direction === 'higher';
          return (
            <div key={m.value} style={styles.chartPanel}>
              <div style={styles.chartLabel}>
                <span>{info.label} {info.unit && `(${info.unit})`}</span>
                <span style={{
                  ...styles.dirBadge,
                  background: isGood ? 'rgba(78,205,196,0.15)' : 'rgba(255,107,107,0.15)',
                  color: isGood ? '#4ecdc4' : '#ff6b6b',
                }}>
                  {isGood ? '↑ Higher is better' : '↓ Lower is better'}
                </span>
              </div>
              <MetricsChart
                metrics={metrics}
                metricKey={m.value}
                workload={workload}
                chaosEvents={chaosEvents}
                hiddenAgents={hiddenAgents}
                targetLine={targetOps}
                height={chartHeight}
              />
            </div>
          );
        })}
      </div>

      <ChaosPanel
        topology={topology}
        benchmarkRunning={benchmarkRunning || activeCount > 0}
        chaosEvents={chaosEvents}
      />

      <AgentStatusBar
        metrics={metrics}
        connected={connected}
        hiddenAgents={hiddenAgents}
        onToggleAgent={toggleAgent}
      />

      <EventLog entries={logEntries} activeAgents={activeCount} activeWorkload={currentWorkload} />
    </div>
  )
}
