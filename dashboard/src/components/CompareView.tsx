import { useState, useEffect, useMemo } from 'react'
import { type MetricSnapshot, type MetricKey, type WorkloadType, WORKLOAD_TYPES, WORKLOAD_LABELS, getMetricInfo } from '../types'
import { useApi } from '../hooks/useApi'
import { MetricsChart } from './MetricsChart'
import { MetricSelector } from './MetricSelector'

interface Props {
  runIds: string[];
}

interface RunData {
  id: string;
  metrics: MetricSnapshot[];
}

const LINE_STYLES = [
  { opacity: 1.0, dash: '' },
  { opacity: 0.7, dash: '5,5' },
  { opacity: 0.5, dash: '10,5' },
  { opacity: 0.8, dash: '2,3' },
  { opacity: 0.6, dash: '8,3,2,3' },
  { opacity: 0.9, dash: '15,5' },
  { opacity: 0.55, dash: '4,4' },
  { opacity: 0.75, dash: '12,4,4,4' },
];

const styles = {
  header: {
    display: 'flex',
    gap: 12,
    alignItems: 'center',
    marginBottom: 16,
  } as const,
  label: {
    fontSize: 12,
    color: 'var(--text-secondary)',
  } as const,
  card: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius-lg)',
    padding: 12,
    marginBottom: 16,
  } as const,
  title: {
    fontSize: 13,
    fontWeight: 500,
    color: 'var(--text-primary)',
    marginBottom: 8,
  } as const,
  empty: {
    textAlign: 'center' as const,
    color: 'var(--text-muted)',
    padding: 48,
  },
  legend: {
    display: 'flex',
    gap: 16,
    flexWrap: 'wrap' as const,
    marginBottom: 12,
    fontSize: 12,
    color: 'var(--text-secondary)',
  },
  legendItem: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
  } as const,
  legendSwatch: {
    width: 24,
    height: 3,
    borderRadius: 2,
  } as const,
}

export function CompareView({ runIds }: Props) {
  const { getRun } = useApi();
  const [runs, setRuns] = useState<RunData[]>([]);
  const [metricKey, setMetricKey] = useState<MetricKey>('ops_per_sec');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (runIds.length < 2) {
      setRuns([]);
      return;
    }

    setLoading(true);
    Promise.all(
      runIds.map((id) => getRun(id).then((r) => ({ id, metrics: r.metrics })))
    )
      .then(setRuns)
      .catch(() => setRuns([]))
      .finally(() => setLoading(false));
  }, [runIds, getRun]);

  const groupedRuns = useMemo(() => {
    return runs.map((run) => ({
      id: run.id,
      metrics: groupMetrics(run.metrics),
    }));
  }, [runs]);

  if (runIds.length < 2) {
    return <div style={styles.empty}>Select two or more runs to compare</div>;
  }

  if (loading) {
    return <div style={styles.empty}>Loading comparison data...</div>;
  }

  return (
    <div style={{ maxHeight: 'calc(100vh - 300px)', overflowY: 'auto', overflowX: 'hidden' }}>
      <div style={styles.header}>
        <span style={styles.label}>Metric:</span>
        <MetricSelector value={metricKey} onChange={setMetricKey} />
      </div>

      <div style={styles.legend}>
        {runs.map((run, idx) => {
          const style = LINE_STYLES[idx % LINE_STYLES.length];
          return (
            <div key={run.id} style={styles.legendItem}>
              <span style={{
                ...styles.legendSwatch,
                background: `rgba(78,205,196,${style.opacity})`,
              }} />
              <span>Run {idx + 1}: {run.id.slice(0, 12)}</span>
            </div>
          );
        })}
      </div>

      {WORKLOAD_TYPES.map((workload) => {
        const hasData = groupedRuns.some((r) => hasWorkloadData(r.metrics, workload));
        if (!hasData) return null;

        const info = getMetricInfo(metricKey);
        const isHigher = info.direction === 'higher';

        return (
          <div key={workload} style={{ marginBottom: 20 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
              <div style={styles.title}>{WORKLOAD_LABELS[workload]} — {info.label} {info.unit && `(${info.unit})`}</div>
              <span style={{
                fontSize: 11, padding: '2px 8px', borderRadius: 4,
                background: isHigher ? 'rgba(78,205,196,0.15)' : 'rgba(255,107,107,0.15)',
                color: isHigher ? '#4ecdc4' : '#ff6b6b',
              }}>
                {isHigher ? '↑ Higher is better' : '↓ Lower is better'}
              </span>
            </div>
            <div style={{
              display: 'grid',
              gridTemplateColumns: runs.length <= 2 ? `repeat(${runs.length}, 1fr)` : 'repeat(2, 1fr)',
              gap: 12,
              maxWidth: '100%',
              overflow: 'hidden',
            }}>
              {groupedRuns.map((run, idx) => (
                <div key={run.id} style={styles.card}>
                  <div style={{ ...styles.label, marginBottom: 4 }}>
                    Run {idx + 1}: {run.id.slice(0, 12)}
                  </div>
                  <MetricsChart
                    metrics={run.metrics}
                    metricKey={metricKey}
                    workload={workload}
                    chaosEvents={[]}
                    height={220}
                  />
                </div>
              ))}
            </div>
          </div>
        );
      })}
    </div>
  )
}

function groupMetrics(metrics: MetricSnapshot[]): Map<string, MetricSnapshot[]> {
  const map = new Map<string, MetricSnapshot[]>();
  for (const m of metrics) {
    const key = `${m.language}-${m.mode}`;
    const arr = map.get(key) ?? [];
    arr.push(m);
    map.set(key, arr);
  }
  return map;
}

function hasWorkloadData(metrics: Map<string, MetricSnapshot[]>, workload: WorkloadType): boolean {
  for (const snaps of metrics.values()) {
    if (snaps.some((s) => s.workload === workload)) return true;
  }
  return false;
}
