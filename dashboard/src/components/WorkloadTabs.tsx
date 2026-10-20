import { WORKLOAD_TYPES, WORKLOAD_LABELS, type WorkloadType, type MetricSnapshot } from '../types'

interface Props {
  active: WorkloadType;
  onChange: (w: WorkloadType) => void;
  metrics: Map<string, MetricSnapshot[]>;
}

const styles = {
  container: {
    display: 'flex',
    gap: 4,
    borderBottom: '1px solid var(--border)',
    marginBottom: 16,
  } as const,
  tab: {
    padding: '8px 16px',
    background: 'transparent',
    color: 'var(--text-secondary)',
    fontSize: 13,
    fontWeight: 500,
    borderBottom: '2px solid transparent',
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    transition: 'all 0.15s',
  } as const,
  activeTab: {
    color: 'var(--text-primary)',
    borderBottomColor: 'var(--accent)',
  } as const,
  dot: {
    width: 6,
    height: 6,
    borderRadius: '50%',
    flexShrink: 0,
  } as const,
}

function hasDataForWorkload(metrics: Map<string, MetricSnapshot[]>, workload: WorkloadType): boolean {
  for (const snapshots of metrics.values()) {
    const recent = snapshots[snapshots.length - 1];
    if (recent && recent.workload === workload) return true;
  }
  return false;
}

export function WorkloadTabs({ active, onChange, metrics }: Props) {
  return (
    <div style={styles.container}>
      {WORKLOAD_TYPES.map((w) => {
        const isActive = w === active;
        const hasData = hasDataForWorkload(metrics, w);
        return (
          <button
            key={w}
            onClick={() => onChange(w)}
            style={{ ...styles.tab, ...(isActive ? styles.activeTab : {}) }}
          >
            <span
              style={{
                ...styles.dot,
                background: hasData ? 'var(--success)' : 'var(--text-muted)',
              }}
            />
            {WORKLOAD_LABELS[w]}
          </button>
        );
      })}
    </div>
  )
}
