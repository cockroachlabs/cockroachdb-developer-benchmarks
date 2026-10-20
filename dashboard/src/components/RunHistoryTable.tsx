import { useState, useEffect } from 'react'
import { type BenchmarkRun, WORKLOAD_LABELS, type WorkloadType } from '../types'
import { useApi } from '../hooks/useApi'

interface Props {
  onSelect?: (run: BenchmarkRun) => void;
  onCompareSelect?: (ids: string[]) => void;
  selectable?: boolean;
}

const styles = {
  container: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius-lg)',
    overflow: 'hidden',
  } as const,
  table: {
    width: '100%',
    borderCollapse: 'collapse',
    fontSize: 13,
  } as const,
  th: {
    textAlign: 'left' as const,
    padding: '10px 12px',
    color: 'var(--text-secondary)',
    fontWeight: 500,
    borderBottom: '1px solid var(--border)',
    fontSize: 12,
  },
  td: {
    padding: '10px 12px',
    borderBottom: '1px solid var(--border)',
    color: 'var(--text-primary)',
  },
  row: {
    cursor: 'pointer',
    transition: 'background 0.1s',
  } as const,
  empty: {
    padding: 32,
    textAlign: 'center' as const,
    color: 'var(--text-muted)',
  },
}

export function RunHistoryTable({ onSelect, onCompareSelect, selectable }: Props) {
  const { getRuns } = useApi();
  const [runs, setRuns] = useState<BenchmarkRun[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getRuns()
      .then(r => setRuns(r ?? []))
      .catch(() => setRuns([]))
      .finally(() => setLoading(false));
  }, [getRuns]);

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  useEffect(() => {
    if (selectable && onCompareSelect) {
      onCompareSelect(Array.from(selected));
    }
  }, [selected, selectable, onCompareSelect]);

  if (loading) {
    return <div style={styles.empty}>Loading...</div>;
  }

  if (runs.length === 0) {
    return (
      <div style={{ ...styles.container, padding: 48, textAlign: 'center' }}>
        <div style={{ fontSize: 32, marginBottom: 12 }}>No benchmark runs yet</div>
        <div style={{ color: '#666', fontSize: 13 }}>
          Start a benchmark from the Live page to see results here.
        </div>
      </div>
    );
  }

  return (
    <div style={styles.container}>
      <table style={styles.table}>
        <thead>
          <tr>
            {selectable && <th style={styles.th}></th>}
            <th style={styles.th}>Date</th>
            <th style={styles.th}>Topology</th>
            <th style={styles.th}>Workloads</th>
            <th style={styles.th}>Duration</th>
            <th style={styles.th}>Languages</th>
            <th style={styles.th}>Agents</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <tr
              key={run.id}
              style={styles.row}
              onClick={() => !selectable && onSelect?.(run)}
              onMouseEnter={(e) => {
                (e.currentTarget as HTMLElement).style.background = 'var(--bg-hover)';
              }}
              onMouseLeave={(e) => {
                (e.currentTarget as HTMLElement).style.background = '';
              }}
            >
              {selectable && (
                <td style={styles.td}>
                  <input
                    type="checkbox"
                    checked={selected.has(run.id)}
                    onChange={() => toggleSelect(run.id)}
                    onClick={(e) => e.stopPropagation()}
                  />
                </td>
              )}
              <td style={styles.td}>
                {new Date(run.timestamp).toLocaleString()}
              </td>
              <td style={styles.td}>{run.topology}</td>
              <td style={styles.td}>
                {run.workloads.map((w) => WORKLOAD_LABELS[w as WorkloadType] || w).join(', ')}
              </td>
              <td style={styles.td}>{run.duration_secs}s</td>
              <td style={styles.td}>
                {[...new Set((run.agents ?? []).map(a => a.language))].join(', ') || '—'}
              </td>
              <td style={styles.td}>{run.agents?.length ?? 0}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
