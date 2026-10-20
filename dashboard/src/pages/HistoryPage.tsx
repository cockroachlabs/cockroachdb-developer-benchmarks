import { useState, useMemo, useCallback } from 'react'
import { type BenchmarkRun, type MetricKey, type MetricSnapshot, WORKLOAD_LABELS, type WorkloadType } from '../types'
import { useApi } from '../hooks/useApi'
import { RunHistoryTable } from '../components/RunHistoryTable'
import { MetricsChart } from '../components/MetricsChart'
import { MetricSelector } from '../components/MetricSelector'
import { WorkloadTabs } from '../components/WorkloadTabs'

const styles = {
  title: {
    fontSize: 18,
    fontWeight: 600,
    marginBottom: 16,
  } as const,
  detail: {
    marginTop: 20,
  } as const,
  detailHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 12,
  } as const,
  back: {
    padding: '6px 12px',
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    color: 'var(--text-primary)',
    borderRadius: 'var(--radius)',
    fontSize: 12,
    cursor: 'pointer',
  } as const,
  meta: {
    fontSize: 13,
    color: 'var(--text-secondary)',
    marginBottom: 12,
  } as const,
  deleteBtn: {
    padding: '6px 12px',
    background: 'rgba(255,107,107,0.1)',
    border: '1px solid rgba(255,107,107,0.3)',
    color: '#ff6b6b',
    borderRadius: 4,
    fontSize: 12,
    cursor: 'pointer',
  } as const,
  headerRow: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: 16,
  } as const,
}

export function HistoryPage() {
  const { getRun, deleteRun, deleteAllRuns } = useApi();
  const [selectedRun, setSelectedRun] = useState<BenchmarkRun | null>(null);
  const [runMetrics, setRunMetrics] = useState<MetricSnapshot[]>([]);
  const [workload, setWorkload] = useState<WorkloadType>('crud');
  const [metricKey, setMetricKey] = useState<MetricKey>('ops_per_sec');
  const [refreshKey, setRefreshKey] = useState(0);

  const handleSelect = useCallback(async (run: BenchmarkRun) => {
    try {
      const detail = await getRun(run.id);
      setSelectedRun(detail.run);
      setRunMetrics(detail.metrics);
      const firstWorkload = detail.run.workloads[0] as WorkloadType | undefined;
      if (firstWorkload) setWorkload(firstWorkload);
    } catch {
      setSelectedRun(run);
      setRunMetrics([]);
    }
  }, [getRun]);

  const handleDeleteRun = useCallback(async (id: string) => {
    await deleteRun(id);
    setSelectedRun(null);
    setRefreshKey(k => k + 1);
  }, [deleteRun]);

  const handleDeleteAll = useCallback(async () => {
    await deleteAllRuns();
    setSelectedRun(null);
    setRefreshKey(k => k + 1);
  }, [deleteAllRuns]);

  const groupedMetrics = useMemo(() => {
    const map = new Map<string, MetricSnapshot[]>();
    for (const m of runMetrics) {
      const key = `${m.language}-${m.mode}`;
      const arr = map.get(key) ?? [];
      arr.push(m);
      map.set(key, arr);
    }
    return map;
  }, [runMetrics]);

  if (selectedRun) {
    return (
      <div>
        <div style={styles.detailHeader}>
          <div style={{ fontSize: 18, fontWeight: 600 }}>Run {selectedRun.id.slice(0, 8)}</div>
          <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
            <MetricSelector value={metricKey} onChange={setMetricKey} />
            <button
              style={styles.deleteBtn}
              onClick={() => handleDeleteRun(selectedRun.id)}
            >
              Delete Run
            </button>
            <button style={styles.back} onClick={() => setSelectedRun(null)}>
              Back
            </button>
          </div>
        </div>
        <div style={styles.meta}>
          {new Date(selectedRun.timestamp).toLocaleString()} |{' '}
          {selectedRun.topology} | {selectedRun.duration_secs}s |{' '}
          {selectedRun.workloads.map((w) => WORKLOAD_LABELS[w as WorkloadType] || w).join(', ')}
        </div>

        <WorkloadTabs active={workload} onChange={setWorkload} metrics={groupedMetrics} />

        <MetricsChart
          metrics={groupedMetrics}
          metricKey={metricKey}
          workload={workload}
          chaosEvents={selectedRun.chaos_events ?? []}
        />
      </div>
    );
  }

  return (
    <div>
      <div style={styles.headerRow}>
        <div style={styles.title}>Run History</div>
        <button style={styles.deleteBtn} onClick={handleDeleteAll}>
          Delete All Runs
        </button>
      </div>
      <RunHistoryTable key={refreshKey} onSelect={handleSelect} />
    </div>
  )
}
