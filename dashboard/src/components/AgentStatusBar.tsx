import { AGENTS, LANGUAGE_COLORS, type MetricSnapshot, agentKey } from '../types'

interface Props {
  metrics: Map<string, MetricSnapshot[]>;
  connected: boolean;
  hiddenAgents?: Set<string>;
  onToggleAgent?: (key: string) => void;
}

const styles = {
  container: {
    display: 'flex',
    gap: 8,
    flexWrap: 'wrap',
    padding: '12px 0',
  } as const,
  dot: {
    width: 8,
    height: 8,
    borderRadius: '50%',
    flexShrink: 0,
  } as const,
  label: {
    fontWeight: 500,
  } as const,
  value: {
    marginLeft: 'auto',
    fontVariantNumeric: 'tabular-nums',
  } as const,
  connDot: {
    width: 5,
    height: 5,
    borderRadius: '50%',
    flexShrink: 0,
  } as const,
}

export function AgentStatusBar({ metrics, connected, hiddenAgents, onToggleAgent }: Props) {
  return (
    <div style={styles.container}>
      {AGENTS.map((agent) => {
        const key = agentKey(agent.language, agent.mode);
        const snaps = metrics.get(key);
        const latest = snaps?.[snaps.length - 1];
        const opsPerSec = latest?.ops_per_sec ?? 0;
        const langColor = (agent as any).color || LANGUAGE_COLORS[agent.language] || '#888';
        const hasData = snaps && snaps.length > 0;
        const isHidden = hiddenAgents?.has(key) ?? false;

        return (
          <div
            key={key}
            onClick={() => onToggleAgent?.(key)}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              padding: '6px 10px',
              background: isHidden ? 'transparent' : 'var(--bg-card)',
              border: `1px solid ${isHidden ? '#333' : 'var(--border)'}`,
              borderRadius: 'var(--radius)',
              fontSize: 12,
              minWidth: 120,
              cursor: 'pointer',
              opacity: isHidden ? 0.4 : 1,
              transition: 'opacity 0.2s, background 0.2s',
            }}
          >
            <span style={{ ...styles.dot, background: langColor }} />
            <span style={{ ...styles.label, color: isHidden ? '#666' : 'var(--text-primary)' }}>
              {agent.label}
            </span>
            <span style={{ ...styles.value, color: isHidden ? '#444' : 'var(--text-secondary)' }}>
              {hasData && opsPerSec > 0 ? `${opsPerSec.toFixed(0)} ops/s` : '--'}
            </span>
            <span
              style={{
                ...styles.connDot,
                background: isHidden ? '#333' : (connected && hasData && opsPerSec > 0 ? '#4ecdc4' : '#555'),
              }}
            />
          </div>
        );
      })}
    </div>
  )
}
